// Package node provides the main orchestrator for an X1-Stratus verification node.
//
// The Node ties together all components:
// - Geyser client for receiving blocks from validators
// - Blockstore for persistent block storage
// - AccountsDB for maintaining account state
// - Replayer for transaction execution and state verification
//
// The node manages the lifecycle of these components, handles synchronization,
// and provides APIs for monitoring sync progress and node health.
package node

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
	"github.com/fortiblox/X1-Stratus/pkg/blockstore"
	"github.com/fortiblox/X1-Stratus/pkg/geyser"
	"github.com/fortiblox/X1-Stratus/pkg/replayer"
	"github.com/fortiblox/X1-Stratus/pkg/rpc"
	"github.com/fortiblox/X1-Stratus/pkg/snapshot"
)

// Node errors.
var (
	ErrAlreadyRunning  = errors.New("node is already running")
	ErrNotRunning      = errors.New("node is not running")
	ErrShuttingDown    = errors.New("node is shutting down")
	ErrConfigInvalid   = errors.New("invalid node configuration")
	ErrInitFailed      = errors.New("node initialization failed")
	ErrSyncFailed      = errors.New("synchronization failed")
	ErrSlotGap         = errors.New("slot gap detected")
	ErrStorageCorrupt  = errors.New("storage corruption detected")
)

// Config holds node configuration.
type Config struct {
	// DataDir is the root directory for all node data.
	// Subdirectories will be created for blockstore and accounts.
	DataDir string

	// GeyserEndpoint is the gRPC endpoint for the Geyser service.
	GeyserEndpoint string

	// GeyserToken is the authentication token for Geyser.
	// Supports environment variable expansion with ${VAR_NAME}.
	GeyserToken string

	// GeyserUseTLS enables TLS for the Geyser connection.
	GeyserUseTLS bool

	// Commitment is the commitment level for block subscriptions.
	// Defaults to "confirmed".
	Commitment string

	// StartSlot is the slot to start syncing from.
	// If 0, starts from the network's current slot.
	StartSlot uint64

	// SnapshotPath is an optional path to load initial state from a snapshot.
	// Supports both X1-Stratus native (.x1snap) and Solana (.tar.zst) snapshot formats.
	SnapshotPath string

	// SolanaSnapshotDir is a directory containing Solana snapshots.
	// If set, the node will automatically find and load the latest snapshot.
	SolanaSnapshotDir string

	// VerifySnapshotHash enables hash verification when loading Solana snapshots.
	// Recommended but may add loading time.
	VerifySnapshotHash bool

	// OnSnapshotProgress is called during snapshot loading with progress updates.
	OnSnapshotProgress func(accountsLoaded uint64)

	// VerifyBankHash enables bank hash verification during replay.
	// Recommended for full verification but adds overhead.
	VerifyBankHash bool

	// SkipSignatureVerification skips ed25519 signature verification.
	// Safe when receiving from trusted validators.
	SkipSignatureVerification bool

	// PruneEnabled enables automatic pruning of old blocks.
	PruneEnabled bool

	// PruneRetainSlots is the number of slots to retain during pruning.
	// Defaults to ~4 days worth of slots.
	PruneRetainSlots uint64

	// MaxSlotLag is the maximum acceptable lag behind the network.
	// If exceeded, the node will attempt to catch up.
	MaxSlotLag uint64

	// RPC server configuration.
	// RPCEnabled enables the JSON-RPC server.
	RPCEnabled bool

	// RPCAddr is the listen address for the RPC server (default ":8899").
	RPCAddr string

	// RPCLogRequests enables logging of RPC requests.
	RPCLogRequests bool

	// Callbacks for monitoring.
	OnSlotProcessed func(slot uint64, bankHash types.Hash)
	OnSyncProgress  func(current, target uint64)
	OnError         func(err error)
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		DataDir:                   "./data",
		GeyserUseTLS:              true,
		Commitment:                "confirmed",
		VerifyBankHash:            true,
		SkipSignatureVerification: false,
		PruneEnabled:              true,
		PruneRetainSlots:          blockstore.DefaultPruneSlots,
		MaxSlotLag:                100,
		VerifySnapshotHash:        true,
		RPCEnabled:                false,
		RPCAddr:                   ":8899",
		RPCLogRequests:            false,
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.DataDir == "" {
		return fmt.Errorf("%w: data directory is required", ErrConfigInvalid)
	}
	if c.GeyserEndpoint == "" {
		return fmt.Errorf("%w: geyser endpoint is required", ErrConfigInvalid)
	}
	return nil
}

