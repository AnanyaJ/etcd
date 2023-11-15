package main

import "golang.org/x/exp/constraints"

type Queue[T any] []T

type LockQueue[T any] struct {
	IsLocked bool
	Queue    Queue[T]
}

type AppliedOp[ReturnType any] struct {
	op     []byte
	result ReturnType
}

type BlockingApp[Key constraints.Ordered, ReturnType any] interface {
	apply(data []byte, access func(func() []any) []any, wait func(Key), signal func(Key)) ReturnType
	getSnapshot() ([]byte, error)
	loadSnapshot(snapshot []byte) error
}
