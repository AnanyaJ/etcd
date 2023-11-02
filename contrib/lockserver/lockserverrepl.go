package main

type LockServerRepl struct {
	proposeC  chan []byte
	opManager *OpManager
	appliedC  <-chan AppliedOp
}

func newReplLockServer(proposeC chan []byte, appliedC <-chan AppliedOp) *LockServerRepl {
	s := &LockServerRepl{
		proposeC:  proposeC,
		opManager: newOpManager(),
		appliedC:  appliedC,
	}
	go s.processApplied()
	return s
}

// Propose op that some RPC handler wants to replicate
func (s *LockServerRepl) startOp(opType int, lockName string, opNum int64) bool {
	op, result := s.opManager.addOp(opType, lockName, opNum)
	s.proposeC <- op.marshal()
	return <-result
}

func (s *LockServerRepl) Acquire(lockName string, opNum int64) {
	s.startOp(AcquireOp, lockName, opNum)
}

func (s *LockServerRepl) Release(lockName string, opNum int64) bool {
	return s.startOp(ReleaseOp, lockName, opNum)
}

func (s *LockServerRepl) IsLocked(lockName string, opNum int64) bool {
	return s.startOp(IsLockedOp, lockName, opNum)
}

func (s *LockServerRepl) processApplied() {
	// ops that been executed to completion
	for appliedOp := range s.appliedC {
		op := lockOpFromBytes(appliedOp.op)
		result := boolFromBytes(appliedOp.result)
		s.opManager.reportOpFinished(op.OpNum, result)
	}
}

func (s *LockServerRepl) apply(
	data []byte,
	access func(KVOp) bool,
	wait func(string),
	signal func(string),
) []byte {
	op := lockOpFromBytes(data)

	// access functions to read and write lock state
	// note: uses provided access callback instead of directly modifying lock state
	// to allow for correct replay after snapshotting
	isLocked := func() bool { return access(KVOp{OpType: GetOp, Key: op.LockName}) }
	setLocked := func(val bool) { access(KVOp{OpType: PutOp, Key: op.LockName, Val: val}) }

	var returnVal bool
	switch op.OpType {
	case AcquireOp:
		for isLocked() {
			wait(op.LockName) // keep waiting while lock is held
		}
		setLocked(true)
		returnVal = true
	case ReleaseOp:
		returnVal = isLocked()
		if isLocked() {
			setLocked(false)
			signal(op.LockName)
		}
	case IsLockedOp:
		returnVal = isLocked()
	}

	return marshal(returnVal)
}
