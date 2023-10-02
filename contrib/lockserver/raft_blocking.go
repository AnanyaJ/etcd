package main

import (
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"go.etcd.io/raft/v3/raftpb"
)

type RaftBlockingNode struct {
	commitC          <-chan *commit
	snapshotterReady <-chan *snap.Snapshotter
	apply            func(op []byte, wait func(Key), signal func(Key)) []byte
}

func newRaftBlockingNode(
	id int, peers []string, join bool,
	getSnapshot func() ([]byte, error),
	proposeC <-chan []byte,
	confChangeC <-chan raftpb.ConfChange,
	apply func(op []byte, wait func(Key), signal func(Key)) []byte,
) <-chan error {
	commitC, errorC, snapshotterReady := newRaftNode(id, peers, join, getSnapshot, proposeC, confChangeC)
	// TODO: deal with snapshots
	n := &RaftBlockingNode{commitC, snapshotterReady, apply}
	go n.applyCommittedOps()
	return errorC
}

func (rn *RaftBlockingNode) applyCommittedOps() {
	for {
		committed := <-rn.commitC
		for _, op := range committed.data {
			CreateCoro(rn.apply, op, func(Key) {}, func(Key) {})

		}
	}
}
