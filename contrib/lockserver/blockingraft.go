package main

import (
	"fmt"
	"log"

	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"golang.org/x/exp/constraints"
)

type Access[Out any] struct {
	In  []byte
	Out Out
}

type CoroWithAccesses[Out any] struct {
	OpData     []byte
	Resume     func() (Status, []byte)
	Accesses   []Access[Out]
	NumResumes int
}

type RSMState[In, Out any] interface {
	access(in In) Out
}

type PartialOp[Out any] struct {
	OpData     []byte
	Accesses   []Access[Out]
	NumResumes int
}

type Snapshot[Key constraints.Ordered, In, Out any] struct {
	State  RSMState[In, Out]
	Queues map[Key]Queue[PartialOp[Out]]
}

type BlockingRaftNode[Key constraints.Ordered, In, Out any] struct {
	snapshotter *snap.Snapshotter
	commitC     <-chan *commit
	errorC      <-chan error
	appliedC    chan AppliedOp

	state     RSMState[In, Out]
	applyFunc func(wait func(Key), signal func(Key), args ...interface{}) []byte

	queues map[Key]Queue[CoroWithAccesses[Out]]

	currentlyExecuting *CoroWithAccesses[Out]
	replaying          bool
}

func newBlockingRaftNode[Key constraints.Ordered, In, Out any](
	snapshotter *snap.Snapshotter,
	commitC <-chan *commit,
	errorC <-chan error,
	state RSMState[In, Out],
	apply func(op []byte, access func(In) Out, wait func(Key), signal func(Key)) []byte,
) *BlockingRaftNode[Key, In, Out] {
	var n *BlockingRaftNode[Key, In, Out]
	appliedC := make(chan AppliedOp)
	// convert apply function into generic coroutine function
	applyFunc := func(wait func(Key), signal func(Key), args ...interface{}) []byte {
		op := args[0].([]byte)
		return apply(op, n.access, wait, signal)
	}
	n = &BlockingRaftNode[Key, In, Out]{
		snapshotter: snapshotter,
		commitC:     commitC,
		errorC:      errorC,
		appliedC:    appliedC,
		state:       state,
		applyFunc:   applyFunc,
		queues:      make(map[Key]Queue[CoroWithAccesses[Out]]),
	}
	n.loadSnapshot()
	go n.applyCommits()
	return n
}

func (n *BlockingRaftNode[Key, In, Out]) access(in In) Out {
	// coroutine corresponding to this access
	coro := n.currentlyExecuting
	var out Out
	if n.replaying {
		// return saved access output instead of actually reading/modifying RSM state
		// to ensure that coroutine replay matches original execution
		if len(coro.Accesses) == 0 {
			log.Fatalf("Failed to recover partially executed operation during replay (could not find access)")
		}
		access := coro.Accesses[0]
		coro.Accesses = coro.Accesses[1:]

		// verify that next expected input is same as given input
		inBytes, err := encode(in)
		if err != nil {
			log.Fatalf("Could not encode access input: ", err)
		}
		if !equal(inBytes, access.In) {
			log.Fatalf("Access input %v does not match expected input on replay\n", in)
		}

		out = access.Out
	} else {
		out = n.state.access(in)

		// remember access in case we need to replay
		fmt.Println("access: ", in)
		inBytes, err := encode(in)
		if err != nil {
			log.Fatalf("Could not encode access input: ", err)
		}
		coro.Accesses = append(coro.Accesses, Access[Out]{inBytes, out})
	}
	return out
}

func (n *BlockingRaftNode[Key, In, Out]) applyCommits() {
	for commit := range n.commitC {
		if commit == nil {
			// signaled to load snapshot
			n.loadSnapshot()
			continue
		}

		for _, data := range commit.data {
			// new coro is initially the only runnable one
			coro := CreateCoro[Key, []byte](n.applyFunc, data)
			runnable := []CoroWithAccesses[Out]{CoroWithAccesses[Out]{OpData: data, Resume: coro}}
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

func (n *BlockingRaftNode[Key, In, Out]) getSnapshot() ([]byte, error) {
	// store queues of partially completed operations
	queues := make(map[Key]Queue[PartialOp[Out]])
	for key, ops := range n.queues {
		partialOps := Queue[PartialOp[Out]]{}
		for _, op := range ops {
			// save everything except actual coroutine which cannot be serialized
			partialOps = append(partialOps, PartialOp[Out]{op.OpData, op.Accesses, op.NumResumes})
		}
		queues[key] = partialOps
	}
	// also save application's RSM state
	snapshot := Snapshot[Key, In, Out]{n.state, queues}
	return encode(snapshot)
}

func (n *BlockingRaftNode[Key, In, Out]) loadSnapshot() {
	snapshot, err := n.snapshotter.Load()
	if err == snap.ErrNoSnapshot {
		return
	}
	if err != nil {
		log.Fatalf("Failed to load snapshot: ", err)
	}
	if snapshot != nil {
		if err := n.recoverFromSnapshot(snapshot.Data); err != nil {
			log.Fatalf("Could not recover from snapshot: ", err)
		}
	}
}

func (n *BlockingRaftNode[Key, In, Out]) recoverFromSnapshot(data []byte) error {
	var snapshot Snapshot[Key, In, Out]
	if err := decode(data, &snapshot); err != nil {
		return err
	}

	// restore application's RSM state
	n.state = snapshot.State

	// replay partial ops and reconstruct wait queues
	n.replaying = true
	n.queues = map[Key]Queue[CoroWithAccesses[Out]]{}
	for key, partialOps := range snapshot.Queues {
		ops := Queue[CoroWithAccesses[Out]]{}
		for _, partialOp := range partialOps {
			// create new version of coroutine
			resume := CreateCoro[Key, []byte](n.applyFunc, partialOp.OpData)
			coro := CoroWithAccesses[Out]{partialOp.OpData, resume, partialOp.Accesses, partialOp.NumResumes}
			// re-run coroutine until it reaches point at time of snapshot
			for i := 0; i < partialOp.NumResumes; i++ {
				n.currentlyExecuting = &coro
				coro.Resume()
			}
			ops = append(ops, coro)
		}
		n.queues[key] = ops
	}
	n.replaying = false

	return nil
}
