// Package accounts implements the AccountsDB for storing blockchain state.
//
// AccountsDB stores all account data for the X1 blockchain, including:
// - User accounts (balances, data)
// - Program accounts (bytecode)
// - System accounts (vote, stake, etc.)
//
// The implementation is inspired by Solana's AccountsDB architecture but simplified
// for a minimal verification node. Key differences from Solana's implementation:
// - Uses BadgerDB instead of append-vecs for storage
// - Stores only current state, not full history
// - Simpler index structure (BadgerDB handles indexing)
// - Single-threaded accounts hash computation (sufficient for verifier)
//
// # Solana AccountsDB Background
//
// Solana's AccountsDB uses an append-vec storage format where accounts are stored
// sequentially in memory-mapped files. Each account entry contains:
// - StoredMeta: write version, data length, pubkey
// - AccountMeta: lamports, rent_epoch, owner, executable
// - Hash: account hash
// - Data: variable-length account data
//
// The account index maps Pubkey -> Vec<(slot, file_id, offset)> to track where
// each account's data is stored across slots.
//
// For state verification, Solana computes:
// - Account Hash: SHA256 of account fields (lamports, rent_epoch, data, executable, owner, pubkey)
// - Accounts Delta Hash: Merkle root of accounts modified in a block
// - Bank Hash: Hash of parent_bankhash + accounts_delta_hash + num_sigs + blockhash
//
// This package implements a simplified version suitable for verification nodes.
package accounts

