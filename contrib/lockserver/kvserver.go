package main

type KVServer interface {
	Increment(key string, opNum int64)
	Wait(key string, untilValue int, opNum int64)

	getSnapshot() ([]byte, error)
}
