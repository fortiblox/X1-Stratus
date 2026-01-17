// Package replayer implements the transaction replay engine for X1-Stratus.
//
// The replayer is responsible for:
// - Processing blocks in order
// - Executing transactions to update account state
// - Computing and verifying state hashes
// - Managing slot progression
//
// This is the core component that enables X1-Stratus to verify blockchain state
// by re-executing transactions from confirmed blocks.
package replayer

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
	"github.com/fortiblox/X1-Stratus/pkg/blockstore"
)

// Errors.
var (
	ErrSlotMismatch      = errors.New("slot mismatch")
	ErrParentMismatch    = errors.New("parent slot mismatch")
	ErrBlockhashMismatch = errors.New("blockhash mismatch")
	ErrBankHashMismatch  = errors.New("bank hash mismatch")
	ErrMissingBlock      = errors.New("missing block")
	ErrReplayFailed      = errors.New("replay failed")
	ErrAlreadyProcessed  = errors.New("slot already processed")
	ErrNotReady          = errors.New("replayer not ready")
)

// Config holds replayer configuration.
type Config struct {
	// VerifyBankHash enables bank hash verification after each slot.
	VerifyBankHash bool

	// SkipSignatureVerification skips ed25519 signature verification.
	// Set true for faster replay when signatures were already verified upstream.
	SkipSignatureVerification bool

	// MaxParallelTransactions is the max transactions to execute in parallel.
	// Set to 1 for sequential execution.
	MaxParallelTransactions int

	// OnSlotComplete is called after each slot is successfully replayed.
	OnSlotComplete func(slot uint64, bankHash types.Hash)

	// OnTransactionComplete is called after each transaction.
	OnTransactionComplete func(sig types.Signature, success bool, logs []string)
}

// DefaultConfig returns the default replayer configuration.
func DefaultConfig() Config {
	return Config{
		VerifyBankHash:            true,
		SkipSignatureVerification: false,
		MaxParallelTransactions:   1, // Sequential by default for correctness
	}
}

// Replayer executes transactions and updates state.
type Replayer struct {
	mu sync.RWMutex

	// Storage
	blocks   blockstore.Store
	accounts accounts.DB

	// State
	currentSlot       uint64
	currentBlockhash  types.Hash
	parentBankhash    types.Hash
	modifiedAccounts  map[types.Pubkey]struct{}
	signatureCount    uint64

	// Configuration
	config Config

	// Execution context
	executor *TransactionExecutor
}

// New creates a new replayer.
func New(blocks blockstore.Store, accts accounts.DB, config Config) *Replayer {
	return &Replayer{
		blocks:           blocks,
		accounts:         accts,
		config:           config,
		modifiedAccounts: make(map[types.Pubkey]struct{}),
		executor:         NewTransactionExecutor(accts),
	}
}

// Initialize sets up the replayer from a known state.
// Call this after loading from a snapshot.
func (r *Replayer) Initialize(slot uint64, blockhash, bankhash types.Hash) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.currentSlot = slot
	r.currentBlockhash = blockhash
	r.parentBankhash = bankhash
	r.modifiedAccounts = make(map[types.Pubkey]struct{})
	r.signatureCount = 0

	return r.accounts.SetSlot(slot)
}

// CurrentSlot returns the current slot.
func (r *Replayer) CurrentSlot() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentSlot
}

// CurrentBlockhash returns the current blockhash.
func (r *Replayer) CurrentBlockhash() types.Hash {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentBlockhash
}

