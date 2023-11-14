package main

type UnreplQueueLockServer struct {
	proposeC  chan LockOp
	opManager *OpManager
	locks     map[string]*LockQueue[LockOp]
}

// Replicated lock service built using a queue for each lock. This lock server
// performs similarly to the one that uses condition variables, except that it can
// safely be replicated since it uses queues to deterministically schedule
// the Acquire operations. Currently uses a WAL that is persisted on disk, and
// supports snapshotting.
func newUnreplQueueLockServer() *UnreplQueueLockServer {
	s := &UnreplQueueLockServer{
		opManager: newOpManager(),
		locks:     make(map[string]*LockQueue[LockOp]),
	}
	return s
}

func (s *UnreplQueueLockServer) startOp(opType int, lockName string, clientID ClientID, opNum int64) bool {
	op := LockOp{OpType: opType, LockName: lockName, ClientID: clientID, OpNum: opNum}
	result := s.opManager.addOp(opNum)
	go s.applyOp(op)
	return <-result
}

func (s *UnreplQueueLockServer) Acquire(lockName string, clientID ClientID, opNum int64) {
	s.startOp(AcquireOp, lockName, clientID, opNum)
}

func (s *UnreplQueueLockServer) Release(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(ReleaseOp, lockName, clientID, opNum)
}

func (s *UnreplQueueLockServer) IsLocked(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(IsLockedOp, lockName, clientID, opNum)
}

func (s *UnreplQueueLockServer) applyOp(op LockOp) {
	s.addLock(op.LockName)
	lock := s.locks[op.LockName]

	switch op.OpType {
	case AcquireOp:
		if lock.IsLocked {
			lock.Queue = append(lock.Queue, op)
		} else {
			lock.IsLocked = true
			s.opManager.reportOpFinished(op.OpNum, true)
		}
	case ReleaseOp:
		if !lock.IsLocked {
			s.opManager.reportOpFinished(op.OpNum, false) // lock already free
		}

		if len(lock.Queue) > 0 {
			// pass lock to next waiting op
			unblocked := lock.Queue[0]
			lock.Queue = lock.Queue[1:]
			s.opManager.reportOpFinished(unblocked.OpNum, true)
		} else {
			lock.IsLocked = false
		}

		s.opManager.reportOpFinished(op.OpNum, true)
	case IsLockedOp:
		s.opManager.reportOpFinished(op.OpNum, lock.IsLocked)
	}
}

func (s *UnreplQueueLockServer) addLock(lockName string) {
	if s.locks[lockName] != nil {
		return // already exists
	}
	s.locks[lockName] = &LockQueue[LockOp]{Queue: []LockOp{}}
}
