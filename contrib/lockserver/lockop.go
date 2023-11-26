package main

import (
	"encoding/json"
	"log"
)

type LockOp struct {
	ClientID ClientID
	OpNum    int64
	OpType   int
	LockName string
}

func (op LockOp) marshal() []byte {
	data, err := json.Marshal(op)
	if err != nil {
		log.Fatalf("Failed to marshal lock op: %v", err)
	}
	return data
}

func lockOpFromBytes(data []byte) *LockOp {
	var op *LockOp = new(LockOp)
	if err := json.Unmarshal(data, &op); err != nil {
		log.Fatalf("Failed to unmarshal lock op: %v", err)
	}
	return op
}
