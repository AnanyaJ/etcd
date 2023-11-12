package main

import (
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
	SnapshotTestOpG
)

type SnapshotsTestState struct {
	locks map[string]bool
}

func (s *SnapshotsTestState) setup_test() (blockingRaftNode *BlockingRaftNode[string], proposeC chan []byte, confChangeC chan raftpb.ConfChange) {
	clusters := []string{"http://127.0.0.1:9021"}
	proposeC = make(chan []byte)
	confChangeC = make(chan raftpb.ConfChange)
	blockingRaftNode = newBlockingRaftNode[string](1, clusters, false, proposeC, confChangeC, true, s)
	blockingRaftNode.start()
	return blockingRaftNode, proposeC, confChangeC
}

func stop_raft(proposeC chan []byte, confChangeC chan raftpb.ConfChange) {
	close(proposeC)
	close(confChangeC)
	// wait for server to stop
	<-time.After(time.Second)
}

func (s *SnapshotsTestState) restart_from_snapshot(
	t *testing.T,
	node *BlockingRaftNode[string],
	proposeC chan []byte,
	confChangeC chan raftpb.ConfChange,
) (nodeNew *BlockingRaftNode[string], proposeCNew chan []byte, confChangeCNew chan raftpb.ConfChange) {
	snapshot, err := node.getSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	stop_raft(proposeC, confChangeC)
	s = &SnapshotsTestState{locks: make(map[string]bool)}

	node, proposeCNew, confChangeCNew = s.setup_test()
	node.recoverFromSnapshot(snapshot)
	return node, proposeCNew, confChangeCNew
}

func (s *SnapshotsTestState) apply(
	data []byte,
	access func(func() []any) []any,
	wait func(string),
	signal func(string),
) []byte {
	var opType int
	err := json.Unmarshal(data, &opType)
	if err != nil {
		log.Fatalf("Failed to unmarshal applied op")
	}

	isLocked := func(lock string) bool {
		res := access(func() []any { return []any{s.locks[lock]} })
		return res[0].(bool)
	}
	setLocked := func(lock string, val bool) {
		access(func() []any {
			s.locks[lock] = val
			return []any{}
		})
	}

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
		acquire("lock1")
	case SnapshotTestOpD:
		return marshal(isLocked("lock1"))
	case SnapshotTestOpE:
		release("lock2")
	case SnapshotTestOpF:
		return marshal(isLocked("lock2"))
	case SnapshotTestOpG:
		release("lock1")
	}

	return marshal(true)
}

func (s *SnapshotsTestState) getSnapshot() ([]byte, error) {
	return json.Marshal(s.locks)
}

func (s *SnapshotsTestState) loadSnapshot(snapshot []byte) error {
	return json.Unmarshal(snapshot, &s.locks)
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

func check_op_matches(t *testing.T, expectedOp int, receivedOp int) {
	if expectedOp != receivedOp {
		t.Fatalf("Expected operation %d to have completed but operation %d completed instead", expectedOp, receivedOp)
	}
}

func TestSnapshots(t *testing.T) {
	state := &SnapshotsTestState{locks: make(map[string]bool)}
	node, proposeC, confChangeC := state.setup_test()

	// acquire lock2
	proposeC <- marshal_optype(t, SnapshotTestOpA)
	doneOp := check_result_and_get_op_type(t, <-node.appliedC)
	check_op_matches(t, doneOp, SnapshotTestOpA)

	// will block on lock2 since lock1 is free
	proposeC <- marshal_optype(t, SnapshotTestOpC)

	// acquire lock1
	proposeC <- marshal_optype(t, SnapshotTestOpB)
	doneOp = check_result_and_get_op_type(t, <-node.appliedC)
	check_op_matches(t, doneOp, SnapshotTestOpB)

	node, proposeC, confChangeC = state.restart_from_snapshot(t, node, proposeC, confChangeC)

	// make sure that lock1 is still locked
	proposeC <- marshal_optype(t, SnapshotTestOpD)
	doneOp = check_result_and_get_op_type(t, <-node.appliedC)
	check_op_matches(t, doneOp, SnapshotTestOpD)

	// release lock2 so that op C can acquire it
	proposeC <- marshal_optype(t, SnapshotTestOpE)
	doneOp = check_result_and_get_op_type(t, <-node.appliedC)
	check_op_matches(t, doneOp, SnapshotTestOpE)

	// make sure that lock2 is still locked (by op C)
	// This is because in the original execution, op C blocks on lock2 because lock1 is free at the time.
	// Even though after replaying the snapshot, lock1 is held, op C should still try to acquire lock2
	// because otherwise its behavior is not deterministic. Op C will now be blocking on lock1.
	proposeC <- marshal_optype(t, SnapshotTestOpF)
	doneOp = check_result_and_get_op_type(t, <-node.appliedC)
	check_op_matches(t, doneOp, SnapshotTestOpF)

	node, proposeC, confChangeC = state.restart_from_snapshot(t, node, proposeC, confChangeC)

	// release lock1 so that op C can finish
	proposeC <- marshal_optype(t, SnapshotTestOpG)

	doneOp1 := check_result_and_get_op_type(t, <-node.appliedC)
	doneOp2 := check_result_and_get_op_type(t, <-node.appliedC)
	if !((doneOp1 == SnapshotTestOpC && doneOp2 == SnapshotTestOpG) || (doneOp1 == SnapshotTestOpG && doneOp2 == SnapshotTestOpC)) {
		t.Fatalf("Expected ops %d and %d to be applied but got %d and %d instead", SnapshotTestOpC, SnapshotTestOpG, doneOp1, doneOp2)
	}

	// make sure that lock1 is still locked (now by op C)
	proposeC <- marshal_optype(t, SnapshotTestOpD)
	doneOp = check_result_and_get_op_type(t, <-node.appliedC)
	check_op_matches(t, doneOp, SnapshotTestOpD)

	stop_raft(proposeC, confChangeC)
}
