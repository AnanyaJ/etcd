package main

import (
	"encoding/json"
)

type ClientID int64
type OngoingOp struct {
	opNum  int64
	done   bool
	result bool
}
type LockServerRepl struct {
	locks map[ // Propose op that some RPC handler wants to replicate
	// op that has been executed to completion
	// ops that been executed to completion
	// TODO: add this parameter during translation
	// make sure we know which operations have completed
	// report again in case first one was lost
	// @get bool bool (need to specify types of return values)
	// @put
	// keep waiting while lock is held
	// @get bool
	// @put
	// @put
	string]bool
	started   map[ClientID]OngoingOp
	proposeC  chan []byte
	opManager *OpManager
	appliedC  <-chan AppliedOp
}

func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedOp) *LockServerRepl {
	s := &LockServerRepl{locks: make(map[string]bool), proposeC: proposeC, opManager: newOpManager(), appliedC: appliedC}
	go s.processApplied()
	return s
}

func (s *LockServerRepl) startOp(opType int, lockName string, clientID ClientID, opNum int64) bool {
	op := LockOp{OpType: opType, LockName: lockName, ClientID: clientID, OpNum: opNum}
	result := s.opManager.addOp(opNum)
	s.proposeC <- op.marshal()
	return <-result
}
func (s *LockServerRepl) Acquire(lockName string, clientID ClientID, opNum int64) {
	s.startOp(AcquireOp, lockName, clientID, opNum)
}
func (s *LockServerRepl) Release(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(ReleaseOp, lockName, clientID, opNum)
}
func (s *LockServerRepl) IsLocked(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(IsLockedOp, lockName, clientID, opNum)
}
func (s *LockServerRepl) handleApplied(appliedOp AppliedOp) {
	op := lockOpFromBytes(appliedOp.op)
	var ongoingOp OngoingOp
	fromBytes(appliedOp.result, &ongoingOp)
	s.started[op.ClientID] = ongoingOp
	s.opManager.reportOpFinished(op.OpNum, ongoingOp.result)
}
func (s *LockServerRepl) flushApplied() {
	for {
		select {
		case appliedOp := <-s.appliedC:
			s.handleApplied(appliedOp)
		default:
			return
		}
	}
}
func (s *LockServerRepl) processApplied() {
	for appliedOp := range s.appliedC {
		s.handleApplied(appliedOp)
	}
}
func (s *LockServerRepl) apply(data []byte, access func(func() []any) []any, wait func(string), signal func(string)) []byte {
	op := lockOpFromBytes(data)
	s.flushApplied()
	started, ok := s.started[op.ClientID]
	if ok {
		if started.done {
			s.opManager.reportOpFinished(op.OpNum, started.result)
		}
		return marshal(started.result)
	}
	s.started[op.ClientID] = OngoingOp{opNum: op.OpNum, done: false}
	retVals2233406054134070233 := access(func() []any {
		isLocked, ok := s.locks[op.LockName]
		return []any{isLocked, ok}
	})
	isLocked, ok := retVals2233406054134070233[0].(bool), retVals2233406054134070233[1].(bool)
	if !ok {
		access(func() []any {
			s.locks[op.LockName] = false
			return []any{}
		})
	}
	var returnVal bool
	switch op.OpType {
	case AcquireOp:
		for isLocked {
			wait(op.LockName)
			retVals4991596523215700961 := access(func() []any {
				isLocked := s.locks[op.LockName]
				return []any{isLocked}
			})
			isLocked = retVals4991596523215700961[0].(bool)
		}
		access(func() []any {
			s.locks[op.LockName] = true
			return []any{}
		})
		returnVal = true
	case ReleaseOp:
		if isLocked {
			access(func() []any {
				s.locks[op.LockName] = false
				return []any{}
			})
			signal(op.LockName)
		}
		returnVal = isLocked
	case IsLockedOp:
		returnVal = isLocked
	}
	return marshal(OngoingOp{opNum: op.OpNum, done: true, result: returnVal})
}
func (s *LockServerRepl) getSnapshot() ([]byte, error) {
	return json.Marshal(s.locks)
}
func (s *LockServerRepl) loadSnapshot(snapshot []byte) error {
	return json.Unmarshal(snapshot, &s.locks)
}
