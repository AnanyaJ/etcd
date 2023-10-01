package main

import (
	"sync"
)

// Single-server lock service built with condition variables.
// For Acquires, blocks until the lock is available. This blocking is done using Go
// condition variables, which only works because the server is not replicated.
type CondVarLockServer struct {
	mu    *sync.Mutex
	locks map[string]*lock
}

type lock struct {
	mu       *sync.Mutex
	cond     *sync.Cond
	isLocked bool
}

func newCondVarLockServer() *CondVarLockServer {
	var mu sync.Mutex
	return &CondVarLockServer{mu: &mu, locks: make(map[string]*lock)}
}

func (s *CondVarLockServer) Acquire(lockName string) {
	s.addLock(lockName)

	lock := s.locks[lockName]
	lock.mu.Lock()
	defer lock.mu.Unlock()

	// need to loop in case another goroutine grabs the lock without waiting
	// before this one is resumed after a signal
	for lock.isLocked {
		lock.cond.Wait()
	}

	lock.isLocked = true
}

func (s *CondVarLockServer) Release(lockName string) bool {
	s.addLock(lockName)

	lock := s.locks[lockName]
	lock.mu.Lock()
	defer lock.mu.Unlock()

	if !lock.isLocked {
		return false
	}

	lock.isLocked = false
	lock.cond.Signal()
	return true
}

func (s *CondVarLockServer) IsLocked(lockName string) bool {
	s.addLock(lockName)

	lock := s.locks[lockName]
	lock.mu.Lock()
	defer lock.mu.Unlock()

	return lock.isLocked
}

func (s *CondVarLockServer) addLock(lockName string) {
	if s.locks[lockName] != nil {
		return // already exists
	}

	// use global lock to add a new lock
	s.mu.Lock()
	defer s.mu.Unlock()

	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	s.locks[lockName] = &lock{mu: &mu, cond: cond}
}