// Node represents a complete X1-Stratus verification node.
// It manages the lifecycle of all components and coordinates synchronization.
type Node struct {
	config Config

	// Core components
	geyser    *geyser.Client
	blocks    blockstore.Store
	accounts  accounts.DB
	replayer  *replayer.Replayer
	rpcServer *rpc.Server

	// State management
	mu           sync.RWMutex
	running      atomic.Bool
	shuttingDown atomic.Bool
	currentSlot  atomic.Uint64
	latestSlot   atomic.Uint64
	isSyncing    atomic.Bool
	startTime    time.Time
	lastError    error
	lastErrorMu  sync.RWMutex

	// Sync coordination
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	blockQueue chan *geyser.Block

	// Metrics
	blocksProcessed   atomic.Uint64
	txsProcessed      atomic.Uint64
	slotProcessTimeNs atomic.Int64
}

// New creates a new verification node with the given configuration.
// The node is not started until Start() is called.
func New(config *Config) (*Node, error) {
	if config == nil {
		config = &Config{}
	}

	// Apply defaults
	if config.DataDir == "" {
		config.DataDir = DefaultConfig().DataDir
	}
	if config.Commitment == "" {
		config.Commitment = DefaultConfig().Commitment
	}
	if config.PruneRetainSlots == 0 {
		config.PruneRetainSlots = DefaultConfig().PruneRetainSlots
	}
	if config.MaxSlotLag == 0 {
		config.MaxSlotLag = DefaultConfig().MaxSlotLag
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &Node{
		config:     *config,
		blockQueue: make(chan *geyser.Block, 100),
	}, nil
}

// Start initializes all components and begins synchronization.
// This method blocks until the context is cancelled or an error occurs.
func (n *Node) Start(ctx context.Context) error {
	if n.running.Load() {
		return ErrAlreadyRunning
	}

	// Set up cancellable context
	n.ctx, n.cancel = context.WithCancel(ctx)
	n.startTime = time.Now()
	n.running.Store(true)
	n.isSyncing.Store(true)

	// Initialize all components
	if err := n.initialize(); err != nil {
		n.running.Store(false)
		return fmt.Errorf("%w: %v", ErrInitFailed, err)
	}

	// Start the block processing loop
	n.wg.Add(1)
	go n.blockProcessingLoop()

	// Start the sync monitoring loop
	n.wg.Add(1)
	go n.syncMonitorLoop()

	// Connect to Geyser and start receiving blocks
	if err := n.geyser.Connect(n.ctx); err != nil {
		n.Stop()
		return fmt.Errorf("failed to connect to geyser: %w", err)
	}

	// Start the block ingestion loop
	n.wg.Add(1)
	go n.blockIngestionLoop()

	// Start RPC server if enabled
	if n.rpcServer != nil {
		n.wg.Add(1)
		go func() {
			defer n.wg.Done()
			if err := n.rpcServer.Start(n.ctx); err != nil {
				n.setLastError(fmt.Errorf("RPC server error: %w", err))
				if n.config.OnError != nil {
					n.config.OnError(err)
				}
			}
		}()
	}

	return nil
}

// initialize sets up all storage backends and components.
func (n *Node) initialize() error {
	// Create data directories
	if err := os.MkdirAll(n.config.DataDir, 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	// Initialize blockstore
	blockstorePath := filepath.Join(n.config.DataDir, "blockstore", "blockstore.db")
	blocksConfig := blockstore.Config{
		Path:          blockstorePath,
		PruneEnabled:  n.config.PruneEnabled,
		PruneInterval: 1 * time.Hour,
		RetainSlots:   n.config.PruneRetainSlots,
	}

	blocks, err := blockstore.Open(blocksConfig)
	if err != nil {
		return fmt.Errorf("open blockstore: %w", err)
	}
	n.blocks = blocks

	// Initialize accounts database
	accountsPath := filepath.Join(n.config.DataDir, "accounts")
	accountsConfig := accounts.DefaultBadgerDBConfig(accountsPath)
	accts, err := accounts.NewBadgerDB(accountsConfig)
	if err != nil {
		blocks.Close()
		return fmt.Errorf("open accounts database: %w", err)
	}
	n.accounts = accts

	// Load snapshot if provided
	if err := n.loadInitialSnapshot(accts); err != nil {
		n.closeStorage()
		return fmt.Errorf("load snapshot: %w", err)
	}

	// Initialize replayer
	replayerConfig := replayer.Config{
		VerifyBankHash:            n.config.VerifyBankHash,
		SkipSignatureVerification: n.config.SkipSignatureVerification,
		MaxParallelTransactions:   1, // Sequential for correctness
		OnSlotComplete:            n.onSlotComplete,
	}
	n.replayer = replayer.New(blocks, accts, replayerConfig)

	// Initialize replayer state from storage
	currentSlot := blocks.GetLatestSlot()
	n.currentSlot.Store(currentSlot)
	if currentSlot > 0 {
		// Get the blockhash and bankhash for the current slot
		block, err := blocks.GetBlock(currentSlot)
		if err == nil {
			n.replayer.Initialize(currentSlot, block.Blockhash, types.Hash{})
		}
	}

	// Initialize Geyser client
	commitment := parseCommitment(n.config.Commitment)
	geyserConfig := geyser.Config{
		Endpoint:            n.config.GeyserEndpoint,
		Token:               n.config.GeyserToken,
		UseTLS:              n.config.GeyserUseTLS,
		Commitment:          commitment,
		IncludeTransactions: true,
		IncludeEntries:      true,
		IncludeAccounts:     false,
		IncludeVotes:        true,
		IncludeFailed:       true,
		OnConnect:           n.onGeyserConnect,
		OnDisconnect:        n.onGeyserDisconnect,
		OnReconnect:         n.onGeyserReconnect,
	}

	// Set starting slot
	if n.config.StartSlot > 0 {
		geyserConfig.FromSlot = &n.config.StartSlot
	} else if currentSlot > 0 {
		// Resume from last processed slot
		nextSlot := currentSlot + 1
		geyserConfig.FromSlot = &nextSlot
	}

	geyserConfig = geyserConfig.WithDefaults()
	geyserClient, err := geyser.NewClient(geyserConfig)
	if err != nil {
		n.closeStorage()
		return fmt.Errorf("create geyser client: %w", err)
	}
	n.geyser = geyserClient

	// Initialize RPC server if enabled
	if n.config.RPCEnabled {
		rpcConfig := rpc.DefaultConfig()
		rpcConfig.Addr = n.config.RPCAddr
		rpcConfig.LogRequests = n.config.RPCLogRequests
		rpcConfig.EnableCORS = true

		n.rpcServer = rpc.New(rpcConfig, accts, blocks)
	}

	return nil
}

// loadInitialSnapshot loads initial state from a snapshot if configured.
func (n *Node) loadInitialSnapshot(accts accounts.DB) error {
	// Determine which snapshot to load
	var snapshotPath string
	var isSolanaSnapshot bool

	if n.config.SnapshotPath != "" {
		snapshotPath = n.config.SnapshotPath
		// Detect Solana snapshot format by extension
		isSolanaSnapshot = isSolanaSnapshotPath(snapshotPath)
	} else if n.config.SolanaSnapshotDir != "" {
		// Find the latest Solana snapshot in the directory
		latestSnapshot, err := snapshot.FindLatestSnapshot(n.config.SolanaSnapshotDir)
		if err != nil {
			if errors.Is(err, snapshot.ErrSnapshotNotFound) {
				// No snapshot found, continue without loading
				return nil
			}
			return fmt.Errorf("find snapshot: %w", err)
		}
		snapshotPath = latestSnapshot.Path
		isSolanaSnapshot = true
	}

	if snapshotPath == "" {
		return nil
	}

	if isSolanaSnapshot {
		// Load Solana snapshot format
		return n.loadSolanaSnapshot(snapshotPath, accts)
	}

	// Load native X1-Stratus snapshot format
	if snapDB, ok := accts.(accounts.SnapshotableDB); ok {
		return snapDB.LoadSnapshot(snapshotPath)
	}

	return fmt.Errorf("accounts DB does not support snapshot loading")
}

// loadSolanaSnapshot loads a Solana format snapshot (.tar.zst or .tar).
func (n *Node) loadSolanaSnapshot(path string, accts accounts.DB) error {
	// Verify hash if configured
	if n.config.VerifySnapshotHash {
		valid, err := snapshot.VerifySnapshotHash(path)
		if err != nil {
			// Non-fatal: we can still try to load even if verification fails
			// This might happen with some snapshot versions
		} else if !valid {
			return fmt.Errorf("%w: hash in filename does not match content", snapshot.ErrHashMismatch)
		}
	}

	// Use streaming loader for large snapshots
	loader, err := snapshot.NewStreamingLoader(path, accts)
	if err != nil {
		return fmt.Errorf("create snapshot loader: %w", err)
	}

	// Set progress callback if configured
	if n.config.OnSnapshotProgress != nil {
		loader.SetProgressCallback(n.config.OnSnapshotProgress)
	}

	// Load the snapshot
	result, err := loader.Load()
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

	// Initialize replayer state from snapshot
	if n.replayer != nil && result.Slot > 0 {
		n.replayer.Initialize(result.Slot, result.Blockhash, result.BankHash)
	}

	// Update current slot
	n.currentSlot.Store(result.Slot)

	return nil
}

// isSolanaSnapshotPath checks if a path looks like a Solana snapshot.
func isSolanaSnapshotPath(path string) bool {
	// Get the base filename
	base := filepath.Base(path)

	// Solana snapshots are .tar or .tar.zst files
	if len(base) >= 4 && base[len(base)-4:] == ".tar" {
		return true
	}
	if len(base) >= 8 && base[len(base)-8:] == ".tar.zst" {
		return true
	}
	// Check for Solana snapshot naming convention
	if len(base) >= 9 && base[:9] == "snapshot-" {
		return true
	}
	if len(base) >= 21 && base[:21] == "incremental-snapshot-" {
		return true
	}
	return false
}

// closeStorage closes all storage backends.
func (n *Node) closeStorage() {
	if n.accounts != nil {
		n.accounts.Close()
	}
	if n.blocks != nil {
		n.blocks.Close()
	}
}

// blockIngestionLoop receives blocks from Geyser and queues them for processing.
func (n *Node) blockIngestionLoop() {
	defer n.wg.Done()

	for {
		select {
		case <-n.ctx.Done():
			return
		case block, ok := <-n.geyser.Blocks():
			if !ok {
				return
			}
			// Update latest known slot
			if block.Slot > n.latestSlot.Load() {
				n.latestSlot.Store(block.Slot)
			}
			// Queue block for ordered processing
			select {
			case n.blockQueue <- block:
			case <-n.ctx.Done():
				return
			default:
				// Queue full, drop oldest and add new
				select {
				case <-n.blockQueue:
				default:
				}
				n.blockQueue <- block
			}
		}
	}
}

// blockProcessingLoop processes blocks in order.
func (n *Node) blockProcessingLoop() {
	defer n.wg.Done()

	// Buffer for out-of-order blocks
	pendingBlocks := make(map[uint64]*geyser.Block)
	expectedSlot := n.currentSlot.Load() + 1
	if expectedSlot == 1 && n.config.StartSlot > 0 {
		expectedSlot = n.config.StartSlot
	}

	for {
		select {
		case <-n.ctx.Done():
			return
		case block, ok := <-n.blockQueue:
			if !ok {
				return
			}

			// Store in pending buffer
			pendingBlocks[block.Slot] = block

			// Process any blocks we can in order
			for {
				nextBlock, exists := pendingBlocks[expectedSlot]
				if !exists {
					break
				}
				delete(pendingBlocks, expectedSlot)

				if err := n.processBlock(nextBlock); err != nil {
					n.setLastError(fmt.Errorf("process slot %d: %w", expectedSlot, err))
					if n.config.OnError != nil {
						n.config.OnError(err)
					}
					// Skip this slot and continue
				}

				expectedSlot++
			}

			// Clean up old pending blocks (slot gaps)
			for slot := range pendingBlocks {
				if slot < expectedSlot-1000 { // Keep ~1000 slots buffer
					delete(pendingBlocks, slot)
				}
			}
		}
	}
}

// processBlock stores and replays a single block.
func (n *Node) processBlock(geyserBlock *geyser.Block) error {
	startTime := time.Now()

	// Convert geyser block to blockstore block
	block := convertGeyserBlock(geyserBlock)

	// Store in blockstore
	if err := n.blocks.PutBlock(block); err != nil {
		return fmt.Errorf("store block: %w", err)
	}

	// Replay the slot
	result, err := n.replayer.ReplaySlot(block.Slot)
	if err != nil {
		return fmt.Errorf("replay slot: %w", err)
	}

	// Update metrics
	n.blocksProcessed.Add(1)
	n.txsProcessed.Add(uint64(len(result.Transactions)))
	n.slotProcessTimeNs.Store(time.Since(startTime).Nanoseconds())
	n.currentSlot.Store(block.Slot)

	// Update sync state
	if block.Slot >= n.latestSlot.Load()-n.config.MaxSlotLag {
		n.isSyncing.Store(false)
	}

	return nil
}

// convertGeyserBlock converts a geyser.Block to a blockstore.Block.
func convertGeyserBlock(gb *geyser.Block) *blockstore.Block {
	block := &blockstore.Block{
		Slot:              gb.Slot,
		ParentSlot:        gb.ParentSlot,
		Blockhash:         gb.Blockhash,
		PreviousBlockhash: gb.ParentBlockhash,
		BlockTime:         gb.BlockTime,
		BlockHeight:       gb.BlockHeight,
	}

	// Convert transactions
	block.Transactions = make([]blockstore.Transaction, len(gb.Transactions))
	for i, tx := range gb.Transactions {
		block.Transactions[i] = convertGeyserTransaction(&tx)
	}

	// Convert rewards
	block.Rewards = make([]blockstore.Reward, len(gb.Rewards))
	for i, r := range gb.Rewards {
		block.Rewards[i] = blockstore.Reward{
			Pubkey:      r.Pubkey,
			Lamports:    r.Lamports,
			PostBalance: r.PostBalance,
			RewardType:  blockstore.RewardType(r.RewardType),
			Commission:  r.Commission,
		}
	}

	return block
}

// convertGeyserTransaction converts a geyser.Transaction to a blockstore.Transaction.
func convertGeyserTransaction(gt *geyser.Transaction) blockstore.Transaction {
	tx := blockstore.Transaction{
		Signature:  gt.Signature,
		Signatures: gt.Signatures,
		Slot:       0, // Will be set by blockstore
	}

	// Convert message
	tx.Message = blockstore.TransactionMessage{
		AccountKeys:     gt.Message.AccountKeys,
		RecentBlockhash: gt.Message.RecentBlockhash,
		Header: blockstore.MessageHeader{
			NumRequiredSignatures:       gt.Message.Header.NumRequiredSignatures,
			NumReadonlySignedAccounts:   gt.Message.Header.NumReadonlySignedAccounts,
			NumReadonlyUnsignedAccounts: gt.Message.Header.NumReadonlyUnsignedAccounts,
		},
	}

	// Convert instructions
	tx.Message.Instructions = make([]blockstore.Instruction, len(gt.Message.Instructions))
	for i, ix := range gt.Message.Instructions {
		tx.Message.Instructions[i] = blockstore.Instruction{
			ProgramIDIndex: ix.ProgramIDIndex,
			AccountIndexes: ix.AccountIndexes,
			Data:           ix.Data,
		}
	}

	// Convert address table lookups
	tx.Message.AddressTableLookups = make([]blockstore.AddressTableLookup, len(gt.Message.AddressTableLookups))
	for i, atl := range gt.Message.AddressTableLookups {
		tx.Message.AddressTableLookups[i] = blockstore.AddressTableLookup{
			AccountKey:      atl.AccountKey,
			WritableIndexes: atl.WritableIndexes,
			ReadonlyIndexes: atl.ReadonlyIndexes,
		}
	}

	// Convert meta
	if gt.Meta != nil {
		tx.Meta = &blockstore.TransactionMeta{
			Fee:                  gt.Meta.Fee,
			PreBalances:          gt.Meta.PreBalances,
			PostBalances:         gt.Meta.PostBalances,
			LogMessages:          gt.Meta.LogMessages,
			ComputeUnitsConsumed: gt.Meta.ComputeUnitsConsumed,
		}

		if gt.Meta.Err != nil {
			tx.Meta.Err = &blockstore.TransactionError{
				Code:    int(gt.Meta.Err.Code),
				Message: gt.Meta.Err.Message,
			}
		}

		// Convert inner instructions
		tx.Meta.InnerInstructions = make([]blockstore.InnerInstructions, len(gt.Meta.InnerInstructions))
		for i, inner := range gt.Meta.InnerInstructions {
			tx.Meta.InnerInstructions[i] = blockstore.InnerInstructions{
				Index: inner.Index,
			}
			tx.Meta.InnerInstructions[i].Instructions = make([]blockstore.Instruction, len(inner.Instructions))
			for j, ix := range inner.Instructions {
				tx.Meta.InnerInstructions[i].Instructions[j] = blockstore.Instruction{
					ProgramIDIndex: ix.ProgramIDIndex,
					AccountIndexes: ix.AccountIndexes,
					Data:           ix.Data,
				}
			}
		}

		// Convert token balances
		tx.Meta.PreTokenBalances = make([]blockstore.TokenBalance, len(gt.Meta.PreTokenBalances))
		for i, tb := range gt.Meta.PreTokenBalances {
			tx.Meta.PreTokenBalances[i] = convertGeyserTokenBalance(&tb)
		}
		tx.Meta.PostTokenBalances = make([]blockstore.TokenBalance, len(gt.Meta.PostTokenBalances))
		for i, tb := range gt.Meta.PostTokenBalances {
			tx.Meta.PostTokenBalances[i] = convertGeyserTokenBalance(&tb)
		}

		// Convert loaded addresses
		if len(gt.Meta.LoadedWritableAddresses) > 0 || len(gt.Meta.LoadedReadonlyAddresses) > 0 {
			tx.Meta.LoadedAddresses = &blockstore.LoadedAddresses{
				Writable: gt.Meta.LoadedWritableAddresses,
				Readonly: gt.Meta.LoadedReadonlyAddresses,
			}
		}
	}

	return tx
}

// convertGeyserTokenBalance converts a geyser.TokenBalance to a blockstore.TokenBalance.
func convertGeyserTokenBalance(gtb *geyser.TokenBalance) blockstore.TokenBalance {
	return blockstore.TokenBalance{
		AccountIndex: gtb.AccountIndex,
		Mint:         gtb.Mint,
		Owner:        gtb.Owner,
		ProgramID:    gtb.ProgramID,
		UITokenAmount: blockstore.UITokenAmount{
			Amount:         gtb.UITokenAmount.Amount,
			Decimals:       gtb.UITokenAmount.Decimals,
			UIAmount:       gtb.UITokenAmount.UIAmount,
			UIAmountString: gtb.UITokenAmount.UIAmountString,
		},
	}
}

// syncMonitorLoop monitors sync progress and reports status.
func (n *Node) syncMonitorLoop() {
	defer n.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			current := n.currentSlot.Load()
			latest := n.latestSlot.Load()

			// Check if we're caught up
			if latest > 0 && current >= latest-n.config.MaxSlotLag {
				n.isSyncing.Store(false)
			} else if latest > 0 && current < latest-n.config.MaxSlotLag*2 {
				n.isSyncing.Store(true)
			}

			// Report progress
			if n.config.OnSyncProgress != nil && latest > 0 {
				n.config.OnSyncProgress(current, latest)
			}
		}
	}
}

