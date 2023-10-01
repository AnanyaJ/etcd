package main

type LockServer interface {
	Acquire(lockName string) bool
	Release(lockName string) bool
	IsLocked(lockName string) bool
}
