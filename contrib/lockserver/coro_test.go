package main

import (
	"testing"
	"time"
)

func TestCoro(t *testing.T) {
	var first, second int

	coro1 := CreateCoro(f, &first, &second)
	coro2 := CreateCoro(f, &first, &second)

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

func f(wait func(int), signal func(int), args ...interface{}) error {
	first := args[0].(*int)
	second := args[1].(*int)

	// acquire first lock
	for *first == 1 {
		wait(1)
	}
	time.Sleep(100 * time.Millisecond)
	// other coro should not be able to make progress while we sleep
	// so assume we can still acquire lock
	*first = 1

	// acquire second lock
	for *second == 1 {
		wait(2)
	}
	time.Sleep(100 * time.Millisecond)
	*second = 1

	// release first lock
	*first = 0
	signal(1)

	// release second lock
	*second = 0
	signal(2)

	return nil
}
