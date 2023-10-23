package main

type LockServerSnapshot interface {
	// Blocks until given lock is free and then acquires it.
	Acquire(lockName string)
	// Returns true if the lock was released and false if it was already free.
	Release(lockName string) bool
	// Return true iff the lock is currently held.
	IsLocked(lockName string) bool

	getSnapshot() ([]byte, error)
}
