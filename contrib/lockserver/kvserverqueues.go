package main

import (
	"encoding/json"
	"log"

	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
)

type QueueKVServer struct {
	proposeC    chan<- []byte
	opManager   *OpManager
	keys        map[string]*KeyQueue[KVServerOp]
	snapshotter *snap.Snapshotter
}

func newQueueKVServer(snapshotter *snap.Snapshotter, proposeC chan<- []byte, commitC <-chan *commit, errorC <-chan error) *QueueKVServer {
	s := &QueueKVServer{
		proposeC:    proposeC,
		opManager:   newOpManager(),
		keys:        make(map[string]*KeyQueue[KVServerOp]),
		snapshotter: snapshotter,
	}
	s.loadSnapshot()
	// apply commits from raft until error
	go s.applyCommits(commitC, errorC)
	return s
}

// Propose op that some RPC handler wants to replicate
func (kv *QueueKVServer) startOp(opType int, key string, val int, opNum int64) bool {
	op := KVServerOp{OpNum: opNum, OpType: opType, Key: key, Val: val}
	result := kv.opManager.addOp(opNum)
	kv.proposeC <- encodeNoErr(op)
	return <-result
}

func (kv *QueueKVServer) Increment(key string, opNum int64) {
	kv.startOp(IncrementOp, key, 0, opNum)
}

func (kv *QueueKVServer) Wait(key string, untilValue int, opNum int64) {
	kv.startOp(WaitOp, key, untilValue, opNum)
}

func (s *QueueKVServer) applyCommits(commitC <-chan *commit, errorC <-chan error) {
	for commit := range commitC {
		if commit == nil {
			// signaled to load snapshot
			s.loadSnapshot()
			continue
		}

		for _, data := range commit.data {
			var op KVServerOp
			decodeNoErr(data, &op)
			s.addKey(op.Key)

			switch op.OpType {
			case IncrementOp:
				s.keys[op.Key].Value++
				stillBlocked := Queue[KVServerOp]{}
				for _, blocked := range s.keys[op.Key].Queue {
					if s.keys[op.Key].Value == blocked.Val {
						s.opManager.reportOpFinished(blocked.OpNum, true)
					} else {
						stillBlocked = append(stillBlocked, blocked)
					}
				}
				s.keys[op.Key].Queue = stillBlocked
				s.opManager.reportOpFinished(op.OpNum, true)
			case WaitOp:
				if s.keys[op.Key].Value == op.Val {
					s.opManager.reportOpFinished(op.OpNum, true)
				} else {
					s.keys[op.Key].Queue = append(s.keys[op.Key].Queue, op)
				}
			}
		}

		close(commit.applyDoneC)
	}
	if err, ok := <-errorC; ok {
		log.Fatal(err)
	}
}

func (s *QueueKVServer) addKey(key string) {
	if s.keys[key] != nil {
		return // already exists
	}
	s.keys[key] = &KeyQueue[KVServerOp]{Queue: []KVServerOp{}}
}

func (s *QueueKVServer) getSnapshot() ([]byte, error) {
	return json.Marshal(s.keys)
}

func (s *QueueKVServer) loadSnapshot() {
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

func (s *QueueKVServer) recoverFromSnapshot(snapshot []byte) error {
	var keys map[string]*KeyQueue[KVServerOp]
	if err := json.Unmarshal(snapshot, &keys); err != nil {
		return err
	}
	s.keys = keys
	return nil
}
