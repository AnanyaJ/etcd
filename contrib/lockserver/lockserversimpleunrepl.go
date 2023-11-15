package main

import (
	"log"
	"sync"
	"time"
)

// Simple single-server lock service. For Acquires, blocking is accomplished
// by repeatedly sleeping for one second until the lock is free. This server
// could be replicated with e.g. Raft using polling and conditional puts.
type UnreplSimpleLockServer struct {
	mu      *sync.Mutex
	locks   map[string]bool
	timeout int
}

func newUnreplSimpleLockServer(timeout int) *UnreplSimpleLockServer {
	log.Printf("Using simple lock server with timeout %d \n", timeout)
	var mu sync.Mutex
	return &UnreplSimpleLockServer{mu: &mu, locks: make(map[string]bool), timeout: timeout}
}

func (s *UnreplSimpleLockServer) Acquire(lockName string, clientID ClientID, opNum int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for s.locks[lockName] {
		s.mu.Unlock()
		// lock is not available so sleep and try again
		time.Sleep(time.Duration(s.timeout) * time.Millisecond)
		s.mu.Lock()
	}

	s.locks[lockName] = true
}

func (s *UnreplSimpleLockServer) Release(lockName string, clientID ClientID, opNum int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.locks[lockName] {
		return false
	}

	s.locks[lockName] = false
	return true
}

func (s *UnreplSimpleLockServer) IsLocked(lockName string, clientID ClientID, opNum int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.locks[lockName]
}
