package main

import "encoding/json"

type Op struct {
	opType   int
	lockName string
}

type ReplLockServer struct {
	locks map[string]bool
}

func (s *ReplLockServer) apply(data []byte, wait func(string), signal func(string)) []byte {
	op := Op{}
	err := json.Unmarshal(data, &op)
	if err != nil {
		// TODO: propagate error
	}
	switch op.opType {
	case AcquireOp:
		// may be able to remove loop depending on semantics of scheduler
		for s.locks[op.lockName] {
			wait(op.lockName) // keep waiting while lock is held
		}
		s.locks[op.lockName] = true
	case ReleaseOp:
		// release lock if held
		if s.locks[op.lockName] {
			s.locks[op.lockName] = false
			signal(op.lockName)
		}
	case IsLockedOp:
		var isLocked int8
		if s.locks[op.lockName] {
			isLocked = 1
		} else {
			isLocked = 0
		}
		return []byte{byte(isLocked)}
	}
	return []byte{}
}
