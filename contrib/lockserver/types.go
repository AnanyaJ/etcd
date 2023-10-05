package main

type Key interface{}

type Continue struct{}

type Status interface {
	msgType() int
}

type Wait struct {
	key Key
}

type Signal struct {
	key Key
}

type Done struct {
}

func (wait Wait) msgType() int {
	return WaitMsg
}

func (signal Signal) msgType() int {
	return SignalMsg
}

func (done Done) msgType() int {
	return DoneMsg
}
