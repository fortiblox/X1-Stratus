// Package blockstore provides persistent storage for Solana blocks and transactions.
package blockstore

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"sync"

	"github.com/fortiblox/X1-Stratus/internal/types"
	bolt "go.etcd.io/bbolt"
)

// Additional bucket names for indexing.
var (
	// bucketTxStatus stores transaction status separately for faster lookups.
	bucketTxStatus = []byte("tx_status")

	// bucketSlotTransactions maps slot -> list of transaction signatures.
	bucketSlotTransactions = []byte("slot_txs")

	// bucketProgramLogs stores program invocation logs by program ID.
	bucketProgramLogs = []byte("program_logs")
)

// IndexConfig configures which indexes to maintain.
type IndexConfig struct {
	// IndexAddresses enables address-to-signature indexing.
	IndexAddresses bool

	// IndexPrograms enables program-to-transaction indexing.
	IndexPrograms bool

	// IndexMemos enables memo extraction and indexing.
	IndexMemos bool

	// MaxAddressesPerTx limits indexed addresses to prevent index bloat.
	MaxAddressesPerTx int
}

// DefaultIndexConfig returns sensible default indexing options.
func DefaultIndexConfig() IndexConfig {
	return IndexConfig{
		IndexAddresses:    true,
		IndexPrograms:     false, // Disabled by default for space savings.
		IndexMemos:        false,
		MaxAddressesPerTx: 64, // Limit to prevent excessive indexing.
	}
}

// Indexer provides extended indexing capabilities for the blockstore.
type Indexer struct {
	store  *BoltStore
	config IndexConfig
	mu     sync.RWMutex
}

// NewIndexer creates a new indexer for the given store.
func NewIndexer(store *BoltStore, config IndexConfig) (*Indexer, error) {
	idx := &Indexer{
		store:  store,
		config: config,
	}

	// Create additional buckets.
	err := store.db.Update(func(tx *bolt.Tx) error {
		buckets := [][]byte{
			bucketTxStatus,
			bucketSlotTransactions,
		}
		if config.IndexPrograms {
			buckets = append(buckets, bucketProgramLogs)
		}
		for _, name := range buckets {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return fmt.Errorf("create bucket %s: %w", name, err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return idx, nil
}

// IndexBlock indexes all transactions in a block.
// This is typically called after PutBlock for additional indexes.
func (idx *Indexer) IndexBlock(block *Block) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	return idx.store.db.Update(func(tx *bolt.Tx) error {
		// Index slot -> signatures mapping.
		slotTxs := tx.Bucket(bucketSlotTransactions)
		txStatus := tx.Bucket(bucketTxStatus)

		// Collect all signatures for this slot.
		signatures := make([]types.Signature, 0, len(block.Transactions))
		for _, txn := range block.Transactions {
			signatures = append(signatures, txn.Signature)
		}

		// Store slot -> signatures mapping.
		var sigsBuf bytes.Buffer
		if err := gob.NewEncoder(&sigsBuf).Encode(signatures); err != nil {
			return fmt.Errorf("encode signatures: %w", err)
		}
		if err := slotTxs.Put(EncodeSlotKey(block.Slot), sigsBuf.Bytes()); err != nil {
			return err
		}

		// Store individual transaction statuses.
		for _, txn := range block.Transactions {
			status := TransactionStatus{
				Slot:               block.Slot,
				Signature:          txn.Signature,
				ConfirmationStatus: CommitmentProcessed,
			}
			if txn.Meta != nil && txn.Meta.Err != nil {
				status.Err = txn.Meta.Err
			}

			var statusBuf bytes.Buffer
			if err := gob.NewEncoder(&statusBuf).Encode(&status); err != nil {
				return fmt.Errorf("encode status: %w", err)
			}
			if err := txStatus.Put(EncodeSignatureKey(txn.Signature), statusBuf.Bytes()); err != nil {
				return err
			}
		}

		// Index program invocations if enabled.
		if idx.config.IndexPrograms {
			programLogs := tx.Bucket(bucketProgramLogs)
			for _, txn := range block.Transactions {
				for _, inst := range txn.Message.Instructions {
					if int(inst.ProgramIDIndex) < len(txn.Message.AccountKeys) {
						programID := txn.Message.AccountKeys[inst.ProgramIDIndex]
						key := EncodeProgramSlotKey(programID, block.Slot, txn.Signature)
						if err := programLogs.Put(key, []byte{1}); err != nil {
							return err
						}
					}
				}
			}
		}

		return nil
	})
}

// GetTransactionsForSlot returns all transaction signatures in a slot.
func (idx *Indexer) GetTransactionsForSlot(slot uint64) ([]types.Signature, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.getTransactionsForSlotLocked(slot)
}

// getTransactionsForSlotLocked returns all transaction signatures in a slot.
// Caller must hold at least a read lock on idx.mu.
func (idx *Indexer) getTransactionsForSlotLocked(slot uint64) ([]types.Signature, error) {
	var signatures []types.Signature

	err := idx.store.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSlotTransactions)
		if b == nil {
			return nil
		}

		data := b.Get(EncodeSlotKey(slot))
		if data == nil {
			return nil
		}

		return gob.NewDecoder(bytes.NewReader(data)).Decode(&signatures)
	})

	if err != nil {
		return nil, err
	}
	return signatures, nil
}

