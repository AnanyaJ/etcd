package main

import (
	"log"

	"golang.org/x/exp/constraints"
)

type Coro[ReturnType any] struct {
	OpData []byte
	Resume func() (Status, ReturnType)
}

type BlockingKVStore[Key constraints.Ordered, Value any, ReturnType any] struct {
	commitC  <-chan *commit
	errorC   <-chan error
	appliedC chan ReturnType

	applyFunc func(wait func(Key), signal func(Key), args ...interface{}) ReturnType

	kvstore map[Key]Value
	queues  map[Key]Queue[Coro[ReturnType]]
}

func newBlockingKVStore[Key constraints.Ordered, Value any, ReturnType any](
	commitC <-chan *commit,
	errorC <-chan error,
	apply func(op []byte, get func(Key) Value, put func(Key, Value), wait func(Key), signal func(Key)) ReturnType,
) <-chan ReturnType {
	var kv *BlockingKVStore[Key, Value, ReturnType]
	appliedC := make(chan ReturnType)
	applyFunc := func(wait func(Key), signal func(Key), args ...interface{}) ReturnType {
		op := args[0].([]byte)
		return apply(op, kv.get, kv.put, wait, signal)
	}
	kv = &BlockingKVStore[Key, Value, ReturnType]{
		commitC:   commitC,
		errorC:    errorC,
		appliedC:  appliedC,
		applyFunc: applyFunc,
		kvstore:   make(map[Key]Value),
		queues:    make(map[Key]Queue[Coro[ReturnType]]),
	}
	go kv.applyCommits()
	return appliedC
}

func (kv *BlockingKVStore[Key, Value, ReturnType]) get(key Key) Value {
	return kv.kvstore[key]
}

func (kv *BlockingKVStore[Key, Value, ReturnType]) put(key Key, val Value) {
	kv.kvstore[key] = val
}

func (kv *BlockingKVStore[Key, Value, ReturnType]) applyCommits() {
	for commit := range kv.commitC {
		if commit == nil {
			// signaled to load snapshot
			// TODO: load snapshot
			continue
		}

		for _, data := range commit.data {
			// new coro is initially the only runnable one
			coro := CreateCoro[Key, ReturnType](kv.applyFunc, data)
			runnable := []Coro[ReturnType]{Coro[ReturnType]{OpData: data, Resume: coro}}
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
					kv.appliedC <- result
				}
			}
		}

		close(commit.applyDoneC)
	}
	if err, ok := <-kv.errorC; ok {
		log.Fatal(err)
	}
}

func (kv *BlockingKVStore[Key, Value, ReturnType]) getSnapshot() ([]byte, error) {
	// TODO: implement
	return nil, nil
}
