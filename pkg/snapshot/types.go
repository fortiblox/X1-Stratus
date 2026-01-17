// Package snapshot provides functionality for loading Solana/X1 snapshots.
//
// Solana validators periodically create snapshots containing the full account state
// at a specific slot. This package enables X1-Stratus to load these snapshots and
// start verifying from that point, rather than replaying from genesis.
//
// # Snapshot Format
//
// Solana snapshots are tar archives (optionally compressed with zstd) containing:
//
//	snapshot-SLOT-HASH/
//	├── version
//	├── status_cache
//	├── snapshots/SLOT/SLOT
//	│   ├── bank_fields (bincode serialized)
//	│   └── account_paths.txt
//	└── accounts/
//	    └── SLOT.OFFSET (append-vec files)
//
// The append-vec files contain accounts stored sequentially with the following format
// for each account:
//   - StoredMeta: write_version (u64), data_len (u64), pubkey (32 bytes)
//   - AccountMeta: lamports (u64), rent_epoch (u64), owner (32 bytes), executable (bool)
//   - hash (32 bytes)
//   - data (variable length, padded to 8-byte alignment)
package snapshot

import (
	"errors"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// Errors returned by the snapshot package.
var (
	// ErrInvalidSnapshot indicates the snapshot file is malformed.
	ErrInvalidSnapshot = errors.New("invalid snapshot")

	// ErrUnsupportedVersion indicates the snapshot version is not supported.
	ErrUnsupportedVersion = errors.New("unsupported snapshot version")

	// ErrCorruptedData indicates data corruption was detected.
	ErrCorruptedData = errors.New("corrupted snapshot data")

	// ErrMissingManifest indicates the bank fields manifest is missing.
	ErrMissingManifest = errors.New("missing snapshot manifest")

	// ErrMissingAccountFile indicates an expected account file is missing.
	ErrMissingAccountFile = errors.New("missing account file")

	// ErrHashMismatch indicates the computed hash doesn't match expected.
	ErrHashMismatch = errors.New("snapshot hash mismatch")

	// ErrSnapshotNotFound indicates no snapshot was found at the path.
	ErrSnapshotNotFound = errors.New("snapshot not found")

	// ErrDecompressionFailed indicates zstd decompression failed.
	ErrDecompressionFailed = errors.New("decompression failed")
)

// SnapshotInfo contains metadata about a discovered snapshot.
type SnapshotInfo struct {
	// Path is the full path to the snapshot file.
	Path string

	// Slot is the slot at which the snapshot was taken.
	Slot uint64

	// Hash is the truncated hash from the filename.
	Hash string

	// IsCompressed indicates if the snapshot is zstd compressed.
	IsCompressed bool

	// Size is the file size in bytes.
	Size int64
}

// SnapshotResult contains the result of loading a snapshot.
type SnapshotResult struct {
	// Slot is the slot at which the snapshot was taken.
	Slot uint64

	// ParentSlot is the parent slot.
	ParentSlot uint64

	// BlockHeight is the block height at the snapshot slot.
	BlockHeight uint64

	// Blockhash is the blockhash at the snapshot slot.
	Blockhash types.Hash

	// BankHash is the bank hash at the snapshot slot.
	BankHash types.Hash

	// Epoch is the epoch at the snapshot slot.
	Epoch uint64

	// AccountsCount is the total number of accounts loaded.
	AccountsCount uint64

	// Capitalization is the total lamports in all accounts.
	Capitalization uint64

	// AccountsHash is the computed accounts hash.
	AccountsHash types.Hash
}

// BankFields contains the deserialized bank state from the snapshot.
// This is a subset of Solana's BankFields struct, containing only
// the fields needed for X1-Stratus verification.
type BankFields struct {
	// Slot is the slot number.
	Slot uint64

	// ParentSlot is the parent slot number.
	ParentSlot uint64

	// BlockHeight is the block height.
	BlockHeight uint64

	// Blockhash is the blockhash at this slot.
	Blockhash types.Hash

	// ParentBlockhash is the parent's blockhash.
	ParentBlockhash types.Hash

	// Epoch is the current epoch.
	Epoch uint64

	// CollectorID is the leader's pubkey.
	CollectorID types.Pubkey

	// CollectorFees are the fees collected.
	CollectorFees uint64

	// FeeCalculator contains fee parameters.
	FeeCalculator FeeCalculator

	// FeeRateGovernor contains fee rate governance parameters.
	FeeRateGovernor FeeRateGovernor

	// RentCollector contains rent collection parameters.
	RentCollector RentCollector

	// Capitalization is the total lamports.
	Capitalization uint64

	// TransactionCount is the total transaction count.
	TransactionCount uint64

	// SignatureCount is the total signature count.
	SignatureCount uint64

	// MaxTickHeight is the maximum tick height.
	MaxTickHeight uint64

	// HashesPerTick is the number of hashes per tick.
	HashesPerTick *uint64

	// TicksPerSlot is the number of ticks per slot.
	TicksPerSlot uint64

	// NsPerSlot is the target nanoseconds per slot.
	NsPerSlot uint128

	// GenesisCreationTime is the genesis creation timestamp.
	GenesisCreationTime int64

	// SlotsPerYear is the number of slots per year.
	SlotsPerYear float64

	// AccountsDataLen is the total account data size.
	AccountsDataLen uint64

	// Inflation contains inflation parameters.
	Inflation Inflation

	// EpochSchedule contains epoch schedule parameters.
	EpochSchedule EpochSchedule

	// AccountsHash is the accounts hash.
	AccountsHash types.Hash

	// EpochAccountsHash is the epoch accounts hash (optional).
	EpochAccountsHash *types.Hash
}

// uint128 represents a 128-bit unsigned integer.
type uint128 struct {
	Lo uint64
	Hi uint64
}

// FeeCalculator contains fee calculation parameters.
type FeeCalculator struct {
	// LamportsPerSignature is the fee per signature.
	LamportsPerSignature uint64
}

// FeeRateGovernor contains fee rate governance parameters.
type FeeRateGovernor struct {
	// LamportsPerSignature is the current fee.
	LamportsPerSignature uint64

	// TargetLamportsPerSignature is the target fee.
	TargetLamportsPerSignature uint64

	// TargetSignaturesPerSlot is the target signatures per slot.
	TargetSignaturesPerSlot uint64

	// MinLamportsPerSignature is the minimum fee.
	MinLamportsPerSignature uint64

	// MaxLamportsPerSignature is the maximum fee.
	MaxLamportsPerSignature uint64

	// BurnPercent is the fee burn percentage.
	BurnPercent uint8
}

// RentCollector contains rent collection parameters.
type RentCollector struct {
	// Epoch is the current rent epoch.
	Epoch uint64

	// EpochSchedule contains epoch schedule info for rent.
	EpochSchedule EpochSchedule

	// SlotsPerYear is used for rent calculation.
	SlotsPerYear float64

	// Rent contains rent parameters.
	Rent Rent
}

// Rent contains rent configuration.
type Rent struct {
	// LamportsPerByteYear is the rent rate.
	LamportsPerByteYear uint64

	// ExemptionThreshold is the multiplier for rent exemption.
	ExemptionThreshold float64

	// BurnPercent is the rent burn percentage.
	BurnPercent uint8
}

// Inflation contains inflation parameters.
type Inflation struct {
	// Initial is the initial inflation rate.
	Initial float64

	// Terminal is the terminal inflation rate.
	Terminal float64

	// Taper is the rate at which inflation tapers.
	Taper float64

	// Foundation is the foundation portion.
	Foundation float64

	// FoundationTerm is when foundation portion starts.
	FoundationTerm float64
}

// EpochSchedule contains epoch schedule parameters.
type EpochSchedule struct {
	// SlotsPerEpoch is the number of slots per epoch.
	SlotsPerEpoch uint64

	// LeaderScheduleSlotOffset is the leader schedule offset.
	LeaderScheduleSlotOffset uint64

	// Warmup indicates if in warmup period.
	Warmup bool

	// FirstNormalEpoch is the first normal epoch.
	FirstNormalEpoch uint64

	// FirstNormalSlot is the first normal slot.
	FirstNormalSlot uint64
}

// StoredAccountMeta contains metadata for a stored account in an append-vec.
type StoredAccountMeta struct {
	// WriteVersion is the write version when account was stored.
	WriteVersion uint64

	// Pubkey is the account's public key.
	Pubkey types.Pubkey

	// Lamports is the account balance.
	Lamports uint64

	// RentEpoch is the epoch for rent collection.
	RentEpoch uint64

	// Owner is the owning program.
	Owner types.Pubkey

	// Executable indicates if the account is executable.
	Executable bool

	// Hash is the account hash.
	Hash types.Hash

	// Data is the account data.
	Data []byte
}

// AppendVecInfo contains metadata about an append-vec file.
type AppendVecInfo struct {
	// Slot is the slot this append-vec belongs to.
	Slot uint64

	// Offset is the file offset identifier.
	Offset uint64

	// Path is the path to the append-vec file.
	Path string

	// Size is the file size in bytes.
	Size int64
}

// AccountStorageEntry represents an entry in the account storage.
type AccountStorageEntry struct {
	// Slot is the slot.
	Slot uint64

	// ID is the storage ID.
	ID uint64

	// Count is the number of accounts.
	Count uint64

	// File is the filename.
	File string
}
