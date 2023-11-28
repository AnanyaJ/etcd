package main

import (
	"encoding/gob"
	"encoding/json"
)

type ClientID int64
type AppliedLSReplOp AppliedOp[LockOp, OngoingOp]
type OngoingOp struct {
	OpNum  int64
	Done   bool
	Result bool
}
type LockServerSnapshot struct {
	Locks map[ // ongoing     map[ClientID]OngoingOp
	// ongoingLock *sync.Mutex
	// ongoing:     make(map[ClientID]OngoingOp),
	// ongoingLock: &sync.Mutex{},
	// Propose op that some RPC handler wants to replicate
	// ops that been executed to completion
	// s.ongoingLock.Lock()
	// s.ongoing[op.ClientID] = ongoingOp // store result in case of duplicate requests
	// s.ongoingLock.Unlock()
	// inform client of completion
	// TODO: add this parameter during translation
	// @get bool bool (need to specify types of return values)
	// @put
	// keep waiting while lock is held
	// @get bool
	// @put
	// @put
	// s.ongoing = snapshot.Ongoing
	string]bool
	Ongoing map[ClientID]OngoingOp
}
type LockServerRepl struct {
	locks     map[string]bool
	proposeC  chan []byte
	opManager *OpManager
	appliedC  <-chan AppliedLSReplOp
}

func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedLSReplOp) *LockServerRepl {
	gob.Register(OngoingOp{})
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
func (s *LockServerRepl) processApplied() {
	for appliedOp := range s.appliedC {
		op := appliedOp.op
		ongoingOp := appliedOp.result
		if ongoingOp.Done {
			s.opManager.reportOpFinished(op.OpNum, ongoingOp.Result)
		}
	}
}
func (s *LockServerRepl) apply(data []byte, access func(func() []any) []any, wait func(string), signal func(string)) AppliedLSReplOp {
	op := lockOpFromBytes(data)
	retVals1992554826982793435 := access(func() []any {
		isLocked, ok := s.locks[op.LockName]
		return []any{isLocked, ok}
	})
	isLocked, ok := retVals1992554826982793435[0].(bool), retVals1992554826982793435[1].(bool)
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
			retVals4892476052222618333 := access(func() []any {
				isLocked := s.locks[op.LockName]
				return []any{isLocked}
			})
			isLocked = retVals4892476052222618333[0].(bool)
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
	return AppliedLSReplOp{op, OngoingOp{OpNum: op.OpNum, Done: true, Result: returnVal}}
}
func (s *LockServerRepl) getSnapshot() ([]byte, error) {
	return json.Marshal(LockServerSnapshot{Locks: s.locks})
}
func (s *LockServerRepl) loadSnapshot(data []byte) error {
	var snapshot LockServerSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	s.locks = snapshot.Locks
	return nil
}