// onSlotComplete is called when a slot is successfully replayed.
func (n *Node) onSlotComplete(slot uint64, bankHash types.Hash) {
	if n.config.OnSlotProcessed != nil {
		n.config.OnSlotProcessed(slot, bankHash)
	}
}

// onGeyserConnect is called when Geyser connection is established.
func (n *Node) onGeyserConnect() {
	n.setLastError(nil)
}

// onGeyserDisconnect is called when Geyser connection is lost.
func (n *Node) onGeyserDisconnect(err error) {
	if err != nil {
		n.setLastError(err)
		if n.config.OnError != nil {
			n.config.OnError(err)
		}
	}
}

// onGeyserReconnect is called when Geyser reconnection succeeds.
func (n *Node) onGeyserReconnect(attempt int) {
	n.setLastError(nil)
}

// Stop gracefully stops the node.
func (n *Node) Stop() error {
	if !n.running.Load() {
		return ErrNotRunning
	}

	n.shuttingDown.Store(true)
	defer n.shuttingDown.Store(false)

	// Cancel context to stop all goroutines
	if n.cancel != nil {
		n.cancel()
	}

	// Wait for goroutines to finish
	n.wg.Wait()

	// Stop RPC server
	if n.rpcServer != nil {
		n.rpcServer.Stop()
	}

	// Close Geyser client
	if n.geyser != nil {
		n.geyser.Close()
	}

	// Commit any pending changes
	if n.accounts != nil {
		n.accounts.Commit()
	}
	if n.blocks != nil {
		n.blocks.Sync()
	}

	// Close storage
	n.closeStorage()

	n.running.Store(false)
	return nil
}

