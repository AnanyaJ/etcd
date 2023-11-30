package main

import (
	"bytes"
	"compress/zlib"
	"encoding/gob"
	"io"
	"log"
)

func compress(data []byte) ([]byte, error) {
	var buffer bytes.Buffer
	w := zlib.NewWriter(&buffer)
	_, err := w.Write(data)
	w.Close()
	return buffer.Bytes(), err
}

func decompress(compressed []byte) ([]byte, error) {
	buffer := bytes.NewBuffer(compressed)
	r, err := zlib.NewReader(buffer)
	r.Close()
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

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

func encodeNoErr(x any) []byte {
	buf := new(bytes.Buffer)
	encoder := gob.NewEncoder(buf)
	if err := encoder.Encode(x); err != nil {
		log.Fatalf("Failed to perform gob encoding: %v", err)
	}
	return buf.Bytes()
}

func decodeNoErr(data []byte, x any) {
	decoder := gob.NewDecoder(bytes.NewBuffer(data))
	if err := decoder.Decode(x); err != nil {
		log.Fatalf("Failed to perform gob decoding: %v", err)
	}
}
