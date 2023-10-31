package main

type Queue[T any] []T

type LockQueue[T any] struct {
	IsLocked bool
	Queue    Queue[T]
}

type AppliedOp struct {
	op     []byte
	result []byte
}