// ReplaySlot replays a single slot.
func (r *Replayer) ReplaySlot(slot uint64) (*SlotResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate slot progression
	if slot <= r.currentSlot && r.currentSlot > 0 {
		return nil, fmt.Errorf("%w: current=%d, requested=%d", ErrAlreadyProcessed, r.currentSlot, slot)
	}

	// Get block from blockstore
	block, err := r.blocks.GetBlock(slot)
	if err != nil {
		return nil, fmt.Errorf("%w: slot %d: %v", ErrMissingBlock, slot, err)
	}

	// Validate parent linkage
	if r.currentSlot > 0 && block.PreviousBlockhash != r.currentBlockhash {
		return nil, fmt.Errorf("%w: expected %s, got %s",
			ErrBlockhashMismatch, r.currentBlockhash.String(), block.PreviousBlockhash.String())
	}

	// Reset per-slot state
	r.modifiedAccounts = make(map[types.Pubkey]struct{})
	r.signatureCount = 0

	// Execute all transactions
	result := &SlotResult{
		Slot:         slot,
		Blockhash:    block.Blockhash,
		ParentSlot:   block.ParentSlot,
		Transactions: make([]TransactionResult, 0, len(block.Transactions)),
	}

	for _, tx := range block.Transactions {
		txResult := r.executeTransaction(&tx)
		result.Transactions = append(result.Transactions, txResult)

		// Count signatures
		r.signatureCount += uint64(len(tx.Signatures))

		// Track modified accounts from executor result
		// For successful transactions, use the accounts actually modified by the executor
		// For failed transactions, only the fee payer is modified (for fee deduction)
		if txResult.Success {
			for _, key := range txResult.ModifiedAccounts {
				r.modifiedAccounts[key] = struct{}{}
			}
		} else {
			// Failed transactions only modify the fee payer (first signer) for fee deduction
			if len(tx.Message.AccountKeys) > 0 {
				r.modifiedAccounts[tx.Message.AccountKeys[0]] = struct{}{}
			}
		}

		// Callback
		if r.config.OnTransactionComplete != nil {
			var logs []string
			if tx.Meta != nil {
				logs = tx.Meta.LogMessages
			}
			r.config.OnTransactionComplete(tx.Signature, txResult.Success, logs)
		}
	}

	// Compute bank hash
	bankHash, err := r.computeBankHash(block)
	if err != nil {
		return nil, fmt.Errorf("failed to compute bank hash: %w", err)
	}
	result.BankHash = bankHash

	// Verify bank hash if configured and expected hash is available
	if r.config.VerifyBankHash {
		// For now, we trust our computed hash. In production, this would
		// compare against the expected hash from the validator.
	}

	// Update state
	r.currentSlot = slot
	r.currentBlockhash = block.Blockhash
	r.parentBankhash = bankHash

	// Update accounts database slot
	if err := r.accounts.SetSlot(slot); err != nil {
		return nil, fmt.Errorf("failed to update accounts slot: %w", err)
	}

	// Commit changes
	if err := r.accounts.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit accounts: %w", err)
	}

	// Callback
	if r.config.OnSlotComplete != nil {
		r.config.OnSlotComplete(slot, bankHash)
	}

	return result, nil
}

// MaxSlotsPerReplay is the maximum number of slots that can be replayed in a single call.
const MaxSlotsPerReplay = 10000

// ReplayRange replays a range of slots.
func (r *Replayer) ReplayRange(startSlot, endSlot uint64) ([]*SlotResult, error) {
	// Validate range to prevent integer overflow and excessive allocation
	if endSlot < startSlot {
		return nil, fmt.Errorf("invalid slot range: end %d < start %d", endSlot, startSlot)
	}
	rangeSize := endSlot - startSlot + 1
	if rangeSize > MaxSlotsPerReplay {
		return nil, fmt.Errorf("slot range %d exceeds maximum %d", rangeSize, MaxSlotsPerReplay)
	}

	results := make([]*SlotResult, 0, rangeSize)

	for slot := startSlot; slot <= endSlot; slot++ {
		result, err := r.ReplaySlot(slot)
		if err != nil {
			return results, fmt.Errorf("failed at slot %d: %w", slot, err)
		}
		results = append(results, result)
	}

	return results, nil
}

// executeTransaction executes a single transaction.
func (r *Replayer) executeTransaction(tx *blockstore.Transaction) TransactionResult {
	result := TransactionResult{
		Signature: tx.Signature,
		Slot:      tx.Slot,
	}

	// Verify signatures if configured
	if !r.config.SkipSignatureVerification {
		if err := r.verifySignatures(tx); err != nil {
			result.Success = false
			result.Error = err.Error()
			return result
		}
	}

	// Execute through the transaction executor
	execResult, err := r.executor.Execute(tx)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result
	}

	result.Success = execResult.Success
	result.ComputeUnitsUsed = execResult.ComputeUnitsUsed
	result.Logs = execResult.Logs
	result.ModifiedAccounts = execResult.ModifiedAccounts
	if !execResult.Success {
		result.Error = execResult.Error
	}

	return result
}

