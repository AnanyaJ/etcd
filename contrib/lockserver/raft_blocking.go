package main

import (
	"bytes"
	"encoding/gob"
	"log"
	"sync"

	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"go.etcd.io/raft/v3/raftpb"
	"golang.org/x/exp/constraints"
)

type GenCoro Coro[[]byte]

type BlockingOp struct {
	OpNum  int
	OpData []byte
}

type Queue[Key constraints.Ordered] map[Key][]GenCoro

type RaftBlockingNode[Key constraints.Ordered] struct {
	proposeC <-chan []byte
	commitC  <-chan *commit
	errorC   <-chan error
	apply    func(op []byte, wait func(Key), signal func(Key)) []byte

	mu                 sync.Mutex
	proposeWithOpNumsC chan []byte
	nextOpNum          int
	opResults          map[int]chan []byte

	queues Queue[Key]
}

func newRaftBlockingNode[Key constraints.Ordered](
	id int,
	peers []string,
	join bool,
	getSnapshot func() ([]byte, error),
	proposeC <-chan []byte,
	confChangeC <-chan raftpb.ConfChange,
	apply func(op []byte, wait func(Key), signal func(Key)) []byte,
) (<-chan error, <-chan *snap.Snapshotter) {
	proposeWithOpNumsC := make(<-chan []byte)
	commitC, errorC, snapshotterReady := newRaftNode(id, peers, join, getSnapshot, proposeWithOpNumsC, confChangeC, false)
	n := &RaftBlockingNode[Key]{
		proposeC:  proposeC,
		commitC:   commitC,
		errorC:    errorC,
		apply:     apply,
		mu:        sync.Mutex{},
		opResults: make(map[int]chan []byte),
		queues:    make(map[Key][]GenCoro),
	}
	go n.applyCommits()
	return errorC, snapshotterReady
}

func (n *RaftBlockingNode[Key]) processProposals() {
	for prop := range n.proposeC {
		result := make(chan []byte)

		// assign each op a unique op number so that applier knows which
		// result channel to signal on
		n.mu.Lock()
		opNum := n.nextOpNum
		n.nextOpNum++
		n.opResults[opNum] = result
		n.mu.Unlock()

		op := BlockingOp{OpNum: opNum, OpData: prop}

		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(op); err != nil {
			log.Fatal(err)
		}
		n.proposeWithOpNumsC <- buf.Bytes()
	}
}

func (n *RaftBlockingNode[Key]) applyCommits() {
	for commit := range n.commitC {
		if commit == nil {
			// signaled to load snapshot
			// TODO: load snapshot
			continue
		}

		for _, data := range commit.data {
			var op BlockingOp
			dec := gob.NewDecoder(bytes.NewBuffer(data))
			if err := dec.Decode(&op); err != nil {
				log.Fatalf("raft_blocking: could not decode message (%v)", err)
			}

			// new coro is initially the only runnable one
			coro := CreateCoro[[]byte, Key, []byte](n.apply, data)
			runnable := []GenCoro{GenCoro{OpNum: op.OpNum, Resume: coro}}
			// resume coros until no coro can make more progress -- ensures
			// deterministic behavior since timing of ops arriving on
			// started channel cannot be controlled
			for len(runnable) > 0 {
				// resume coros in stack order
				next := runnable[len(runnable)-1]
				runnable = runnable[:len(runnable)-1]

				status, output := next.Resume()
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
						// unblock exactly one coro waiting on lock
						unblocked := queue[0]
						n.queues[key] = queue[1:]
						runnable = append(runnable, unblocked)
					}
				case DoneMsg:
					// inform RPC handler of op completion and result
					resultChan, ok := n.opResults[next.OpNum]
					if ok {
						resultChan <- output
						delete(n.opResults, next.OpNum)
					}
				}
			}
		}

		close(commit.applyDoneC)
	}
	if err, ok := <-n.errorC; ok {
		log.Fatal(err)
	}
}
