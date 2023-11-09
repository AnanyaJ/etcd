package main

import (
	"testing"
	"time"
)

func checkIsLocked(t *testing.T, got bool, isLocked bool) {
	expected := isLocked
	if got != expected {
		t.Fatalf("IsLocked: expected %t, got %t", expected, got)
	}
}

func checkNumAcquired(t *testing.T, hasAcquired []bool, expected int) {
	num := 0
	for i := 0; i < len(hasAcquired); i++ {
		if hasAcquired[i] {
			num++
		}
	}
	if num != expected {
		t.Fatalf("Expected %d client(s) to have acquired the lock thus far, but instead %d acquired it", expected, num)
	}
}

func TestLockServerNoContention(t *testing.T) {
	srv, cli, proposeC, confChangeC := queue_lockserver_setup()
	defer stop_server(srv, proposeC, confChangeC)

	lock1 := "lock1"
	lock2 := "lock2"

	// acquire new lock
	req := makeRequest(t, srv.URL, "acquire", lock1)
	resp := getResponse(t, cli, req)
	checkAcquire(t, resp)

	// lock should now be marked as locked
	req = makeRequest(t, srv.URL, "islocked", lock1)
	resp = getResponse(t, cli, req)
	checkIsLocked(t, resp, true)

	// lock that server has never heard of should be free
	req = makeRequest(t, srv.URL, "islocked", lock2)
	resp = getResponse(t, cli, req)
	checkIsLocked(t, resp, false)

	// try releasing lock that isn't being held
	req = makeRequest(t, srv.URL, "release", lock2)
	resp = getResponse(t, cli, req)
	checkIsLocked(t, resp, false)

	// release lock
	req = makeRequest(t, srv.URL, "release", lock1)
	resp = getResponse(t, cli, req)
	checkIsLocked(t, resp, true)

	// lock should now be free again
	req = makeRequest(t, srv.URL, "islocked", lock1)
	resp = getResponse(t, cli, req)
	checkIsLocked(t, resp, false)
}

func TestLockServerContention(t *testing.T) {
	srv, cli, proposeC, confChangeC := queue_lockserver_setup()
	defer stop_server(srv, proposeC, confChangeC)

	lock1 := "lock1"
	lock2 := "lock2"
	numContending := 4

	hasAcquiredLock := make([]bool, numContending)
	// make concurrent requests for same lock
	for i := 0; i < numContending-1; i++ {
		go func(i int) {
			req := makeRequest(t, srv.URL, "acquire", lock1)
			resp := getResponse(t, cli, req)
			checkAcquire(t, resp)
			hasAcquiredLock[i] = true
		}(i)
	}

	for i := 1; i < numContending-1; i++ {
		// wait for server to give out the lock
		<-time.After(time.Second)

		checkNumAcquired(t, hasAcquiredLock, i)

		// release lock for another client
		req := makeRequest(t, srv.URL, "release", lock1)
		resp := getResponse(t, cli, req)
		checkRelease(t, resp, true)
	}

	// should be able to acquire different lock while others are blocking
	req := makeRequest(t, srv.URL, "acquire", lock2)
	resp := getResponse(t, cli, req)
	checkAcquire(t, resp)

	// add last client waiting for same lock
	go func() {
		req = makeRequest(t, srv.URL, "acquire", lock1)
		resp = getResponse(t, cli, req)
		checkAcquire(t, resp)
		hasAcquiredLock[numContending-1] = true
	}()

	// make sure server does not give out same lock again
	<-time.After(100 * time.Millisecond)
	checkNumAcquired(t, hasAcquiredLock, numContending-1)

	// release lock for last client
	req = makeRequest(t, srv.URL, "release", lock1)
	resp = getResponse(t, cli, req)
	checkRelease(t, resp, true)

	<-time.After(100 * time.Millisecond)
	checkNumAcquired(t, hasAcquiredLock, numContending)

	// release lock so that it becomes free
	req = makeRequest(t, srv.URL, "release", lock1)
	resp = getResponse(t, cli, req)
	checkRelease(t, resp, true)

	// check that lock is available now
	req = makeRequest(t, srv.URL, "islocked", lock1)
	resp = getResponse(t, cli, req)
	checkIsLocked(t, resp, false)
}
