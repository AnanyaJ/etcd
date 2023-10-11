package main

import (
	"bytes"
	"encoding/gob"
	"log"
	"sync"

	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
)

type LockOp struct {
	OpNum    int
	OpType   int
	LockName string
}

type Coro struct {
	OpNum  int
	Resume func() (Status, bool)
}

type LockQueue struct {
	IsLocked bool
	Queue    []Coro
}

type QueueLockServer struct {
	mu          sync.Mutex
	proposeC    chan<- []byte
	locks       map[string]*LockQueue
	nextOpNum   int
	opResults   map[int]chan bool
	snapshotter *snap.Snapshotter
}

// Replicated lock service built using a queue for each lock. This lock server
// performs similarly to the one that uses condition variables, except that it can
// safely be replicated since it uses coroutines to deterministically schedule
// the Acquire and Release operations. Currently uses a WAL that is persisted
// on disk, but does not support snapshotting yet.
func newQueueLockServer(snapshotter *snap.Snapshotter, proposeC chan<- []byte, commitC <-chan *commit, errorC <-chan error) *QueueLockServer {
	s := &QueueLockServer{
		mu:          sync.Mutex{},
		proposeC:    proposeC,
		locks:       make(map[string]*LockQueue),
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

	s.mu.Lock() // lock so that each op is assigned a unique op number
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

			// new coro is initially the only runnable one
			var apply func(lockName string, wait func(Key), signal func(Key)) bool
			switch op.OpType {
			case AcquireOp:
				apply = s.acquire
			case ReleaseOp:
				apply = s.release
			case IsLockedOp:
				apply = s.isLocked
			}

			coro := CreateCoro[string, bool](apply, op.LockName)
			runnable := []Coro{Coro{OpNum: op.OpNum, Resume: coro}}
			// resume coros until no coro can make more progress -- ensures
			// deterministic behavior since timing of ops arriving on
			// started channel cannot be controlled
			for len(runnable) > 0 {
				// resume coros in stack order
				next := runnable[len(runnable)-1]
				runnable = runnable[:len(runnable)-1]

				status, output := next.Resume()
				switch status.msgType() {
				case WaitMsg:
					key := status.(Wait).key.(string)
					s.locks[key].Queue = append(s.locks[key].Queue, next)
				case SignalMsg:
					// this coro is still not blocked so add back to runnable stack
					runnable = append(runnable, next)
					key := status.(Signal).key.(string)
					queue := s.locks[key].Queue
					if len(queue) > 0 {
						// unblock exactly one coro waiting on lock
						unblocked := queue[0]
						s.locks[key].Queue = queue[1:]
						runnable = append(runnable, unblocked)
					}
				case DoneMsg:
					// inform RPC handler of op completion and result
					s.opResults[next.OpNum] <- output
				}
			}

		}
		close(commit.applyDoneC)
	}
	if err, ok := <-errorC; ok {
		log.Fatal(err)
	}
}

func (s *QueueLockServer) acquire(lockName string, wait func(Key), signal func(Key)) bool {
	// locks are unnecessary here because code between wait/signal calls is
	// executed atomically and there is only one thread applying ops
	s.addLock(lockName)

	lock := s.locks[lockName]
	for lock.IsLocked {
		wait(lockName)
	}

	lock.IsLocked = true
	return true
}

func (s *QueueLockServer) release(lockName string, wait func(Key), signal func(Key)) bool {
	s.addLock(lockName)
	lock := s.locks[lockName]

	if !lock.IsLocked {
		return false // lock already free
	}

	lock.IsLocked = false
	signal(lockName)
	return true
}

func (s *QueueLockServer) isLocked(lockName string, wait func(Key), signal func(Key)) bool {
	s.addLock(lockName)
	return s.locks[lockName].IsLocked
}

func (s *QueueLockServer) addLock(lockName string) {
	if s.locks[lockName] != nil {
		return // already exists
	}
	s.locks[lockName] = &LockQueue{Queue: []Coro{}}
}

func (s *QueueLockServer) getSnapshot() ([]byte, error) {
	return []byte{}, nil
	// return json.Marshal(s.locks)
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
	// var locks map[string]*LockQueue
	// if err := json.Unmarshal(snapshot, &locks); err != nil {
	// 	return err
	// }
	// s.locks = locks
	return nil
}
