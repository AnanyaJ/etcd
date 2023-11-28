package main

// import "encoding/json"

// const (
// 	IncrementOp int = iota
// 	WaitOp
// )

// type AppliedKVOp AppliedOp[KVServerOp, struct{}]

// type KVServerRepl struct {
// 	kv map[string]int

// 	proposeC  chan []byte
// 	opManager *OpManager
// 	appliedC  <-chan AppliedKVOp
// }

// func newKVServer(proposeC chan []byte, appliedC <-chan AppliedKVOp) *KVServerRepl {
// 	kv := &KVServerRepl{
// 		kv:        make(map[string]int),
// 		proposeC:  proposeC,
// 		opManager: newOpManager(),
// 		appliedC:  appliedC,
// 	}
// 	go kv.processApplied()
// 	return kv
// }

// // Propose op that some RPC handler wants to replicate
// func (kv *KVServerRepl) startOp(opType int, key string, val int, opNum int64) bool {
// 	op := KVServerOp{OpNum: opNum, OpType: opType, Key: key, Val: val}
// 	result := kv.opManager.addOp(opNum)
// 	kv.proposeC <- encodeNoErr(op)
// 	return <-result
// }

// func (kv *KVServerRepl) Increment(key string, opNum int64) {
// 	kv.startOp(IncrementOp, key, 0, opNum)
// }

// func (kv *KVServerRepl) Wait(key string, untilValue int, opNum int64) {
// 	kv.startOp(WaitOp, key, untilValue, opNum)
// }

// func (kv *KVServerRepl) processApplied() {
// 	// ops that been executed to completion
// 	for appliedOp := range kv.appliedC {
// 		kv.opManager.reportOpFinished(appliedOp.op.OpNum, true)
// 	}
// }

// func (kv *KVServerRepl) apply(
// 	data []byte,
// 	access func(func() []any) []any,
// 	wait func(string),
// 	signal func(string),
// 	broadcast func(string),
// ) AppliedKVOp {
// 	var op KVServerOp
// 	decodeNoErr(data, &op)

// 	switch op.OpType {
// 	case IncrementOp:
// 		kv.kv[op.Key]++ // @put
// 		broadcast(op.Key)
// 	case WaitOp:
// 		val := kv.kv[op.Key] // @get int
// 		for val < op.Val {
// 			wait(op.Key)
// 			val = kv.kv[op.Key] // @get int
// 		}
// 	}

// 	return AppliedKVOp{op, struct{}{}}
// }

// func (kv *KVServerRepl) getSnapshot() ([]byte, error) {
// 	return json.Marshal(kv.kv)
// }

// func (kv *KVServerRepl) loadSnapshot(snapshot []byte) error {
// 	return json.Unmarshal(snapshot, &kv.kv)
// }
