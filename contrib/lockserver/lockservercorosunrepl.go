package main

type LockCoroUnrepl struct {
	OpNum  int64
	Resume func() (Status, bool)
}

type UnreplCoroLockServer struct {
	proposeC  chan LockOp
	opManager *OpManager
	locks     map[string]*LockQueue[LockCoro]
}

// Replicated lock service built using coroutines for each lock. This lock server
// performs similarly to the one that uses condition variables, except that it can
// safely be replicated since it uses coroutines to deterministically schedule
// the Acquire and Release operations. Currently uses a WAL that is persisted
// on disk, but does not support snapshotting yet.
func newUnreplCoroLockServer() *UnreplCoroLockServer {
	proposeC := make(chan LockOp)
	s := &UnreplCoroLockServer{
		proposeC:  proposeC,
		opManager: newOpManager(),
		locks:     make(map[string]*LockQueue[LockCoro]),
	}
	go s.applyProposals()
	return s
}

func (s *UnreplCoroLockServer) startOp(opType int, lockName string, clientID ClientID, opNum int64) bool {
	op := LockOp{OpType: opType, LockName: lockName, ClientID: clientID, OpNum: opNum}
	result := s.opManager.addOp(opNum)
	s.proposeC <- op
	return <-result
}

func (s *UnreplCoroLockServer) Acquire(lockName string, clientID ClientID, opNum int64) {
	s.startOp(AcquireOp, lockName, clientID, opNum)
}

func (s *UnreplCoroLockServer) Release(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(ReleaseOp, lockName, clientID, opNum)
}

func (s *UnreplCoroLockServer) IsLocked(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(IsLockedOp, lockName, clientID, opNum)
}

func (s *UnreplCoroLockServer) applyProposals() {
	for op := range s.proposeC {
		var apply func(lockName string, wait func(string), signal func(string)) bool
		switch op.OpType {
		case AcquireOp:
			apply = s.acquire
		case ReleaseOp:
			apply = s.release
		case IsLockedOp:
			apply = s.isLocked
		}
		applyFunc := func(wait func(string), signal func(string), args ...interface{}) bool {
			lockName := args[0].(string)
			return apply(lockName, wait, signal)
		}

		// new coro is initially the only runnable one
		coro := CreateCoro[string, bool](applyFunc, op.LockName)
		runnable := []LockCoro{LockCoro{OpNum: op.OpNum, Resume: coro}}
		// resume coros until no coro can make more progress -- ensures
		// deterministic behavior since timing of ops arriving on
		// started channel cannot be controlled
		for len(runnable) > 0 {
			// resume coros in stack order
			next := runnable[len(runnable)-1]
			runnable = runnable[:len(runnable)-1]

			status, result := next.Resume()
			switch status.msgType() {
			case WaitMsg:
				key := status.(Wait[string]).key
				lock := s.locks[key]
				lock.Queue = append(lock.Queue, next)
			case SignalMsg:
				// this coro is still not blocked so add back to runnable stack
				runnable = append(runnable, next)
				key := status.(Signal[string]).key
				lock := s.locks[key]
				queue := lock.Queue
				if len(queue) > 0 {
					// unblock exactly one coro waiting on lock
					unblocked := queue[0]
					lock.Queue = queue[1:]
					runnable = append(runnable, unblocked)
				}
			case DoneMsg:
				s.opManager.reportOpFinished(next.OpNum, result)
			}
		}
	}
}

func (s *UnreplCoroLockServer) acquire(lockName string, wait func(string), signal func(string)) bool {
	// locks are unnecessary here because code between wait/signal calls is
	// executed atomically and there is only one thread applying ops
	s.addLock(lockName)

	lock := s.locks[lockName]
	for lock.IsLocked {
		wait(lockName)
	}

	lock.IsLocked = true
	return true
}

func (s *UnreplCoroLockServer) release(lockName string, wait func(string), signal func(string)) bool {
	s.addLock(lockName)
	lock := s.locks[lockName]

	if !lock.IsLocked {
		return false // lock already free
	}

	lock.IsLocked = false
	signal(lockName)
	return true
}

func (s *UnreplCoroLockServer) isLocked(lockName string, wait func(string), signal func(string)) bool {
	s.addLock(lockName)
	return s.locks[lockName].IsLocked
}

func (s *UnreplCoroLockServer) addLock(lockName string) {
	if s.locks[lockName] != nil {
		return // already exists
	}
	s.locks[lockName] = &LockQueue[LockCoro]{Queue: []LockCoro{}}
}
