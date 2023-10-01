//  keyyright 2015 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a  keyy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"io"
	"log"
	"net/http"
	"strconv"
)

// Handler for a http based key-value store backed by raft
type httpLSAPI struct {
	server LockServer
}

func (h *httpLSAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.RequestURI
	defer r.Body.Close()
	switch r.Method {
	case http.MethodPut:
		lock, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Failed to read on PUT (%v)\n", err)
			http.Error(w, "Failed on PUT", http.StatusBadRequest)
			return
		}

		lockName := string(lock)

		var success bool
		if key == "/acquire" {
			success = h.server.Acquire(lockName)
		} else if key == "/release" {
			success = h.server.Release(lockName)
		} else if key == "/islocked" {
			success = h.server.IsLocked(lockName)
		}

		var response string
		if success {
			response = "true\n"
		} else {
			response = "false\n"
		}
		w.Write([]byte(string(response)))

	default:
		w.Header().Set("Allow", http.MethodPut)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// serveHTTPLSAPI starts a lock server with a GET/PUT API and listens.
func serveHTTPLSAPI(server LockServer, port int, stop <-chan struct{}) {
	srv := http.Server{
		Addr: ":" + strconv.Itoa(port),
		Handler: &httpLSAPI{
			server: server,
		},
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
		log.Printf("Listening on port %d...\n", port)
	}()

	<-stop
}
