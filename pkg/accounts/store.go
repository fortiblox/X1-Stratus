// Package accounts provides the BadgerDB-backed storage implementation.
package accounts

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/badger/v4"
	"github.com/fortiblox/X1-Stratus/internal/types"
)

// Key prefixes for BadgerDB storage.
// Using prefixes allows efficient iteration over specific data types.
var (
	// prefixAccount is the prefix for account data.
	// Key format: prefixAccount + pubkey (32 bytes)
	prefixAccount = []byte{0x01}

	// prefixMeta is the prefix for metadata.
	// Key format: prefixMeta + key name
	prefixMeta = []byte{0x02}

	// metaSlot is the key for storing current slot.
	metaSlot = append(prefixMeta, []byte("slot")...)

	// metaAccountsCount is the key for storing accounts count.
	metaAccountsCount = append(prefixMeta, []byte("count")...)
)

// BadgerDBConfig contains configuration for BadgerDB.
type BadgerDBConfig struct {
	// Path is the directory path for the database.
	Path string

	// InMemory runs the database in memory (for testing).
	InMemory bool

	// SyncWrites ensures writes are synced to disk.
	// Setting to false improves performance but risks data loss on crash.
	SyncWrites bool

	// NumCompactors is the number of compaction workers.
	NumCompactors int

	// NumMemtables is the number of memtables.
	NumMemtables int

	// ValueLogFileSize is the size of each value log file.
	ValueLogFileSize int64

	// Logger is an optional logger. Set to nil to disable logging.
	Logger badger.Logger
}

// DefaultBadgerDBConfig returns default configuration.
func DefaultBadgerDBConfig(path string) BadgerDBConfig {
	return BadgerDBConfig{
		Path:             path,
		InMemory:         false,
		SyncWrites:       false, // Async for performance
		NumCompactors:    4,
		NumMemtables:     5,
		ValueLogFileSize: 256 << 20, // 256MB
		Logger:           nil,       // Disable logging by default
	}
}

// BadgerDB is a BadgerDB-backed implementation of the accounts database.
// It provides efficient key-value storage optimized for SSDs.
//
// BadgerDB uses an LSM (Log-Structured Merge) tree architecture:
// - Keys are stored in an LSM tree for fast lookups
// - Values are stored in a separate value log (vLog) for large values
// - This separation is ideal for account data which can be up to 10MB
//
// Key design decisions:
// - Accounts are stored with pubkey as key (32 bytes)
// - Account data is serialized in a compact binary format
// - Slot and count metadata are stored with special prefixes
// - Batch writes are used for atomic updates
type BadgerDB struct {
	db *badger.DB

	// slot is cached in memory for fast access
	slot atomic.Uint64

	// accountsCount is cached in memory
	accountsCount atomic.Uint64

	// mu protects concurrent writes
	mu sync.RWMutex

	// closed indicates if the database is closed
	closed atomic.Bool
}

// NewBadgerDB creates a new BadgerDB-backed accounts database.
func NewBadgerDB(cfg BadgerDBConfig) (*BadgerDB, error) {
	opts := badger.DefaultOptions(cfg.Path)

	if cfg.InMemory {
		opts = opts.WithInMemory(true)
	}

	opts = opts.
		WithSyncWrites(cfg.SyncWrites).
		WithNumCompactors(cfg.NumCompactors).
		WithNumMemtables(cfg.NumMemtables).
		WithValueLogFileSize(cfg.ValueLogFileSize).
		WithLogger(cfg.Logger)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open badger: %w", err)
	}

	bdb := &BadgerDB{
		db: db,
	}

	// Load metadata
	if err := bdb.loadMetadata(); err != nil {
		db.Close()
		return nil, fmt.Errorf("load metadata: %w", err)
	}

	return bdb, nil
}

