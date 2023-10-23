package main

import (
	"log"

	"golang.org/x/exp/constraints"
)

type KVCoro struct {
	OpData []byte
	Resume func() (Status, []byte)
}

type Queue[Key constraints.Ordered] map[Key][]KVCoro

type BlockingKVStore[Key constraints.Ordered, Value any] struct {
	commitC  <-chan *commit
	errorC   <-chan error
	appliedC chan AppliedOp

	applyFunc func(op []byte, get func(Key) Value, put func(Key, Value), wait func(Key), signal func(Key)) []byte

	kvstore map[Key]Value
	queues  Queue[Key]
}

func newBlockingKVStore[Key constraints.Ordered, Value any](
	commitC <-chan *commit,
	errorC <-chan error,
	applyFunc func(op []byte, get func(Key) Value, put func(Key, Value), wait func(Key), signal func(Key)) []byte,
) <-chan AppliedOp {
	var kv *BlockingKVStore[Key, Value]
	appliedC := make(chan AppliedOp)
	kv = &BlockingKVStore[Key, Value]{
		commitC:   commitC,
		errorC:    errorC,
		appliedC:  appliedC,
		applyFunc: applyFunc,
		kvstore:   make(map[Key]Value),
		queues:    make(map[Key][]KVCoro),
	}
	go kv.applyCommits()
	return appliedC
}

func (kv *BlockingKVStore[Key, Value]) get(key Key) Value {
	return kv.kvstore[key]
}

func (kv *BlockingKVStore[Key, Value]) put(key Key, val Value) {
	kv.kvstore[key] = val
}

func (kv *BlockingKVStore[Key, Value]) applyCommits() {
	for commit := range kv.commitC {
		if commit == nil {
			// signaled to load snapshot
			// TODO: load snapshot
			continue
		}

		for _, data := range commit.data {
			// new coro is initially the only runnable one
			coro := CreateKVCoro[[]byte, []byte, Key, Value](kv.applyFunc, data, kv.get, kv.put)
			runnable := []KVCoro{KVCoro{OpData: data, Resume: coro}}
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
					kv.queues[key] = append(kv.queues[key], next)
				case SignalMsg:
					// this coro is still not blocked so add back to runnable stack
					runnable = append(runnable, next)
					key := status.(Signal[Key]).key
					queue := kv.queues[key]
					if len(queue) > 0 {
						// unblock exactly one coro waiting on key
						unblocked := queue[0]
						kv.queues[key] = queue[1:]
						runnable = append(runnable, unblocked)
					}
				case DoneMsg:
					// inform client that op has completed
					kv.appliedC <- AppliedOp{op: next.OpData, result: result}
				}
			}
		}

		close(commit.applyDoneC)
	}
	if err, ok := <-kv.errorC; ok {
		log.Fatal(err)
	}
}

func (kv *BlockingKVStore[Key, Value]) getSnapshot() ([]byte, error) {
	// TODO: implement
	return nil, nil
}
