package main

import (
	"encoding/json"
	"log"
	"sync"
)

type LockServerRepl struct {
	proposeC chan []byte
	appliedC <-chan AppliedOp

	mu        sync.Mutex
	nextOpNum int
	opResults map[int]chan bool
}

func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedOp) *LockServerRepl {
	s := &LockServerRepl{
		proposeC:  proposeC,
		appliedC:  appliedC,
		mu:        sync.Mutex{},
		opResults: make(map[int]chan bool),
	}
	go s.processApplied()
	return s
}

// Propose op that some RPC handler wants to replicate
func (s *LockServerRepl) startOp(OpType int, lockName string) bool {
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

func (s *LockServerRepl) Acquire(lockName string) {
	s.startOp(AcquireOp, lockName)
}

func (s *LockServerRepl) Release(lockName string) bool {
	return s.startOp(ReleaseOp, lockName)
}

func (s *LockServerRepl) IsLocked(lockName string) bool {
	return s.startOp(IsLockedOp, lockName)
}

func (s *LockServerRepl) processApplied() {
	// ops that been executed to completion by blocking Raft
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

func (s *LockServerRepl) apply(
	data []byte,
	access func(KVOp) bool,
	wait func(string),
	signal func(string),
) []byte {
	op := LockOp{}
	err := json.Unmarshal(data, &op)
	if err != nil {
		log.Fatalf("Apply failed to unmarshal op %v: %v \n", data, err)
	}

	// access functions to read and write lock state
	// note: uses provided access callback instead of directly modifying lock state
	// to allow for correct replay after snapshotting
	isLocked := func() bool { return access(KVOp{opType: GetOp, key: op.LockName}) }
	setLocked := func(val bool) { access(KVOp{opType: PutOp, key: op.LockName, val: val}) }

	var returnVal bool
	switch op.OpType {
	case AcquireOp:
		for isLocked() {
			wait(op.LockName) // keep waiting while lock is held
		}
		setLocked(true)
		returnVal = true
	case ReleaseOp:
		returnVal = isLocked()
		if isLocked() {
			setLocked(false)
			signal(op.LockName)
		}
	case IsLockedOp:
		returnVal = isLocked()
	}

	ret, err := json.Marshal(returnVal)
	if err != nil {
		log.Fatal(err)
	}
	return ret
}
