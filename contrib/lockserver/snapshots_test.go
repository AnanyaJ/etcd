package main

import (
	"encoding/gob"
	"encoding/json"
	"log"
	"testing"
	"time"

	"go.etcd.io/raft/v3/raftpb"
)

const (
	SnapshotTestOpA int = iota
	SnapshotTestOpB
	SnapshotTestOpC
	SnapshotTestOpD
	SnapshotTestOpE
	SnapshotTestOpF
)

func snapshots_test_setup() (blockingRaftNode *BlockingRaftNode[string, KVOp, bool], proposeC chan []byte, confChangeC chan raftpb.ConfChange) {
	clusters := []string{"http://127.0.0.1:9021"}
	proposeC = make(chan []byte)
	confChangeC = make(chan raftpb.ConfChange)

	getSnapshot := func() ([]byte, error) { return blockingRaftNode.getSnapshot() }

	commitC, errorC, snapshotterReady := newRaftNode(1, clusters, false, getSnapshot, proposeC, confChangeC, true)

	gob.Register(KVState{})
	lockState := make(KVState)
	blockingRaftNode = newBlockingRaftNode[string, KVOp, bool](<-snapshotterReady, commitC, errorC, lockState, apply)

	return blockingRaftNode, proposeC, confChangeC
}

func apply(
	data []byte,
	access func(KVOp) bool,
	wait func(string),
	signal func(string),
) []byte {
	var opType int
	err := json.Unmarshal(data, &opType)
	if err != nil {
		log.Fatalf("Failed to unmarshal applied op")
	}

	isLocked := func(lock string) bool { return access(KVOp{OpType: GetOp, Key: lock}) }
	setLocked := func(lock string, val bool) { access(KVOp{OpType: PutOp, Key: lock, Val: val}) }

	acquire := func(lock string) {
		for isLocked(lock) {
			wait(lock) // keep waiting while lock is held
		}
		setLocked(lock, true)
	}
	release := func(lock string) {
		if isLocked(lock) {
			setLocked(lock, false)
			signal(lock)
		}
	}

	switch opType {
	case SnapshotTestOpA:
		acquire("lock2")
	case SnapshotTestOpB:
		acquire("lock1")
	case SnapshotTestOpC:
		if !isLocked("lock1") {
			acquire("lock2")
		}
	case SnapshotTestOpD:
		return marshal(isLocked("lock1"))
	case SnapshotTestOpE:
		release("lock2")
	case SnapshotTestOpF:
		return marshal(isLocked("lock2"))
	}

	return marshal(true)
}

func marshal_optype(t *testing.T, opType int) []byte {
	op, err := json.Marshal(&opType)
	if err != nil {
		t.Fatal(err)
	}
	return op
}

func check_result_and_get_op_type(t *testing.T, op AppliedOp) int {
	var res bool
	err := json.Unmarshal(op.result, &res)
	if err != nil {
		t.Fatal(err)
	}
	if res != true {
		t.Fatal("Expected result true but got false")
	}

	var opType int
	err = json.Unmarshal(op.op, &opType)
	if err != nil {
		t.Fatal(err)
	}
	return opType
}

func TestSnapshots(t *testing.T) {
	node, proposeC, confChangeC := snapshots_test_setup()

	// acquire lock2
	proposeC <- marshal_optype(t, SnapshotTestOpA)
	doneOp := check_result_and_get_op_type(t, <-node.appliedC)
	if doneOp != SnapshotTestOpA {
		t.Fatalf("Expected operation %d to be applied but got %d instead", SnapshotTestOpA, doneOp)
	}

	// will block on lock2 since lock1 is free
	proposeC <- marshal_optype(t, SnapshotTestOpC)

	// acquire lock1
	proposeC <- marshal_optype(t, SnapshotTestOpB)
	doneOp = check_result_and_get_op_type(t, <-node.appliedC)
	if doneOp != SnapshotTestOpB {
		t.Fatalf("Expected operation %d to be applied but got %d instead", SnapshotTestOpB, doneOp)
	}

	snapshot, err := node.getSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	close(proposeC)
	close(confChangeC)

	<-time.After(time.Second)

	// replay from snapshot
	node, proposeC, confChangeC = snapshots_test_setup()
	node.recoverFromSnapshot(snapshot)

	// make sure that lock1 is still locked
	proposeC <- marshal_optype(t, SnapshotTestOpD)
	doneOp = check_result_and_get_op_type(t, <-node.appliedC)
	if doneOp != SnapshotTestOpD {
		t.Fatalf("Expected operation %d to be applied but got %d instead", SnapshotTestOpD, doneOp)
	}

	// release lock2 so that op C can acquire it
	proposeC <- marshal_optype(t, SnapshotTestOpE)

	doneOp1 := check_result_and_get_op_type(t, <-node.appliedC)
	doneOp2 := check_result_and_get_op_type(t, <-node.appliedC)

	if !((doneOp1 == SnapshotTestOpC && doneOp2 == SnapshotTestOpE) || (doneOp1 == SnapshotTestOpE && doneOp2 == SnapshotTestOpC)) {
		t.Fatalf("Expected ops %d and %d to be applied but got %d and %d instead", SnapshotTestOpC, SnapshotTestOpE, doneOp1, doneOp2)
	}

	// make sure that lock2 is still locked, now by op C
	// This is because in the original execution, op C blocks on lock2 because lock1 is free at the time.
	// Even though after replaying the snapshot, lock1 is held, op C should still try to acquire lock2
	// because otherwise its behavior is not deterministic.
	proposeC <- marshal_optype(t, SnapshotTestOpF)
	doneOp = check_result_and_get_op_type(t, <-node.appliedC)
	if doneOp != SnapshotTestOpF {
		t.Fatalf("Expected operation %d to be applied but got %d instead", SnapshotTestOpF, doneOp)
	}

	close(proposeC)
	close(confChangeC)
}
