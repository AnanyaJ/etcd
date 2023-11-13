package main

// import "encoding/json"

// type ClientID int64

// type OngoingOp struct {
// 	opNum  int64
// 	done   bool
// 	result bool
// }

// type LockServerRepl struct {
// 	locks   map[string]bool
// 	started map[ClientID]OngoingOp

// 	proposeC  chan []byte
// 	opManager *OpManager
// 	appliedC  <-chan AppliedOp
// }

// func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedOp) *LockServerRepl {
// 	s := &LockServerRepl{
// 		locks:     make(map[string]bool),
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

// func (s *LockServerRepl) handleApplied(appliedOp AppliedOp) {
// 	op := lockOpFromBytes(appliedOp.op)
// 	var ongoingOp OngoingOp
// 	fromBytes(appliedOp.result, &ongoingOp)
// 	s.started[op.ClientID] = ongoingOp
// 	s.opManager.reportOpFinished(op.OpNum, ongoingOp.result)
// }

// func (s *LockServerRepl) flushApplied() {
// 	for {
// 		select {
// 		// op that has been executed to completion
// 		case appliedOp := <-s.appliedC:
// 			s.handleApplied(appliedOp)
// 		default:
// 			return
// 		}
// 	}
// }

// func (s *LockServerRepl) processApplied() {
// 	// ops that been executed to completion
// 	for appliedOp := range s.appliedC {
// 		s.handleApplied(appliedOp)
// 	}
// }

// func (s *LockServerRepl) apply(
// 	data []byte,
// 	access func(func() []any) []any, // TODO: add this parameter during translation
// 	wait func(string),
// 	signal func(string),
// ) []byte {
// 	op := lockOpFromBytes(data)

// 	s.flushApplied() // make sure we know which operations have completed
// 	started, ok := s.started[op.ClientID]
// 	if ok {
// 		if started.done {
// 			// report again in case first one was lost
// 			s.opManager.reportOpFinished(op.OpNum, started.result)
// 		}
// 		return marshal(started.result)
// 	}

// 	s.started[op.ClientID] = OngoingOp{opNum: op.OpNum, done: false}

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

// 	return marshal(OngoingOp{opNum: op.OpNum, done: true, result: returnVal})
// }

// func (s *LockServerRepl) getSnapshot() ([]byte, error) {
// 	return json.Marshal(s.locks)
// }

// func (s *LockServerRepl) loadSnapshot(snapshot []byte) error {
// 	return json.Unmarshal(snapshot, &s.locks)
// }