// GetTransactionsByProgram returns transactions that invoked a specific program.
func (idx *Indexer) GetTransactionsByProgram(programID types.Pubkey, opts *ProgramQueryOptions) ([]types.Signature, error) {
	if !idx.config.IndexPrograms {
		return nil, fmt.Errorf("program indexing not enabled")
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if opts == nil {
		opts = &ProgramQueryOptions{Limit: 1000}
	}
	if opts.Limit <= 0 {
		opts.Limit = 1000
	}

	var results []types.Signature

	err := idx.store.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProgramLogs)
		if b == nil {
			return nil
		}

		c := b.Cursor()
		prefix := programID[:]

		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			// Decode the signature from the key.
			if len(k) >= 32+8+64 { // program (32) + slot (8) + signature (64)
				var sig types.Signature
				copy(sig[:], k[40:])
				results = append(results, sig)

				if len(results) >= opts.Limit {
					break
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// ProgramQueryOptions configures program transaction queries.
type ProgramQueryOptions struct {
	// Limit is the maximum number of signatures to return.
	Limit int

	// MinSlot filters to transactions in slots >= MinSlot.
	MinSlot *uint64

	// MaxSlot filters to transactions in slots <= MaxSlot.
	MaxSlot *uint64
}

// UpdateCommitmentForSlot updates the commitment level for all transactions in a slot.
func (idx *Indexer) UpdateCommitmentForSlot(slot uint64, commitment CommitmentLevel) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	signatures, err := idx.getTransactionsForSlotLocked(slot)
	if err != nil {
		return err
	}

	return idx.store.db.Update(func(tx *bolt.Tx) error {
		txStatus := tx.Bucket(bucketTxStatus)
		if txStatus == nil {
			return nil
		}

		for _, sig := range signatures {
			key := EncodeSignatureKey(sig)
			data := txStatus.Get(key)
			if data == nil {
				continue
			}

			var status TransactionStatus
			if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&status); err != nil {
				continue
			}

			status.ConfirmationStatus = commitment

			var buf bytes.Buffer
			if err := gob.NewEncoder(&buf).Encode(&status); err != nil {
				continue
			}
			if err := txStatus.Put(key, buf.Bytes()); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetQuickStatus returns just the transaction status without full transaction data.
func (idx *Indexer) GetQuickStatus(signature types.Signature) (*TransactionStatus, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var status TransactionStatus

	err := idx.store.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketTxStatus)
		if b == nil {
			return ErrTransactionNotFound
		}

		data := b.Get(EncodeSignatureKey(signature))
		if data == nil {
			return ErrTransactionNotFound
		}

		return gob.NewDecoder(bytes.NewReader(data)).Decode(&status)
	})

	if err != nil {
		return nil, err
	}
	return &status, nil
}

// BatchIndexTransactions indexes multiple transactions efficiently.
func (idx *Indexer) BatchIndexTransactions(slot uint64, txns []Transaction, blockTime *int64) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	return idx.store.db.Update(func(tx *bolt.Tx) error {
		txBySig := tx.Bucket(bucketTxBySignature)
		addrSigs := tx.Bucket(bucketAddressSignatures)
		txStatus := tx.Bucket(bucketTxStatus)
		slotTxs := tx.Bucket(bucketSlotTransactions)

		// Collect signatures for slot index.
		signatures := make([]types.Signature, 0, len(txns))

		for i := range txns {
			txn := &txns[i]
			txn.Slot = slot
			signatures = append(signatures, txn.Signature)

			sigKey := EncodeSignatureKey(txn.Signature)

			// Store full transaction.
			var txBuf bytes.Buffer
			if err := gob.NewEncoder(&txBuf).Encode(txn); err != nil {
				return fmt.Errorf("encode transaction: %w", err)
			}
			if err := txBySig.Put(sigKey, txBuf.Bytes()); err != nil {
				return err
			}

			// Store transaction status.
			status := TransactionStatus{
				Slot:               slot,
				Signature:          txn.Signature,
				ConfirmationStatus: CommitmentProcessed,
			}
			if txn.Meta != nil && txn.Meta.Err != nil {
				status.Err = txn.Meta.Err
			}

			var statusBuf bytes.Buffer
			if err := gob.NewEncoder(&statusBuf).Encode(&status); err != nil {
				return fmt.Errorf("encode status: %w", err)
			}
			if err := txStatus.Put(sigKey, statusBuf.Bytes()); err != nil {
				return err
			}

			// Index by address.
			if idx.config.IndexAddresses {
				sigInfo := SignatureInfo{
					Signature: txn.Signature,
					Slot:      slot,
					BlockTime: blockTime,
				}
				if txn.Meta != nil && txn.Meta.Err != nil {
					sigInfo.Err = txn.Meta.Err
				}

				var sigInfoBuf bytes.Buffer
				if err := gob.NewEncoder(&sigInfoBuf).Encode(&sigInfo); err != nil {
					return fmt.Errorf("encode sig info: %w", err)
				}
				sigInfoData := sigInfoBuf.Bytes()

				// Limit addresses indexed per transaction.
				addressCount := len(txn.Message.AccountKeys)
				if idx.config.MaxAddressesPerTx > 0 && addressCount > idx.config.MaxAddressesPerTx {
					addressCount = idx.config.MaxAddressesPerTx
				}

				for j := 0; j < addressCount; j++ {
					addr := txn.Message.AccountKeys[j]
					addrSlotKey := EncodeAddressSlotKey(addr, slot)
					if err := addrSigs.Put(addrSlotKey, sigInfoData); err != nil {
						return err
					}
				}
			}
		}

		// Store slot -> signatures mapping.
		var sigsBuf bytes.Buffer
		if err := gob.NewEncoder(&sigsBuf).Encode(signatures); err != nil {
			return fmt.Errorf("encode signatures: %w", err)
		}
		return slotTxs.Put(EncodeSlotKey(slot), sigsBuf.Bytes())
	})
}

// RebuildAddressIndex rebuilds the address index from existing blocks.
// This is useful after enabling address indexing on an existing database.
func (idx *Indexer) RebuildAddressIndex(startSlot, endSlot uint64, progressFn func(slot uint64)) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	return idx.store.db.Update(func(tx *bolt.Tx) error {
		blocks := tx.Bucket(bucketBlocks)
		addrSigs := tx.Bucket(bucketAddressSignatures)

		if blocks == nil || addrSigs == nil {
			return nil
		}

		c := blocks.Cursor()
		startKey := EncodeSlotKey(startSlot)
		endKey := EncodeSlotKey(endSlot)

		for k, v := c.Seek(startKey); k != nil && bytes.Compare(k, endKey) <= 0; k, v = c.Next() {
			var block Block
			if err := gob.NewDecoder(bytes.NewReader(v)).Decode(&block); err != nil {
				continue // Skip corrupted blocks.
			}

			if progressFn != nil {
				progressFn(block.Slot)
			}

			for _, txn := range block.Transactions {
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
					continue
				}
				sigInfoData := sigInfoBuf.Bytes()

				for _, addr := range txn.Message.AccountKeys {
					addrSlotKey := EncodeAddressSlotKey(addr, block.Slot)
					if err := addrSigs.Put(addrSlotKey, sigInfoData); err != nil {
						return err
					}
				}
			}
		}

		return nil
	})
}

