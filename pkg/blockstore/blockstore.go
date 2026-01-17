// Package blockstore provides persistent storage for Solana blocks and transactions.
package blockstore

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
	bolt "go.etcd.io/bbolt"
)

var (
	// ErrBlockNotFound is returned when a block doesn't exist.
	ErrBlockNotFound = errors.New("block not found")

	// ErrTransactionNotFound is returned when a transaction doesn't exist.
	ErrTransactionNotFound = errors.New("transaction not found")

	// ErrSlotNotFound is returned when a slot doesn't exist.
	ErrSlotNotFound = errors.New("slot not found")

	// ErrClosed is returned when operating on a closed blockstore.
	ErrClosed = errors.New("blockstore closed")

	// ErrInvalidSlot is returned for invalid slot numbers.
	ErrInvalidSlot = errors.New("invalid slot number")
)

// Bucket names for BoltDB.
var (
	// bucketBlocks stores complete block data keyed by slot.
	bucketBlocks = []byte("blocks")

	// bucketSlotMeta stores slot metadata keyed by slot.
	bucketSlotMeta = []byte("slot_meta")

	// bucketTxBySignature indexes transactions by signature.
	bucketTxBySignature = []byte("tx_by_sig")

	// bucketAddressSignatures indexes signatures by address+slot.
	bucketAddressSignatures = []byte("addr_sigs")

	// bucketRoots tracks finalized root slots.
	bucketRoots = []byte("roots")

	// bucketMetadata stores blockstore metadata.
	bucketMetadata = []byte("metadata")
)

// Metadata keys.
var (
	keyLatestSlot     = []byte("latest_slot")
	keyFinalizedSlot  = []byte("finalized_slot")
	keyConfirmedSlot  = []byte("confirmed_slot")
	keyOldestSlot     = []byte("oldest_slot")
	keyBlockCount     = []byte("block_count")
	keyTransactionCount = []byte("transaction_count")
)

// Config holds blockstore configuration options.
type Config struct {
	// Path is the directory path for the blockstore database.
	Path string

	// MaxOpenFiles controls the maximum number of open file handles.
	MaxOpenFiles int

	// NoSync disables fsync after each write (faster but less durable).
	NoSync bool

	// PruneEnabled enables automatic pruning of old blocks.
	PruneEnabled bool

	// PruneInterval is how often to run the pruning routine.
	PruneInterval time.Duration

	// RetainSlots is the number of slots to retain during pruning.
	RetainSlots uint64

	// ReadOnly opens the database in read-only mode.
	ReadOnly bool
}

// DefaultConfig returns the default blockstore configuration.
func DefaultConfig(path string) Config {
	return Config{
		Path:          path,
		MaxOpenFiles:  1000,
		NoSync:        false,
		PruneEnabled:  true,
		PruneInterval: 1 * time.Hour,
		RetainSlots:   DefaultPruneSlots,
		ReadOnly:      false,
	}
}

// Store is the main blockstore interface.
type Store interface {
	// Block operations
	GetBlock(slot uint64) (*Block, error)
	PutBlock(block *Block) error
	HasBlock(slot uint64) bool
	DeleteBlock(slot uint64) error

	// Slot metadata operations
	GetSlotMeta(slot uint64) (*SlotMeta, error)
	PutSlotMeta(meta *SlotMeta) error
	SetCommitment(slot uint64, commitment CommitmentLevel) error

	// Transaction operations
	GetTransaction(signature types.Signature) (*Transaction, error)
	GetTransactionStatus(signature types.Signature) (*TransactionStatus, error)

	// Index operations (see index.go for extended interface)
	GetSignaturesForAddress(address types.Pubkey, opts *SignatureQueryOptions) ([]SignatureInfo, error)

	// Slot progression
	GetLatestSlot() uint64
	GetFinalizedSlot() uint64
	GetConfirmedSlot() uint64
	GetOldestSlot() uint64
	SetLatestSlot(slot uint64) error
	SetFinalizedSlot(slot uint64) error
	SetConfirmedSlot(slot uint64) error

	// Root management (finalized slots)
	IsRoot(slot uint64) bool
	SetRoot(slot uint64) error
	GetRoots(start, end uint64) ([]uint64, error)

	// Maintenance
	Prune(keepSlots uint64) (uint64, error)
	GetStats() (*Stats, error)
	Sync() error
	Close() error
}

