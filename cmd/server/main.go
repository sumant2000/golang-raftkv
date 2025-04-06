package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

type Server struct {
	raft *raft.Raft
	fsm  *KeyValueFSM
	addr string
}

type KeyValueFSM struct {
	store map[string]string
}

type command struct {
	Op    string `json:"op"`
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

func NewKeyValueFSM() *KeyValueFSM {
	return &KeyValueFSM{
		store: make(map[string]string),
	}
}

func (f *KeyValueFSM) Apply(log *raft.Log) interface{} {
	var cmd command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("failed to unmarshal command: %v", err)
	}

	switch cmd.Op {
	case "set":
		f.store[cmd.Key] = cmd.Value
		return nil
	case "delete":
		delete(f.store, cmd.Key)
		return nil
	default:
		return fmt.Errorf("unknown command op: %s", cmd.Op)
	}
}

func (f *KeyValueFSM) Snapshot() (raft.FSMSnapshot, error) {
	return &kvSnapshot{store: f.store}, nil
}

func (f *KeyValueFSM) Restore(rc io.ReadCloser) error {
	var store map[string]string
	if err := json.NewDecoder(rc).Decode(&store); err != nil {
		return err
	}
	f.store = store
	return nil
}

type kvSnapshot struct {
	store map[string]string
}

func (s *kvSnapshot) Persist(sink raft.SnapshotSink) error {
	err := json.NewEncoder(sink).Encode(s.store)
	if err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *kvSnapshot) Release() {}

func NewServer(raftDir, nodeID, raftAddr string) (*Server, error) {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)

	// Create the FSM
	fsm := NewKeyValueFSM()

	// Create the snapshot store
	snapshotStore, err := raft.NewFileSnapshotStore(raftDir, 1, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("file snapshot store: %s", err)
	}

	// Create the log store
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(raftDir, "raft.db"))
	if err != nil {
		return nil, fmt.Errorf("new bolt store: %s", err)
	}

	// Create the stable store
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(raftDir, "stable.db"))
	if err != nil {
		return nil, fmt.Errorf("new bolt store: %s", err)
	}

	// Create the transport
	transport, err := raft.NewTCPTransport(raftAddr, nil, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("new tcp transport: %s", err)
	}

	// Create the Raft instance
	r, err := raft.NewRaft(config, fsm, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, fmt.Errorf("new raft: %s", err)
	}

	return &Server{
		raft: r,
		fsm:  fsm,
		addr: raftAddr,
	}, nil
}

func (s *Server) Start() error {
	// Start the HTTP server
	r := mux.NewRouter()
	r.HandleFunc("/key/{key}", s.handleGet).Methods("GET")
	r.HandleFunc("/key/{key}", s.handleSet).Methods("PUT")
	r.HandleFunc("/join", s.handleJoin).Methods("POST")

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("HTTP server shutdown error: %v", err)
	}

	return nil
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	if s.raft.State() != raft.Leader {
		http.Error(w, "not leader", http.StatusServiceUnavailable)
		return
	}

	value, ok := s.fsm.store[key]
	if !ok {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"value": value})
}

func (s *Server) handleSet(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	if s.raft.State() != raft.Leader {
		http.Error(w, "not leader", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cmd := command{
		Op:    "set",
		Key:   key,
		Value: req.Value,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	f := s.raft.Apply(data, 5*time.Second)
	if err := f.Error(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID      string `json:"node_id"`
		RaftAddress string `json:"raft_address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.raft.State() != raft.Leader {
		http.Error(w, "not leader", http.StatusServiceUnavailable)
		return
	}

	configFuture := s.raft.AddVoter(
		raft.ServerID(req.NodeID),
		raft.ServerAddress(req.RaftAddress),
		0, 0)
	if err := configFuture.Error(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	raftDir := flag.String("raft-dir", "raft", "Raft data directory")
	nodeID := flag.String("node-id", "", "Node ID")
	raftAddr := flag.String("raft-addr", "", "Raft bind address")
	flag.Parse()

	if *nodeID == "" || *raftAddr == "" {
		log.Fatal("node-id and raft-addr are required")
	}

	server, err := NewServer(*raftDir, *nodeID, *raftAddr)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
