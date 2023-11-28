package main

import "golang.org/x/exp/constraints"

type Queue[T any] []T

type LockQueue[T any] struct {
	IsLocked bool
	Queue    Queue[T]
}

type AppliedOp[OpType, ResultType any] struct {
	op     OpType
	result ResultType
}

type AppliedLockOp AppliedOp[LockOp, bool]

type BlockingApp[Key constraints.Ordered, ReturnType any] interface {
	apply(data []byte, access func(func() []any) []any, wait func(Key), signal func(Key), broadcast func(Key)) ReturnType
	getSnapshot() ([]byte, error)
	loadSnapshot(snapshot []byte) error
}
