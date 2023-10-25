package main

import (
	"sync"
)

type OpManager struct {
	mu        sync.Mutex
	nextOpNum int
	opResults map[int]chan bool
}

func newOpManager() *OpManager {
	return &OpManager{
		mu:        sync.Mutex{},
		nextOpNum: 0,
		opResults: make(map[int]chan bool),
	}
}

func (om *OpManager) addOp(opType int, lockName string) (LockOp, chan bool) {
	result := make(chan bool)

	// assign each op a unique op number so that applier knows which
	// result channel to signal on
	om.mu.Lock()
	opNum := om.nextOpNum
	om.nextOpNum++
	om.opResults[opNum] = result
	om.mu.Unlock()

	op := LockOp{OpNum: opNum, OpType: opType, LockName: lockName}

	return op, result
}

func (om *OpManager) reportOpFinished(opNum int, result bool) {
	resultChan, ok := om.opResults[opNum]
	if ok { // this replica is responsible for delivering result to RPC handler
		resultChan <- result
		delete(om.opResults, opNum)
	}
}