// DeleteIndexesForSlot removes all indexes for a slot.
// This is called during pruning to clean up index data.
func (idx *Indexer) DeleteIndexesForSlot(slot uint64) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Get signatures for this slot first (use locked version to avoid deadlock).
	signatures, err := idx.getTransactionsForSlotLocked(slot)
	if err != nil {
		return err
	}

	return idx.store.db.Update(func(tx *bolt.Tx) error {
		slotKey := EncodeSlotKey(slot)

		// Delete slot -> signatures mapping.
		slotTxs := tx.Bucket(bucketSlotTransactions)
		if slotTxs != nil {
			if err := slotTxs.Delete(slotKey); err != nil {
				return err
			}
		}

		// Delete transaction statuses.
		txStatus := tx.Bucket(bucketTxStatus)
		if txStatus != nil {
			for _, sig := range signatures {
				if err := txStatus.Delete(EncodeSignatureKey(sig)); err != nil {
					return err
				}
			}
		}

		// Delete program logs if indexed.
		if idx.config.IndexPrograms {
			programLogs := tx.Bucket(bucketProgramLogs)
			if programLogs != nil {
				// We need to scan for keys with this slot.
				c := programLogs.Cursor()
				var keysToDelete [][]byte
				for k, _ := c.First(); k != nil; k, _ = c.Next() {
					// Key format: [32-byte program][8-byte slot][64-byte signature]
					if len(k) >= 40 {
						keySlot := binary.BigEndian.Uint64(k[32:40])
						if keySlot == slot {
							keysToDelete = append(keysToDelete, append([]byte{}, k...))
						}
					}
				}
				for _, k := range keysToDelete {
					if err := programLogs.Delete(k); err != nil {
						return err
					}
				}
			}
		}

		return nil
	})
}

