package main

import "golang.org/x/exp/constraints"

type Queue[T any] []T

type LockQueue[T any] struct {
	IsLocked bool
	Queue    Queue[T]
}

type AppliedOp struct {
	op     []byte
	result []byte
}

type BlockingApp[Key constraints.Ordered] interface {
	apply(data []byte, access func(func() []any) []any, wait func(Key), signal func(Key)) []byte
	getSnapshot() ([]byte, error)
	loadSnapshot(snapshot []byte) error
}
