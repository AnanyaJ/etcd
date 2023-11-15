package main

import (
	"encoding/json"
	"log"
	"time"

	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
)

type SimpleLockServer struct {
	proposeC    chan<- []byte
	opManager   *OpManager
	locks       map[string]bool
	timeout     int
	snapshotter *snap.Snapshotter
}

func newSimpleLockServer(snapshotter *snap.Snapshotter, proposeC chan<- []byte, commitC <-chan *commit, errorC <-chan error, timeout int) *SimpleLockServer {
	s := &SimpleLockServer{
		proposeC:    proposeC,
		opManager:   newOpManager(),
		locks:       make(map[string]bool),
		timeout:     timeout,
		snapshotter: snapshotter,
	}
	s.loadSnapshot()
	// apply commits from raft until error
	go s.applyCommits(commitC, errorC)
	return s
}

func (s *SimpleLockServer) startOp(opType int, lockName string, clientID ClientID, opNum int64) bool {
	op := LockOp{OpType: opType, LockName: lockName, ClientID: clientID, OpNum: opNum}
	result := s.opManager.addOp(opNum)
	s.proposeC <- op.marshal()
	return <-result
}

func (s *SimpleLockServer) Acquire(lockName string, clientID ClientID, opNum int64) {
	for {
		// keep trying until acquisition succeeds
		result := s.startOp(AcquireOp, lockName, clientID, opNum)
		if result {
			return
		}
		time.Sleep(time.Duration(s.timeout) * time.Millisecond)
	}
}

func (s *SimpleLockServer) Release(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(ReleaseOp, lockName, clientID, opNum)
}

func (s *SimpleLockServer) IsLocked(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(IsLockedOp, lockName, clientID, opNum)
}

func (s *SimpleLockServer) applyCommits(commitC <-chan *commit, errorC <-chan error) {
	for commit := range commitC {
		if commit == nil {
			// signaled to load snapshot
			s.loadSnapshot()
			continue
		}

		for _, data := range commit.data {
			op := lockOpFromBytes(data)

			switch op.OpType {
			case AcquireOp:
				canAcquire := !s.locks[op.LockName]
				if canAcquire {
					s.locks[op.LockName] = true
				}
				s.opManager.reportOpFinished(op.OpNum, canAcquire)
			case ReleaseOp:
				wasLocked := s.locks[op.LockName]
				s.locks[op.LockName] = false
				s.opManager.reportOpFinished(op.OpNum, wasLocked)
			case IsLockedOp:
				s.opManager.reportOpFinished(op.OpNum, s.locks[op.LockName])
			}
		}

		close(commit.applyDoneC)
	}
	if err, ok := <-errorC; ok {
		log.Fatal(err)
	}
}

func (s *SimpleLockServer) getSnapshot() ([]byte, error) {
	return json.Marshal(s.locks)
}

func (s *SimpleLockServer) loadSnapshot() {
	snapshot, err := s.snapshotter.Load()
	if err == snap.ErrNoSnapshot {
		return
	}
	if err != nil {
		log.Panic(err)
	}
	if snapshot != nil {
		log.Printf("loading snapshot at term %d and index %d", snapshot.Metadata.Term, snapshot.Metadata.Index)
		if err := s.recoverFromSnapshot(snapshot.Data); err != nil {
			log.Panic(err)
		}
	}
}

func (s *SimpleLockServer) recoverFromSnapshot(snapshot []byte) error {
	var locks map[string]bool
	if err := json.Unmarshal(snapshot, &locks); err != nil {
		return err
	}
	s.locks = locks
	return nil
}
