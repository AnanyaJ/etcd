package main

type LockServer interface {
	Acquire(lockName string)
	Release(lockName string) bool
	IsLocked(lockName string) bool
}