import (
	"encoding/binary"
	"errors"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

var (
	// ErrNotImplemented is returned for unimplemented features.
	ErrNotImplemented = errors.New("not implemented")

	// ErrAccountNotFound is returned when an account doesn't exist.
	ErrAccountNotFound = errors.New("account not found")

	// ErrCorrupted is returned when data corruption is detected.
	ErrCorrupted = errors.New("data corrupted")

	// ErrClosed is returned when operating on a closed database.
	ErrClosed = errors.New("database closed")

	// ErrInvalidData is returned when account data is malformed.
	ErrInvalidData = errors.New("invalid account data")

	// ErrSnapshotNotFound is returned when a snapshot doesn't exist.
	ErrSnapshotNotFound = errors.New("snapshot not found")
)

// Account represents a single account in the state.
// This matches Solana's account structure.
type Account struct {
	// Lamports is the account balance in lamports (1 SOL = 1e9 lamports).
	Lamports uint64

	// Data is the account data. For program accounts, this contains bytecode.
	// Maximum size is 10MB (10 * 1024 * 1024 bytes).
	Data []byte

	// Owner is the program that owns this account.
	// Only the owner program can modify the account data.
	Owner types.Pubkey

	// Executable indicates if this is a program account.
	// Executable accounts cannot have their data modified.
	Executable bool

	// RentEpoch is the epoch at which rent was last collected.
	// Set to u64::MAX for rent-exempt accounts.
	RentEpoch uint64
}

// Clone creates a deep copy of the account.
func (a *Account) Clone() *Account {
	if a == nil {
		return nil
	}
	dataCopy := make([]byte, len(a.Data))
	copy(dataCopy, a.Data)
	return &Account{
		Lamports:   a.Lamports,
		Data:       dataCopy,
		Owner:      a.Owner,
		Executable: a.Executable,
		RentEpoch:  a.RentEpoch,
	}
}

// IsZero returns true if the account has no lamports and no data.
// Zero accounts are typically deleted from storage.
func (a *Account) IsZero() bool {
	return a.Lamports == 0 && len(a.Data) == 0
}

// DataLen returns the length of account data.
func (a *Account) DataLen() int {
	return len(a.Data)
}

// Size returns the total serialized size of the account.
func (a *Account) Size() int {
	// 8 (lamports) + 8 (data_len) + data + 32 (owner) + 1 (executable) + 8 (rent_epoch)
	return 8 + 8 + len(a.Data) + 32 + 1 + 8
}

// Serialize encodes the account to bytes for storage.
// Format: lamports (8) + data_len (8) + data + owner (32) + executable (1) + rent_epoch (8)
func (a *Account) Serialize() []byte {
	size := a.Size()
	buf := make([]byte, size)
	offset := 0

	// Lamports
	binary.LittleEndian.PutUint64(buf[offset:], a.Lamports)
	offset += 8

	// Data length
	binary.LittleEndian.PutUint64(buf[offset:], uint64(len(a.Data)))
	offset += 8

	// Data
	copy(buf[offset:], a.Data)
	offset += len(a.Data)

	// Owner
	copy(buf[offset:], a.Owner[:])
	offset += 32

	// Executable
	if a.Executable {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}
	offset += 1

	// RentEpoch
	binary.LittleEndian.PutUint64(buf[offset:], a.RentEpoch)

	return buf
}

// DeserializeAccount decodes an account from bytes.
func DeserializeAccount(data []byte) (*Account, error) {
	if len(data) < 57 { // Minimum: 8 + 8 + 0 + 32 + 1 + 8
		return nil, ErrInvalidData
	}

	offset := 0

	// Lamports
	lamports := binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// Data length
	dataLen := binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// Maximum account data size is 10MB
	const maxAccountDataSize = 10 * 1024 * 1024
	if dataLen > maxAccountDataSize {
		return nil, ErrInvalidData
	}

	// Validate data length
	if uint64(len(data)-offset) < dataLen+41 { // 32 (owner) + 1 (executable) + 8 (rent_epoch)
		return nil, ErrInvalidData
	}

	// Data
	accountData := make([]byte, dataLen)
	copy(accountData, data[offset:offset+int(dataLen)])
	offset += int(dataLen)

	// Owner
	var owner types.Pubkey
	copy(owner[:], data[offset:offset+32])
	offset += 32

	// Executable
	executable := data[offset] != 0
	offset += 1

	// RentEpoch
	rentEpoch := binary.LittleEndian.Uint64(data[offset:])

	return &Account{
		Lamports:   lamports,
		Data:       accountData,
		Owner:      owner,
		Executable: executable,
		RentEpoch:  rentEpoch,
	}, nil
}

// DB is the accounts database interface.
// Implementations must be safe for concurrent read access.
type DB interface {
	// GetAccount retrieves an account by public key.
	// Returns ErrAccountNotFound if the account doesn't exist.
	GetAccount(pubkey types.Pubkey) (*Account, error)

	// SetAccount stores an account.
	// If the account is zero (no lamports and no data), it will be deleted.
	SetAccount(pubkey types.Pubkey, account *Account) error

	// DeleteAccount removes an account.
	// Returns nil if the account doesn't exist.
	DeleteAccount(pubkey types.Pubkey) error

	// HasAccount checks if an account exists.
	HasAccount(pubkey types.Pubkey) (bool, error)

	// GetSlot returns the current slot.
	GetSlot() uint64

	// SetSlot updates the current slot.
	SetSlot(slot uint64) error

	// AccountsCount returns the total number of accounts.
	AccountsCount() (uint64, error)

	// Commit commits pending changes to disk.
	Commit() error

	// Close closes the database.
	Close() error
}

// HashableDB extends DB with hash computation capabilities.
type HashableDB interface {
	DB

	// ComputeAccountHash computes the hash of a single account.
	ComputeAccountHash(pubkey types.Pubkey, account *Account) types.Hash

	// ComputeAccountsHash computes the accounts hash for state verification.
	// This is equivalent to Solana's Epoch Accounts Hash.
	ComputeAccountsHash() (types.Hash, error)

	// ComputeDeltaHash computes the hash of accounts modified in a slot.
	// The pubkeys must be sorted.
	ComputeDeltaHash(modifiedPubkeys []types.Pubkey) (types.Hash, error)
}

// SnapshotableDB extends DB with snapshot capabilities.
type SnapshotableDB interface {
	DB

	// CreateSnapshot creates a snapshot at the current slot.
	CreateSnapshot(path string) error

	// LoadSnapshot loads state from a snapshot.
	LoadSnapshot(path string) error

	// GetSnapshotSlot returns the slot of a snapshot file.
	GetSnapshotSlot(path string) (uint64, error)
}

// FullDB combines all database capabilities.
type FullDB interface {
	HashableDB
	SnapshotableDB
}

// MemoryDB is an in-memory implementation of DB for testing.
type MemoryDB struct {
	accounts map[types.Pubkey]*Account
	slot     uint64
	closed   bool
}

// NewMemoryDB creates a new in-memory accounts database.
func NewMemoryDB() *MemoryDB {
	return &MemoryDB{
		accounts: make(map[types.Pubkey]*Account),
	}
}

// GetAccount retrieves an account.
func (m *MemoryDB) GetAccount(pubkey types.Pubkey) (*Account, error) {
	if m.closed {
		return nil, ErrClosed
	}
	acc, ok := m.accounts[pubkey]
	if !ok {
		return nil, ErrAccountNotFound
	}
	return acc.Clone(), nil
}

// SetAccount stores an account.
func (m *MemoryDB) SetAccount(pubkey types.Pubkey, account *Account) error {
	if m.closed {
		return ErrClosed
	}
	if account.IsZero() {
		delete(m.accounts, pubkey)
		return nil
	}
	m.accounts[pubkey] = account.Clone()
	return nil
}

// DeleteAccount removes an account.
func (m *MemoryDB) DeleteAccount(pubkey types.Pubkey) error {
	if m.closed {
		return ErrClosed
	}
	delete(m.accounts, pubkey)
	return nil
}

// HasAccount checks if an account exists.
func (m *MemoryDB) HasAccount(pubkey types.Pubkey) (bool, error) {
	if m.closed {
		return false, ErrClosed
	}
	_, ok := m.accounts[pubkey]
	return ok, nil
}

// GetSlot returns the current slot.
func (m *MemoryDB) GetSlot() uint64 {
	return m.slot
}

// SetSlot updates the current slot.
func (m *MemoryDB) SetSlot(slot uint64) error {
	if m.closed {
		return ErrClosed
	}
	m.slot = slot
	return nil
}

// AccountsCount returns the number of accounts.
func (m *MemoryDB) AccountsCount() (uint64, error) {
	if m.closed {
		return 0, ErrClosed
	}
	return uint64(len(m.accounts)), nil
}

// Commit is a no-op for MemoryDB.
func (m *MemoryDB) Commit() error {
	if m.closed {
		return ErrClosed
	}
	return nil
}

// Close closes the database.
func (m *MemoryDB) Close() error {
	m.closed = true
	m.accounts = nil
	return nil
}

// GetAllAccounts returns all accounts (for testing/debugging).
func (m *MemoryDB) GetAllAccounts() map[types.Pubkey]*Account {
	result := make(map[types.Pubkey]*Account, len(m.accounts))
	for k, v := range m.accounts {
		result[k] = v.Clone()
	}
	return result
}
