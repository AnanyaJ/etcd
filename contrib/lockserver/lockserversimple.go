package main

import (
	"sync"
	"time"
)

// Simple single-server lock service. For Acquires, blocking is accomplished
// by repeatedly sleeping for one second until the lock is free. This server
// could be replicated with e.g. Raft using polling and conditional puts.
type SimpleLockServer struct {
	mu    *sync.Mutex
	locks map[string]bool
}

func newSimpleLockServer() *SimpleLockServer {
	var mu sync.Mutex
	return &SimpleLockServer{mu: &mu, locks: make(map[string]bool)}
}

func (s *SimpleLockServer) Acquire(lockName string, opNum int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for s.locks[lockName] {
		s.mu.Unlock()
		// lock is not available so sleep and try again
		time.Sleep(time.Second)
		s.mu.Lock()
	}

	s.locks[lockName] = true
}

func (s *SimpleLockServer) Release(lockName string, opNum int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.locks[lockName] {
		return false
	}

	s.locks[lockName] = false
	return true
}

func (s *SimpleLockServer) IsLocked(lockName string, opNum int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.locks[lockName]
}