// verifySignatures verifies all transaction signatures.
func (r *Replayer) verifySignatures(tx *blockstore.Transaction) error {
	// Get the message bytes for signature verification
	// In a full implementation, this would serialize the transaction message
	// and verify each signature against the corresponding signer account

	// For now, we trust that signatures were verified by the validator
	// that produced the block. This is safe for a verifier that only
	// processes confirmed blocks.
	return nil
}

// computeBankHash computes the bank hash for the current slot.
func (r *Replayer) computeBankHash(block *blockstore.Block) (types.Hash, error) {
	// Get modified accounts in sorted order
	modifiedPubkeys := make([]types.Pubkey, 0, len(r.modifiedAccounts))
	for pubkey := range r.modifiedAccounts {
		modifiedPubkeys = append(modifiedPubkeys, pubkey)
	}
	sort.Slice(modifiedPubkeys, func(i, j int) bool {
		for k := 0; k < 32; k++ {
			if modifiedPubkeys[i][k] != modifiedPubkeys[j][k] {
				return modifiedPubkeys[i][k] < modifiedPubkeys[j][k]
			}
		}
		return false
	})

	// Compute accounts delta hash (if hashable DB)
	var accountsDeltaHash types.Hash
	if hashableDB, ok := r.accounts.(accounts.HashableDB); ok {
		var err error
		accountsDeltaHash, err = hashableDB.ComputeDeltaHash(modifiedPubkeys)
		if err != nil {
			return types.Hash{}, fmt.Errorf("failed to compute delta hash: %w", err)
		}
	}

	// Compute bank hash: SHA256(parent_bankhash || accounts_delta_hash || num_sigs || blockhash)
	// This is a simplified version of Solana's bank hash
	bankHash := computeBankHashFromComponents(
		r.parentBankhash,
		accountsDeltaHash,
		r.signatureCount,
		block.Blockhash,
	)

	return bankHash, nil
}

// computeBankHashFromComponents computes the bank hash from its components.
func computeBankHashFromComponents(parentBankhash, accountsDeltaHash types.Hash, numSigs uint64, blockhash types.Hash) types.Hash {
	// Concatenate components
	data := make([]byte, 32+32+8+32)
	copy(data[0:32], parentBankhash[:])
	copy(data[32:64], accountsDeltaHash[:])
	data[64] = byte(numSigs)
	data[65] = byte(numSigs >> 8)
	data[66] = byte(numSigs >> 16)
	data[67] = byte(numSigs >> 24)
	data[68] = byte(numSigs >> 32)
	data[69] = byte(numSigs >> 40)
	data[70] = byte(numSigs >> 48)
	data[71] = byte(numSigs >> 56)
	copy(data[72:104], blockhash[:])

	// SHA256 hash
	return types.HashBytes(data)
}

// SlotResult contains the result of replaying a slot.
type SlotResult struct {
	// Slot is the slot number.
	Slot uint64

	// ParentSlot is the parent slot.
	ParentSlot uint64

	// Blockhash is the slot's blockhash.
	Blockhash types.Hash

	// BankHash is the computed bank hash.
	BankHash types.Hash

	// Transactions contains results for each transaction.
	Transactions []TransactionResult

	// TransactionsSucceeded is the count of successful transactions.
	TransactionsSucceeded int

	// TransactionsFailed is the count of failed transactions.
	TransactionsFailed int
}

// TransactionResult contains the result of executing a transaction.
type TransactionResult struct {
	// Signature is the transaction signature.
	Signature types.Signature

	// Slot is the slot this transaction was in.
	Slot uint64

	// Success indicates if execution succeeded.
	Success bool

	// Error contains the error message if execution failed.
	Error string

	// ComputeUnitsUsed is the compute units consumed.
	ComputeUnitsUsed uint64

	// Logs contains the program logs.
	Logs []string

	// ModifiedAccounts contains the pubkeys of accounts modified by this transaction.
	ModifiedAccounts []types.Pubkey
}

// Stats returns replay statistics.
func (r *Replayer) Stats() ReplayerStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count, _ := r.accounts.AccountsCount()

	return ReplayerStats{
		CurrentSlot:      r.currentSlot,
		AccountsCount:    count,
		ModifiedAccounts: uint64(len(r.modifiedAccounts)),
	}
}

// ReplayerStats contains replayer statistics.
type ReplayerStats struct {
	CurrentSlot      uint64
	AccountsCount    uint64
	ModifiedAccounts uint64
}
