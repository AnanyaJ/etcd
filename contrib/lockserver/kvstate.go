package main

const (
	GetOp int = iota
	PutOp
)

type KVOp struct {
	opType int
	key    string
	val    bool
}

type KVState map[string]bool

func (kv KVState) access(op KVOp) bool {
	if op.opType == PutOp {
		kv[op.key] = op.val
	}
	return kv[op.key]
}
