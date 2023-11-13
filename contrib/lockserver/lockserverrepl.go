package main

// import "encoding/json"

// type ClientID int64

// type OngoingOp struct {
// 	OpNum  int64
// 	Done   bool
// 	Result bool
// }

// type LockServerRepl struct {
// 	locks   map[string]bool
// 	ongoing map[ClientID]OngoingOp

// 	proposeC  chan []byte
// 	opManager *OpManager
// 	appliedC  <-chan AppliedOp
// }

// func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedOp) *LockServerRepl {
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

// func (s *LockServerRepl) handleApplied(appliedOp AppliedOp) {
// 	op := lockOpFromBytes(appliedOp.op)
// 	var ongoingOp OngoingOp
// 	decodeNoErr(appliedOp.result, &ongoingOp)
// 	if ongoingOp.Done {
// 		s.ongoing[op.ClientID] = ongoingOp                       // store result in case of duplicate requests
// 		s.opManager.reportOpFinished(op.OpNum, ongoingOp.Result) // inform client of completion
// 	}
// }

// func (s *LockServerRepl) processApplied() {
// 	// ops that been executed to completion
// 	for appliedOp := range s.appliedC {
// 		s.handleApplied(appliedOp)
// 	}
// }

// func (s *LockServerRepl) flushApplied() {
// 	// process any already-applied ops
// 	for {
// 		select {
// 		case appliedOp := <-s.appliedC:
// 			s.handleApplied(appliedOp)
// 		default:
// 			return
// 		}
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
// 	ongoing, ok := s.ongoing[op.ClientID]
// 	if ok && ongoing.OpNum == op.OpNum {
// 		return encodeNoErr(ongoing) // already started or finished applying
// 	}
// 	s.ongoing[op.ClientID] = OngoingOp{OpNum: op.OpNum, Done: false}

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

// 	return encodeNoErr(OngoingOp{OpNum: op.OpNum, Done: true, Result: returnVal})
// }

// func (s *LockServerRepl) getSnapshot() ([]byte, error) {
// 	return json.Marshal(s.locks)
// }

// func (s *LockServerRepl) loadSnapshot(snapshot []byte) error {
// 	return json.Unmarshal(snapshot, &s.locks)
// }
