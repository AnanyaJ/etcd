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
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"

	"go.etcd.io/raft/v3/raftpb"
)

// Handler for a http based key-value store backed by raft
type httpLSAPI struct {
	server      LockServer
	confChangeC chan<- raftpb.ConfChange
}

type Request struct {
	Lock  string
	OpNum int64
}

func (h *httpLSAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.RequestURI
	defer r.Body.Close()
	switch r.Method {
	case http.MethodPut:
		reqBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Failed to read on PUT (%v)\n", err)
			http.Error(w, "Failed on PUT", http.StatusBadRequest)
			return
		}

		var req Request
		var response string
		err = json.Unmarshal(reqBytes, &req)
		if err != nil {
			response = "request failed\n"
		} else {
			var ret bool
			if key == "/acquire" {
				h.server.Acquire(req.Lock, req.OpNum)
				ret = true
			} else if key == "/release" {
				ret = h.server.Release(req.Lock, req.OpNum)
			} else if key == "/islocked" {
				ret = h.server.IsLocked(req.Lock, req.OpNum)
			}

			if ret {
				response = "true\n"
			} else {
				response = "false\n"
			}
		}

		w.Write([]byte(string(response)))
	case http.MethodPost:
		url, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Failed to read on POST (%v)\n", err)
			http.Error(w, "Failed on POST", http.StatusBadRequest)
			return
		}

		nodeID, err := strconv.ParseUint(key[1:], 0, 64)
		if err != nil {
			log.Printf("Failed to convert ID for conf change (%v)\n", err)
			http.Error(w, "Failed on POST", http.StatusBadRequest)
			return
		}

		cc := raftpb.ConfChange{
			Type:    raftpb.ConfChangeAddNode,
			NodeID:  nodeID,
			Context: url,
		}
		h.confChangeC <- cc
		// Optimistic that raft will apply the conf change
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		nodeID, err := strconv.ParseUint(key[1:], 0, 64)
		if err != nil {
			log.Printf("Failed to convert ID for conf change (%v)\n", err)
			http.Error(w, "Failed on DELETE", http.StatusBadRequest)
			return
		}

		cc := raftpb.ConfChange{
			Type:   raftpb.ConfChangeRemoveNode,
			NodeID: nodeID,
		}
		h.confChangeC <- cc

		// Optimistic that raft will apply the conf change
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", http.MethodPut)
		w.Header().Add("Allow", http.MethodPost)
		w.Header().Add("Allow", http.MethodDelete)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// serveHTTPLSAPI starts a Lock server with a GET/PUT API and listens.
func serveHTTPLSAPI(server LockServer, port int, confChangeC chan<- raftpb.ConfChange, errorC <-chan error) {
	srv := http.Server{
		Addr: ":" + strconv.Itoa(port),
		Handler: &httpLSAPI{
			server:      server,
			confChangeC: confChangeC,
		},
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
		log.Printf("Listening on port %d...\n", port)
	}()

	// exit when raft goes down
	if err, ok := <-errorC; ok {
		log.Fatal(err)
	}
}
