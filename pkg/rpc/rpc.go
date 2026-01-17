// Package rpc implements the JSON-RPC 2.0 interface for X1-Stratus.
//
// Supported methods:
// - getHealth: Node health status
// - getSlot: Current slot
// - getVersion: Node version
// - getBalance: Account balance
// - getAccountInfo: Full account details
// - getBlock: Block by slot
// - getTransaction: Transaction by signature
// - sendTransaction: Forward transaction to validators
// - simulateTransaction: Simulate transaction execution
package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

var (
	// ErrNotImplemented is returned for unimplemented methods.
	ErrNotImplemented = errors.New("not implemented")
)

// Server is the JSON-RPC server.
type Server struct {
	// addr is the listen address.
	addr string

	// TODO: Add dependencies
	// accountsDB accounts.DB
	// blockstore blockstore.Store
}

// New creates a new RPC server.
func New(addr string) *Server {
	return &Server{
		addr: addr,
	}
}

// Start starts the RPC server.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRPC)

	server := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	return server.ListenAndServe()
}

// handleRPC handles incoming JSON-RPC requests.
func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      interface{}     `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, req.ID, -32700, "Parse error")
		return
	}

	result, err := s.dispatch(req.Method, req.Params)
	if err != nil {
		writeError(w, req.ID, -32601, err.Error())
		return
	}

	writeResult(w, req.ID, result)
}

// dispatch routes RPC methods to handlers.
func (s *Server) dispatch(method string, params json.RawMessage) (interface{}, error) {
	switch method {
	case "getHealth":
		return s.getHealth()
	case "getSlot":
		return s.getSlot()
	case "getVersion":
		return s.getVersion()
	default:
		return nil, ErrNotImplemented
	}
}

func (s *Server) getHealth() (string, error) {
	return "ok", nil
}

func (s *Server) getSlot() (uint64, error) {
	// TODO: Return actual slot from AccountsDB
	return 0, nil
}

func (s *Server) getVersion() (map[string]string, error) {
	return map[string]string{
		"solana-core": "stratus-dev",
		"feature-set": "0",
	}, nil
}

func writeResult(w http.ResponseWriter, id interface{}, result interface{}) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, id interface{}, code int, message string) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
