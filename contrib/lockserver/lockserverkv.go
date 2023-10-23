package main

import (
	"encoding/json"
	"log"
	"sync"
)

type LockServerKV struct {
	proposeC chan []byte
	appliedC <-chan AppliedOp

	mu        sync.Mutex
	nextOpNum int
	opResults map[int]chan bool
}

func newKVLockServer(proposeC chan []byte, appliedC <-chan AppliedOp) *LockServerKV {
	s := &LockServerKV{
		proposeC:  proposeC,
		appliedC:  appliedC,
		mu:        sync.Mutex{},
		opResults: make(map[int]chan bool),
	}
	go s.processApplied()
	return s
}

func (s *LockServerKV) startOp(OpType int, lockName string) bool {
	result := make(chan bool)

	// assign each op a unique op number so that applier knows which
	// result channel to signal on
	s.mu.Lock()
	opNum := s.nextOpNum
	s.nextOpNum++
	s.opResults[opNum] = result
	s.mu.Unlock()

	op := LockOp{OpNum: opNum, OpType: OpType, LockName: lockName}

	data, err := json.Marshal(&op)
	if err != nil {
		log.Fatal(err)
	}
	s.proposeC <- data
	return <-result
}

func (s *LockServerKV) Acquire(lockName string) {
	s.startOp(AcquireOp, lockName)
}

func (s *LockServerKV) Release(lockName string) bool {
	return s.startOp(ReleaseOp, lockName)
}

func (s *LockServerKV) IsLocked(lockName string) bool {
	return s.startOp(IsLockedOp, lockName)
}

func (s *LockServerKV) processApplied() {
	for appliedOp := range s.appliedC {
		op := LockOp{}
		err := json.Unmarshal(appliedOp.op, &op)
		if err != nil {
			log.Fatalf("Failed to unmarshal applied op")
		}

		// inform RPC handler of op completion and result
		resultChan, ok := s.opResults[op.OpNum]
		if ok {
			var res bool
			err := json.Unmarshal(appliedOp.result, &res)
			if err != nil {
				log.Fatalf("Failed to unmarshal result of applied op")
			}
			resultChan <- res
			delete(s.opResults, op.OpNum)
		}
	}
}

func (s *LockServerKV) apply(
	data []byte,
	get func(string) bool,
	put func(string, bool),
	wait func(string),
	signal func(string),
) []byte {
	op := LockOp{}
	err := json.Unmarshal(data, &op)
	if err != nil {
		log.Fatalf("Apply failed to unmarshal op %v: %v \n", data, err)
	}
	var returnVal bool
	switch op.OpType {
	case AcquireOp:
		// may be able to remove loop depending on semantics of scheduler
		for get(op.LockName) {
			wait(op.LockName) // keep waiting while lock is held
		}
		put(op.LockName, true)
		returnVal = true
	case ReleaseOp:
		// release lock if held
		returnVal = get(op.LockName)
		if get(op.LockName) {
			put(op.LockName, false)
			signal(op.LockName)
		}
	case IsLockedOp:
		returnVal = get(op.LockName)
	}

	ret, err := json.Marshal(returnVal)
	if err != nil {
		log.Fatal(err)
	}
	return ret
}