// Stats contains blockstore statistics.
type Stats struct {
	// LatestSlot is the most recent slot stored.
	LatestSlot uint64

	// FinalizedSlot is the most recent finalized slot.
	FinalizedSlot uint64

	// ConfirmedSlot is the most recent confirmed slot.
	ConfirmedSlot uint64

	// OldestSlot is the oldest slot still retained.
	OldestSlot uint64

	// BlockCount is the total number of blocks stored.
	BlockCount uint64

	// TransactionCount is the total number of transactions indexed.
	TransactionCount uint64

	// DatabaseSize is the size of the database file in bytes.
	DatabaseSize int64
}

// SignatureQueryOptions configures signature queries.
type SignatureQueryOptions struct {
	// Limit is the maximum number of signatures to return.
	Limit int

	// Before returns signatures before (not including) this signature.
	Before *types.Signature

	// Until returns signatures until (not including) this slot.
	Until *uint64

	// MinSlot filters signatures to those in slots >= MinSlot.
	MinSlot *uint64

	// Commitment filters by minimum commitment level.
	Commitment CommitmentLevel
}

// BoltStore implements Store using BoltDB.
type BoltStore struct {
	db     *bolt.DB
	config Config

	// Cached slot values for fast reads.
	mu             sync.RWMutex
	latestSlot     uint64
	finalizedSlot  uint64
	confirmedSlot  uint64
	oldestSlot     uint64
	blockCount     uint64
	transactionCount uint64

	// Pruning control.
	pruneStop chan struct{}
	pruneWG   sync.WaitGroup

	closed bool
}

// Open creates or opens a blockstore at the given path.
func Open(config Config) (*BoltStore, error) {
	// Ensure directory exists.
	dir := filepath.Dir(config.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	// Open BoltDB.
	opts := &bolt.Options{
		Timeout:  5 * time.Second,
		NoSync:   config.NoSync,
		ReadOnly: config.ReadOnly,
	}

	db, err := bolt.Open(config.Path, 0600, opts)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := &BoltStore{
		db:        db,
		config:    config,
		pruneStop: make(chan struct{}),
	}

	// Initialize buckets (skip in read-only mode).
	if !config.ReadOnly {
		if err := store.initBuckets(); err != nil {
			db.Close()
			return nil, fmt.Errorf("init buckets: %w", err)
		}
	}

	// Load cached values.
	if err := store.loadCachedValues(); err != nil {
		db.Close()
		return nil, fmt.Errorf("load cached values: %w", err)
	}

	// Start pruning goroutine if enabled.
	if config.PruneEnabled && !config.ReadOnly {
		store.startPruning()
	}

	return store, nil
}

// initBuckets creates all required buckets.
func (s *BoltStore) initBuckets() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		buckets := [][]byte{
			bucketBlocks,
			bucketSlotMeta,
			bucketTxBySignature,
			bucketAddressSignatures,
			bucketRoots,
			bucketMetadata,
		}
		for _, name := range buckets {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return fmt.Errorf("create bucket %s: %w", name, err)
			}
		}
		return nil
	})
}

// loadCachedValues loads frequently-accessed values into memory.
func (s *BoltStore) loadCachedValues() error {
	return s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMetadata)
		if meta == nil {
			return nil // Empty database, no values to load.
		}

		if v := meta.Get(keyLatestSlot); v != nil {
			s.latestSlot = DecodeSlotKey(v)
		}
		if v := meta.Get(keyFinalizedSlot); v != nil {
			s.finalizedSlot = DecodeSlotKey(v)
		}
		if v := meta.Get(keyConfirmedSlot); v != nil {
			s.confirmedSlot = DecodeSlotKey(v)
		}
		if v := meta.Get(keyOldestSlot); v != nil {
			s.oldestSlot = DecodeSlotKey(v)
		}
		if v := meta.Get(keyBlockCount); v != nil {
			s.blockCount = DecodeSlotKey(v)
		}
		if v := meta.Get(keyTransactionCount); v != nil {
			s.transactionCount = DecodeSlotKey(v)
		}
		return nil
	})
}

// startPruning starts the background pruning goroutine.
func (s *BoltStore) startPruning() {
	s.pruneWG.Add(1)
	go func() {
		defer s.pruneWG.Done()
		ticker := time.NewTicker(s.config.PruneInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if _, err := s.Prune(s.config.RetainSlots); err != nil {
					// Log error but continue.
					fmt.Printf("blockstore prune error: %v\n", err)
				}
			case <-s.pruneStop:
				return
			}
		}
	}()
}

