package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"log"
)

func encode(x any) ([]byte, error) {
	buf := new(bytes.Buffer)
	encoder := gob.NewEncoder(buf)
	if err := encoder.Encode(x); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decode(data []byte, x any) error {
	decoder := gob.NewDecoder(bytes.NewBuffer(data))
	err := decoder.Decode(x)
	return err
}

func equal(first []byte, second []byte) bool {
	if len(first) != len(second) {
		return false
	}
	for i, x := range first {
		if x != second[i] {
			return false
		}
	}
	return true
}

func marshal(x any) []byte {
	ret, err := json.Marshal(x)
	if err != nil {
		log.Fatal(err)
	}
	return ret
}

func boolFromBytes(data []byte) bool {
	var ret bool
	err := json.Unmarshal(data, &ret)
	if err != nil {
		log.Fatalf("Failed to unmarshal result of applied op")
	}
	return ret
}

func fromBytes(data []byte, to any) {
	err := json.Unmarshal(data, to)
	if err != nil {
		log.Fatalf("Failed to unmarshal result of applied op")
	}
}
