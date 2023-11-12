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
	"strings"

	"go.etcd.io/raft/v3/raftpb"
)

func main() {
	cluster := flag.String("cluster", "http://127.0.0.1:12379", "comma separated cluster peers")
	id := flag.Int("id", 1, "node ID")
	lsport := flag.Int("port", 12380, "lock server port")
	join := flag.Bool("join", false, "join an existing cluster")
	impl := flag.String("impl", "blockingraft", "lock server implementation to use (simple, condvar, queues, coros, kvlocks, blockingraft, kv)")
	clearLog := flag.Bool("clearlog", false, "whether to use a fresh log, removing all previous persistent state")
	flag.Parse()

	proposeC := make(chan []byte)
	defer close(proposeC)
	confChangeC := make(chan raftpb.ConfChange)
	defer close(confChangeC)

	getSnapshot := func() ([]byte, error) { return nil, nil }
	commitC, errorC, snapshotterReady := newRaftNode(*id, strings.Split(*cluster, ","), *join, getSnapshot, proposeC, confChangeC, *clearLog)
	snapshotter := <-snapshotterReady

	var lockServer LockServer
	switch *impl {
	case "simple":
		lockServer = newSimpleLockServer()
	case "condvar":
		lockServer = newCondVarLockServer()
	case "queues":
		lockServer = newQueueLockServer(snapshotter, proposeC, commitC, errorC)
	case "coros":
		lockServer = newCoroLockServer(snapshotter, proposeC, commitC, errorC)
	case "kvlocks":
		var kvLockServer *LockServerKV
		appliedC := newBlockingKVStore[string, bool](commitC, errorC, kvLockServer.apply)
		kvLockServer = newKVLockServer(proposeC, appliedC)
		lockServer = kvLockServer
	case "blockingraft":
		var lockServerRepl *LockServerRepl
		blockingRaftNode := newBlockingRaftNode[string](*id, strings.Split(*cluster, ","), *join, proposeC, confChangeC, *clearLog, lockServerRepl)
		lockServerRepl = newReplLockServer(proposeC, blockingRaftNode.appliedC)
		blockingRaftNode.start()
		lockServer = lockServerRepl
	case "kv":
		var kvServer *KVServer
		blockingRaftNode := newBlockingRaftNode[string](*id, strings.Split(*cluster, ","), *join, proposeC, confChangeC, *clearLog, kvServer)
		kvServer = newKVServer(proposeC, blockingRaftNode.appliedC)
		blockingRaftNode.start()
		serveHTTPKVAPI(kvServer, *lsport, confChangeC, errorC)
		return
	}

	// the key-value http handler will propose updates to raft
	serveHTTPLSAPI(lockServer, *lsport, confChangeC, errorC)
}
