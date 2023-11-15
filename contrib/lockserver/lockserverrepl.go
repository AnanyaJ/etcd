package main

// import (
// 	"encoding/gob"
// 	"encoding/json"
// )

// type ClientID int64

// type OngoingOp struct {
// 	OpNum  int64
// 	Done   bool
// 	Result bool
// }

// type LockServerSnapshot struct {
// 	Locks   map[string]bool
// 	Ongoing map[ClientID]OngoingOp
// }

// type LockServerRepl struct {
// 	locks   map[string]bool
// 	ongoing map[ClientID]OngoingOp

// 	proposeC  chan []byte
// 	opManager *OpManager
// 	appliedC  <-chan AppliedOp[LockOp, OngoingOp]
// }

// func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedOp[LockOp, OngoingOp]) *LockServerRepl {
// 	gob.Register(OngoingOp{})
// 	s := &LockServerRepl{
// 		locks:     make(map[string]bool),
// 		ongoing:   make(map[ClientID]OngoingOp),
// 		proposeC:  proposeC,
// 		opManager: newOpManager(),
// 		appliedC:  appliedC,
// 	}
// 	go s.processApplied()
// 	return s
// }

// // Propose op that some RPC handler wants to replicate
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
// 	// ops that been executed to completion
// 	for appliedOp := range s.appliedC {
// 		op := appliedOp.op
// 		ongoingOp := appliedOp.result
// 		if ongoingOp.Done {
// 			s.ongoing[op.ClientID] = ongoingOp                       // store result in case of duplicate requests
// 			s.opManager.reportOpFinished(op.OpNum, ongoingOp.Result) // inform client of completion
// 		}
// 	}
// }

// func (s *LockServerRepl) apply(
// 	data []byte,
// 	access func(func() []any) []any, // TODO: add this parameter during translation
// 	wait func(string),
// 	signal func(string),
// ) AppliedOp[LockOp, OngoingOp] {
// 	op := lockOpFromBytes(data)

// 	ongoing, ok := s.ongoing[op.ClientID] // @get OngoingOp bool
// 	if ok && ongoing.OpNum == op.OpNum {
// 		return AppliedOp[LockOp, OngoingOp]{op, ongoing} // already started or finished applying
// 	}
// 	s.ongoing[op.ClientID] = OngoingOp{OpNum: op.OpNum, Done: false} // @put

// 	isLocked, ok := s.locks[op.LockName] // @get bool bool (need to specify types of return values)
// 	if !ok {
// 		s.locks[op.LockName] = false // @put
// 	}

// 	var returnVal bool
// 	switch op.OpType {
// 	case AcquireOp:
// 		for isLocked {
// 			wait(op.LockName)               // keep waiting while lock is held
// 			isLocked = s.locks[op.LockName] // @get bool
// 		}
// 		s.locks[op.LockName] = true // @put
// 		returnVal = true
// 	case ReleaseOp:
// 		if isLocked {
// 			s.locks[op.LockName] = false // @put
// 			signal(op.LockName)
// 		}
// 		returnVal = isLocked
// 	case IsLockedOp:
// 		returnVal = isLocked
// 	}

// 	return AppliedOp[LockOp, OngoingOp]{op, OngoingOp{OpNum: op.OpNum, Done: true, Result: returnVal}}
// }

// func (s *LockServerRepl) getSnapshot() ([]byte, error) {
// 	return json.Marshal(LockServerSnapshot{Locks: s.locks, Ongoing: s.ongoing})
// }

// func (s *LockServerRepl) loadSnapshot(data []byte) error {
// 	var snapshot LockServerSnapshot
// 	if err := json.Unmarshal(data, &snapshot); err != nil {
// 		return err
// 	}
// 	s.locks = snapshot.Locks
// 	s.ongoing = snapshot.Ongoing
// 	return nil
// }
