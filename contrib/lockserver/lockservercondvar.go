package main

import (
	"sync"
)

// Single-server lock service built using Go condition variables. This lock server
// is more performant than one that uses polling (and does not require picking a
// sleep duration), but currently cannot be replicated using e.g. Raft due to the use of
// condition variables.
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

func (s *CondVarLockServer) Acquire(lockName string, opNum int) {
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

func (s *CondVarLockServer) Release(lockName string, opNum int) bool {
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

func (s *CondVarLockServer) IsLocked(lockName string, opNum int) bool {
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
