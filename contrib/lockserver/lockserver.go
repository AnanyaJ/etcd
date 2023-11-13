package main

type LockServer interface {
	// Blocks until given lock is free and then acquires it.
	Acquire(lockName string, clientID ClientID, opNum int64)
	// Returns true if the lock was released and false if it was already free.
	Release(lockName string, clientID ClientID, opNum int64) bool
	// Return true iff the lock is currently held.
	IsLocked(lockName string, clientID ClientID, opNum int64) bool
}
