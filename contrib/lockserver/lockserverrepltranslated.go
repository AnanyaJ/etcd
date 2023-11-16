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
	Locks map[ // Propose op that some RPC handler wants to replicate
	// ops that been executed to completion
	// store result in case of duplicate requests
	// inform client of completion
	// TODO: add this parameter during translation
	// @get OngoingOp bool
	// already started or finished applying
	// @put
	// @get bool bool (need to specify types of return values)
	// @put
	// keep waiting while lock is held
	// @get bool
	// @put
	// @put
	string]bool
	Ongoing map[ClientID]OngoingOp
}
type LockServerRepl struct {
	locks     map[string]bool
	ongoing   map[ClientID]OngoingOp
	proposeC  chan []byte
	opManager *OpManager
	appliedC  <-chan AppliedLSReplOp
}

func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedLSReplOp) *LockServerRepl {
	gob.Register(OngoingOp{})
	s := &LockServerRepl{locks: make(map[string]bool), ongoing: make(map[ClientID]OngoingOp), proposeC: proposeC, opManager: newOpManager(), appliedC: appliedC}
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
			s.ongoing[op.ClientID] = ongoingOp
			s.opManager.reportOpFinished(op.OpNum, ongoingOp.Result)
		}
	}
}
func (s *LockServerRepl) apply(data []byte, access func(func() []any) []any, wait func(string), signal func(string)) AppliedLSReplOp {
	op := lockOpFromBytes(data)
	retVals3894464442764287959 := access(func() []any {
		ongoing, ok := s.ongoing[op.ClientID]
		return []any{ongoing, ok}
	})
	ongoing, ok := retVals3894464442764287959[0].(OngoingOp), retVals3894464442764287959[1].(bool)
	if ok && ongoing.OpNum == op.OpNum {
		return AppliedLSReplOp{op, ongoing}
	}
	access(func() []any {
		s.ongoing[op.ClientID] = OngoingOp{OpNum: op.OpNum, Done: false}
		return []any{}
	})
	retVals5514724011473101821 := access(func() []any {
		isLocked, ok := s.locks[op.LockName]
		return []any{isLocked, ok}
	})
	isLocked, ok := retVals5514724011473101821[0].(bool), retVals5514724011473101821[1].(bool)
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
			retVals109645646277137003 := access(func() []any {
				isLocked := s.locks[op.LockName]
				return []any{isLocked}
			})
			isLocked = retVals109645646277137003[0].(bool)
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
	return json.Marshal(LockServerSnapshot{Locks: s.locks, Ongoing: s.ongoing})
}
func (s *LockServerRepl) loadSnapshot(data []byte) error {
	var snapshot LockServerSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	s.locks = snapshot.Locks
	s.ongoing = snapshot.Ongoing
	return nil
}
