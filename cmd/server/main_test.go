package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	// Create a temporary directory for Raft data
	raftDir, err := os.MkdirTemp("", "raft-test")
	require.NoError(t, err)
	defer os.RemoveAll(raftDir)

	// Create a test server
	server, err := NewServer(raftDir, "test-node", "localhost:0")
	require.NoError(t, err)

	// Start the server in a goroutine
	go func() {
		err := server.Start()
		require.NoError(t, err)
	}()

	// Wait for the server to become leader
	time.Sleep(2 * time.Second)

	// Test setting a key
	req := httptest.NewRequest("PUT", "/key/test", nil)
	req.Body = http.NoBody
	w := httptest.NewRecorder()
	server.handleSet(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test getting a non-existent key
	req = httptest.NewRequest("GET", "/key/test", nil)
	w = httptest.NewRecorder()
	server.handleGet(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test joining a new node
	req = httptest.NewRequest("POST", "/join", nil)
	req.Body = http.NoBody
	w = httptest.NewRecorder()
	server.handleJoin(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFSM(t *testing.T) {
	fsm := NewKeyValueFSM()

	// Test applying a set command
	cmd := command{
		Op:    "set",
		Key:   "test",
		Value: "value",
	}
	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	log := &raft.Log{
		Data: data,
	}
	err = fsm.Apply(log)
	require.NoError(t, err)

	// Verify the value was set
	assert.Equal(t, "value", fsm.store["test"])

	// Test applying a delete command
	cmd = command{
		Op:  "delete",
		Key: "test",
	}
	data, err = json.Marshal(cmd)
	require.NoError(t, err)

	log = &raft.Log{
		Data: data,
	}
	err = fsm.Apply(log)
	require.NoError(t, err)

	// Verify the value was deleted
	_, exists := fsm.store["test"]
	assert.False(t, exists)
}

func TestSnapshot(t *testing.T) {
	fsm := NewKeyValueFSM()

	// Add some test data
	fsm.store["key1"] = "value1"
	fsm.store["key2"] = "value2"

	// Create a snapshot
	snapshot, err := fsm.Snapshot()
	require.NoError(t, err)

	// Create a pipe for the snapshot
	r, w := io.Pipe()
	go func() {
		defer w.Close()
		err := snapshot.Persist(raft.NewSnapshotSink(w))
		require.NoError(t, err)
	}()

	// Create a new FSM and restore from snapshot
	newFSM := NewKeyValueFSM()
	err = newFSM.Restore(r)
	require.NoError(t, err)

	// Verify the data was restored
	assert.Equal(t, "value1", newFSM.store["key1"])
	assert.Equal(t, "value2", newFSM.store["key2"])
}
