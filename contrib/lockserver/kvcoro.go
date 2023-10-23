package main

import "golang.org/x/exp/constraints"

// TODO: propagate panics to caller
func CreateKVCoro[In any, Out any, Key constraints.Ordered, Value any](
	f func(in In, get func(Key) Value, put func(Key, Value), wait func(Key), signal func(Key)) Out,
	in In,
	get func(Key) Value,
	put func(Key, Value),
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
		out = f(in, get, put, wait, signal)
		isDone = true
		// ensure that resume returns when function completes
		cstatus <- Done{}
	}()
	return resume
}
