package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func setup() (srv *httptest.Server, cli *http.Client) {
	var ls LockServer
	ls = newQueueLockServer()

	srv = httptest.NewServer(&httpLSAPI{
		server: ls,
	})

	// wait server started
	<-time.After(time.Second)

	cli = srv.Client()

	return srv, cli
}

func makeRequest(t *testing.T, serverURL string, reqType string, lockName string) *http.Request {
	url := fmt.Sprintf("%s/%s", serverURL, reqType)
	body := bytes.NewBufferString(lockName)
	req, err := http.NewRequest(http.MethodPut, url, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/html; charset=utf-8")
	return req
}

func parseResponse(t *testing.T, resp *http.Response) bool {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	return strings.Contains(string(data), "true")
}

func getResponse(t *testing.T, cli *http.Client, req *http.Request) bool {
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return parseResponse(t, resp)
}

func checkAcquire(t *testing.T, got bool) {
	expected := true
	if got != expected {
		t.Fatalf("Acquire: expected %t, got %t", expected, got)
	}
}

func checkRelease(t *testing.T, got bool, wasLocked bool) {
	expected := wasLocked
	if got != expected {
		t.Fatalf("Release: expected %t, got %t", expected, got)
	}
}

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
	srv, cli := setup()
	defer srv.Close()

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
	srv, cli := setup()
	defer srv.Close()

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
		<-time.After(100 * time.Millisecond)

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
