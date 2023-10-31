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

func marshal(b bool) []byte {
	ret, err := json.Marshal(b)
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
