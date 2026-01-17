// Package rpc implements the JSON-RPC 2.0 server for X1-Stratus.
//
// The server provides a Solana-compatible JSON-RPC API that allows clients
// to query account state, blocks, transactions, and cluster information.
//
// Supported methods:
//   - Account: getAccountInfo, getBalance, getMultipleAccounts, getProgramAccounts
//   - Block: getBlock, getBlockHeight, getBlockTime, getBlocks, getBlocksWithLimit
//   - Transaction: getTransaction, getSignaturesForAddress, getSignatureStatuses
//   - Cluster: getSlot, getSlotLeader, getHealth, getVersion, getGenesisHash
//   - Info: getEpochInfo, getEpochSchedule, getLatestBlockhash, getMinimumBalanceForRentExemption
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
	"github.com/fortiblox/X1-Stratus/pkg/blockstore"
)

// Config holds RPC server configuration.
type Config struct {
	// Addr is the listen address (host:port).
	Addr string

	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration

	// MaxRequestSize is the maximum allowed request body size in bytes.
	MaxRequestSize int64

	// EnableCORS enables CORS headers for browser access.
	EnableCORS bool

	// AllowedOrigins specifies allowed CORS origins (empty means all).
	AllowedOrigins []string

	// LogRequests enables request logging.
	LogRequests bool

	// Identity is the node identity pubkey.
	Identity types.Pubkey

	// GenesisHash is the genesis blockhash.
	GenesisHash types.Hash
}

// DefaultConfig returns a default RPC server configuration.
func DefaultConfig() Config {
	return Config{
		Addr:           ":8899",
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxRequestSize: 50 * 1024, // 50KB
		EnableCORS:     true,
		LogRequests:    false,
	}
}

// Server is the JSON-RPC 2.0 server.
type Server struct {
	config Config

	// Dependencies
	accountsDB accounts.DB
	blockstore blockstore.Store

	// State
	identity    types.Pubkey
	genesisHash types.Hash
	healthy     bool
	healthMu    sync.RWMutex

	// HTTP server
	server *http.Server

	// Method handlers
	handlers map[string]handlerFunc

	// Lifecycle
	mu      sync.RWMutex
	running bool
}

// handlerFunc is a JSON-RPC method handler.
type handlerFunc func(params json.RawMessage) (interface{}, *RPCError)

// New creates a new RPC server.
func New(config Config, accountsDB accounts.DB, blockstore blockstore.Store) *Server {
	s := &Server{
		config:      config,
		accountsDB:  accountsDB,
		blockstore:  blockstore,
		identity:    config.Identity,
		genesisHash: config.GenesisHash,
		healthy:     true,
		handlers:    make(map[string]handlerFunc),
	}

	// Register all method handlers
	s.registerHandlers()

	return s
}

// registerHandlers registers all RPC method handlers.
func (s *Server) registerHandlers() {
	// Account methods
	s.handlers["getAccountInfo"] = s.getAccountInfo
	s.handlers["getBalance"] = s.getBalance
	s.handlers["getMultipleAccounts"] = s.getMultipleAccounts
	s.handlers["getProgramAccounts"] = s.getProgramAccounts

	// Block methods
	s.handlers["getBlock"] = s.getBlock
	s.handlers["getBlockHeight"] = s.getBlockHeight
	s.handlers["getBlockTime"] = s.getBlockTime
	s.handlers["getBlocks"] = s.getBlocks
	s.handlers["getBlocksWithLimit"] = s.getBlocksWithLimit

	// Transaction methods
	s.handlers["getTransaction"] = s.getTransaction
	s.handlers["getSignaturesForAddress"] = s.getSignaturesForAddress
	s.handlers["getSignatureStatuses"] = s.getSignatureStatuses

	// Cluster methods
	s.handlers["getSlot"] = s.getSlot
	s.handlers["getSlotLeader"] = s.getSlotLeader
	s.handlers["getHealth"] = s.getHealth
	s.handlers["getVersion"] = s.getVersion
	s.handlers["getGenesisHash"] = s.getGenesisHash
	s.handlers["getIdentity"] = s.getIdentity

	// Info methods
	s.handlers["getEpochInfo"] = s.getEpochInfo
	s.handlers["getEpochSchedule"] = s.getEpochSchedule
	s.handlers["getLatestBlockhash"] = s.getLatestBlockhash
	s.handlers["getMinimumBalanceForRentExemption"] = s.getMinimumBalanceForRentExemption

	// Additional commonly used methods
	s.handlers["getRecentBlockhash"] = s.getRecentBlockhash
	s.handlers["getFirstAvailableBlock"] = s.getFirstAvailableBlock
	s.handlers["getBlockCommitment"] = s.getBlockCommitment
	s.handlers["getClusterNodes"] = s.getClusterNodes
	s.handlers["getRecentPerformanceSamples"] = s.getRecentPerformanceSamples
	s.handlers["isBlockhashValid"] = s.isBlockhashValid
}

