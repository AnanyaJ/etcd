package main

import "golang.org/x/exp/constraints"

type Queue[T any] []T

type LockQueue[T any] struct {
	IsLocked bool
	Queue    Queue[T]
}

type KeyQueue[T any] struct {
	Value int
	Queue Queue[T]
}

type AppliedOp[OpType, ResultType any] struct {
	op     OpType
	result ResultType
}

type AppliedLockOp AppliedOp[LockOp, bool]

type BlockingApp[CondVar constraints.Ordered, ReturnType any] interface {
	apply(op []byte, access func(func() []any) []any, wait func(CondVar), signal func(CondVar), broadcast func(CondVar)) ReturnType
	getSnapshot() ([]byte, error)
	loadSnapshot(snapshot []byte) error
}
