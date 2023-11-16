package main

type LockServerKV struct {
	proposeC  chan []byte
	opManager *OpManager
	appliedC  <-chan AppliedLockOp
}

func newKVLockServer(proposeC chan []byte, appliedC <-chan AppliedLockOp) *LockServerKV {
	s := &LockServerKV{
		proposeC:  proposeC,
		appliedC:  appliedC,
		opManager: newOpManager(),
	}
	go s.processApplied()
	return s
}

func (s *LockServerKV) startOp(opType int, lockName string, clientID ClientID, opNum int64) bool {
	op := LockOp{OpType: opType, LockName: lockName, ClientID: clientID, OpNum: opNum}
	result := s.opManager.addOp(opNum)
	s.proposeC <- op.marshal()
	return <-result
}

func (s *LockServerKV) Acquire(lockName string, clientID ClientID, opNum int64) {
	s.startOp(AcquireOp, lockName, clientID, opNum)
}

func (s *LockServerKV) Release(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(ReleaseOp, lockName, clientID, opNum)
}

func (s *LockServerKV) IsLocked(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(IsLockedOp, lockName, clientID, opNum)
}

func (s *LockServerKV) processApplied() {
	for appliedOp := range s.appliedC {
		s.opManager.reportOpFinished(appliedOp.op.OpNum, appliedOp.result)
	}
}

func (s *LockServerKV) apply(
	data []byte,
	get func(string) bool,
	put func(string, bool),
	wait func(string),
	signal func(string),
) AppliedLockOp {
	op := lockOpFromBytes(data)
	var returnVal bool
	switch op.OpType {
	case AcquireOp:
		// may be able to remove loop depending on semantics of scheduler
		for get(op.LockName) {
			wait(op.LockName) // keep waiting while lock is held
		}
		put(op.LockName, true)
		returnVal = true
	case ReleaseOp:
		// release lock if held
		returnVal = get(op.LockName)
		if get(op.LockName) {
			put(op.LockName, false)
			signal(op.LockName)
		}
	case IsLockedOp:
		returnVal = get(op.LockName)
	}

	return AppliedLockOp{op, returnVal}
}
