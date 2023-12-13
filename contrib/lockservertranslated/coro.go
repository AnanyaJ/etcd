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

func CreateCoro[Key constraints.Ordered, Out any](
	f func(wait func(Key), signal func(Key), args ...interface{}) Out,
	args ...interface{},
) (resume func() (Status, Out)) {
	cin := make(chan Continue)
	cstatus := make(chan Status)
	var out Out
	isDone := false
	resume = func() (Status, Out) {
		if isDone {
			// already done executing so don't want to wait
			// for another status
			return Done{}, out
		}
		// unblock function
		cin <- Continue{}
		// pause when it hands back control
		return <-cstatus, out
	}
	wait := func(key Key) {
		cstatus <- Wait[Key]{key: key}
		<-cin
	}
	signal := func(key Key) {
		cstatus <- Signal[Key]{key: key}
		<-cin
	}
	go func() {
		<-cin
		out = f(wait, signal, args...)
		isDone = true
		// ensure that resume returns when function completes
		cstatus <- Done{}
	}()
	return resume
}
