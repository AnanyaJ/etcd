package main

const (
	GetOp int = iota
	PutOp
)

type KVOp struct {
	OpType int
	Key    string
	Val    bool
}

type KVState map[string]bool

func (kv KVState) access(op KVOp) bool {
	if op.OpType == PutOp {
		kv[op.Key] = op.Val
	}
	return kv[op.Key]
}
