package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func lockname(num int) string {
	return fmt.Sprintf("lock%d", num)
}

func benchmark(b *testing.B, srv *httptest.Server, cli *http.Client) {
	numLocks := 5
	numContending := 100

	var wg sync.WaitGroup

	for i := 0; i < numContending; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			// acquire some lock
			lock := rand.Intn(numLocks)
			req := makeRequest(b, srv.URL, "acquire", lockname(lock))
			resp := getResponse(b, cli, req)
			checkAcquire(b, resp)

			for i := 0; i < 100000; i++ { // do some computation with lock held
			}

			// release lock
			req = makeRequest(b, srv.URL, "release", lockname(lock))
			resp = getResponse(b, cli, req)
			checkRelease(b, resp, true)
		}(i)
	}

	wg.Wait()
}

func BenchmarkQueueLS(b *testing.B) {
	srv, cli, proposeC, confChangeC := queue_lockserver_setup()
	defer stop_server(srv, proposeC, confChangeC)

	benchmark(b, srv, cli)
}

func BenchmarkTranslatedLS(b *testing.B) {
	srv, cli, proposeC, confChangeC := translated_lockserver_setup()
	defer stop_server(srv, proposeC, confChangeC)

	benchmark(b, srv, cli)
}
