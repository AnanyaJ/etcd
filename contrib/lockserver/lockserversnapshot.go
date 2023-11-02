package main

type LockServerSnapshot interface {
	// Blocks until given lock is free and then acquires it.
	Acquire(lockName string, opNum int)
	// Returns true if the lock was released and false if it was already free.
	Release(lockName string, opNum int) bool
	// Return true iff the lock is currently held.
	IsLocked(lockName string, opNum int) bool

	getSnapshot() ([]byte, error)
}
