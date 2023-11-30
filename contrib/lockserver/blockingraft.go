package main

import (
	"log"

	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"go.etcd.io/raft/v3/raftpb"
	"golang.org/x/exp/constraints"
)

type CoroWithAccesses[ReturnType any] struct {
	OpData     []byte
	Resume     func() (Status, ReturnType)
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

type BlockingRaftNode[Key constraints.Ordered, ReturnType any] struct {
	snapshotterReady <-chan *snap.Snapshotter
	commitC          <-chan *commit
	errorC           <-chan error
	appliedC         chan ReturnType

	applyFunc        func(wait func(Key), signal func(Key), broadcast func(Key), args ...interface{}) ReturnType
	snapshotFunc     func() ([]byte, error)
	loadSnapshotFunc func([]byte) error

	queues map[Key]Queue[CoroWithAccesses[ReturnType]]

	currentlyExecuting *CoroWithAccesses[ReturnType]
	replaying          bool
	accessNum          int
}

func newBlockingRaftNode[Key constraints.Ordered, ReturnType any](
	id int,
	peers []string,
	join bool,
	proposeC <-chan []byte,
	confChangeC <-chan raftpb.ConfChange,
	clearLog bool,
) (*BlockingRaftNode[Key, ReturnType], <-chan error, <-chan ReturnType) {
	var n *BlockingRaftNode[Key, ReturnType]
	getSnapshot := func() ([]byte, error) { return n.getSnapshot() }
	commitC, errorC, snapshotterReady := newRaftNode(id, peers, join, getSnapshot, proposeC, confChangeC, clearLog)
	appliedC := make(chan ReturnType)
	n = &BlockingRaftNode[Key, ReturnType]{
		snapshotterReady: snapshotterReady,
		commitC:          commitC,
		errorC:           errorC,
		appliedC:         appliedC,
		queues:           make(map[Key]Queue[CoroWithAccesses[ReturnType]]),
	}
	return n, errorC, appliedC
}

func (n *BlockingRaftNode[Key, ReturnType]) start(app BlockingApp[Key, ReturnType]) {
	// convert apply function into generic coroutine function
	n.applyFunc = func(wait func(Key), signal func(Key), broadcast func(Key), args ...interface{}) ReturnType {
		op := args[0].([]byte)
		return app.apply(op, n.access, wait, signal, broadcast)
	}
	n.snapshotFunc = func() ([]byte, error) { return app.getSnapshot() }
	n.loadSnapshotFunc = func(snapshot []byte) error { return app.loadSnapshot(snapshot) }

	n.loadSnapshot()
	go n.applyCommits()
}

func (n *BlockingRaftNode[Key, ReturnType]) access(accessFunc func() []any) []any {
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

func (n *BlockingRaftNode[Key, ReturnType]) applyCommits() {
	for commit := range n.commitC {
		if commit == nil {
			// signaled to load snapshot
			n.loadSnapshot()
			continue
		}

		for _, data := range commit.data {
			// new coro is initially the only runnable one
			coro := CreateCoro[Key, ReturnType](n.applyFunc, data)
			runnable := []CoroWithAccesses[ReturnType]{CoroWithAccesses[ReturnType]{OpData: data, Resume: coro}}
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
					n.queues[key] = append(n.queues[key], next)
				case SignalMsg:
					// this coro is still not blocked so add back to runnable stack
					runnable = append(runnable, next)
					key := status.(Signal[Key]).key
					queue := n.queues[key]
					if len(queue) > 0 {
						// unblock exactly one coro waiting on key
						unblocked := queue[0]
						n.queues[key] = queue[1:]
						runnable = append(runnable, unblocked)
					}
				case BroadcastMsg:
					// this coro is still not blocked so add back to runnable stack
					runnable = append(runnable, next)
					key := status.(Broadcast[Key]).key
					// unblock all waiters
					for _, unblocked := range n.queues[key] {
						runnable = append(runnable, unblocked)
					}
					// empty queue
					n.queues[key] = n.queues[key][0:0]
				case DoneMsg:
					// inform client that op has completed
					n.appliedC <- result
				}
			}
		}

		close(commit.applyDoneC)
	}
	if err, ok := <-n.errorC; ok {
		log.Fatal(err)
	}
}

func (n *BlockingRaftNode[Key, ReturnType]) getSnapshot() ([]byte, error) {
	// store queues of partially completed operations
	queues := make(map[Key]Queue[PartialOp])
	for key, ops := range n.queues {
		partialOps := Queue[PartialOp]{}
		for _, op := range ops {
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

	data, err := encode(snapshot)
	if err != nil {
		return nil, err
	}
	return compress(data)
}

func (n *BlockingRaftNode[Key, ReturnType]) loadSnapshot() {
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

func (n *BlockingRaftNode[Key, ReturnType]) recoverFromSnapshot(compressed []byte) error {
	data, err := decompress(compressed)
	if err != nil {
		return err
	}

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
	n.queues = map[Key]Queue[CoroWithAccesses[ReturnType]]{}
	for key, partialOps := range snapshot.Queues {
		ops := Queue[CoroWithAccesses[ReturnType]]{}
		for _, partialOp := range partialOps {
			// create new version of coroutine
			resume := CreateCoro[Key, ReturnType](n.applyFunc, partialOp.OpData)
			coro := CoroWithAccesses[ReturnType]{partialOp.OpData, resume, partialOp.Accesses, partialOp.NumResumes}
			// re-run coroutine until it reaches point at time of snapshot
			n.currentlyExecuting = &coro
			n.accessNum = 0
			for i := 0; i < partialOp.NumResumes; i++ {
				coro.Resume()
			}
			ops = append(ops, coro)
		}
		n.queues[key] = ops
	}
	n.replaying = false

	return nil
}