// GetBlock retrieves a block by slot number.
func (s *BoltStore) GetBlock(slot uint64) (*Block, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrClosed
	}
	s.mu.RUnlock()

	var block Block
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketBlocks)
		if b == nil {
			return ErrBlockNotFound
		}

		data := b.Get(EncodeSlotKey(slot))
		if data == nil {
			return ErrBlockNotFound
		}

		return gob.NewDecoder(bytes.NewReader(data)).Decode(&block)
	})

	if err != nil {
		return nil, err
	}
	return &block, nil
}

// PutBlock stores a block and indexes its transactions.
func (s *BoltStore) PutBlock(block *Block) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrClosed
	}
	s.mu.RUnlock()

	// Encode block.
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(block); err != nil {
		return fmt.Errorf("encode block: %w", err)
	}
	blockData := buf.Bytes()

	// Encode slot metadata.
	meta := &SlotMeta{
		Slot:              block.Slot,
		ParentSlot:        block.ParentSlot,
		BlockTime:         block.BlockTime,
		BlockHeight:       block.BlockHeight,
		Commitment:        CommitmentProcessed, // Default to processed.
		IsComplete:        true,
		TransactionCount:  uint64(len(block.Transactions)),
		Blockhash:         block.Blockhash,
		PreviousBlockhash: block.PreviousBlockhash,
	}

	var metaBuf bytes.Buffer
	if err := gob.NewEncoder(&metaBuf).Encode(meta); err != nil {
		return fmt.Errorf("encode slot meta: %w", err)
	}
	metaData := metaBuf.Bytes()

	txCount := uint64(len(block.Transactions))

	err := s.db.Update(func(tx *bolt.Tx) error {
		// Store block.
		blocks := tx.Bucket(bucketBlocks)
		slotKey := EncodeSlotKey(block.Slot)
		if err := blocks.Put(slotKey, blockData); err != nil {
			return err
		}

		// Store slot metadata.
		slotMeta := tx.Bucket(bucketSlotMeta)
		if err := slotMeta.Put(slotKey, metaData); err != nil {
			return err
		}

		// Index transactions.
		txBySig := tx.Bucket(bucketTxBySignature)
		addrSigs := tx.Bucket(bucketAddressSignatures)

		for i := range block.Transactions {
			txn := &block.Transactions[i]
			txn.Slot = block.Slot // Ensure slot is set.

			// Index by signature.
			sigKey := EncodeSignatureKey(txn.Signature)
			var txBuf bytes.Buffer
			if err := gob.NewEncoder(&txBuf).Encode(txn); err != nil {
				return fmt.Errorf("encode transaction: %w", err)
			}
			if err := txBySig.Put(sigKey, txBuf.Bytes()); err != nil {
				return err
			}

			// Index by address (for all accounts in the transaction).
			sigInfo := SignatureInfo{
				Signature: txn.Signature,
				Slot:      block.Slot,
				BlockTime: block.BlockTime,
			}
			if txn.Meta != nil && txn.Meta.Err != nil {
				sigInfo.Err = txn.Meta.Err
			}

			var sigInfoBuf bytes.Buffer
			if err := gob.NewEncoder(&sigInfoBuf).Encode(&sigInfo); err != nil {
				return fmt.Errorf("encode sig info: %w", err)
			}
			sigInfoData := sigInfoBuf.Bytes()

			for _, addr := range txn.Message.AccountKeys {
				addrSlotKey := EncodeAddressSlotKey(addr, block.Slot)
				// Append signature to existing list or create new.
				existing := addrSigs.Get(addrSlotKey)
				if existing == nil {
					if err := addrSigs.Put(addrSlotKey, sigInfoData); err != nil {
						return err
					}
				} else {
					// Append (simple approach: concatenate).
					combined := append(existing, sigInfoData...)
					if err := addrSigs.Put(addrSlotKey, combined); err != nil {
						return err
					}
				}
			}
		}

		// Update metadata.
		metadata := tx.Bucket(bucketMetadata)
		if err := metadata.Put(keyLatestSlot, slotKey); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Update cached values.
	s.mu.Lock()
	if block.Slot > s.latestSlot {
		s.latestSlot = block.Slot
	}
	if s.oldestSlot == 0 || block.Slot < s.oldestSlot {
		s.oldestSlot = block.Slot
	}
	s.blockCount++
	s.transactionCount += txCount
	s.mu.Unlock()

	return nil
}

// HasBlock checks if a block exists for the given slot.
func (s *BoltStore) HasBlock(slot uint64) bool {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return false
	}
	s.mu.RUnlock()

	exists := false
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketBlocks)
		if b != nil && b.Get(EncodeSlotKey(slot)) != nil {
			exists = true
		}
		return nil
	})
	return exists
}

