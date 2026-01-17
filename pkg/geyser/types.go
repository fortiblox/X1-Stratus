// Package geyser provides a client for consuming Solana Geyser data via gRPC.
//
// This package implements a Yellowstone/Dragon's Mouth compatible gRPC client
// for streaming real-time blockchain data from validators. It is designed for
// X1-Stratus verification nodes that need to receive confirmed blocks for replay.
//
// The client supports:
// - Block subscriptions with configurable filters
// - Slot status updates for consensus tracking
// - Automatic reconnection with exponential backoff
// - Multiple commitment levels (processed, confirmed, finalized)
package geyser

import (
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// CommitmentLevel represents the confirmation status for subscriptions.
// This mirrors the Solana/Yellowstone gRPC commitment levels.
type CommitmentLevel int32

const (
	// CommitmentProcessed indicates data is processed by the node but may rollback.
	// This is the fastest but least reliable commitment level.
	CommitmentProcessed CommitmentLevel = 0

	// CommitmentConfirmed indicates data has received 2/3+ stake votes.
	// This is the recommended level for verification nodes.
	CommitmentConfirmed CommitmentLevel = 1

	// CommitmentFinalized indicates data is permanent and irreversible.
	// Has additional latency compared to confirmed.
	CommitmentFinalized CommitmentLevel = 2
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

// SlotStatus represents the status of a slot in the cluster.
type SlotStatus int32

const (
	// SlotStatusProcessed indicates the slot was processed by this node.
	SlotStatusProcessed SlotStatus = 0

	// SlotStatusConfirmed indicates the slot received supermajority votes.
	SlotStatusConfirmed SlotStatus = 1

	// SlotStatusFinalized indicates the slot is permanently finalized.
	SlotStatusFinalized SlotStatus = 2
)

// String returns the string representation of the slot status.
func (s SlotStatus) String() string {
	switch s {
	case SlotStatusProcessed:
		return "processed"
	case SlotStatusConfirmed:
		return "confirmed"
	case SlotStatusFinalized:
		return "finalized"
	default:
		return "unknown"
	}
}

// Block represents a complete block received from Geyser.
// This is the primary data structure for block subscription updates.
type Block struct {
	// Slot is the slot number of this block.
	Slot uint64

	// ParentSlot is the parent slot number.
	ParentSlot uint64

	// Blockhash is the unique hash identifying this block.
	Blockhash types.Hash

	// ParentBlockhash links to the parent block.
	ParentBlockhash types.Hash

	// BlockTime is the Unix timestamp when the block was produced.
	// May be nil if not available.
	BlockTime *int64

	// BlockHeight is the block height (number of blocks since genesis).
	// May be nil for older blocks.
	BlockHeight *uint64

	// Transactions contains all transactions in this block.
	Transactions []Transaction

	// Entries contains the PoH entries for this block.
	// Only populated if IncludeEntries was requested.
	Entries []Entry

	// Rewards contains staking/voting rewards distributed in this block.
	Rewards []Reward

	// ExecutedTransactionCount is the number of successfully executed transactions.
	ExecutedTransactionCount uint64

	// ReceivedAt is when this block was received by the client.
	ReceivedAt time.Time
}

// Transaction represents a transaction within a Geyser block update.
type Transaction struct {
	// Signature is the primary signature (transaction ID).
	Signature types.Signature

	// Signatures contains all signatures on this transaction.
	Signatures []types.Signature

	// Message contains the transaction message.
	Message TransactionMessage

	// Meta contains execution metadata.
	Meta *TransactionMeta

	// Index is the position of this transaction in the block.
	Index uint64

	// IsVote indicates if this is a vote transaction.
	IsVote bool
}

// TransactionMessage represents the transaction message content.
type TransactionMessage struct {
	// Header describes signature and account requirements.
	Header MessageHeader

	// AccountKeys lists all static accounts referenced by this transaction.
	AccountKeys []types.Pubkey

	// RecentBlockhash is used for transaction deduplication and expiry.
	RecentBlockhash types.Hash

	// Instructions contains the program instructions.
	Instructions []CompiledInstruction

	// AddressTableLookups for versioned transactions (v0).
	AddressTableLookups []AddressTableLookup

	// IsLegacy indicates if this is a legacy (non-versioned) transaction.
	IsLegacy bool
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

// CompiledInstruction represents a compiled instruction in a transaction.
type CompiledInstruction struct {
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

	// WritableIndexes are indexes of writable accounts in the table.
	WritableIndexes []uint8

	// ReadonlyIndexes are indexes of readonly accounts in the table.
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

	// LoadedWritableAddresses are writable addresses loaded from lookup tables.
	LoadedWritableAddresses []types.Pubkey

	// LoadedReadonlyAddresses are readonly addresses loaded from lookup tables.
	LoadedReadonlyAddresses []types.Pubkey

	// ReturnData contains the return data from the transaction.
	ReturnData *ReturnData
}

// TransactionError represents a transaction execution error.
type TransactionError struct {
	// Code is the error code.
	Code int32

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
	Instructions []CompiledInstruction
}

// ReturnData contains the return data from a transaction.
type ReturnData struct {
	// ProgramID is the program that returned the data.
	ProgramID types.Pubkey

	// Data is the returned data bytes.
	Data []byte
}

// Entry represents a PoH entry in a block.
type Entry struct {
	// Slot is the containing slot.
	Slot uint64

	// Index is the entry index within the slot.
	Index uint64

	// NumHashes is the PoH tick count.
	NumHashes uint64

	// Hash is the entry hash.
	Hash types.Hash

	// ExecutedTransactionCount is the number of transactions in this entry.
	ExecutedTransactionCount uint64

	// StartingTransactionIndex is the index of the first transaction.
	StartingTransactionIndex uint64
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
type RewardType int32

const (
	// RewardTypeUnspecified is an unspecified reward type.
	RewardTypeUnspecified RewardType = 0

	// RewardTypeFee is for transaction fee rewards.
	RewardTypeFee RewardType = 1

	// RewardTypeRent is for rent rewards.
	RewardTypeRent RewardType = 2

	// RewardTypeStaking is for staking rewards.
	RewardTypeStaking RewardType = 3

	// RewardTypeVoting is for voting rewards.
	RewardTypeVoting RewardType = 4
)

// String returns the string representation of the reward type.
func (r RewardType) String() string {
	switch r {
	case RewardTypeFee:
		return "fee"
	case RewardTypeRent:
		return "rent"
	case RewardTypeStaking:
		return "staking"
	case RewardTypeVoting:
		return "voting"
	default:
		return "unspecified"
	}
}

// SlotUpdate represents a slot status update from Geyser.
type SlotUpdate struct {
	// Slot is the slot number.
	Slot uint64

	// ParentSlot is the parent slot (for fork tracking).
	ParentSlot *uint64

	// Status is the current slot status.
	Status SlotStatus

	// ReceivedAt is when this update was received.
	ReceivedAt time.Time
}

// AccountUpdate represents an account update from Geyser.
// Used when subscribing to account updates (not primary for block verification).
type AccountUpdate struct {
	// Pubkey is the account address.
	Pubkey types.Pubkey

	// Slot is the slot where this update occurred.
	Slot uint64

	// Lamports is the account balance.
	Lamports uint64

	// Owner is the program that owns the account.
	Owner types.Pubkey

	// Executable indicates if the account contains executable code.
	Executable bool

	// RentEpoch is the epoch at which rent is due.
	RentEpoch uint64

	// Data is the account data bytes.
	Data []byte

	// WriteVersion is a monotonic version for ordering updates.
	WriteVersion uint64

	// TxnSignature is the transaction that caused this update.
	TxnSignature *types.Signature

	// ReceivedAt is when this update was received.
	ReceivedAt time.Time
}

// SubscribeUpdate is the union type for all subscription updates.
type SubscribeUpdate struct {
	// Filters contains the filter names that matched this update.
	Filters []string

	// CreatedAt is when this update was created on the server.
	CreatedAt time.Time

	// Update contains the actual update data.
	// One of: Block, Slot, Transaction, Account, Entry, Ping, Pong
	Block       *Block
	Slot        *SlotUpdate
	Transaction *Transaction
	Account     *AccountUpdate
	Entry       *Entry
	Ping        *PingUpdate
	Pong        *PongUpdate
}

// PingUpdate represents a ping from the server.
type PingUpdate struct {
	// Timestamp is when the ping was sent.
	Timestamp time.Time
}

// PongUpdate represents a pong response from the server.
type PongUpdate struct {
	// ID is the ping ID being responded to.
	ID int32
}

// BlockFilter configures which blocks to receive.
type BlockFilter struct {
	// AccountInclude filters to blocks containing these accounts.
	AccountInclude []types.Pubkey

	// IncludeTransactions includes full transaction data.
	IncludeTransactions bool

	// IncludeAccounts includes account updates (not needed for replay).
	IncludeAccounts bool

	// IncludeEntries includes PoH entries for verification.
	IncludeEntries bool
}

// TransactionFilter configures which transactions to receive.
type TransactionFilter struct {
	// Vote includes vote transactions.
	Vote *bool

	// Failed includes failed transactions.
	Failed *bool

	// Signature filters to a specific transaction signature.
	Signature *types.Signature

	// AccountInclude filters to transactions involving these accounts.
	AccountInclude []types.Pubkey

	// AccountExclude excludes transactions involving these accounts.
	AccountExclude []types.Pubkey

	// AccountRequired requires transactions to involve ALL of these accounts.
	AccountRequired []types.Pubkey
}

// SlotFilter configures slot status updates.
type SlotFilter struct {
	// FilterByCommitment only emits updates at the specified commitment level.
	FilterByCommitment *CommitmentLevel
}

// AccountFilter configures which accounts to receive updates for.
type AccountFilter struct {
	// Account filters to specific account pubkeys.
	Account []types.Pubkey

	// Owner filters to accounts owned by these programs.
	Owner []types.Pubkey

	// DataSize filters to accounts with this exact data size.
	DataSize *uint64

	// TokenAccountState filters to valid token accounts.
	TokenAccountState bool

	// NonemptyTxnSignature only includes updates with transaction signatures.
	NonemptyTxnSignature bool
}

// ClientHealth represents the health status of the Geyser client.
type ClientHealth struct {
	// Connected indicates if the client is connected.
	Connected bool

	// LastSlot is the last slot received.
	LastSlot uint64

	// LastUpdate is when the last update was received.
	LastUpdate time.Time

	// Provider is the name/endpoint of the current provider.
	Provider string

	// Latency is the estimated latency to the provider.
	Latency time.Duration

	// ReconnectCount is the number of reconnections since start.
	ReconnectCount int

	// LastError is the last error encountered, if any.
	LastError error
}

// SubscribeRequest represents a subscription request to the Geyser service.
type SubscribeRequest struct {
	// Blocks configures block subscriptions. Key is filter name.
	Blocks map[string]*BlockFilter

	// Slots configures slot subscriptions. Key is filter name.
	Slots map[string]*SlotFilter

	// Transactions configures transaction subscriptions. Key is filter name.
	Transactions map[string]*TransactionFilter

	// Accounts configures account subscriptions. Key is filter name.
	Accounts map[string]*AccountFilter

	// Commitment is the commitment level for all subscriptions.
	Commitment CommitmentLevel

	// FromSlot is the starting slot for historical replay.
	FromSlot *uint64

	// Ping sends a ping request.
	Ping *PingRequest
}

// PingRequest is a keepalive ping request.
type PingRequest struct {
	// ID is a unique identifier for this ping.
	ID int32
}
