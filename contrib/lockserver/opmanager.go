package main

import "sync"

type OpManager struct {
	mu        *sync.Mutex
	opResults map[int64]chan bool
}

func newOpManager() *OpManager {
	return &OpManager{
		mu:        &sync.Mutex{},
		opResults: make(map[int64]chan bool),
	}
}

func (om *OpManager) addOp(opNum int64) chan bool {
	om.mu.Lock()
	defer om.mu.Unlock()

	result := make(chan bool)
	om.opResults[opNum] = result
	return result
}

func (om *OpManager) reportOpFinished(opNum int64, result bool) {
	om.mu.Lock()
	defer om.mu.Unlock()

	resultChan, ok := om.opResults[opNum]
	if ok { // this replica is responsible for delivering result to RPC handler
		resultChan <- result
		delete(om.opResults, opNum)
	}
}
