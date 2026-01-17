package blockstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

func TestBlockstore(t *testing.T) {
	// Create temporary directory for test database.
	tmpDir, err := os.MkdirTemp("", "blockstore_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Open blockstore.
	config := DefaultConfig(filepath.Join(tmpDir, "blockstore.db"))
	config.PruneEnabled = false // Disable for testing.

	store, err := Open(config)
	if err != nil {
		t.Fatalf("failed to open blockstore: %v", err)
	}
	defer store.Close()

	// Create test data.
	blockTime := time.Now().Unix()
	blockHeight := uint64(100)

	testPubkey, _ := types.PubkeyFromBytes(make([]byte, 32))
	testSig, _ := types.SignatureFromBytes(make([]byte, 64))
	testHash, _ := types.HashFromBytes(make([]byte, 32))

	block := &Block{
		Slot:              1000,
		ParentSlot:        999,
		Blockhash:         testHash,
		PreviousBlockhash: testHash,
		BlockTime:         &blockTime,
		BlockHeight:       &blockHeight,
		Transactions: []Transaction{
			{
				Signature:  testSig,
				Signatures: []types.Signature{testSig},
				Message: TransactionMessage{
					AccountKeys:     []types.Pubkey{testPubkey},
					RecentBlockhash: testHash,
					Instructions: []Instruction{
						{
							ProgramIDIndex: 0,
							AccountIndexes: []uint8{0},
							Data:           []byte{1, 2, 3, 4},
						},
					},
					Header: MessageHeader{
						NumRequiredSignatures:       1,
						NumReadonlySignedAccounts:   0,
						NumReadonlyUnsignedAccounts: 0,
					},
				},
				Meta: &TransactionMeta{
					Fee:                  5000,
					PreBalances:          []uint64{1000000},
					PostBalances:         []uint64{995000},
					LogMessages:          []string{"Program log: Hello"},
					ComputeUnitsConsumed: 1000,
				},
			},
		},
	}

	// Test PutBlock.
	t.Run("PutBlock", func(t *testing.T) {
		if err := store.PutBlock(block); err != nil {
			t.Fatalf("failed to put block: %v", err)
		}
	})

	// Test GetBlock.
	t.Run("GetBlock", func(t *testing.T) {
		retrieved, err := store.GetBlock(1000)
		if err != nil {
			t.Fatalf("failed to get block: %v", err)
		}
		if retrieved.Slot != 1000 {
			t.Errorf("expected slot 1000, got %d", retrieved.Slot)
		}
		if len(retrieved.Transactions) != 1 {
			t.Errorf("expected 1 transaction, got %d", len(retrieved.Transactions))
		}
	})

	// Test HasBlock.
	t.Run("HasBlock", func(t *testing.T) {
		if !store.HasBlock(1000) {
			t.Error("expected block to exist")
		}
		if store.HasBlock(9999) {
			t.Error("expected block to not exist")
		}
	})

	// Test GetSlotMeta.
	t.Run("GetSlotMeta", func(t *testing.T) {
		meta, err := store.GetSlotMeta(1000)
		if err != nil {
			t.Fatalf("failed to get slot meta: %v", err)
		}
		if meta.TransactionCount != 1 {
			t.Errorf("expected 1 transaction, got %d", meta.TransactionCount)
		}
	})

	// Test GetTransaction.
	t.Run("GetTransaction", func(t *testing.T) {
		txn, err := store.GetTransaction(testSig)
		if err != nil {
			t.Fatalf("failed to get transaction: %v", err)
		}
		if txn.Meta.Fee != 5000 {
			t.Errorf("expected fee 5000, got %d", txn.Meta.Fee)
		}
	})

	// Test SetCommitment.
	t.Run("SetCommitment", func(t *testing.T) {
		if err := store.SetCommitment(1000, CommitmentConfirmed); err != nil {
			t.Fatalf("failed to set commitment: %v", err)
		}
		meta, _ := store.GetSlotMeta(1000)
		if meta.Commitment != CommitmentConfirmed {
			t.Errorf("expected confirmed, got %v", meta.Commitment)
		}
	})

	// Test SetRoot.
	t.Run("SetRoot", func(t *testing.T) {
		if err := store.SetRoot(1000); err != nil {
			t.Fatalf("failed to set root: %v", err)
		}
		if !store.IsRoot(1000) {
			t.Error("expected slot to be root")
		}
	})

	// Test GetRoots.
	t.Run("GetRoots", func(t *testing.T) {
		roots, err := store.GetRoots(0, 2000)
		if err != nil {
			t.Fatalf("failed to get roots: %v", err)
		}
		if len(roots) != 1 || roots[0] != 1000 {
			t.Errorf("expected [1000], got %v", roots)
		}
	})

	// Test GetSignaturesForAddress.
	t.Run("GetSignaturesForAddress", func(t *testing.T) {
		sigs, err := store.GetSignaturesForAddress(testPubkey, nil)
		if err != nil {
			t.Fatalf("failed to get signatures: %v", err)
		}
		if len(sigs) != 1 {
			t.Errorf("expected 1 signature, got %d", len(sigs))
		}
	})

	// Test GetStats.
	t.Run("GetStats", func(t *testing.T) {
		stats, err := store.GetStats()
		if err != nil {
			t.Fatalf("failed to get stats: %v", err)
		}
		if stats.LatestSlot != 1000 {
			t.Errorf("expected latest slot 1000, got %d", stats.LatestSlot)
		}
		if stats.BlockCount != 1 {
			t.Errorf("expected 1 block, got %d", stats.BlockCount)
		}
	})

	// Test DeleteBlock.
	t.Run("DeleteBlock", func(t *testing.T) {
		if err := store.DeleteBlock(1000); err != nil {
			t.Fatalf("failed to delete block: %v", err)
		}
		if store.HasBlock(1000) {
			t.Error("expected block to be deleted")
		}
	})
}

func TestIndexer(t *testing.T) {
	// Create temporary directory for test database.
	tmpDir, err := os.MkdirTemp("", "blockstore_indexer_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Open blockstore.
	config := DefaultConfig(filepath.Join(tmpDir, "blockstore.db"))
	config.PruneEnabled = false

	store, err := Open(config)
	if err != nil {
		t.Fatalf("failed to open blockstore: %v", err)
	}
	defer store.Close()

	// Create indexer.
	idxConfig := DefaultIndexConfig()
	idxConfig.IndexPrograms = true
	indexer, err := NewIndexer(store, idxConfig)
	if err != nil {
		t.Fatalf("failed to create indexer: %v", err)
	}

	// Create test data.
	testPubkey, _ := types.PubkeyFromBytes(make([]byte, 32))
	testSig, _ := types.SignatureFromBytes(make([]byte, 64))
	testHash, _ := types.HashFromBytes(make([]byte, 32))
	blockTime := time.Now().Unix()

	block := &Block{
		Slot:      2000,
		Blockhash: testHash,
		BlockTime: &blockTime,
		Transactions: []Transaction{
			{
				Signature: testSig,
				Message: TransactionMessage{
					AccountKeys: []types.Pubkey{testPubkey},
					Instructions: []Instruction{
						{ProgramIDIndex: 0, Data: []byte{1}},
					},
				},
			},
		},
	}

	// Store block and index.
	if err := store.PutBlock(block); err != nil {
		t.Fatalf("failed to put block: %v", err)
	}
	if err := indexer.IndexBlock(block); err != nil {
		t.Fatalf("failed to index block: %v", err)
	}

	// Test GetTransactionsForSlot.
	t.Run("GetTransactionsForSlot", func(t *testing.T) {
		sigs, err := indexer.GetTransactionsForSlot(2000)
		if err != nil {
			t.Fatalf("failed to get transactions: %v", err)
		}
		if len(sigs) != 1 {
			t.Errorf("expected 1 signature, got %d", len(sigs))
		}
	})

	// Test GetQuickStatus.
	t.Run("GetQuickStatus", func(t *testing.T) {
		status, err := indexer.GetQuickStatus(testSig)
		if err != nil {
			t.Fatalf("failed to get status: %v", err)
		}
		if status.Slot != 2000 {
			t.Errorf("expected slot 2000, got %d", status.Slot)
		}
	})

	// Test UpdateCommitmentForSlot.
	t.Run("UpdateCommitmentForSlot", func(t *testing.T) {
		if err := indexer.UpdateCommitmentForSlot(2000, CommitmentFinalized); err != nil {
			t.Fatalf("failed to update commitment: %v", err)
		}
		status, _ := indexer.GetQuickStatus(testSig)
		if status.ConfirmationStatus != CommitmentFinalized {
			t.Errorf("expected finalized, got %v", status.ConfirmationStatus)
		}
	})

	// Test GetTransactionsByProgram.
	t.Run("GetTransactionsByProgram", func(t *testing.T) {
		sigs, err := indexer.GetTransactionsByProgram(testPubkey, nil)
		if err != nil {
			t.Fatalf("failed to get program transactions: %v", err)
		}
		if len(sigs) != 1 {
			t.Errorf("expected 1 signature, got %d", len(sigs))
		}
	})
}

func TestCommitmentLevelString(t *testing.T) {
	tests := []struct {
		level    CommitmentLevel
		expected string
	}{
		{CommitmentProcessed, "processed"},
		{CommitmentConfirmed, "confirmed"},
		{CommitmentFinalized, "finalized"},
		{CommitmentLevel(99), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			if tc.level.String() != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, tc.level.String())
			}
		})
	}
}

func TestKeyEncoding(t *testing.T) {
	// Test slot key encoding.
	t.Run("SlotKey", func(t *testing.T) {
		slot := uint64(12345678)
		encoded := EncodeSlotKey(slot)
		decoded := DecodeSlotKey(encoded)
		if decoded != slot {
			t.Errorf("expected %d, got %d", slot, decoded)
		}
	})

	// Test address+slot key encoding.
	t.Run("AddressSlotKey", func(t *testing.T) {
		addr, _ := types.PubkeyFromBytes(make([]byte, 32))
		slot := uint64(99999)
		encoded := EncodeAddressSlotKey(addr, slot)
		decodedAddr, decodedSlot := DecodeAddressSlotKey(encoded)
		if decodedAddr != addr || decodedSlot != slot {
			t.Errorf("address/slot mismatch")
		}
	})
}
