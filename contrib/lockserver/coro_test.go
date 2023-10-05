package main

import (
	"testing"
	"time"
)

type lockpair struct {
	first  *int
	second *int
}

func TestCoro(t *testing.T) {
	var first, second int
	locks := lockpair{first: &first, second: &second}

	coro1 := CreateCoro(f, locks)
	coro2 := CreateCoro(f, locks)

	status, err := coro1()
	if status.msgType() != SignalMsg || err != nil {
		t.Fatalf("First coro should be at first signal")
	}

	status, err = coro2()
	if status.msgType() != WaitMsg || err != nil {
		t.Fatalf("Second coro should have to wait for second lock")
	}

	status, err = coro2()
	if status.msgType() != WaitMsg || err != nil {
		t.Fatalf("Second coro should still be waiting for second lock")
	}

	status, err = coro1()
	if status.msgType() != SignalMsg || err != nil {
		t.Fatalf("First coro should be at second signal")
	}

	status, err = coro1()
	if status.msgType() != DoneMsg || err != nil {
		t.Fatalf("First coro should have completed")
	}

	status, err = coro2()
	if status.msgType() != SignalMsg || err != nil {
		t.Fatalf("Second coro should be at first signal")
	}

	status, err = coro2()
	if status.msgType() != SignalMsg || err != nil {
		t.Fatalf("Second coro should be at second signal")
	}

	status, err = coro2()
	if status.msgType() != DoneMsg || err != nil {
		t.Fatalf("Second coro should have completed")
	}
}

func f(locks lockpair, wait func(Key), signal func(Key)) error {
	// acquire first lock
	for *locks.first == 1 {
		wait(1)
	}
	time.Sleep(100 * time.Millisecond)
	// other coro should not be able to make progress while we sleep
	// so assume we can still acquire lock
	*locks.first = 1

	// acquire second lock
	for *locks.second == 1 {
		wait(2)
	}
	time.Sleep(100 * time.Millisecond)
	*locks.second = 1

	// release first lock
	*locks.first = 0
	signal(1)

	// release second lock
	*locks.second = 0
	signal(2)

	return nil
}
