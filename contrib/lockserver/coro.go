package main

type Continue struct{}
type Key interface{}

// TODO: propagate panics to caller
func CreateCoro[In any](
	f func(in In, wait func(Key), signal func(Key)),
	in In,
	onWait func(Key),
	onSignal func(Key),
) (resume func() bool) {
	cin := make(chan Continue)
	cout := make(chan Continue)
	isDone := false
	resume = func() (done bool) {
		if isDone {
			return true
		}
		cin <- Continue{}
		<-cout
		return isDone
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
		f(in, wait, signal)
		isDone = true
		cout <- Continue{}
	}()
	return resume
}
