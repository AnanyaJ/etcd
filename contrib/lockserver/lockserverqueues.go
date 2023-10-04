package main

import "sync"

type QueueLockServer struct {
	mu    *sync.Mutex
	locks map[string]*lock_queue
}

type lock_queue struct {
	mu       *sync.Mutex
	isLocked bool
	queue    []chan struct{}
}

// Single-server lock service built using a queue for each lock. This lock server
// behaves similarly to the one that uses condition variables. It also cannot be
// replicated as-is due to the non-deterministic ordering of the Acquire()
// wakeups relative to the starting of other operations.
func newQueueLockServer() *QueueLockServer {
	var mu sync.Mutex
	return &QueueLockServer{mu: &mu, locks: make(map[string]*lock_queue)}
}

func (s *QueueLockServer) Acquire(lockName string) {
	s.addLock(lockName)

	lock := s.locks[lockName]
	lock.mu.Lock()
	defer lock.mu.Unlock()

	// need to loop since another operation may start and grab the lock
	// after it is released and this one is pulled off the queue
	for lock.isLocked {
		available := make(chan struct{})
		// add to queue
		lock.queue = append(lock.queue, available)
		lock.mu.Unlock()
		// wait until pulled off of queue
		<-available
		lock.mu.Lock()
	}

	lock.isLocked = true
}

func (s *QueueLockServer) Release(lockName string) bool {
	s.addLock(lockName)

	lock := s.locks[lockName]
	lock.mu.Lock()
	defer lock.mu.Unlock()

	if !lock.isLocked {
		return false
	}

	lock.isLocked = false
	if len(lock.queue) > 0 {
		// pull waiting operation off of queue and un-block it
		next := lock.queue[0]
		lock.queue = lock.queue[1:]
		next <- struct{}{}
	}
	return true
}

func (s *QueueLockServer) IsLocked(lockName string) bool {
	s.addLock(lockName)

	lock := s.locks[lockName]
	lock.mu.Lock()
	defer lock.mu.Unlock()

	return lock.isLocked
}

func (s *QueueLockServer) addLock(lockName string) {
	if s.locks[lockName] != nil {
		return // already exists
	}

	// use global lock to add a new lock
	s.mu.Lock()
	defer s.mu.Unlock()

	var mu sync.Mutex
	s.locks[lockName] = &lock_queue{mu: &mu, queue: []chan struct{}{}}
}
