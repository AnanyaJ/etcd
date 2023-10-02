package main

import "encoding/json"

const (
	Acquire int = iota
	Release
	IsLocked
)

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
	case Acquire:
		// may be able to remove for loop depending on semantics of scheduler
		for s.locks[op.lockName] {
			wait(op.lockName)
		}
		s.locks[op.lockName] = true
	case Release:
		if s.locks[op.lockName] {
			s.locks[op.lockName] = false
			signal(op.lockName)
		}
	case IsLocked:
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
