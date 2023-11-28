package main

import (
	"encoding/gob"
	"encoding/json"
	"sync"
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
	// @put
	// @get OngoingOp bool
	// @put
	// already started or finished applying
	// @put
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
	locks       map[string]bool
	ongoing     map[ClientID]OngoingOp
	ongoingLock *sync.Mutex
	proposeC    chan []byte
	opManager   *OpManager
	appliedC    <-chan AppliedLSReplOp
}

func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedLSReplOp) *LockServerRepl {
	gob.Register(OngoingOp{})
	s := &LockServerRepl{locks: make(map[string]bool), ongoing: make(map[ClientID]OngoingOp), ongoingLock: &sync.Mutex{}, proposeC: proposeC, opManager: newOpManager(), appliedC: appliedC}
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
			s.ongoingLock.Lock()
			s.ongoing[op.ClientID] = ongoingOp
			s.ongoingLock.Unlock()
			s.opManager.reportOpFinished(op.OpNum, ongoingOp.Result)
		}
	}
}
func (s *LockServerRepl) apply(data []byte, access func(func() []any) []any, wait func(string), signal func(string), broadcast func(string)) AppliedLSReplOp {
	op := lockOpFromBytes(data)
	access(func() []any {
		s.ongoingLock.Lock()
		return []any{}
	})
	retVals199036035226684983 := access(func() []any {
		ongoing, ok := s.ongoing[op.ClientID]
		return []any{ongoing, ok}
	})
	ongoing, ok := retVals199036035226684983[0].(OngoingOp), retVals199036035226684983[1].(bool)
	if ok && ongoing.OpNum == op.OpNum {
		access(func() []any {
			s.ongoingLock.Unlock()
			return []any{}
		})
		return AppliedLSReplOp{op, ongoing}
	}
	access(func() []any {
		s.ongoing[op.ClientID] = OngoingOp{OpNum: op.OpNum, Done: false}
		return []any{}
	})
	access(func() []any {
		s.ongoingLock.Unlock()
		return []any{}
	})
	retVals3933261911023622856 := access(func() []any {
		isLocked, ok := s.locks[op.LockName]
		return []any{isLocked, ok}
	})
	isLocked, ok := retVals3933261911023622856[0].(bool), retVals3933261911023622856[1].(bool)
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
			retVals8850193706589257113 := access(func() []any {
				isLocked := s.locks[op.LockName]
				return []any{isLocked}
			})
			isLocked = retVals8850193706589257113[0].(bool)
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
