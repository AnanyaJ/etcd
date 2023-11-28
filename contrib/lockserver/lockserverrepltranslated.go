package main

// import (
// 	"encoding/gob"
// 	"encoding/json"
// )

// type ClientID int64
// type AppliedLSReplOp AppliedOp[LockOp, OngoingOp]
// type OngoingOp struct {
// 	OpNum  int64
// 	Done   bool
// 	Result bool
// }
// type LockServerSnapshot struct {
// 	Locks map[ // Propose op that some RPC handler wants to replicate
// 	// ops that been executed to completion
// 	// inform client of completion
// 	// TODO: add this parameter during translation
// 	// @get bool bool (need to specify types of return values)
// 	// @put
// 	// keep waiting while lock is held
// 	// @get bool
// 	// @put
// 	// @put
// 	string]bool
// 	Ongoing map[ClientID]OngoingOp
// }
// type LockServerRepl struct {
// 	locks     map[string]bool
// 	proposeC  chan []byte
// 	opManager *OpManager
// 	appliedC  <-chan AppliedLSReplOp
// }

// func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedLSReplOp) *LockServerRepl {
// 	gob.Register(OngoingOp{})
// 	s := &LockServerRepl{locks: make(map[string]bool), proposeC: proposeC, opManager: newOpManager(), appliedC: appliedC}
// 	go s.processApplied()
// 	return s
// }

// func (s *LockServerRepl) startOp(opType int, lockName string, clientID ClientID, opNum int64) bool {
// 	op := LockOp{OpType: opType, LockName: lockName, ClientID: clientID, OpNum: opNum}
// 	result := s.opManager.addOp(opNum)
// 	s.proposeC <- op.marshal()
// 	return <-result
// }
// func (s *LockServerRepl) Acquire(lockName string, clientID ClientID, opNum int64) {
// 	s.startOp(AcquireOp, lockName, clientID, opNum)
// }
// func (s *LockServerRepl) Release(lockName string, clientID ClientID, opNum int64) bool {
// 	return s.startOp(ReleaseOp, lockName, clientID, opNum)
// }
// func (s *LockServerRepl) IsLocked(lockName string, clientID ClientID, opNum int64) bool {
// 	return s.startOp(IsLockedOp, lockName, clientID, opNum)
// }
// func (s *LockServerRepl) processApplied() {
// 	for appliedOp := range s.appliedC {
// 		op := appliedOp.op
// 		ongoingOp := appliedOp.result
// 		if ongoingOp.Done {
// 			s.opManager.reportOpFinished(op.OpNum, ongoingOp.Result)
// 		}
// 	}
// }
// func (s *LockServerRepl) apply(data []byte, access func(func() []any) []any, wait func(string), signal func(string)) AppliedLSReplOp {
// 	op := lockOpFromBytes(data)
// 	return AppliedLSReplOp{op, OngoingOp{OpNum: op.OpNum, Done: true, Result: true}}
// }
// func (s *LockServerRepl) getSnapshot() ([]byte, error) {
// 	return json.Marshal(LockServerSnapshot{Locks: s.locks})
// }
// func (s *LockServerRepl) loadSnapshot(data []byte) error {
// 	var snapshot LockServerSnapshot
// 	if err := json.Unmarshal(data, &snapshot); err != nil {
// 		return err
// 	}
// 	s.locks = snapshot.Locks
// 	return nil
// }
