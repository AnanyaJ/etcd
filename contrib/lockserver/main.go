// Copyright 2015 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"log"
	"os"
	"runtime/pprof"
	"strings"
	"time"

	"go.etcd.io/raft/v3/raftpb"
)

func main() {
	cluster := flag.String("cluster", "http://127.0.0.1:12379", "comma separated cluster peers")
	id := flag.Int("id", 1, "node ID")
	lsport := flag.Int("port", 12380, "lock server port")
	join := flag.Bool("join", false, "join an existing cluster")
	impl := flag.String("impl", "blockingraft", "lock server implementation to use (condvar, unreplsimple, unreplcoros, unreplqueues, simple, queues, coros, kvlocks, blockingraft, kv)")
	clearLog := flag.Bool("clearlog", false, "whether to use a fresh log, removing all previous persistent state")
	timeout := flag.Int("timeout", 100, "number of milliseconds simple lock server waits between acquire attempts")
	profile := flag.String("profile", "", "write cpu profile to file")
	flag.Parse()

	if *profile != "" {
		f, err := os.Create(*profile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		go func() {
			// profile for 10 seconds
			<-time.After(10 * time.Second)
			pprof.StopCPUProfile()
		}()
	}

	proposeC := make(chan []byte)
	defer close(proposeC)
	confChangeC := make(chan raftpb.ConfChange)
	defer close(confChangeC)

	getSnapshot := func() ([]byte, error) { return nil, nil }

	switch *impl {
	case "condvar":
		lockServer := newCondVarLockServer()
		serveHTTPLSAPI(lockServer, *lsport, confChangeC, make(<-chan error))
	case "unreplsimple":
		lockServer := newUnreplSimpleLockServer(*timeout)
		serveHTTPLSAPI(lockServer, *lsport, confChangeC, make(<-chan error))
	case "unreplqueues":
		lockServer := newUnreplQueueLockServer()
		serveHTTPLSAPI(lockServer, *lsport, confChangeC, make(<-chan error))
	case "unreplcoros":
		lockServer := newUnreplCoroLockServer()
		serveHTTPLSAPI(lockServer, *lsport, confChangeC, make(<-chan error))
	case "simple":
		commitC, errorC, snapshotterReady := newRaftNode(*id, strings.Split(*cluster, ","), *join, getSnapshot, proposeC, confChangeC, *clearLog)
		snapshotter := <-snapshotterReady
		lockServer := newSimpleLockServer(snapshotter, proposeC, commitC, errorC, *timeout)
		serveHTTPLSAPI(lockServer, *lsport, confChangeC, errorC)
	case "queues":
		commitC, errorC, snapshotterReady := newRaftNode(*id, strings.Split(*cluster, ","), *join, getSnapshot, proposeC, confChangeC, *clearLog)
		snapshotter := <-snapshotterReady
		lockServer := newQueueLockServer(snapshotter, proposeC, commitC, errorC)
		serveHTTPLSAPI(lockServer, *lsport, confChangeC, errorC)
	case "coros":
		commitC, errorC, snapshotterReady := newRaftNode(*id, strings.Split(*cluster, ","), *join, getSnapshot, proposeC, confChangeC, *clearLog)
		snapshotter := <-snapshotterReady
		lockServer := newCoroLockServer(snapshotter, proposeC, commitC, errorC)
		serveHTTPLSAPI(lockServer, *lsport, confChangeC, errorC)
	case "kvlocks":
		commitC, errorC, _ := newRaftNode(*id, strings.Split(*cluster, ","), *join, getSnapshot, proposeC, confChangeC, *clearLog)
		var kvLockServer *LockServerKV
		appliedC := newBlockingKVStore[string, bool](commitC, errorC, kvLockServer.apply)
		kvLockServer = newKVLockServer(proposeC, appliedC)
		serveHTTPLSAPI(kvLockServer, *lsport, confChangeC, errorC)
	case "blockingraft":
		blockingRaftNode, errorC, appliedC := newBlockingRaftNode[string, OngoingOp](*id, strings.Split(*cluster, ","), *join, proposeC, confChangeC, *clearLog)
		lockServerRepl := newReplLockServer(proposeC, appliedC)
		blockingRaftNode.start(lockServerRepl)
		// the http handler will propose updates to raft
		serveHTTPLSAPI(lockServerRepl, *lsport, confChangeC, errorC)
	case "kv":
		blockingRaftNode, errorC, appliedC := newBlockingRaftNode[string, struct{}](*id, strings.Split(*cluster, ","), *join, proposeC, confChangeC, *clearLog)
		kvServer := newKVServer(proposeC, appliedC)
		blockingRaftNode.start(kvServer)
		serveHTTPKVAPI(kvServer, *lsport, confChangeC, errorC)
		return
	}

}
