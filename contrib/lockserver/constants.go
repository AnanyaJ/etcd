package main

const (
	WaitMsg int = iota
	SignalMsg
	BroadcastMsg
	DoneMsg
)

const (
	AcquireOp int = iota
	ReleaseOp
	IsLockedOp
)
