package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"log"
	"sync"

	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
)

type QueueLockServer struct {
	mu          sync.Mutex
	proposeC    chan<- []byte
	locks       map[string]*LockQueue[LockOp]
	nextOpNum   int
	opResults   map[int]chan bool
	snapshotter *snap.Snapshotter
}

// Replicated lock service built using a queue for each lock. This lock server
// performs similarly to the one that uses condition variables, except that it can
// safely be replicated since it uses queues to deterministically schedule
// the Acquire operations. Currently uses a WAL that is persisted on disk, and
// supports snapshotting.
func newQueueLockServer(snapshotter *snap.Snapshotter, proposeC chan<- []byte, commitC <-chan *commit, errorC <-chan error) *QueueLockServer {
	s := &QueueLockServer{
		mu:          sync.Mutex{},
		proposeC:    proposeC,
		locks:       make(map[string]*LockQueue[LockOp]),
		nextOpNum:   0,
		opResults:   make(map[int]chan bool),
		snapshotter: snapshotter,
	}
	s.loadSnapshot()
	// apply commits from raft until error
	go s.applyCommits(commitC, errorC)
	return s
}

func (s *QueueLockServer) startOp(opType int, lockName string) bool {
	result := make(chan bool)

	// assign each op a unique op number so that applier knows which
	// result channel to signal on
	s.mu.Lock()
	opNum := s.nextOpNum
	s.nextOpNum++
	s.opResults[opNum] = result
	s.mu.Unlock()

	op := LockOp{OpNum: opNum, OpType: opType, LockName: lockName}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(op); err != nil {
		log.Fatal(err)
	}
	s.proposeC <- buf.Bytes()
	return <-result
}

func (s *QueueLockServer) Acquire(lockName string) {
	s.startOp(AcquireOp, lockName)
}

func (s *QueueLockServer) Release(lockName string) bool {
	return s.startOp(ReleaseOp, lockName)
}

func (s *QueueLockServer) IsLocked(lockName string) bool {
	return s.startOp(IsLockedOp, lockName)
}

func (s *QueueLockServer) applyCommits(commitC <-chan *commit, errorC <-chan error) {
	for commit := range commitC {
		if commit == nil {
			// signaled to load snapshot
			s.loadSnapshot()
			continue
		}

		for _, data := range commit.data {
			var op LockOp
			dec := gob.NewDecoder(bytes.NewBuffer(data))
			if err := dec.Decode(&op); err != nil {
				log.Fatalf("lockserver: could not decode message (%v)", err)
			}

			s.addLock(op.LockName)
			lock := s.locks[op.LockName]

			switch op.OpType {
			case AcquireOp:
				if lock.IsLocked {
					lock.Queue = append(lock.Queue, op)
				} else {
					lock.IsLocked = true
					s.returnResult(op, true)
				}
			case ReleaseOp:
				if !lock.IsLocked {
					s.returnResult(op, false) // lock already free
				}

				if len(lock.Queue) > 0 {
					// pass lock to next waiting op
					unblocked := lock.Queue[0]
					lock.Queue = lock.Queue[1:]
					s.returnResult(unblocked, true)
				} else {
					lock.IsLocked = false
				}

				s.returnResult(op, true)
			case IsLockedOp:
				s.returnResult(op, lock.IsLocked)
			}
		}

		close(commit.applyDoneC)
	}
	if err, ok := <-errorC; ok {
		log.Fatal(err)
	}
}

func (s *QueueLockServer) returnResult(op LockOp, result bool) {
	resultChan, ok := s.opResults[op.OpNum]
	if ok { // this replica is responsible for delivering result
		resultChan <- result
		delete(s.opResults, op.OpNum)
	}
}

func (s *QueueLockServer) addLock(lockName string) {
	if s.locks[lockName] != nil {
		return // already exists
	}
	s.locks[lockName] = &LockQueue[LockOp]{Queue: []LockOp{}}
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
	var locks map[string]*LockQueue[LockOp]
	if err := json.Unmarshal(snapshot, &locks); err != nil {
		return err
	}
	s.locks = locks
	return nil
}
