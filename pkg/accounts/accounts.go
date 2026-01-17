// Package accounts implements the AccountsDB for storing blockchain state.
//
// AccountsDB stores all account data for the X1 blockchain, including:
// - User accounts (balances, data)
// - Program accounts (bytecode)
// - System accounts (vote, stake, etc.)
//
// The implementation prioritizes memory efficiency and fast reads,
// using memory-mapped files and LRU caching.
package accounts

import (
	"errors"
)

var (
	// ErrNotImplemented is returned for unimplemented features.
	ErrNotImplemented = errors.New("not implemented")

	// ErrAccountNotFound is returned when an account doesn't exist.
	ErrAccountNotFound = errors.New("account not found")

	// ErrCorrupted is returned when data corruption is detected.
	ErrCorrupted = errors.New("data corrupted")
)

// Pubkey is a 32-byte public key.
type Pubkey [32]byte

// Account represents a single account in the state.
type Account struct {
	// Lamports is the account balance.
	Lamports uint64

	// Data is the account data.
	Data []byte

	// Owner is the program that owns this account.
	Owner Pubkey

	// Executable indicates if this is a program account.
	Executable bool

	// RentEpoch is the epoch at which rent was last collected.
	RentEpoch uint64
}

// DB is the accounts database interface.
type DB interface {
	// GetAccount retrieves an account by public key.
	GetAccount(pubkey Pubkey) (*Account, error)

	// SetAccount stores an account.
	SetAccount(pubkey Pubkey, account *Account) error

	// DeleteAccount removes an account.
	DeleteAccount(pubkey Pubkey) error

	// GetSlot returns the current slot.
	GetSlot() uint64

	// SetSlot updates the current slot.
	SetSlot(slot uint64) error

	// Commit commits pending changes.
	Commit() error

	// Close closes the database.
	Close() error
}

// MemoryDB is an in-memory implementation of DB for testing.
type MemoryDB struct {
	accounts map[Pubkey]*Account
	slot     uint64
}

// NewMemoryDB creates a new in-memory accounts database.
func NewMemoryDB() *MemoryDB {
	return &MemoryDB{
		accounts: make(map[Pubkey]*Account),
	}
}

// GetAccount retrieves an account.
func (m *MemoryDB) GetAccount(pubkey Pubkey) (*Account, error) {
	acc, ok := m.accounts[pubkey]
	if !ok {
		return nil, ErrAccountNotFound
	}
	return acc, nil
}

// SetAccount stores an account.
func (m *MemoryDB) SetAccount(pubkey Pubkey, account *Account) error {
	m.accounts[pubkey] = account
	return nil
}

// DeleteAccount removes an account.
func (m *MemoryDB) DeleteAccount(pubkey Pubkey) error {
	delete(m.accounts, pubkey)
	return nil
}

// GetSlot returns the current slot.
func (m *MemoryDB) GetSlot() uint64 {
	return m.slot
}

// SetSlot updates the current slot.
func (m *MemoryDB) SetSlot(slot uint64) error {
	m.slot = slot
	return nil
}

// Commit is a no-op for MemoryDB.
func (m *MemoryDB) Commit() error {
	return nil
}

// Close is a no-op for MemoryDB.
func (m *MemoryDB) Close() error {
	return nil
}

// TODO: Implement BadgerDB-backed AccountsDB for production use.
// type BadgerDB struct { ... }
