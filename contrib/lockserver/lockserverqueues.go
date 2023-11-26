package main

import (
	"encoding/json"
	"log"

	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
)

type QueueLockServer struct {
	proposeC    chan<- []byte
	opManager   *OpManager
	locks       map[string]*LockQueue[*LockOp]
	snapshotter *snap.Snapshotter
}

// Replicated lock service built using a queue for each lock. This lock server
// performs similarly to the one that uses condition variables, except that it can
// safely be replicated since it uses queues to deterministically schedule
// the Acquire operations. Currently uses a WAL that is persisted on disk, and
// supports snapshotting.
func newQueueLockServer(snapshotter *snap.Snapshotter, proposeC chan<- []byte, commitC <-chan *commit, errorC <-chan error) *QueueLockServer {
	s := &QueueLockServer{
		proposeC:    proposeC,
		opManager:   newOpManager(),
		locks:       make(map[string]*LockQueue[*LockOp]),
		snapshotter: snapshotter,
	}
	s.loadSnapshot()
	// apply commits from raft until error
	go s.applyCommits(commitC, errorC)
	return s
}

func (s *QueueLockServer) startOp(opType int, lockName string, clientID ClientID, opNum int64) bool {
	op := LockOp{OpType: opType, LockName: lockName, ClientID: clientID, OpNum: opNum}
	result := s.opManager.addOp(opNum)
	s.proposeC <- op.marshal()
	return <-result
}

func (s *QueueLockServer) Acquire(lockName string, clientID ClientID, opNum int64) {
	s.startOp(AcquireOp, lockName, clientID, opNum)
}

func (s *QueueLockServer) Release(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(ReleaseOp, lockName, clientID, opNum)
}

func (s *QueueLockServer) IsLocked(lockName string, clientID ClientID, opNum int64) bool {
	return s.startOp(IsLockedOp, lockName, clientID, opNum)
}

func (s *QueueLockServer) applyCommits(commitC <-chan *commit, errorC <-chan error) {
	for commit := range commitC {
		if commit == nil {
			// signaled to load snapshot
			s.loadSnapshot()
			continue
		}

		for _, data := range commit.data {
			op := lockOpFromBytes(data)
			s.addLock(op.LockName)
			lock := s.locks[op.LockName]

			switch op.OpType {
			case AcquireOp:
				if lock.IsLocked {
					lock.Queue = append(lock.Queue, op)
				} else {
					lock.IsLocked = true
					s.opManager.reportOpFinished(op.OpNum, true)
				}
			case ReleaseOp:
				if !lock.IsLocked {
					s.opManager.reportOpFinished(op.OpNum, false) // lock already free
				}

				if len(lock.Queue) > 0 {
					// pass lock to next waiting op
					unblocked := lock.Queue[0]
					lock.Queue = lock.Queue[1:]
					s.opManager.reportOpFinished(unblocked.OpNum, true)
				} else {
					lock.IsLocked = false
				}

				s.opManager.reportOpFinished(op.OpNum, true)
			case IsLockedOp:
				s.opManager.reportOpFinished(op.OpNum, lock.IsLocked)
			}
		}

		close(commit.applyDoneC)
	}
	if err, ok := <-errorC; ok {
		log.Fatal(err)
	}
}

func (s *QueueLockServer) addLock(lockName string) {
	if s.locks[lockName] != nil {
		return // already exists
	}
	s.locks[lockName] = &LockQueue[*LockOp]{Queue: []*LockOp{}}
}

func (s *QueueLockServer) getSnapshot() ([]byte, error) {
	return json.Marshal(s.locks)
}

func (s *QueueLockServer) loadSnapshot() {
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

func (s *QueueLockServer) recoverFromSnapshot(snapshot []byte) error {
	var locks map[string]*LockQueue[*LockOp]
	if err := json.Unmarshal(snapshot, &locks); err != nil {
		return err
	}
	s.locks = locks
	return nil
}