// DeleteBlock removes a block and its associated indexes.
func (s *BoltStore) DeleteBlock(slot uint64) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrClosed
	}
	s.mu.RUnlock()

	// First, get the block to find transactions to deindex.
	block, err := s.GetBlock(slot)
	if err != nil {
		if errors.Is(err, ErrBlockNotFound) {
			return nil // Already deleted.
		}
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		slotKey := EncodeSlotKey(slot)

		// Delete block.
		if err := tx.Bucket(bucketBlocks).Delete(slotKey); err != nil {
			return err
		}

		// Delete slot metadata.
		if err := tx.Bucket(bucketSlotMeta).Delete(slotKey); err != nil {
			return err
		}

		// Delete transaction indexes.
		txBySig := tx.Bucket(bucketTxBySignature)
		addrSigs := tx.Bucket(bucketAddressSignatures)

		for _, txn := range block.Transactions {
			// Remove signature index.
			if err := txBySig.Delete(EncodeSignatureKey(txn.Signature)); err != nil {
				return err
			}

			// Remove address indexes.
			for _, addr := range txn.Message.AccountKeys {
				addrSlotKey := EncodeAddressSlotKey(addr, slot)
				if err := addrSigs.Delete(addrSlotKey); err != nil {
					return err
				}
			}
		}

		// Delete root entry if exists.
		if err := tx.Bucket(bucketRoots).Delete(slotKey); err != nil {
			return err
		}

		return nil
	})
}

// GetSlotMeta retrieves metadata for a slot.
func (s *BoltStore) GetSlotMeta(slot uint64) (*SlotMeta, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrClosed
	}
	s.mu.RUnlock()

	var meta SlotMeta
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSlotMeta)
		if b == nil {
			return ErrSlotNotFound
		}

		data := b.Get(EncodeSlotKey(slot))
		if data == nil {
			return ErrSlotNotFound
		}

		return gob.NewDecoder(bytes.NewReader(data)).Decode(&meta)
	})

	if err != nil {
		return nil, err
	}
	return &meta, nil
}

// PutSlotMeta stores slot metadata.
func (s *BoltStore) PutSlotMeta(meta *SlotMeta) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrClosed
	}
	s.mu.RUnlock()

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(meta); err != nil {
		return fmt.Errorf("encode slot meta: %w", err)
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSlotMeta).Put(EncodeSlotKey(meta.Slot), buf.Bytes())
	})
}

// SetCommitment updates the commitment level for a slot.
func (s *BoltStore) SetCommitment(slot uint64, commitment CommitmentLevel) error {
	meta, err := s.GetSlotMeta(slot)
	if err != nil {
		return err
	}

	meta.Commitment = commitment
	if err := s.PutSlotMeta(meta); err != nil {
		return err
	}

	// Update finalized/confirmed slots.
	s.mu.Lock()
	defer s.mu.Unlock()

	switch commitment {
	case CommitmentFinalized:
		if slot > s.finalizedSlot {
			s.finalizedSlot = slot
			s.db.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(bucketMetadata).Put(keyFinalizedSlot, EncodeSlotKey(slot))
			})
		}
	case CommitmentConfirmed:
		if slot > s.confirmedSlot {
			s.confirmedSlot = slot
			s.db.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(bucketMetadata).Put(keyConfirmedSlot, EncodeSlotKey(slot))
			})
		}
	}

	return nil
}

// GetTransaction retrieves a transaction by signature.
func (s *BoltStore) GetTransaction(signature types.Signature) (*Transaction, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrClosed
	}
	s.mu.RUnlock()

	var txn Transaction
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketTxBySignature)
		if b == nil {
			return ErrTransactionNotFound
		}

		data := b.Get(EncodeSignatureKey(signature))
		if data == nil {
			return ErrTransactionNotFound
		}

		return gob.NewDecoder(bytes.NewReader(data)).Decode(&txn)
	})

	if err != nil {
		return nil, err
	}
	return &txn, nil
}

