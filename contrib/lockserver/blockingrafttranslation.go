package main

import (
	"log"

	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"golang.org/x/exp/constraints"
)

type CoroWithAccessesT struct {
	OpData     []byte
	Resume     func() (Status, []byte)
	Accesses   []any
	NumResumes int
}

type PartialOpT struct {
	OpData     []byte
	Accesses   []any
	NumResumes int
}

type SnapshotT[Key constraints.Ordered] struct {
	State  []byte
	Queues map[Key]Queue[PartialOpT]
}

type BlockingRaftNodeTranslation[Key constraints.Ordered] struct {
	snapshotterReady chan *snap.Snapshotter
	commitC          <-chan *commit
	errorC           <-chan error
	appliedC         chan AppliedOp

	applyFunc        func(wait func(Key), signal func(Key), args ...interface{}) []byte
	snapshotFunc     func() ([]byte, error)
	loadSnapshotFunc func([]byte) error

	queues map[Key]Queue[CoroWithAccessesT]

	currentlyExecuting *CoroWithAccessesT
	replaying          bool
	accessNum          int
}

func newBlockingRaftNodeTranslation[Key constraints.Ordered](
	snapshotterReady chan *snap.Snapshotter,
	commitC <-chan *commit,
	errorC <-chan error,
	apply func(op []byte, access func(func() any) any, wait func(Key), signal func(Key)) []byte,
	snapshotFunc func() ([]byte, error),
	loadSnapshotFunc func([]byte) error,
) *BlockingRaftNodeTranslation[Key] {
	var n *BlockingRaftNodeTranslation[Key]
	appliedC := make(chan AppliedOp)
	// convert apply function into generic coroutine function
	applyFunc := func(wait func(Key), signal func(Key), args ...interface{}) []byte {
		op := args[0].([]byte)
		return apply(op, n.access, wait, signal)
	}
	n = &BlockingRaftNodeTranslation[Key]{
		snapshotterReady: snapshotterReady,
		commitC:          commitC,
		errorC:           errorC,
		appliedC:         appliedC,
		applyFunc:        applyFunc,
		snapshotFunc:     snapshotFunc,
		loadSnapshotFunc: loadSnapshotFunc,
		queues:           make(map[Key]Queue[CoroWithAccessesT]),
	}
	go n.start()
	return n
}

func (n *BlockingRaftNodeTranslation[Key]) start() {
	n.loadSnapshot()
	go n.applyCommits()
}

func (n *BlockingRaftNodeTranslation[Key]) access(accessFunc func() any) any {
	// coroutine corresponding to this access
	coro := n.currentlyExecuting
	var out any
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

func (n *BlockingRaftNodeTranslation[Key]) applyCommits() {
	for commit := range n.commitC {
		if commit == nil {
			// signaled to load snapshot
			n.loadSnapshot()
			continue
		}

		for _, data := range commit.data {
			// new coro is initially the only runnable one
			coro := CreateCoro[Key, []byte](n.applyFunc, data)
			runnable := []CoroWithAccessesT{CoroWithAccessesT{OpData: data, Resume: coro}}
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

func (n *BlockingRaftNodeTranslation[Key]) getSnapshot() ([]byte, error) {
	// store queues of partially completed operations
	queues := make(map[Key]Queue[PartialOpT])
	for key, ops := range n.queues {
		PartialOpTs := Queue[PartialOpT]{}
		for _, op := range ops {
			// save everything except actual coroutine which cannot be serialized
			PartialOpTs = append(PartialOpTs, PartialOpT{op.OpData, op.Accesses, op.NumResumes})
		}
		queues[key] = PartialOpTs
	}
	// also save application's RSM state
	state, err := n.snapshotFunc()
	if err != nil {
		return nil, err
	}
	snapshot := SnapshotT[Key]{state, queues}
	return encode(snapshot)
}

func (n *BlockingRaftNodeTranslation[Key]) loadSnapshot() {
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

func (n *BlockingRaftNodeTranslation[Key]) recoverFromSnapshot(data []byte) error {
	var snapshot SnapshotT[Key]
	if err := decode(data, &snapshot); err != nil {
		return err
	}

	// restore application's RSM state
	if err := n.loadSnapshotFunc(snapshot.State); err != nil {
		return err
	}

	// replay partial ops and reconstruct wait queues
	n.replaying = true
	n.queues = map[Key]Queue[CoroWithAccessesT]{}
	for key, partialOps := range snapshot.Queues {
		ops := Queue[CoroWithAccessesT]{}
		for _, partialOp := range partialOps {
			// create new version of coroutine
			resume := CreateCoro[Key, []byte](n.applyFunc, partialOp.OpData)
			coro := CoroWithAccessesT{partialOp.OpData, resume, partialOp.Accesses, partialOp.NumResumes}
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