// Start starts the RPC server.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRPC)

	s.server = &http.Server{
		Addr:         s.config.Addr,
		Handler:      s.corsMiddleware(mux),
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(shutdownCtx)
	}()

	if s.config.LogRequests {
		log.Printf("[RPC] Server starting on %s", s.config.Addr)
	}

	err := s.server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Stop stops the RPC server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// SetHealthy sets the server health status.
func (s *Server) SetHealthy(healthy bool) {
	s.healthMu.Lock()
	s.healthy = healthy
	s.healthMu.Unlock()
}

// IsHealthy returns the current health status.
func (s *Server) IsHealthy() bool {
	s.healthMu.RLock()
	defer s.healthMu.RUnlock()
	return s.healthy
}

// corsMiddleware adds CORS headers if enabled.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	if !s.config.EnableCORS {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			allowed := len(s.config.AllowedOrigins) == 0
			for _, allowedOrigin := range s.config.AllowedOrigins {
				if allowedOrigin == origin || allowedOrigin == "*" {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, solana-client")
				w.Header().Set("Access-Control-Max-Age", "3600")
			}
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleRPC handles incoming JSON-RPC requests.
func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	// Only accept POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && contentType != "application/json" {
		s.writeError(w, nil, ErrInvalidRequest)
		return
	}

	// Read request body with size limit
	body, err := io.ReadAll(io.LimitReader(r.Body, s.config.MaxRequestSize))
	if err != nil {
		s.writeError(w, nil, ErrParseError)
		return
	}

	// Check if this is a batch request
	if len(body) > 0 && body[0] == '[' {
		s.handleBatchRequest(w, body)
		return
	}

	// Parse single request
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeError(w, nil, ErrParseError)
		return
	}

	// Validate request
	if req.JSONRPC != JSONRPCVersion {
		s.writeError(w, req.ID, ErrInvalidRequest)
		return
	}

	// Log request if enabled
	if s.config.LogRequests {
		log.Printf("[RPC] %s id=%v", req.Method, req.ID)
	}

	// Dispatch to handler
	result, rpcErr := s.dispatch(req.Method, req.Params)
	if rpcErr != nil {
		s.writeError(w, req.ID, rpcErr)
		return
	}

	s.writeResult(w, req.ID, result)
}

