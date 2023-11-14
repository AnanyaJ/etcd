package main

import (
	"encoding/gob"
	"encoding/json"
)

type ClientID int64
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
	appliedC  <-chan AppliedOp
}

func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedOp) *LockServerRepl {
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
		op := lockOpFromBytes(appliedOp.op)
		var ongoingOp OngoingOp
		decodeNoErr(appliedOp.result, &ongoingOp)
		if ongoingOp.Done {
			s.ongoing[op.ClientID] = ongoingOp
			s.opManager.reportOpFinished(op.OpNum, ongoingOp.Result)
		}
	}
}
func (s *LockServerRepl) apply(data []byte, access func(func() []any) []any, wait func(string), signal func(string)) []byte {
	op := lockOpFromBytes(data)
	retVals4115410823380355554 := access(func() []any {
		ongoing, ok := s.ongoing[op.ClientID]
		return []any{ongoing, ok}
	})
	ongoing, ok := retVals4115410823380355554[0].(OngoingOp), retVals4115410823380355554[1].(bool)
	if ok && ongoing.OpNum == op.OpNum {
		return encodeNoErr(ongoing)
	}
	access(func() []any {
		s.ongoing[op.ClientID] = OngoingOp{OpNum: op.OpNum, Done: false}
		return []any{}
	})
	retVals4599741972382674047 := access(func() []any {
		isLocked, ok := s.locks[op.LockName]
		return []any{isLocked, ok}
	})
	isLocked, ok := retVals4599741972382674047[0].(bool), retVals4599741972382674047[1].(bool)
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
			retVals4458129848464405839 := access(func() []any {
				isLocked := s.locks[op.LockName]
				return []any{isLocked}
			})
			isLocked = retVals4458129848464405839[0].(bool)
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
	return encodeNoErr(OngoingOp{OpNum: op.OpNum, Done: true, Result: returnVal})
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
