.PHONY: build test clean

build:
	go build -o raftkv ./cmd/server

test:
	go test -v ./...

clean:
	rm -f raftkv
	go clean

run:
	./raftkv --node-id node1 --raft-addr localhost:8000

run-node2:
	./raftkv --node-id node2 --raft-addr localhost:8001

run-node3:
	./raftkv --node-id node3 --raft-addr localhost:8002

all: clean build test 