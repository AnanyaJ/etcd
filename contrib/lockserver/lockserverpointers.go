package main

import (
	"encoding/json"
	"log"
)

type LockServerPointers struct {
	state []byte

	proposeC  chan []byte
	opManager *OpManager
	appliedC  <-chan AppliedLockOp
}

func newPointersLockServer(proposeC chan []byte, appliedC <-chan AppliedLockOp) *LockServerPointers {
	locks := make(map[string]bool)
	state, err := json.Marshal(locks)
	if err != nil {
		log.Fatalf("Failed to marshal state")
	}

	s := &LockServerPointers{
		state:     state,
		proposeC:  proposeC,
		opManager: newOpManager(),
		appliedC:  appliedC,
	}
	go s.processApplied()
	return s
}

func (s *LockServerPointers) startOp(opType int, lockName string, clientID ClientID, opNum int64) bool {
	op := LockOp{OpType: opType, LockName: lockName, ClientID: clientID, OpNum: opNum}
	result := s.opManager.addOp(opNum)
	s.proposeC <- op.marshal()
	return <-result
}

func (s *LockServerPointers) Acquire(lockName string, clientID ClientID, opNum int64) {
	s.startOp(AcquireOp, lockName, clientID, opNum)
}

func (s *LockServerPointers) Release(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(ReleaseOp, lockName, clientID, opNum)
}

func (s *LockServerPointers) IsLocked(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(IsLockedOp, lockName, clientID, opNum)
}

func (s *LockServerPointers) processApplied() {
	// ops that been executed to completion
	for appliedOp := range s.appliedC {
		s.opManager.reportOpFinished(appliedOp.op.OpNum, appliedOp.result)
	}
}

func (s *LockServerPointers) getLocks(access func(func() []any) []any) map[string]bool {
	var locks map[string]bool
	err := json.Unmarshal(s.state, &locks) // @get error
	if err != nil {
		log.Fatalf("Failed to unmarshal state")
	}
	return locks
}

func (s *LockServerPointers) save(locks map[string]bool) {
	var err error
	s.state, err = json.Marshal(locks)
	if err != nil {
		log.Fatalf("Failed to marshal state")
	}
}

func (s *LockServerPointers) apply(
	data []byte,
	access func(func() []any) []any,
	wait func(string),
	signal func(string),
) AppliedLockOp {
	op := lockOpFromBytes(data)

	locks := s.getLocks(access)
	// note that the below must be annotated even though it does not directly
	// read from s.state
	isLocked := locks[op.LockName] // @get bool

	var returnVal bool
	switch op.OpType {
	case AcquireOp:
		for isLocked {
			wait(op.LockName) // keep waiting while lock is held
			locks = s.getLocks(access)
			isLocked = locks[op.LockName] // @get bool
		}
		locks[op.LockName] = true // does not need an annotation since s.state is not modified
		s.save(locks)             // @put
		returnVal = true
	case ReleaseOp:
		if isLocked {
			locks[op.LockName] = false
			s.save(locks) // @put
			signal(op.LockName)
		}
		returnVal = isLocked
	case IsLockedOp:
		returnVal = isLocked
	}

	return AppliedLockOp{op, returnVal}
}

func (s *LockServerPointers) getSnapshot() ([]byte, error) {
	return s.state, nil
}

func (s *LockServerPointers) loadSnapshot(snapshot []byte) error {
	s.state = snapshot
	return nil
}
