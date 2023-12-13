package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
)

type Request struct {
	Lock     string
	ClientID int
	OpNum    int
}

func makeRequest(client *http.Client, url string, lockNum int, clientNum int, opNum int) {
	request := Request{Lock: fmt.Sprintf("lock%d", lockNum), ClientID: clientNum, OpNum: opNum}
	payload, err := json.Marshal(request)
	if err != nil {
		log.Fatalln("Failed to marshal request ", request)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(payload))
	if err != nil {
		log.Fatalln("Error creating request: ", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalln("Request failed ", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalln("Response has error status ", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln("Error reading response body:", err)
	}

	if string(body) != "true\n" {
		log.Fatalln("Expected response true, received ", string(body))
	}
}

func main() {
	servers := flag.String("servers", "http://127.0.0.1:12380", "comma separated server nodes")
	numLocks := flag.Int("numLocks", 5, "number of distinct locks that clients try to acquire")
	numClients := flag.Int("numClients", 100, "number of clients simultaneously competing for locks")
	flag.Parse()

	serverNodes := strings.Split(*servers, ",")
	numNodes := len(serverNodes)

	var wg sync.WaitGroup

	for i := 0; i < *numClients; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			client := &http.Client{}

			// pick random lock to acquire
			lock := rand.Intn(*numLocks)

			url := serverNodes[rand.Intn(numNodes)]
			acquireURL := fmt.Sprintf("%s/acquire", url)

			makeRequest(client, acquireURL, lock, i, 2*i)

			for i := 0; i < 100000; i++ { // do some computation with lock held
			}

			releaseURL := fmt.Sprintf("%s/release", url)
			makeRequest(client, releaseURL, lock, i, 2*i+1)
		}(i)
	}

	wg.Wait()
}