// GetTransactionStatus returns the status of a transaction.
func (s *BoltStore) GetTransactionStatus(signature types.Signature) (*TransactionStatus, error) {
	txn, err := s.GetTransaction(signature)
	if err != nil {
		return nil, err
	}

	meta, err := s.GetSlotMeta(txn.Slot)
	if err != nil {
		return nil, err
	}

	status := &TransactionStatus{
		Slot:               txn.Slot,
		Signature:          txn.Signature,
		ConfirmationStatus: meta.Commitment,
	}

	if txn.Meta != nil && txn.Meta.Err != nil {
		status.Err = txn.Meta.Err
	}

	// Calculate confirmations if not finalized.
	if meta.Commitment != CommitmentFinalized {
		s.mu.RLock()
		if s.latestSlot > txn.Slot {
			confs := s.latestSlot - txn.Slot
			status.Confirmations = &confs
		}
		s.mu.RUnlock()
	}

	return status, nil
}

// GetSignaturesForAddress returns signatures for transactions involving an address.
func (s *BoltStore) GetSignaturesForAddress(address types.Pubkey, opts *SignatureQueryOptions) ([]SignatureInfo, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrClosed
	}
	s.mu.RUnlock()

	if opts == nil {
		opts = &SignatureQueryOptions{Limit: 1000}
	}
	if opts.Limit <= 0 {
		opts.Limit = 1000
	}

	var results []SignatureInfo

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAddressSignatures)
		if b == nil {
			return nil
		}

		c := b.Cursor()

		// Determine start position.
		prefix := address[:]
		startKey := make([]byte, 40)
		copy(startKey[:32], prefix)

		// Start from the end (most recent) if no MinSlot specified.
		if opts.MinSlot != nil {
			copy(startKey[32:], EncodeSlotKey(*opts.MinSlot))
		} else {
			// Start from highest slot.
			for i := 32; i < 40; i++ {
				startKey[i] = 0xFF
			}
		}

		// Iterate in reverse order (newest first).
		// Seek positions at first key >= startKey. If that key doesn't have our
		// prefix, we need to go back to find entries with our prefix.
		k, v := c.Seek(startKey)
		if k == nil || !bytes.HasPrefix(k, prefix) {
			// No exact match or past our prefix; go to previous entry
			k, v = c.Prev()
		}

		for ; k != nil; k, v = c.Prev() {
			// Check prefix matches.
			if !bytes.HasPrefix(k, prefix) {
				break
			}

			// Decode signature info.
			var info SignatureInfo
			if err := gob.NewDecoder(bytes.NewReader(v)).Decode(&info); err != nil {
				continue // Skip corrupted entries.
			}

			// Apply filters.
			if opts.Until != nil && info.Slot >= *opts.Until {
				continue
			}

			results = append(results, info)

			if len(results) >= opts.Limit {
				break
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// GetLatestSlot returns the most recent slot.
func (s *BoltStore) GetLatestSlot() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latestSlot
}

// GetFinalizedSlot returns the most recent finalized slot.
func (s *BoltStore) GetFinalizedSlot() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.finalizedSlot
}

// GetConfirmedSlot returns the most recent confirmed slot.
func (s *BoltStore) GetConfirmedSlot() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.confirmedSlot
}

// GetOldestSlot returns the oldest slot still stored.
func (s *BoltStore) GetOldestSlot() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.oldestSlot
}

// SetLatestSlot updates the latest slot marker.
func (s *BoltStore) SetLatestSlot(slot uint64) error {
	s.mu.Lock()
	s.latestSlot = slot
	s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMetadata).Put(keyLatestSlot, EncodeSlotKey(slot))
	})
}

// SetFinalizedSlot updates the finalized slot marker.
func (s *BoltStore) SetFinalizedSlot(slot uint64) error {
	s.mu.Lock()
	s.finalizedSlot = slot
	s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMetadata).Put(keyFinalizedSlot, EncodeSlotKey(slot))
	})
}

// SetConfirmedSlot updates the confirmed slot marker.
func (s *BoltStore) SetConfirmedSlot(slot uint64) error {
	s.mu.Lock()
	s.confirmedSlot = slot
	s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMetadata).Put(keyConfirmedSlot, EncodeSlotKey(slot))
	})
}

// IsRoot checks if a slot is a finalized root.
func (s *BoltStore) IsRoot(slot uint64) bool {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return false
	}
	s.mu.RUnlock()

	isRoot := false
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRoots)
		if b != nil && b.Get(EncodeSlotKey(slot)) != nil {
			isRoot = true
		}
		return nil
	})
	return isRoot
}

