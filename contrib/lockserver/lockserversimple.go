package main

import (
	"sync"
)

type server struct {
	mu    *sync.Mutex
	locks map[string]bool
}

func newLockServer() *server {
	var mu sync.Mutex
	return &server{mu: &mu, locks: make(map[string]bool)}
}

func (s *server) Acquire(lockName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.locks[lockName] {
		return false
	}

	s.locks[lockName] = true
	return true
}

func (s *server) Release(lockName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.locks[lockName] {
		return false
	}

	s.locks[lockName] = false
	return true
}

func (s *server) IsLocked(lockName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.locks[lockName]
}