// loadMetadata loads slot and count from disk.
func (b *BadgerDB) loadMetadata() error {
	return b.db.View(func(txn *badger.Txn) error {
		// Load slot
		item, err := txn.Get(metaSlot)
		if err == badger.ErrKeyNotFound {
			b.slot.Store(0)
		} else if err != nil {
			return err
		} else {
			err = item.Value(func(val []byte) error {
				if len(val) >= 8 {
					b.slot.Store(binary.LittleEndian.Uint64(val))
				}
				return nil
			})
			if err != nil {
				return err
			}
		}

		// Load count
		item, err = txn.Get(metaAccountsCount)
		if err == badger.ErrKeyNotFound {
			b.accountsCount.Store(0)
		} else if err != nil {
			return err
		} else {
			err = item.Value(func(val []byte) error {
				if len(val) >= 8 {
					b.accountsCount.Store(binary.LittleEndian.Uint64(val))
				}
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// accountKey returns the BadgerDB key for an account.
func accountKey(pubkey types.Pubkey) []byte {
	key := make([]byte, 1+32)
	key[0] = prefixAccount[0]
	copy(key[1:], pubkey[:])
	return key
}

// GetAccount retrieves an account by public key.
func (b *BadgerDB) GetAccount(pubkey types.Pubkey) (*Account, error) {
	if b.closed.Load() {
		return nil, ErrClosed
	}

	var account *Account

	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(accountKey(pubkey))
		if err == badger.ErrKeyNotFound {
			return ErrAccountNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			acc, err := DeserializeAccount(val)
			if err != nil {
				return err
			}
			account = acc
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return account, nil
}

// SetAccount stores an account.
func (b *BadgerDB) SetAccount(pubkey types.Pubkey, account *Account) error {
	if b.closed.Load() {
		return ErrClosed
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if account exists for count tracking
	exists, err := b.hasAccountLocked(pubkey)
	if err != nil {
		return err
	}

	// Handle zero accounts (deletion)
	if account.IsZero() {
		if exists {
			err := b.db.Update(func(txn *badger.Txn) error {
				return txn.Delete(accountKey(pubkey))
			})
			if err != nil {
				return err
			}
			b.accountsCount.Add(^uint64(0)) // Decrement
		}
		return nil
	}

	// Serialize and store
	data := account.Serialize()
	err = b.db.Update(func(txn *badger.Txn) error {
		return txn.Set(accountKey(pubkey), data)
	})
	if err != nil {
		return err
	}

	// Update count if new account
	if !exists {
		b.accountsCount.Add(1)
	}

	return nil
}

// DeleteAccount removes an account.
func (b *BadgerDB) DeleteAccount(pubkey types.Pubkey) error {
	if b.closed.Load() {
		return ErrClosed
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if account exists
	exists, err := b.hasAccountLocked(pubkey)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	err = b.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(accountKey(pubkey))
	})
	if err != nil {
		return err
	}

	b.accountsCount.Add(^uint64(0)) // Decrement
	return nil
}

// HasAccount checks if an account exists.
func (b *BadgerDB) HasAccount(pubkey types.Pubkey) (bool, error) {
	if b.closed.Load() {
		return false, ErrClosed
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.hasAccountLocked(pubkey)
}

// hasAccountLocked checks if an account exists (caller must hold lock).
func (b *BadgerDB) hasAccountLocked(pubkey types.Pubkey) (bool, error) {
	var exists bool
	err := b.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(accountKey(pubkey))
		if err == badger.ErrKeyNotFound {
			exists = false
			return nil
		}
		if err != nil {
			return err
		}
		exists = true
		return nil
	})
	return exists, err
}

// GetSlot returns the current slot.
func (b *BadgerDB) GetSlot() uint64 {
	return b.slot.Load()
}

// SetSlot updates the current slot.
func (b *BadgerDB) SetSlot(slot uint64) error {
	if b.closed.Load() {
		return ErrClosed
	}

	b.slot.Store(slot)
	return nil
}

// AccountsCount returns the total number of accounts.
func (b *BadgerDB) AccountsCount() (uint64, error) {
	if b.closed.Load() {
		return 0, ErrClosed
	}
	return b.accountsCount.Load(), nil
}

// Commit commits pending changes to disk.
// This persists metadata (slot, count) and syncs the database.
func (b *BadgerDB) Commit() error {
	if b.closed.Load() {
		return ErrClosed
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Persist metadata
	return b.db.Update(func(txn *badger.Txn) error {
		// Save slot
		slotBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(slotBuf, b.slot.Load())
		if err := txn.Set(metaSlot, slotBuf); err != nil {
			return err
		}

		// Save count
		countBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(countBuf, b.accountsCount.Load())
		if err := txn.Set(metaAccountsCount, countBuf); err != nil {
			return err
		}

		return nil
	})
}

// Close closes the database.
func (b *BadgerDB) Close() error {
	if b.closed.Swap(true) {
		return ErrClosed
	}

	// Commit before closing
	if err := b.Commit(); err != nil {
		// Log but don't fail
		_ = err
	}

	return b.db.Close()
}

// IterateAccounts iterates over all accounts in sorted pubkey order.
// The callback receives each pubkey and account.
// Return an error from the callback to stop iteration.
func (b *BadgerDB) IterateAccounts(fn func(pubkey types.Pubkey, account *Account) error) error {
	if b.closed.Load() {
		return ErrClosed
	}

	return b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefixAccount
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()

			// Extract pubkey from key
			if len(key) != 33 { // 1 prefix + 32 pubkey
				continue
			}
			var pubkey types.Pubkey
			copy(pubkey[:], key[1:])

			// Get account
			err := item.Value(func(val []byte) error {
				account, err := DeserializeAccount(val)
				if err != nil {
					return err
				}
				return fn(pubkey, account)
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// GetAccountsInRange returns accounts with pubkeys in the given range.
// startPubkey is inclusive, endPubkey is exclusive.
func (b *BadgerDB) GetAccountsInRange(startPubkey, endPubkey types.Pubkey) ([]*AccountEntry, error) {
	if b.closed.Load() {
		return nil, ErrClosed
	}

	var entries []*AccountEntry

	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefixAccount
		it := txn.NewIterator(opts)
		defer it.Close()

		startKey := accountKey(startPubkey)
		endKey := accountKey(endPubkey)

		for it.Seek(startKey); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()

			// Check if we've passed the end
			if bytes.Compare(key, endKey) >= 0 {
				break
			}

			// Extract pubkey
			if len(key) != 33 {
				continue
			}
			var pubkey types.Pubkey
			copy(pubkey[:], key[1:])

			// Get account
			err := item.Value(func(val []byte) error {
				account, err := DeserializeAccount(val)
				if err != nil {
					return err
				}
				entries = append(entries, &AccountEntry{
					Pubkey:  pubkey,
					Account: account,
				})
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return entries, nil
}

// AccountEntry pairs a pubkey with its account.
type AccountEntry struct {
	Pubkey  types.Pubkey
	Account *Account
}

// BatchWriter provides efficient batch writes.
type BatchWriter struct {
	db      *BadgerDB
	batch   *badger.WriteBatch
	added   int64
	deleted int64
}

// NewBatchWriter creates a new batch writer.
func (b *BadgerDB) NewBatchWriter() *BatchWriter {
	return &BatchWriter{
		db:    b,
		batch: b.db.NewWriteBatch(),
	}
}

// SetAccount adds an account write to the batch.
func (bw *BatchWriter) SetAccount(pubkey types.Pubkey, account *Account) error {
	if account.IsZero() {
		return bw.DeleteAccount(pubkey)
	}

	// Check if account exists
	exists, _ := bw.db.HasAccount(pubkey)

	data := account.Serialize()
	if err := bw.batch.Set(accountKey(pubkey), data); err != nil {
		return err
	}

	if !exists {
		bw.added++
	}

	return nil
}

// DeleteAccount adds an account deletion to the batch.
func (bw *BatchWriter) DeleteAccount(pubkey types.Pubkey) error {
	exists, _ := bw.db.HasAccount(pubkey)
	if exists {
		if err := bw.batch.Delete(accountKey(pubkey)); err != nil {
			return err
		}
		bw.deleted++
	}
	return nil
}

// Flush writes all batched operations to disk.
func (bw *BatchWriter) Flush() error {
	if err := bw.batch.Flush(); err != nil {
		return err
	}

	// Update count
	bw.db.accountsCount.Add(uint64(bw.added))
	if bw.deleted > 0 {
		bw.db.accountsCount.Add(^uint64(bw.deleted - 1)) // Subtract deleted
	}

	bw.added = 0
	bw.deleted = 0
	return nil
}

// Cancel cancels the batch without writing.
func (bw *BatchWriter) Cancel() {
	bw.batch.Cancel()
	bw.added = 0
	bw.deleted = 0
}

// RunGC runs garbage collection on the value log.
// This should be called periodically to reclaim space.
func (b *BadgerDB) RunGC() error {
	if b.closed.Load() {
		return ErrClosed
	}
	return b.db.RunValueLogGC(0.5)
}

// Sync ensures all writes are persisted to disk.
func (b *BadgerDB) Sync() error {
	if b.closed.Load() {
		return ErrClosed
	}
	return b.db.Sync()
}

// Size returns the size of the database in bytes.
func (b *BadgerDB) Size() (lsm, vlog int64) {
	return b.db.Size()
}

// Verify that BadgerDB implements DB interface.
var _ DB = (*BadgerDB)(nil)