// Status returns the current sync status.
func (n *Node) Status() *Status {
	health := ClientHealth{}
	if n.geyser != nil {
		geyserHealth := n.geyser.Health()
		health = ClientHealth{
			Connected:      geyserHealth.Connected,
			LastSlot:       geyserHealth.LastSlot,
			LastUpdate:     geyserHealth.LastUpdate,
			Latency:        geyserHealth.Latency,
			ReconnectCount: geyserHealth.ReconnectCount,
		}
	}

	var accountsCount uint64
	if n.accounts != nil {
		accountsCount, _ = n.accounts.AccountsCount()
	}

	var blockstoreStats *blockstore.Stats
	if n.blocks != nil {
		blockstoreStats, _ = n.blocks.GetStats()
	}

	var rpcAddr string
	if n.rpcServer != nil {
		rpcAddr = n.config.RPCAddr
	}

	return &Status{
		CurrentSlot:      n.currentSlot.Load(),
		LatestSlot:       n.latestSlot.Load(),
		AccountsCount:    accountsCount,
		IsSyncing:        n.isSyncing.Load(),
		IsRunning:        n.running.Load(),
		Uptime:           time.Since(n.startTime),
		BlocksProcessed:  n.blocksProcessed.Load(),
		TxsProcessed:     n.txsProcessed.Load(),
		AvgSlotTimeMs:    float64(n.slotProcessTimeNs.Load()) / float64(time.Millisecond),
		GeyserHealth:     health,
		BlockstoreStats:  blockstoreStats,
		RPCAddr:          rpcAddr,
		LastError:        n.getLastError(),
	}
}

