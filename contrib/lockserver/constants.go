package main

const (
	WaitMsg int = iota
	SignalMsg
	DoneMsg
)

const (
	AcquireOp int = iota
	ReleaseOp
	IsLockedOp
)