// EncodeProgramSlotKey encodes a program+slot+signature composite key.
// Format: [32-byte program][8-byte slot big-endian][64-byte signature]
func EncodeProgramSlotKey(program types.Pubkey, slot uint64, sig types.Signature) []byte {
	key := make([]byte, 104) // 32 + 8 + 64
	copy(key[:32], program[:])
	binary.BigEndian.PutUint64(key[32:40], slot)
	copy(key[40:], sig[:])
	return key
}

// SlotIterator provides iteration over slots in the blockstore.
type SlotIterator struct {
	store     *BoltStore
	tx        *bolt.Tx
	cursor    *bolt.Cursor
	started   bool
	ascending bool
}

// NewSlotIterator creates an iterator over slots.
func (s *BoltStore) NewSlotIterator(ascending bool) (*SlotIterator, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrClosed
	}
	s.mu.RUnlock()

	tx, err := s.db.Begin(false)
	if err != nil {
		return nil, err
	}

	bucket := tx.Bucket(bucketSlotMeta)
	if bucket == nil {
		tx.Rollback()
		return nil, fmt.Errorf("slot_meta bucket not found")
	}

	return &SlotIterator{
		store:     s,
		tx:        tx,
		cursor:    bucket.Cursor(),
		ascending: ascending,
	}, nil
}

// Next advances the iterator and returns the next slot metadata.
// Returns nil when iteration is complete.
func (it *SlotIterator) Next() (*SlotMeta, error) {
	var k, v []byte

	if !it.started {
		it.started = true
		if it.ascending {
			k, v = it.cursor.First()
		} else {
			k, v = it.cursor.Last()
		}
	} else {
		if it.ascending {
			k, v = it.cursor.Next()
		} else {
			k, v = it.cursor.Prev()
		}
	}

	if k == nil {
		return nil, nil // End of iteration.
	}

	var meta SlotMeta
	if err := gob.NewDecoder(bytes.NewReader(v)).Decode(&meta); err != nil {
		return nil, fmt.Errorf("decode slot meta: %w", err)
	}

	return &meta, nil
}

// Close releases resources held by the iterator.
func (it *SlotIterator) Close() error {
	return it.tx.Rollback()
}

// CompactIndexes compacts the index buckets to reclaim space.
// This should be called periodically after heavy pruning.
func (s *BoltStore) CompactIndexes() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrClosed
	}

	// BoltDB doesn't have explicit compaction like RocksDB.
	// The best we can do is trigger a sync and let the OS
	// handle file space management.
	return s.db.Sync()
}