// Status contains the current node status.
type Status struct {
	// CurrentSlot is the most recently processed slot.
	CurrentSlot uint64

	// LatestSlot is the latest slot seen from the network.
	LatestSlot uint64

	// AccountsCount is the total number of accounts in the database.
	AccountsCount uint64

	// IsSyncing indicates if the node is currently catching up.
	IsSyncing bool

	// IsRunning indicates if the node is running.
	IsRunning bool

	// Uptime is how long the node has been running.
	Uptime time.Duration

	// BlocksProcessed is the total number of blocks processed.
	BlocksProcessed uint64

	// TxsProcessed is the total number of transactions processed.
	TxsProcessed uint64

	// AvgSlotTimeMs is the average slot processing time in milliseconds.
	AvgSlotTimeMs float64

	// GeyserHealth contains Geyser client health information.
	GeyserHealth ClientHealth

	// BlockstoreStats contains blockstore statistics.
	BlockstoreStats *blockstore.Stats

	// RPCAddr is the RPC server address if enabled.
	RPCAddr string

	// LastError is the most recent error encountered.
	LastError error
}

// ClientHealth contains Geyser client health information.
type ClientHealth struct {
	Connected      bool
	LastSlot       uint64
	LastUpdate     time.Time
	Latency        time.Duration
	ReconnectCount int
}

