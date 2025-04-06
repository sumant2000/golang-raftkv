# RaftKV - Distributed Key-Value Store

RaftKV is a distributed key-value store that uses the Raft consensus algorithm for replication and consistency. It's built in Go and demonstrates the implementation of distributed systems concepts similar to dqlite.

## Features

- Distributed key-value storage with Raft consensus
- HTTP API for key-value operations
- Dynamic cluster membership
- Snapshot support for state recovery
- Leader election and failover

## Architecture

The system consists of multiple nodes that form a Raft cluster. Each node:
- Maintains a local key-value store
- Participates in Raft consensus
- Exposes an HTTP API for operations
- Can join or leave the cluster dynamically

## Getting Started

### Prerequisites

- Go 1.21 or later
- Make (optional, for using Makefile commands)

### Building

```bash
go build -o raftkv ./cmd/server
```

### Running a Node

To start a new node:

```bash
./raftkv --node-id node1 --raft-addr localhost:8000
```

To join an existing cluster:

```bash
curl -X POST http://localhost:8080/join \
  -H "Content-Type: application/json" \
  -d '{"node_id": "node2", "raft_address": "localhost:8001"}'
```

### API Usage

#### Set a Key-Value Pair

```bash
curl -X PUT http://localhost:8080/key/mykey \
  -H "Content-Type: application/json" \
  -d '{"value": "myvalue"}'
```

#### Get a Value

```bash
curl http://localhost:8080/key/mykey
```

## Testing

Run the test suite:

```bash
go test ./...
```

## Implementation Details

The project demonstrates several key concepts relevant to the dqlite position:

1. **Raft Consensus**: Implementation of the Raft consensus algorithm for distributed coordination
2. **State Machine**: Finite State Machine (FSM) implementation for key-value operations
3. **Network Transport**: TCP-based transport layer for Raft communication
4. **Persistence**: BoltDB-based storage for Raft logs and snapshots
5. **HTTP API**: RESTful interface for key-value operations

## Why This Project?

This project showcases skills relevant to the dqlite position:

- Deep understanding of distributed systems and consensus algorithms
- Experience with Go programming and C interop (through Raft implementation)
- Knowledge of asynchronous programming and concurrency patterns
- Implementation of reliable distributed storage systems
- Testing and reliability considerations

## License

MIT 