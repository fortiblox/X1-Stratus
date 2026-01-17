// Package blockstore provides persistent storage for Solana blocks and transactions.
//
// This package implements a lightweight blockstore designed for verification nodes
// that only need to store confirmed/finalized blocks (not shreds). It provides:
// - Block storage with slot-based indexing
// - Transaction lookup by signature
// - Address-to-transaction indexing
// - Automatic pruning of old blocks
//
// The blockstore uses BoltDB for persistent storage, providing ACID guarantees
// and efficient read performance suitable for verification workloads.
package blockstore

import (
	"encoding/binary"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// CommitmentLevel represents the confirmation status of a block.
// Solana uses three levels: Processed, Confirmed, and Finalized.
type CommitmentLevel uint8

const (
	// CommitmentProcessed indicates the block was processed by a leader
	// but may not yet have cluster-wide votes. Least secure level.
	CommitmentProcessed CommitmentLevel = iota

	// CommitmentConfirmed indicates the block received a supermajority
	// vote (>=66% of stake). Strong assurance of finality.
	CommitmentConfirmed

	// CommitmentFinalized indicates the block is confirmed AND has 31+
	// confirmed blocks built on top of it. Effectively irreversible.
	CommitmentFinalized
)

// String returns the string representation of the commitment level.
func (c CommitmentLevel) String() string {
	switch c {
	case CommitmentProcessed:
		return "processed"
	case CommitmentConfirmed:
		return "confirmed"
	case CommitmentFinalized:
		return "finalized"
	default:
		return "unknown"
	}
}

// SlotMeta contains metadata about a slot in the blockstore.
// This is a simplified version of Solana's SlotMeta, optimized for
// verification nodes that don't process shreds.
type SlotMeta struct {
	// Slot is the slot number.
	Slot uint64

	// ParentSlot is the parent slot number.
	ParentSlot uint64

	// BlockTime is the Unix timestamp when the block was produced.
	// May be nil if the block time is not available.
	BlockTime *int64

	// BlockHeight is the number of blocks produced since genesis.
	// May be nil for older blocks.
	BlockHeight *uint64

	// Commitment is the current commitment level of this slot.
	Commitment CommitmentLevel

	// IsComplete indicates whether the block data is fully stored.
	IsComplete bool

	// TransactionCount is the number of transactions in this slot.
	TransactionCount uint64

	// Blockhash is the hash of this block.
	Blockhash types.Hash

	// PreviousBlockhash is the hash of the previous block.
	PreviousBlockhash types.Hash
}

// Block represents a complete block with all transactions.
// This is the primary data structure for block storage.
type Block struct {
	// Slot is the slot number.
	Slot uint64

	// ParentSlot is the parent slot number.
	ParentSlot uint64

	// Blockhash is the unique hash identifying this block.
	Blockhash types.Hash

	// PreviousBlockhash links to the parent block.
	PreviousBlockhash types.Hash

	// BlockTime is the Unix timestamp (optional).
	BlockTime *int64

	// BlockHeight is the block height (optional).
	BlockHeight *uint64

	// Transactions contains all transactions in this block.
	Transactions []Transaction

	// Rewards contains staking rewards distributed in this block (optional).
	Rewards []Reward
}

// Transaction represents a transaction within a block.
type Transaction struct {
	// Signature is the first signature, used as the transaction ID.
	Signature types.Signature

	// Signatures contains all signatures on this transaction.
	Signatures []types.Signature

	// Message is the serialized transaction message.
	Message TransactionMessage

	// Meta contains execution metadata (logs, consumed units, etc.).
	Meta *TransactionMeta

	// Slot is the slot this transaction was included in.
	Slot uint64
}

// TransactionMessage represents the transaction message content.
type TransactionMessage struct {
	// AccountKeys lists all accounts referenced by this transaction.
	AccountKeys []types.Pubkey

	// RecentBlockhash is used for transaction deduplication.
	RecentBlockhash types.Hash

	// Instructions contains the program instructions.
	Instructions []Instruction

	// Header describes signature and account requirements.
	Header MessageHeader

	// AddressTableLookups for versioned transactions (v0).
	AddressTableLookups []AddressTableLookup
}

// MessageHeader describes the account types in a transaction.
type MessageHeader struct {
	// NumRequiredSignatures is the number of signatures required.
	NumRequiredSignatures uint8

	// NumReadonlySignedAccounts is the number of readonly signer accounts.
	NumReadonlySignedAccounts uint8

	// NumReadonlyUnsignedAccounts is the number of readonly non-signer accounts.
	NumReadonlyUnsignedAccounts uint8
}

// Instruction represents a single instruction in a transaction.
type Instruction struct {
	// ProgramIDIndex is the index of the program account in AccountKeys.
	ProgramIDIndex uint8

	// AccountIndexes lists the account indexes this instruction uses.
	AccountIndexes []uint8

	// Data is the instruction data passed to the program.
	Data []byte
}

// AddressTableLookup represents a lookup into an address lookup table.
type AddressTableLookup struct {
	// AccountKey is the address table account.
	AccountKey types.Pubkey

	// WritableIndexes are indexes of writable accounts.
	WritableIndexes []uint8

	// ReadonlyIndexes are indexes of readonly accounts.
	ReadonlyIndexes []uint8
}

// TransactionMeta contains metadata about transaction execution.
type TransactionMeta struct {
	// Err contains the error if the transaction failed, nil on success.
	Err *TransactionError

	// Fee is the transaction fee in lamports.
	Fee uint64

	// PreBalances are account balances before execution.
	PreBalances []uint64

	// PostBalances are account balances after execution.
	PostBalances []uint64

	// PreTokenBalances are token balances before execution.
	PreTokenBalances []TokenBalance

	// PostTokenBalances are token balances after execution.
	PostTokenBalances []TokenBalance

	// InnerInstructions contains CPI instructions.
	InnerInstructions []InnerInstructions

	// LogMessages contains program log output.
	LogMessages []string

	// ComputeUnitsConsumed is the total compute units used.
	ComputeUnitsConsumed uint64

	// LoadedAddresses contains addresses loaded from lookup tables.
	LoadedAddresses *LoadedAddresses
}

// TransactionError represents a transaction execution error.
type TransactionError struct {
	// Code is the error code.
	Code int

	// Message is a human-readable error description.
	Message string
}

// TokenBalance represents a token account balance.
type TokenBalance struct {
	// AccountIndex is the index in the transaction's account list.
	AccountIndex uint8

	// Mint is the token mint address.
	Mint types.Pubkey

	// Owner is the token account owner.
	Owner types.Pubkey

	// UITokenAmount contains the balance in various formats.
	UITokenAmount UITokenAmount

	// ProgramID is the token program (Token or Token-2022).
	ProgramID types.Pubkey
}

// UITokenAmount represents a token amount with UI formatting.
type UITokenAmount struct {
	// Amount is the raw amount as a string.
	Amount string

	// Decimals is the token's decimal places.
	Decimals uint8

	// UIAmount is the human-readable amount (optional).
	UIAmount *float64

	// UIAmountString is the string representation.
	UIAmountString string
}

// InnerInstructions groups CPI instructions by their invoking instruction.
type InnerInstructions struct {
	// Index is the index of the instruction that invoked these.
	Index uint8

	// Instructions are the inner CPI instructions.
	Instructions []Instruction
}

// LoadedAddresses contains addresses loaded from address lookup tables.
type LoadedAddresses struct {
	// Writable addresses loaded from lookup tables.
	Writable []types.Pubkey

	// Readonly addresses loaded from lookup tables.
	Readonly []types.Pubkey
}

// Reward represents a staking/voting reward.
type Reward struct {
	// Pubkey is the account that received the reward.
	Pubkey types.Pubkey

	// Lamports is the reward amount (can be negative for penalties).
	Lamports int64

	// PostBalance is the account balance after the reward.
	PostBalance uint64

	// RewardType describes the type of reward.
	RewardType RewardType

	// Commission is the vote account commission when the reward was credited.
	Commission *uint8
}

// RewardType identifies the type of reward.
type RewardType uint8

const (
	// RewardTypeFee is for transaction fee rewards.
	RewardTypeFee RewardType = iota

	// RewardTypeRent is for rent rewards.
	RewardTypeRent

	// RewardTypeStaking is for staking rewards.
	RewardTypeStaking

	// RewardTypeVoting is for voting rewards.
	RewardTypeVoting
)

// TransactionStatus stores the execution status of a transaction.
// Used for the transaction status index.
type TransactionStatus struct {
	// Slot is the slot the transaction was processed in.
	Slot uint64

	// Signature is the transaction signature.
	Signature types.Signature

	// Err is the error if execution failed.
	Err *TransactionError

	// ConfirmationStatus is the commitment level.
	ConfirmationStatus CommitmentLevel

	// Confirmations is the number of confirmed blocks since this transaction.
	// Nil for finalized transactions.
	Confirmations *uint64
}

// SignatureInfo is stored in the address-to-signature index.
type SignatureInfo struct {
	// Signature is the transaction signature.
	Signature types.Signature

	// Slot is the slot containing this transaction.
	Slot uint64

	// Err is present if the transaction failed.
	Err *TransactionError

	// Memo is the transaction memo (if any).
	Memo *string

	// BlockTime is the block timestamp.
	BlockTime *int64
}

// SlotRange represents a range of slots for queries.
type SlotRange struct {
	// Start is the first slot (inclusive).
	Start uint64

	// End is the last slot (inclusive).
	End uint64
}

// Helper functions for key encoding.

// EncodeSlotKey encodes a slot number as a big-endian 8-byte key.
// Big-endian ensures proper lexicographic ordering.
func EncodeSlotKey(slot uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, slot)
	return key
}

