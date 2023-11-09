package main

import "encoding/json"

type LockServerTranslate struct {
	locks map[string]bool

	proposeC  chan []byte
	opManager *OpManager
	appliedC  <-chan AppliedOp
}

func newTranslateLockServer(proposeC chan []byte, appliedC <-chan AppliedOp) *LockServerTranslate {
	s := &LockServerTranslate{
		locks:     make(map[string]bool),
		proposeC:  proposeC,
		opManager: newOpManager(),
		appliedC:  appliedC,
	}
	go s.processApplied()
	return s
}

// Propose op that some RPC handler wants to replicate
func (s *LockServerTranslate) startOp(opType int, lockName string, opNum int64) bool {
	op, result := s.opManager.addOp(opType, lockName, opNum)
	s.proposeC <- op.marshal()
	return <-result
}

func (s *LockServerTranslate) Acquire(lockName string, opNum int64) {
	s.startOp(AcquireOp, lockName, opNum)
}

func (s *LockServerTranslate) Release(lockName string, opNum int64) bool {
	return s.startOp(ReleaseOp, lockName, opNum)
}

func (s *LockServerTranslate) IsLocked(lockName string, opNum int64) bool {
	return s.startOp(IsLockedOp, lockName, opNum)
}

func (s *LockServerTranslate) processApplied() {
	// ops that been executed to completion
	for appliedOp := range s.appliedC {
		op := lockOpFromBytes(appliedOp.op)
		result := boolFromBytes(appliedOp.result)
		s.opManager.reportOpFinished(op.OpNum, result)
	}
}

func (s *LockServerTranslate) apply(
	data []byte,
	access func(func() any) any, // TODO: add this parameter during translation
	wait func(string),
	signal func(string),
) []byte {
	op := lockOpFromBytes(data)

	// TODO: allow @gets to return more than one value
	// note that RSM state can only be exposed through return values, not pointer parameters
	isLocked := s.locks[op.LockName] // @get bool (need to specify type of return value)

	var returnVal bool
	switch op.OpType {
	case AcquireOp:
		for isLocked {
			wait(op.LockName)               // keep waiting while lock is held
			isLocked = s.locks[op.LockName] // @get bool
		}
		s.locks[op.LockName] = true // @put
		returnVal = true
	case ReleaseOp:
		if isLocked {
			s.locks[op.LockName] = false // @put
			signal(op.LockName)
		}
		returnVal = isLocked
	case IsLockedOp:
		returnVal = isLocked
	}

	return marshal(returnVal)
}

func (s *LockServerTranslate) getSnapshot() ([]byte, error) {
	return json.Marshal(s.locks)
}

func (s *LockServerTranslate) loadSnapshot(snapshot []byte) error {
	return json.Unmarshal(snapshot, &s.locks)
}
