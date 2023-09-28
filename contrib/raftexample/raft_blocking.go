package main

import (
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"go.etcd.io/raft/v3/raftpb"
)

type RaftBlockingNode struct {
	commitC          <-chan *commit
	snapshotterReady <-chan *snap.Snapshotter
	apply            func(string) error
}

func newRaftBlockingNode(id int, peers []string, join bool, getSnapshot func() ([]byte, error), proposeC <-chan string,
	confChangeC <-chan raftpb.ConfChange, apply func(string) error) <-chan error {
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
			err := rn.apply(op)
			if err != nil {
				// TODO: handle case where op fails to be applied
			}
		}
	}
}
