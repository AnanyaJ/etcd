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

func queue_lockserver_setup() (srv *httptest.Server, cli *http.Client, proposeC chan []byte, confChangeC chan raftpb.ConfChange) {
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
	<-time.After(time.Second * 3)

	cli = srv.Client()

	return srv, cli, proposeC, confChangeC
}

func translated_lockserver_setup() (srv *httptest.Server, cli *http.Client, proposeC chan []byte, confChangeC chan raftpb.ConfChange) {
	clusters := []string{"http://127.0.0.1:9021"}
	proposeC = make(chan []byte)
	confChangeC = make(chan raftpb.ConfChange)

	var ls *LockServerRepl
	raftNode, _, appliedC := newBlockingRaftNode[string](1, clusters, false, proposeC, confChangeC, true, ls)
	ls = newReplLockServer(proposeC, appliedC)
	raftNode.start()

	srv = httptest.NewServer(&httpLSAPI{
		server: ls,
	})

	// wait for server to start
	<-time.After(time.Second * 3)

	cli = srv.Client()

	return srv, cli, proposeC, confChangeC
}

func stop_server(srv *httptest.Server, proposeC chan []byte, confChangeC chan raftpb.ConfChange) {
	srv.Close()
	close(proposeC)
	close(confChangeC)
	// wait for server to stop
	<-time.After(time.Second)
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func makeRequest(t testing.TB, serverURL string, reqType string, lockName string) *http.Request {
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

func parseResponse(t testing.TB, resp *http.Response) bool {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	return strings.Contains(string(data), "true")
}

func getResponse(t testing.TB, cli *http.Client, req *http.Request) bool {
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return parseResponse(t, resp)
}

func checkAcquire(t testing.TB, got bool) {
	expected := true
	if got != expected {
		t.Fatalf("Acquire: expected %t, got %t", expected, got)
	}
}

func checkRelease(t testing.TB, got bool, wasLocked bool) {
	expected := wasLocked
	if got != expected {
		t.Fatalf("Release: expected %t, got %t", expected, got)
	}
}
