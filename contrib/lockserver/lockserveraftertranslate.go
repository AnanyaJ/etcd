package main

import (
	"encoding/json"
)

type LockServerAfterTranslate struct {
	locks map[string]bool

	proposeC  chan []byte
	opManager *OpManager
	appliedC  <-chan AppliedOp
}

func newAfterTranslateLockServer(proposeC chan []byte, appliedC <-chan AppliedOp) *LockServerAfterTranslate {
	s := &LockServerAfterTranslate{
		locks:     make(map[string]bool),
		proposeC:  proposeC,
		opManager: newOpManager(),
		appliedC:  appliedC,
	}
	go s.processApplied()
	return s
}

// Propose op that some RPC handler wants to replicate
func (s *LockServerAfterTranslate) startOp(opType int, lockName string, opNum int64) bool {
	op, result := s.opManager.addOp(opType, lockName, opNum)
	s.proposeC <- op.marshal()
	return <-result
}

func (s *LockServerAfterTranslate) Acquire(lockName string, opNum int64) {
	s.startOp(AcquireOp, lockName, opNum)
}

func (s *LockServerAfterTranslate) Release(lockName string, opNum int64) bool {
	return s.startOp(ReleaseOp, lockName, opNum)
}

func (s *LockServerAfterTranslate) IsLocked(lockName string, opNum int64) bool {
	return s.startOp(IsLockedOp, lockName, opNum)
}

func (s *LockServerAfterTranslate) processApplied() {
	// ops that been executed to completion
	for appliedOp := range s.appliedC {
		op := lockOpFromBytes(appliedOp.op)
		result := boolFromBytes(appliedOp.result)
		s.opManager.reportOpFinished(op.OpNum, result)
	}
}

func (s *LockServerAfterTranslate) apply(
	data []byte,
	access func(func() any) any,
	wait func(string),
	signal func(string),
) []byte {
	op := lockOpFromBytes(data)

	isLocked := access(func() any { return s.locks[op.LockName] }).(bool) // @get

	var returnVal bool
	switch op.OpType {
	case AcquireOp:
		for isLocked {
			wait(op.LockName)                                                    // keep waiting while lock is held
			isLocked = access(func() any { return s.locks[op.LockName] }).(bool) // @get
		}
		access(func() any { s.locks[op.LockName] = true; return 1 }) // @put
		returnVal = true
	case ReleaseOp:
		if isLocked {
			access(func() any { s.locks[op.LockName] = false; return 1 }) // @put
			signal(op.LockName)
		}
		returnVal = isLocked
	case IsLockedOp:
		returnVal = isLocked
	}

	return marshal(returnVal)
}

func (s *LockServerAfterTranslate) getSnapshot() ([]byte, error) {
	return json.Marshal(s.locks)
}

func (s *LockServerAfterTranslate) loadSnapshot(snapshot []byte) error {
	err := json.Unmarshal(snapshot, &s.locks)
	return err
}