// handleBatchRequest handles batch JSON-RPC requests.
func (s *Server) handleBatchRequest(w http.ResponseWriter, body []byte) {
	var requests []Request
	if err := json.Unmarshal(body, &requests); err != nil {
		s.writeError(w, nil, ErrParseError)
		return
	}

	if len(requests) == 0 {
		s.writeError(w, nil, ErrInvalidRequest)
		return
	}

	responses := make([]Response, len(requests))
	for i, req := range requests {
		if req.JSONRPC != JSONRPCVersion {
			responses[i] = Response{
				JSONRPC: JSONRPCVersion,
				ID:      req.ID,
				Error:   ErrInvalidRequest,
			}
			continue
		}

		result, rpcErr := s.dispatch(req.Method, req.Params)
		if rpcErr != nil {
			responses[i] = Response{
				JSONRPC: JSONRPCVersion,
				ID:      req.ID,
				Error:   rpcErr,
			}
		} else {
			responses[i] = Response{
				JSONRPC: JSONRPCVersion,
				ID:      req.ID,
				Result:  result,
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

// dispatch routes RPC methods to their handlers.
func (s *Server) dispatch(method string, params json.RawMessage) (interface{}, *RPCError) {
	handler, ok := s.handlers[method]
	if !ok {
		return nil, NewRPCError(MethodNotFound, fmt.Sprintf("Method not found: %s", method))
	}

	return handler(params)
}

// writeResult writes a successful response.
func (s *Server) writeResult(w http.ResponseWriter, id interface{}, result interface{}) {
	resp := Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// writeError writes an error response.
func (s *Server) writeError(w http.ResponseWriter, id interface{}, err *RPCError) {
	resp := Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   err,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Additional commonly used methods

// getRecentBlockhash returns the recent blockhash (deprecated, use getLatestBlockhash).
func (s *Server) getRecentBlockhash(params json.RawMessage) (interface{}, *RPCError) {
	latestSlot := s.blockstore.GetLatestSlot()
	block, err := s.blockstore.GetBlock(latestSlot)
	if err != nil {
		return nil, InternalServerErrorf("failed to get latest block: %v", err)
	}

	// Calculate fee calculator (simplified)
	feeCalculator := map[string]uint64{
		"lamportsPerSignature": 5000,
	}

	return ResponseWithContext{
		Context: Context{Slot: latestSlot},
		Value: map[string]interface{}{
			"blockhash":     block.Blockhash.String(),
			"feeCalculator": feeCalculator,
		},
	}, nil
}

// getFirstAvailableBlock returns the slot of the lowest confirmed block.
func (s *Server) getFirstAvailableBlock(params json.RawMessage) (interface{}, *RPCError) {
	return s.blockstore.GetOldestSlot(), nil
}

// getBlockCommitment returns commitment for a particular block.
func (s *Server) getBlockCommitment(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing slot parameter")
	}

	var slot uint64
	if err := json.Unmarshal(args[0], &slot); err != nil {
		return nil, InvalidParamsError("invalid slot")
	}

	// Check if block exists
	if !s.blockstore.HasBlock(slot) {
		return BlockCommitment{
			Commitment: nil,
			TotalStake: 0,
		}, nil
	}

	// Return placeholder commitment (we don't track actual stake votes)
	return BlockCommitment{
		Commitment: []uint64{0},
		TotalStake: 0,
	}, nil
}

// getClusterNodes returns information about all cluster nodes.
func (s *Server) getClusterNodes(params json.RawMessage) (interface{}, *RPCError) {
	// Return just this node as we're a verifier
	return []ClusterNode{
		{
			Pubkey:  s.identity.String(),
			Version: strPtr(SolanaCore),
		},
	}, nil
}

// getRecentPerformanceSamples returns recent performance samples.
func (s *Server) getRecentPerformanceSamples(params json.RawMessage) (interface{}, *RPCError) {
	// Parse limit
	limit := 720 // Default
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err == nil && len(args) > 0 {
		var l int
		if err := json.Unmarshal(args[0], &l); err == nil && l > 0 {
			limit = l
		}
	}

	if limit > 720 {
		limit = 720
	}

	// Return empty samples (we don't track performance metrics)
	return []interface{}{}, nil
}

// isBlockhashValid checks if a blockhash is valid and recent.
func (s *Server) isBlockhashValid(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing blockhash parameter")
	}

	var blockhashStr string
	if err := json.Unmarshal(args[0], &blockhashStr); err != nil {
		return nil, InvalidParamsError("invalid blockhash")
	}

	blockhash, err := types.HashFromBase58(blockhashStr)
	if err != nil {
		return nil, InvalidParamsError("invalid blockhash format")
	}

	currentSlot := s.blockstore.GetLatestSlot()

	// Check recent blocks for this blockhash
	// Blockhashes are valid for ~150 blocks
	const validRange = 150
	startSlot := currentSlot
	if startSlot > validRange {
		startSlot -= validRange
	} else {
		startSlot = 0
	}

	valid := false
	for slot := startSlot; slot <= currentSlot; slot++ {
		block, err := s.blockstore.GetBlock(slot)
		if err != nil {
			continue
		}
		if block.Blockhash == blockhash {
			valid = true
			break
		}
	}

	return ResponseWithContext{
		Context: Context{Slot: currentSlot},
		Value:   valid,
	}, nil
}

// Helper functions

func strPtr(s string) *string {
	return &s
}
