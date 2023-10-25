package main

type Queue[T any] []T

type LockQueue[T any] struct {
	IsLocked bool
	Queue    Queue[T]
}

type Coro struct {
	OpData []byte
	Resume func() (Status, []byte)
}

type AppliedOp struct {
	op     []byte
	result []byte
}