// DecodeSlotKey decodes a slot number from a big-endian 8-byte key.
func DecodeSlotKey(key []byte) uint64 {
	if len(key) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(key)
}

// EncodeSignatureKey encodes a signature as a key (raw bytes).
func EncodeSignatureKey(sig types.Signature) []byte {
	return sig[:]
}

// EncodeAddressSlotKey encodes an address+slot composite key.
// Format: [32-byte address][8-byte slot big-endian]
func EncodeAddressSlotKey(addr types.Pubkey, slot uint64) []byte {
	key := make([]byte, 40) // 32 + 8
	copy(key[:32], addr[:])
	binary.BigEndian.PutUint64(key[32:], slot)
	return key
}

// DecodeAddressSlotKey decodes an address+slot composite key.
func DecodeAddressSlotKey(key []byte) (types.Pubkey, uint64) {
	var addr types.Pubkey
	if len(key) < 40 {
		return addr, 0
	}
	copy(addr[:], key[:32])
	slot := binary.BigEndian.Uint64(key[32:])
	return addr, slot
}

// DefaultPruneAge is the default age after which blocks are pruned.
// 2 epochs at ~2 days per epoch = ~4 days.
const DefaultPruneAge = 4 * 24 * time.Hour

// DefaultPruneSlots is approximately 4 days worth of slots.
// At 2.5 slots/second: 4 * 24 * 60 * 60 * 2.5 = 864,000 slots.
const DefaultPruneSlots uint64 = 864_000
