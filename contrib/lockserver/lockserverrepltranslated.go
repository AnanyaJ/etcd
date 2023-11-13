package main

import "encoding/json"

type ClientID int64
type OngoingOp struct {
	OpNum  int64
	Done   bool
	Result bool
}
type LockServerRepl struct {
	locks map // Propose op that some RPC handler wants to replicate
	// store result in case of duplicate requests
	// inform client of completion
	// ops that been executed to completion
	// process any already-applied ops
	// TODO: add this parameter during translation
	// make sure we know which operations have completed
	// already started or finished applying
	// @get bool bool (need to specify types of return values)
	// @put
	// keep waiting while lock is held
	// @get bool
	// @put
	// @put
	[string]bool
	ongoing   map[ClientID]OngoingOp
	proposeC  chan []byte
	opManager *OpManager
	appliedC  <-chan AppliedOp
}

func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedOp) *LockServerRepl {
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
func (s *LockServerRepl) handleApplied(appliedOp AppliedOp) {
	op := lockOpFromBytes(appliedOp.op)
	var ongoingOp OngoingOp
	decodeNoErr(appliedOp.result, &ongoingOp)
	if ongoingOp.Done {
		s.ongoing[op.ClientID] = ongoingOp
		s.opManager.reportOpFinished(op.OpNum, ongoingOp.Result)
	}
}
func (s *LockServerRepl) processApplied() {
	for appliedOp := range s.appliedC {
		s.handleApplied(appliedOp)
	}
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
func (s *LockServerRepl) apply(data []byte, access func(func() []any) []any, wait func(string), signal func(string)) []byte {
	op := lockOpFromBytes(data)
	s.flushApplied()
	ongoing, ok := s.ongoing[op.ClientID]
	if ok && ongoing.OpNum == op.OpNum {
		return encodeNoErr(ongoing)
	}
	s.ongoing[op.ClientID] = OngoingOp{OpNum: op.OpNum, Done: false}
	retVals6472599221004935873 := access(func() []any {
		isLocked, ok := s.locks[op.LockName]
		return []any{isLocked, ok}
	})
	isLocked, ok := retVals6472599221004935873[0].(bool), retVals6472599221004935873[1].(bool)
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
			retVals3192852826934793604 := access(func() []any {
				isLocked := s.locks[op.LockName]
				return []any{isLocked}
			})
			isLocked = retVals3192852826934793604[0].(bool)
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
	return json.Marshal(s.locks)
}
func (s *LockServerRepl) loadSnapshot(snapshot []byte) error {
	return json.Unmarshal(snapshot, &s.locks)
}
