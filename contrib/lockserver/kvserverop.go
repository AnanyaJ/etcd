package main

import (
	"encoding/json"
	"log"
)

type KVServerOp struct {
	OpNum  int64
	OpType int
	Key    string
	Val    int
}

func (op KVServerOp) marshal() []byte {
	data, err := json.Marshal(&op)
	if err != nil {
		log.Fatal(err)
	}
	return data
}

func kvStoreOpFromBytes(data []byte) KVServerOp {
	op := KVServerOp{}
	err := json.Unmarshal(data, &op)
	if err != nil {
		log.Fatalf("Failed to unmarshal applied op")
	}
	return op
}
