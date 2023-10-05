package main

import "log"

type LockOp struct {
	opType   int
	lockName string
	result   chan bool
}

type Coro struct {
	op   *LockOp
	coro func() (Status, bool)
}

type LockQueue struct {
	isLocked bool
	queue    []Coro
}

type QueueLockServer struct {
	locks    map[string]*LockQueue
	started  chan LockOp
	runnable []Coro
}

// Single-server lock service built using a queue for each lock. This lock server
// performs similarly to the one that uses condition variables, except that it can
// safely be replicated since it uses coroutines to deterministically schedule
// the Acquire and Release operations.
func newQueueLockServer() *QueueLockServer {
	s := QueueLockServer{locks: make(map[string]*LockQueue), started: make(chan LockOp), runnable: []Coro{}}
	go s.schedule()
	return &s
}

func (s *QueueLockServer) Acquire(lockName string) {
	result := make(chan bool)
	s.started <- LockOp{opType: AcquireOp, lockName: lockName, result: result}
	<-result
}

func (s *QueueLockServer) Release(lockName string) bool {
	result := make(chan bool)
	s.started <- LockOp{opType: ReleaseOp, lockName: lockName, result: result}
	return <-result
}

func (s *QueueLockServer) IsLocked(lockName string) bool {
	result := make(chan bool)
	s.started <- LockOp{opType: IsLockedOp, lockName: lockName, result: result}
	return <-result
}

func (s *QueueLockServer) schedule() {
	for {
		op := <-s.started
		coro := CreateCoro[LockOp, bool](s.apply, op)
		// new coro is initially the only runnable one
		s.runnable = append(s.runnable, Coro{op: &op, coro: coro})
		// resume coros until no coro can make more progress -- ensures
		// deterministic behavior since timing of ops arriving on
		// started channel cannot be controlled
		for len(s.runnable) > 0 {
			// resume coros in stack order
			next := s.runnable[len(s.runnable)-1]
			s.runnable = s.runnable[:len(s.runnable)-1]

			status, output := next.coro()
			switch status.msgType() {
			case WaitMsg:
				s.locks[op.lockName].queue = append(s.locks[op.lockName].queue, next)
			case SignalMsg:
				// this coro is still not blocked so add back to runnable stack
				s.runnable = append(s.runnable, next)
				queue := s.locks[op.lockName].queue
				if len(queue) > 0 {
					// unblock exactly one coro waiting on lock
					unblocked := queue[0]
					s.locks[op.lockName].queue = queue[1:]
					s.runnable = append(s.runnable, unblocked)
				}
			case DoneMsg:
				// inform RPC handler of op completion and result
				next.op.result <- output
			}
		}
	}
}

func (s *QueueLockServer) apply(op LockOp, wait func(Key), signal func(Key)) bool {
	// locks are unnecessary here because code between wait/signal calls is
	// executed atomically and there is only one thread applying ops
	switch op.opType {
	case AcquireOp:
		s.addLock(op.lockName)
		lock := s.locks[op.lockName]

		wait(op.lockName)
		if lock.isLocked {
			log.Fatalf("Lock %s still held even after waiting \n", op.lockName)
		}

		lock.isLocked = true
		return true
	case ReleaseOp:
		s.addLock(op.lockName)
		lock := s.locks[op.lockName]

		if !lock.isLocked {
			return false // lock already free
		}

		lock.isLocked = false
		signal(op.lockName)
		return true
	case IsLockedOp:
		s.addLock(op.lockName)
		return s.locks[op.lockName].isLocked
	}
	return true
}

func (s *QueueLockServer) addLock(lockName string) {
	if s.locks[lockName] != nil {
		return // already exists
	}
	s.locks[lockName] = &LockQueue{queue: []Coro{}}
}
