package main

import (
	"encoding/json"
	"log"
)

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
