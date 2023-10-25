package main

import "golang.org/x/exp/constraints"

type Continue struct{}

type Status interface {
	msgType() int
}

type Wait[Key constraints.Ordered] struct {
	key Key
}

type Signal[Key constraints.Ordered] struct {
	key Key
}

type Done struct {
}

func (wait Wait[Key]) msgType() int {
	return WaitMsg
}

func (signal Signal[Key]) msgType() int {
	return SignalMsg
}

func (done Done) msgType() int {
	return DoneMsg
}

type Coro[Result any] struct {
	OpNum  int
	Resume func() (Status, Result)
}

type LockOp struct {
	OpNum    int
	OpType   int
	LockName string
}

type LockQueue[T any] struct {
	IsLocked bool
	Queue    []T
}

type BlockingCoro struct {
	OpData []byte
	Resume func() (Status, []byte)
}

type Queue[Key constraints.Ordered] map[Key][]BlockingCoro

type AppliedOp struct {
	op     []byte
	result []byte
}
