package main

import (
	"log"

	"github.com/gammazero/deque"

	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"go.etcd.io/raft/v3/raftpb"
	"golang.org/x/exp/constraints"
)

type CoroWithAccesses struct {
	OpData     []byte
	Resume     func() (Status, []byte)
	Accesses   [][]any
	NumResumes int
}

type PartialOp struct {
	OpData     []byte
	Accesses   [][]any
	NumResumes int
}

type Snapshot[Key constraints.Ordered] struct {
	State  []byte
	Queues map[Key]Queue[PartialOp]
}

type BlockingRaftNode[Key constraints.Ordered] struct {
	snapshotterReady <-chan *snap.Snapshotter
	commitC          <-chan *commit
	errorC           <-chan error
	appliedC         chan AppliedOp

	applyFunc        func(wait func(Key), signal func(Key), args ...interface{}) []byte
	snapshotFunc     func() ([]byte, error)
	loadSnapshotFunc func([]byte) error

	queues map[Key]*deque.Deque[CoroWithAccesses]

	currentlyExecuting *CoroWithAccesses
	replaying          bool
	accessNum          int
}

func newBlockingRaftNode[Key constraints.Ordered](
	id int,
	peers []string,
	join bool,
	proposeC <-chan []byte,
	confChangeC <-chan raftpb.ConfChange,
	clearLog bool,
) (*BlockingRaftNode[Key], <-chan error, <-chan AppliedOp) {
	var n *BlockingRaftNode[Key]
	getSnapshot := func() ([]byte, error) { return n.getSnapshot() }
	commitC, errorC, snapshotterReady := newRaftNode(id, peers, join, getSnapshot, proposeC, confChangeC, clearLog)
	appliedC := make(chan AppliedOp)
	n = &BlockingRaftNode[Key]{
		snapshotterReady: snapshotterReady,
		commitC:          commitC,
		errorC:           errorC,
		appliedC:         appliedC,
		queues:           make(map[Key]*deque.Deque[CoroWithAccesses]),
	}
	return n, errorC, appliedC
}

func (n *BlockingRaftNode[Key]) start(app BlockingApp[Key]) {
	// convert apply function into generic coroutine function
	n.applyFunc = func(wait func(Key), signal func(Key), args ...interface{}) []byte {
		op := args[0].([]byte)
		return app.apply(op, n.access, wait, signal)
	}
	n.snapshotFunc = func() ([]byte, error) { return app.getSnapshot() }
	n.loadSnapshotFunc = func(snapshot []byte) error { return app.loadSnapshot(snapshot) }

	n.loadSnapshot()
	go n.applyCommits()
}

func (n *BlockingRaftNode[Key]) access(accessFunc func() []any) []any {
	// coroutine corresponding to this access
	coro := n.currentlyExecuting
	var out []any
	if n.replaying {
		// return saved access output instead of actually reading/modifying RSM state
		// to ensure that coroutine replay matches original execution
		if len(coro.Accesses) == 0 {
			log.Fatalf("Failed to recover partially executed operation during replay (could not find access)")
		}
		access := coro.Accesses[n.accessNum]
		n.accessNum++
		out = access
	} else {
		out = accessFunc()
		// remember access in case we need to replay
		coro.Accesses = append(coro.Accesses, out)
	}
	return out
}

func (n *BlockingRaftNode[Key]) applyCommits() {
	for commit := range n.commitC {
		if commit == nil {
			// signaled to load snapshot
			n.loadSnapshot()
			continue
		}

		for _, data := range commit.data {
			// new coro is initially the only runnable one
			coro := CreateCoro[Key, []byte](n.applyFunc, data)
			runnable := []CoroWithAccesses{CoroWithAccesses{OpData: data, Resume: coro}}
			// resume coros until no coro can make more progress -- ensures
			// deterministic behavior since timing of ops arriving on
			// started channel cannot be controlled
			for len(runnable) > 0 {
				// resume coros in stack order
				next := runnable[len(runnable)-1]
				runnable = runnable[:len(runnable)-1]

				n.currentlyExecuting = &next
				// track how far into execution each coro is
				next.NumResumes++
				status, result := next.Resume()

				switch status.msgType() {
				case WaitMsg:
					key := status.(Wait[Key]).key
					if n.queues[key] == nil {
						n.queues[key] = &deque.Deque[CoroWithAccesses]{}
					}
					n.queues[key].PushBack(next)
				case SignalMsg:
					// this coro is still not blocked so add back to runnable stack
					runnable = append(runnable, next)
					key := status.(Signal[Key]).key
					queue, ok := n.queues[key]
					if ok && queue.Len() > 0 {
						// unblock exactly one coro waiting on key
						unblocked := queue.PopFront()
						runnable = append(runnable, unblocked)
					}
				case DoneMsg:
					// inform client that op has completed
					n.appliedC <- AppliedOp{op: next.OpData, result: result}
				}
			}
		}

		close(commit.applyDoneC)
	}
	if err, ok := <-n.errorC; ok {
		log.Fatal(err)
	}
}

func (n *BlockingRaftNode[Key]) getSnapshot() ([]byte, error) {
	// store queues of partially completed operations
	queues := make(map[Key]Queue[PartialOp])
	for key, ops := range n.queues {
		partialOps := Queue[PartialOp]{}
		for i := 0; i < ops.Len(); i++ {
			op := ops.At(i)
			// save everything except actual coroutine which cannot be serialized
			partialOps = append(partialOps, PartialOp{op.OpData, op.Accesses, op.NumResumes})
		}
		queues[key] = partialOps
	}
	// also save application's RSM state
	state, err := n.snapshotFunc()
	if err != nil {
		return nil, err
	}
	snapshot := Snapshot[Key]{state, queues}
	return encode(snapshot)
}

func (n *BlockingRaftNode[Key]) loadSnapshot() {
	snapshotter := <-n.snapshotterReady
	snapshot, err := snapshotter.Load()
	if err == snap.ErrNoSnapshot {
		return
	}
	if err != nil {
		log.Fatalf("Failed to load snapshot: %v", err)
	}
	if snapshot != nil {
		if err := n.recoverFromSnapshot(snapshot.Data); err != nil {
			log.Fatalf("Could not recover from snapshot: %v", err)
		}
	}
}

func (n *BlockingRaftNode[Key]) recoverFromSnapshot(data []byte) error {
	var snapshot Snapshot[Key]
	if err := decode(data, &snapshot); err != nil {
		return err
	}

	// restore application's RSM state
	if err := n.loadSnapshotFunc(snapshot.State); err != nil {
		return err
	}

	// replay partial ops and reconstruct wait queues
	n.replaying = true
	n.queues = map[Key]*deque.Deque[CoroWithAccesses]{}
	for key, partialOps := range snapshot.Queues {
		ops := deque.Deque[CoroWithAccesses]{}
		for _, partialOp := range partialOps {
			// create new version of coroutine
			resume := CreateCoro[Key, []byte](n.applyFunc, partialOp.OpData)
			coro := CoroWithAccesses{partialOp.OpData, resume, partialOp.Accesses, partialOp.NumResumes}
			// re-run coroutine until it reaches point at time of snapshot
			n.currentlyExecuting = &coro
			n.accessNum = 0
			for i := 0; i < partialOp.NumResumes; i++ {
				coro.Resume()
			}
			ops.PushBack(coro)
		}
		n.queues[key] = &ops
	}
	n.replaying = false

	return nil
}
