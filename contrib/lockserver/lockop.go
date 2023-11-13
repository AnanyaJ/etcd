package main

type LockOp struct {
	ClientID ClientID
	OpNum    int64
	OpType   int
	LockName string
}

func (op LockOp) marshal() []byte {
	return encodeNoErr(op)
}

func lockOpFromBytes(data []byte) LockOp {
	var op LockOp
	decodeNoErr(data, &op)
	return op
}
