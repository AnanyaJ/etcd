package main

type Coro struct {
	coro   func() (Status, bool)
	result chan bool
}

type LockQueue struct {
	isLocked bool
	queue    []Coro
}

type QueueLockServer struct {
	locks   map[string]*LockQueue
	started chan Coro
}

// Single-server lock service built using a queue for each lock. This lock server
// performs similarly to the one that uses condition variables, except that it can
// safely be replicated since it uses coroutines to deterministically schedule
// the Acquire and Release operations.
func newQueueLockServer() *QueueLockServer {
	s := QueueLockServer{locks: make(map[string]*LockQueue), started: make(chan Coro)}
	go s.schedule()
	return &s
}

func (s *QueueLockServer) startOp(apply func(string, func(Key), func(Key)) bool, lockName string) bool {
	coro := CreateCoro[string, bool](apply, lockName)
	result := make(chan bool)
	s.started <- Coro{coro: coro, result: result}
	return <-result
}

func (s *QueueLockServer) Acquire(lockName string) {
	acquire := func(lockName string, wait func(Key), signal func(Key)) bool {
		// locks are unnecessary here because code between wait/signal calls is
		// executed atomically and there is only one thread applying ops
		s.addLock(lockName)

		lock := s.locks[lockName]
		for lock.isLocked {
			wait(lockName)
		}

		lock.isLocked = true
		return true
	}
	s.startOp(acquire, lockName)
}

func (s *QueueLockServer) Release(lockName string) bool {
	release := func(lockName string, wait func(Key), signal func(Key)) bool {
		s.addLock(lockName)
		lock := s.locks[lockName]

		if !lock.isLocked {
			return false // lock already free
		}

		lock.isLocked = false
		signal(lockName)
		return true
	}
	return s.startOp(release, lockName)
}

func (s *QueueLockServer) IsLocked(lockName string) bool {
	isLocked := func(lockName string, wait func(Key), signal func(Key)) bool {
		s.addLock(lockName)
		return s.locks[lockName].isLocked
	}
	return s.startOp(isLocked, lockName)
}

func (s *QueueLockServer) schedule() {
	for {
		coro := <-s.started
		// new coro is initially the only runnable one
		runnable := []Coro{coro}
		// resume coros until no coro can make more progress -- ensures
		// deterministic behavior since timing of ops arriving on
		// started channel cannot be controlled
		for len(runnable) > 0 {
			// resume coros in stack order
			next := runnable[len(runnable)-1]
			runnable = runnable[:len(runnable)-1]

			status, output := next.coro()
			switch status.msgType() {
			case WaitMsg:
				key := status.(Wait).key.(string)
				s.locks[key].queue = append(s.locks[key].queue, next)
			case SignalMsg:
				// this coro is still not blocked so add back to runnable stack
				runnable = append(runnable, next)
				key := status.(Signal).key.(string)
				queue := s.locks[key].queue
				if len(queue) > 0 {
					// unblock exactly one coro waiting on lock
					unblocked := queue[0]
					s.locks[key].queue = queue[1:]
					runnable = append(runnable, unblocked)
				}
			case DoneMsg:
				// inform RPC handler of op completion and result
				next.result <- output
			}
		}
	}
}

func (s *QueueLockServer) addLock(lockName string) {
	if s.locks[lockName] != nil {
		return // already exists
	}
	s.locks[lockName] = &LockQueue{queue: []Coro{}}
}
