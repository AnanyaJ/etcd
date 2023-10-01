package main

import (
	"sync"
	"time"
)

// Simple single-server lock service.
// For Acquires, blocks until the lock is available. This blocking is done naively with
// polling, by repeatedly sleeping for one second until the lock is free.
type SimpleLockServer struct {
	mu    *sync.Mutex
	locks map[string]bool
}

func newSimpleLockServer() *SimpleLockServer {
	var mu sync.Mutex
	return &SimpleLockServer{mu: &mu, locks: make(map[string]bool)}
}

func (s *SimpleLockServer) Acquire(lockName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for s.locks[lockName] {
		s.mu.Unlock()
		time.Sleep(time.Second)
		s.mu.Lock()
	}

	s.locks[lockName] = true
}

func (s *SimpleLockServer) Release(lockName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.locks[lockName] {
		return false
	}

	s.locks[lockName] = false
	return true
}

func (s *SimpleLockServer) IsLocked(lockName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.locks[lockName]
}
