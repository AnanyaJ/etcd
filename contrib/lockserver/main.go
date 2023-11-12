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

	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"go.etcd.io/raft/v3/raftpb"
)

func main() {
	cluster := flag.String("cluster", "http://127.0.0.1:12379", "comma separated cluster peers")
	id := flag.Int("id", 1, "node ID")
	lsport := flag.Int("port", 12380, "lock server port")
	join := flag.Bool("join", false, "join an existing cluster")
	impl := flag.String("impl", "blockingrafttranslate", "lock server implementation to use (simple, condvar, queues, coros, kvlocks, blockingrafttranslate, kv)")
	clearLog := flag.Bool("clearlog", false, "whether to use a fresh log, removing all previous persistent state")
	flag.Parse()

	proposeC := make(chan []byte)
	defer close(proposeC)
	confChangeC := make(chan raftpb.ConfChange)
	defer close(confChangeC)

	var blockingRaftNode *BlockingRaftNode[string]
	getSnapshot := func() ([]byte, error) { return nil, nil }
	if *impl == "blockingraft" {
		getSnapshot = func() ([]byte, error) { return blockingRaftNode.getSnapshot() }
	} else if *impl == "blockingrafttranslate" || *impl == "kv" {
		getSnapshot = func() ([]byte, error) { return blockingRaftNode.getSnapshot() }
	}

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
		var translatedLockServer *LockServerRepl
		apply := func(data []byte, access func(func() []any) []any, wait func(string), signal func(string)) []byte {
			return translatedLockServer.apply(data, access, wait, signal)
		}
		snapshot := func() ([]byte, error) { return translatedLockServer.getSnapshot() }
		loadSnapshot := func(snapshot []byte) error { return translatedLockServer.loadSnapshot(snapshot) }
		snapshotterReady := make(chan *snap.Snapshotter)
		blockingRaftNode = newBlockingRaftNode[string](snapshotterReady, commitC, errorC, apply, snapshot, loadSnapshot)
		translatedLockServer = newReplLockServer(proposeC, blockingRaftNode.appliedC)
		snapshotterReady <- snapshotter
		lockServer = translatedLockServer
	case "kv":
		var kvServer *KVServer
		apply := func(data []byte, access func(func() []any) []any, wait func(string), signal func(string)) []byte {
			return kvServer.apply(data, access, wait, signal)
		}
		snapshot := func() ([]byte, error) { return kvServer.getSnapshot() }
		loadSnapshot := func(snapshot []byte) error { return kvServer.loadSnapshot(snapshot) }
		snapshotterReady := make(chan *snap.Snapshotter)
		blockingRaftNode = newBlockingRaftNode[string](snapshotterReady, commitC, errorC, apply, snapshot, loadSnapshot)
		kvServer = newKVServer(proposeC, blockingRaftNode.appliedC)
		snapshotterReady <- snapshotter
		serveHTTPKVAPI(kvServer, *lsport, confChangeC, errorC)
		return
	}

	// the key-value http handler will propose updates to raft
	serveHTTPLSAPI(lockServer, *lsport, confChangeC, errorC)
}
