package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.etcd.io/raft/v3/raftpb"
)

func lockserver_test_setup() (srv *httptest.Server, cli *http.Client, proposeC chan []byte, confChangeC chan raftpb.ConfChange) {
	clusters := []string{"http://127.0.0.1:9021"}
	proposeC = make(chan []byte)
	confChangeC = make(chan raftpb.ConfChange)

	var ls LockServerSnapshot
	getSnapshot := func() ([]byte, error) { return ls.getSnapshot() }
	commitC, errorC, snapshotterReady := newRaftNode(1, clusters, false, getSnapshot, proposeC, confChangeC, true)

	ls = newQueueLockServer(<-snapshotterReady, proposeC, commitC, errorC)

	srv = httptest.NewServer(&httpLSAPI{
		server: ls,
	})

	// wait for server to start
	<-time.After(time.Second)

	cli = srv.Client()

	return srv, cli, proposeC, confChangeC
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func makeRequest(t *testing.T, serverURL string, reqType string, lockName string) *http.Request {
	url := fmt.Sprintf("%s/%s", serverURL, reqType)
	body, err := json.Marshal(&Request{Lock: lockName, OpNum: nrand()})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
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
	srv, cli, proposeC, confChangeC := lockserver_test_setup()
	defer srv.Close()
	defer close(proposeC)
	defer close(confChangeC)

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
	srv, cli, proposeC, confChangeC := lockserver_test_setup()
	defer srv.Close()
	defer close(proposeC)
	defer close(confChangeC)

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
