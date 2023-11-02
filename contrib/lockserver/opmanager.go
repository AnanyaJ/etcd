package main

type OpManager struct {
	opResults map[int]chan bool
}

func newOpManager() *OpManager {
	return &OpManager{
		opResults: make(map[int]chan bool),
	}
}

func (om *OpManager) addOp(opType int, lockName string, opNum int) (LockOp, chan bool) {
	result := make(chan bool)
	om.opResults[opNum] = result
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
