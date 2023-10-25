package main

import "golang.org/x/exp/constraints"

// TODO: propagate panics to caller
func CreateBlockingCoro[FnIn, FnOut any, Key constraints.Ordered, In, Out any](
	f func(in FnIn, access func(In) Out, wait func(Key), signal func(Key)) FnOut,
	in FnIn,
	access func(In) Out,
) (resume func() (Status, FnOut)) {
	cin := make(chan Continue)
	cstatus := make(chan Status)
	var out FnOut
	isDone := false
	resume = func() (Status, FnOut) {
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
		out = f(in, access, wait, signal)
		isDone = true
		// ensure that resume returns when function completes
		cstatus <- Done{}
	}()
	return resume
}
