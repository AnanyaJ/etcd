package main

// import "encoding/json"

// type LockServerTranslate struct {
// 	locks map // Propose op that some RPC handler wants to replicate
// 	// ops that been executed to completion
// 	// TODO: add this parameter during translation
// 	// TODO: allow gets to return more than one value
// 	// @get bool (need to specify type of return value)
// 	// keep waiting while lock is held
// 	// @get bool
// 	// @put
// 	// @put
// 	[string]bool
// 	proposeC  chan []byte
// 	opManager *OpManager
// 	appliedC  <-chan AppliedOp
// }

// func newTranslateLockServer(proposeC chan []byte, appliedC <-chan AppliedOp) *LockServerTranslate {
// 	s := &LockServerTranslate{locks: make(map[string]bool), proposeC: proposeC, opManager: newOpManager(), appliedC: appliedC}
// 	go s.processApplied()
// 	return s
// }

// func (s *LockServerTranslate) startOp(opType int, lockName string, opNum int64) bool {
//  op := LockOp{OpType: opType, LockName: lockName, OpNum: opNum}
//  result := s.opManager.addOp(opNum)
// 	s.proposeC <- op.marshal()
// 	return <-result
// }
// func (s *LockServerTranslate) Acquire(lockName string, opNum int64) {
// 	s.startOp(AcquireOp, lockName, opNum)
// }
// func (s *LockServerTranslate) Release(lockName string, opNum int64) bool {
// 	return s.startOp(ReleaseOp, lockName, opNum)
// }
// func (s *LockServerTranslate) IsLocked(lockName string, opNum int64) bool {
// 	return s.startOp(IsLockedOp, lockName, opNum)
// }
// func (s *LockServerTranslate) processApplied() {
// 	for appliedOp := range s.appliedC {
// 		op := lockOpFromBytes(appliedOp.op)
// 		result := boolFromBytes(appliedOp.result)
// 		s.opManager.reportOpFinished(op.OpNum, result)
// 	}
// }
// func (s *LockServerTranslate) apply(data []byte, access func(func() any) any, wait func(string), signal func(string)) []byte {
// 	op := lockOpFromBytes(data)
// 	isLocked := access(func() any {
// 		return s.locks[op.LockName]
// 	}).(bool)
// 	var returnVal bool
// 	switch op.OpType {
// 	case AcquireOp:
// 		for isLocked {
// 			wait(op.LockName)
// 			isLocked = access(func() any {
// 				return s.locks[op.LockName]
// 			}).(bool)
// 		}
// 		access(func() any {
// 			s.locks[op.LockName] = true
// 			return 1
// 		})
// 		returnVal = true
// 	case ReleaseOp:
// 		if isLocked {
// 			access(func() any {
// 				s.locks[op.LockName] = false
// 				return 1
// 			})
// 			signal(op.LockName)
// 		}
// 		returnVal = isLocked
// 	case IsLockedOp:
// 		returnVal = isLocked
// 	}
// 	return marshal(returnVal)
// }
// func (s *LockServerTranslate) getSnapshot() ([]byte, error) {
// 	return json.Marshal(s.locks)
// }
// func (s *LockServerTranslate) loadSnapshot(snapshot []byte) error {
// 	return json.Unmarshal(snapshot, &s.locks)
// }
