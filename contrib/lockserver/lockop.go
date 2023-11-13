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
	data, err := json.Marshal(&op)
	if err != nil {
		log.Fatal(err)
	}
	return data
}

func lockOpFromBytes(data []byte) LockOp {
	op := LockOp{}
	err := json.Unmarshal(data, &op)
	if err != nil {
		log.Fatalf("Failed to unmarshal applied op")
	}
	return op
}
