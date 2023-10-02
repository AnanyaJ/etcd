package main

type Continue struct{}
type Key interface{}

// TODO: propagate panics to caller
func CreateCoro[In, Out any](
	f func(in In, wait func(Key), signal func(Key)) Out,
	in In,
	onWait func(Key),
	onSignal func(Key),
) (resume func() (Out, bool)) {
	cin := make(chan Continue)
	cout := make(chan Continue)
	var out Out
	isDone := false
	resume = func() (output Out, done bool) {
		if isDone {
			return out, true
		}
		cin <- Continue{}
		<-cout
		return out, isDone
	}
	wait := func(key Key) {
		onWait(key)
		cout <- Continue{}
		<-cin
	}
	signal := func(key Key) {
		onSignal(key)
		cout <- Continue{}
		<-cin
	}
	go func() {
		<-cin
		out = f(in, wait, signal)
		isDone = true
		cout <- Continue{}
	}()
	return resume
}