// GetBlock retrieves a block by slot from the blockstore.
func (n *Node) GetBlock(slot uint64) (*blockstore.Block, error) {
	if n.blocks == nil {
		return nil, ErrNotRunning
	}
	return n.blocks.GetBlock(slot)
}

// GetAccount retrieves an account by pubkey from the accounts database.
func (n *Node) GetAccount(pubkey types.Pubkey) (*accounts.Account, error) {
	if n.accounts == nil {
		return nil, ErrNotRunning
	}
	return n.accounts.GetAccount(pubkey)
}

// GetTransaction retrieves a transaction by signature.
func (n *Node) GetTransaction(sig types.Signature) (*blockstore.Transaction, error) {
	if n.blocks == nil {
		return nil, ErrNotRunning
	}
	return n.blocks.GetTransaction(sig)
}

// setLastError safely sets the last error.
func (n *Node) setLastError(err error) {
	n.lastErrorMu.Lock()
	n.lastError = err
	n.lastErrorMu.Unlock()
}

// getLastError safely gets the last error.
func (n *Node) getLastError() error {
	n.lastErrorMu.RLock()
	defer n.lastErrorMu.RUnlock()
	return n.lastError
}

// parseCommitment parses a commitment string to a CommitmentLevel.
func parseCommitment(s string) geyser.CommitmentLevel {
	switch s {
	case "processed":
		return geyser.CommitmentProcessed
	case "finalized":
		return geyser.CommitmentFinalized
	default:
		return geyser.CommitmentConfirmed
	}
}
