package main

import "encoding/json"

const (
	IncrementOp int = iota
	WaitOp
)

type KVServer struct {
	kv map // Propose op that some RPC handler wants to replicate
	// ops that been executed to completion
	// @put
	// @get int
	// @get int
	[string]int
	proposeC  chan []byte
	opManager *OpManager
	appliedC  <-chan AppliedOp
}

func newKVServer(proposeC chan []byte, appliedC <-chan AppliedOp) *KVServer {
	kv := &KVServer{kv: make(map[string]int), proposeC: proposeC, opManager: newOpManager(), appliedC: appliedC}
	go kv.processApplied()
	return kv
}

func (kv *KVServer) startOp(opType int, key string, val int, opNum int64) bool {
	op := KVServerOp{OpNum: opNum, OpType: opType, Key: key, Val: val}
	result := kv.opManager.addOp(opNum)
	kv.proposeC <- op.marshal()
	return <-result
}
func (kv *KVServer) Increment(key string, opNum int64) {
	kv.startOp(IncrementOp, key, 0, opNum)
}
func (kv *KVServer) Wait(key string, untilValue int, opNum int64) bool {
	return kv.startOp(WaitOp, key, untilValue, opNum)
}
func (kv *KVServer) processApplied() {
	for appliedOp := range kv.appliedC {
		op := kvStoreOpFromBytes(appliedOp.op)
		result := boolFromBytes(appliedOp.result)
		kv.opManager.reportOpFinished(op.OpNum, result)
	}
}
func (kv *KVServer) apply(data []byte, access func(func() []any) []any, wait func(string), signal func(string)) []byte {
	op := kvStoreOpFromBytes(data)
	switch op.OpType {
	case IncrementOp:
		access(func() []any {
			kv.kv[op.Key]++
			return []any{}
		})
		signal(op.Key)
	case WaitOp:
		val := access(func() []any {
			return []any{kv.kv[op.Key]}
		})[0].(int)
		for val != op.Val {
			wait(op.Key)
			val = access(func() []any {
				return []any{kv.kv[op.Key]}
			})[0].(int)
		}
	}
	return marshal(true)
}
func (kv *KVServer) getSnapshot() ([]byte, error) {
	return json.Marshal(kv.kv)
}
func (kv *KVServer) loadSnapshot(snapshot []byte) error {
	return json.Unmarshal(snapshot, &kv.kv)
}
