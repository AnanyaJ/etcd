package main

import (
	"log"

	"golang.org/x/exp/constraints"
)

type RSMState[In, Out any] interface {
	access(in In) Out
}

type BlockingRaftNode[Key constraints.Ordered, In, Out any] struct {
	commitC  <-chan *commit
	errorC   <-chan error
	appliedC chan AppliedOp

	state     RSMState[In, Out]
	applyFunc func(wait func(Key), signal func(Key), args ...interface{}) []byte

	queues map[Key]Queue[Coro]
}

func newBlockingRaftNode[Key constraints.Ordered, In, Out any](
	commitC <-chan *commit,
	errorC <-chan error,
	state RSMState[In, Out],
	apply func(op []byte, access func(In) Out, wait func(Key), signal func(Key)) []byte,
) <-chan AppliedOp {
	var n *BlockingRaftNode[Key, In, Out]
	appliedC := make(chan AppliedOp)
	applyFunc := func(wait func(Key), signal func(Key), args ...interface{}) []byte {
		op := args[0].([]byte)
		return apply(op, n.access, wait, signal)
	}
	n = &BlockingRaftNode[Key, In, Out]{
		commitC:   commitC,
		errorC:    errorC,
		appliedC:  appliedC,
		state:     state,
		applyFunc: applyFunc,
		queues:    make(map[Key]Queue[Coro]),
	}
	go n.applyCommits()
	return appliedC
}

func (n *BlockingRaftNode[Key, In, Out]) access(in In) Out {
	// TODO: control accesses on replay by returning
	// saved access outputs directly instead of actually
	// calling access() on state
	return n.state.access(in)
}

func (n *BlockingRaftNode[Key, In, Out]) applyCommits() {
	for commit := range n.commitC {
		if commit == nil {
			// signaled to load snapshot
			// TODO: load snapshot
			continue
		}

		for _, data := range commit.data {
			// new coro is initially the only runnable one
			coro := CreateCoro[Key, []byte](n.applyFunc, data)
			runnable := []Coro{Coro{OpData: data, Resume: coro}}
			// resume coros until no coro can make more progress -- ensures
			// deterministic behavior since timing of ops arriving on
			// started channel cannot be controlled
			for len(runnable) > 0 {
				// resume coros in stack order
				next := runnable[len(runnable)-1]
				runnable = runnable[:len(runnable)-1]

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
	// TODO: implement
	return nil, nil
}
