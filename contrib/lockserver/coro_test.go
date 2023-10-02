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

	numSignals := 0
	coro1 := CreateCoro(
		f,
		locks,
		func(key Key) { t.Fatalf("First coro should not have to wait for any locks") },
		func(key Key) { numSignals++ },
	)

	coro2WaitedForSecondLock := false
	coro2 := CreateCoro(
		f,
		locks,
		func(key Key) {
			if key != 2 {
				t.Fatalf("Second coro should only wait for second lock")
			}
			coro2WaitedForSecondLock = true
		},
		func(key Key) { numSignals++ },
	)

	done := coro1()
	if numSignals != 1 || done {
		t.Fatalf("First coro should be at first signal")
	}

	done = coro2()
	if !coro2WaitedForSecondLock || done {
		t.Fatalf("Second coro should have to wait for second lock")
	}

	done = coro2()
	if done {
		t.Fatalf("Second coro should still be waiting for second lock")
	}

	done = coro1()
	if numSignals != 2 || done {
		t.Fatalf("First coro should be at second signal")
	}

	done = coro1()
	if !done {
		t.Fatalf("First coro should have completed")
	}

	done = coro2()
	if numSignals != 3 || done {
		t.Fatalf("Second coro should be at first signal")
	}

	done = coro2()
	if numSignals != 4 || done {
		t.Fatalf("Second coro should be at second signal")
	}

	done = coro2()
	if !done {
		t.Fatalf("Second coro should have completed")
	}
}

func f(locks lockpair, wait func(Key), signal func(Key)) {
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
}
