### Building lockserver

```sh
go build -o lockserver
```

### Running single node lockserver

First start a single-member cluster:

```sh
lockserver --id 1 --cluster http://127.0.0.1:12379 --port 12380
```

Each lockserver process maintains a single raft instance and a lock server.
The process's list of comma separated peers (--cluster), its raft ID index into the peer list (--id), and http lock server port (--port) are passed through the command line.

### Running a local cluster

First install [goreman](https://github.com/mattn/goreman), which manages Procfile-based applications.

The [Procfile script](./Procfile) will set up a local example cluster. Start it with:

```sh
goreman start
```

This will bring up three raftexample instances.

Now it's possible to send a lock operation request to any member of the cluster.

### Fault Tolerance

To test cluster recovery, first start a cluster:
```sh
goreman start
```

Next, remove a node and check cluster availability:

```sh
goreman run stop lockserver2
```

Finally, bring the node back up and verify it recovers:
```sh
goreman run start lockserver2
```

### Dynamic cluster reconfiguration

Nodes can be added to or removed from a running cluster using requests to the REST API.

For example, suppose we have a 3-node cluster that was started with the commands:
```sh
lockserver --id 1 --cluster http://127.0.0.1:12379,http://127.0.0.1:22379,http://127.0.0.1:32379 --port 12380
lockserver --id 2 --cluster http://127.0.0.1:12379,http://127.0.0.1:22379,http://127.0.0.1:32379 --port 22380
lockserver --id 3 --cluster http://127.0.0.1:12379,http://127.0.0.1:22379,http://127.0.0.1:32379 --port 32380
```

A fourth node with ID 4 can be added by issuing a POST:
```sh
curl -L http://127.0.0.1:12380/4 -XPOST -d http://127.0.0.1:42379
```

Then the new node can be started as the others were, using the --join option:
```sh
lockserver --id 4 --cluster http://127.0.0.1:12379,http://127.0.0.1:22379,http://127.0.0.1:32379,http://127.0.0.1:42379 --port 42380 --join
```

The new node should join the cluster and be able to service key/value requests.

We can remove a node using a DELETE request:
```sh
curl -L http://127.0.0.1:12380/3 -XDELETE
```

Node 3 should shut itself down once the cluster has processed this request.