// SetRoot marks a slot as a finalized root.
func (s *BoltStore) SetRoot(slot uint64) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrClosed
	}
	s.mu.RUnlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		slotKey := EncodeSlotKey(slot)
		// Store empty value; key presence indicates root.
		return tx.Bucket(bucketRoots).Put(slotKey, []byte{1})
	})
}

// GetRoots returns all root slots in the given range.
func (s *BoltStore) GetRoots(start, end uint64) ([]uint64, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrClosed
	}
	s.mu.RUnlock()

	var roots []uint64

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRoots)
		if b == nil {
			return nil
		}

		c := b.Cursor()
		startKey := EncodeSlotKey(start)
		endKey := EncodeSlotKey(end)

		for k, _ := c.Seek(startKey); k != nil && bytes.Compare(k, endKey) <= 0; k, _ = c.Next() {
			roots = append(roots, DecodeSlotKey(k))
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return roots, nil
}

// Prune removes blocks older than the specified retention window.
// Returns the number of blocks pruned.
func (s *BoltStore) Prune(keepSlots uint64) (uint64, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return 0, ErrClosed
	}
	latestSlot := s.latestSlot
	s.mu.RUnlock()

	if latestSlot <= keepSlots {
		return 0, nil // Nothing to prune.
	}

	pruneBeforeSlot := latestSlot - keepSlots
	var pruned uint64

	// Find slots to prune.
	var slotsToPrune []uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketBlocks)
		if b == nil {
			return nil
		}

		c := b.Cursor()
		maxKey := EncodeSlotKey(pruneBeforeSlot)

		for k, _ := c.First(); k != nil && bytes.Compare(k, maxKey) < 0; k, _ = c.Next() {
			slotsToPrune = append(slotsToPrune, DecodeSlotKey(k))
		}
		return nil
	})

	if err != nil {
		return 0, err
	}

	// Delete in batches to avoid long transactions.
	const batchSize = 100
	for i := 0; i < len(slotsToPrune); i += batchSize {
		end := i + batchSize
		if end > len(slotsToPrune) {
			end = len(slotsToPrune)
		}
		batch := slotsToPrune[i:end]

		for _, slot := range batch {
			if err := s.DeleteBlock(slot); err != nil {
				return pruned, fmt.Errorf("delete slot %d: %w", slot, err)
			}
			pruned++
		}
	}

	// Update oldest slot.
	if len(slotsToPrune) > 0 {
		s.mu.Lock()
		s.oldestSlot = pruneBeforeSlot
		s.blockCount -= pruned
		s.mu.Unlock()

		s.db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(bucketMetadata).Put(keyOldestSlot, EncodeSlotKey(pruneBeforeSlot))
		})
	}

	return pruned, nil
}

// GetStats returns blockstore statistics.
func (s *BoltStore) GetStats() (*Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosed
	}

	stats := &Stats{
		LatestSlot:       s.latestSlot,
		FinalizedSlot:    s.finalizedSlot,
		ConfirmedSlot:    s.confirmedSlot,
		OldestSlot:       s.oldestSlot,
		BlockCount:       s.blockCount,
		TransactionCount: s.transactionCount,
	}

	// Get database size.
	info, err := os.Stat(s.config.Path)
	if err == nil {
		stats.DatabaseSize = info.Size()
	}

	return stats, nil
}

// Sync forces a sync of the database to disk.
func (s *BoltStore) Sync() error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrClosed
	}
	s.mu.RUnlock()

	return s.db.Sync()
}

// Close shuts down the blockstore.
func (s *BoltStore) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	// Stop pruning.
	close(s.pruneStop)
	s.pruneWG.Wait()

	// Persist final metadata.
	s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMetadata)
		meta.Put(keyLatestSlot, EncodeSlotKey(s.latestSlot))
		meta.Put(keyFinalizedSlot, EncodeSlotKey(s.finalizedSlot))
		meta.Put(keyConfirmedSlot, EncodeSlotKey(s.confirmedSlot))
		meta.Put(keyOldestSlot, EncodeSlotKey(s.oldestSlot))
		meta.Put(keyBlockCount, EncodeSlotKey(s.blockCount))
		meta.Put(keyTransactionCount, EncodeSlotKey(s.transactionCount))
		return nil
	})

	return s.db.Close()
}

// Verify interface compliance.
var _ Store = (*BoltStore)(nil)
