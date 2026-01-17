// Package replayer implements tests for the transaction replay engine.
package replayer

import (
	"encoding/binary"
	"testing"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
	"github.com/fortiblox/X1-Stratus/pkg/blockstore"
)

// mockBlockStore implements blockstore.Store for testing.
type mockBlockStore struct {
	blocks        map[uint64]*blockstore.Block
	latestSlot    uint64
	finalizedSlot uint64
	confirmedSlot uint64
}

func newMockBlockStore() *mockBlockStore {
	return &mockBlockStore{
		blocks: make(map[uint64]*blockstore.Block),
	}
}

func (m *mockBlockStore) GetBlock(slot uint64) (*blockstore.Block, error) {
	block, ok := m.blocks[slot]
	if !ok {
		return nil, blockstore.ErrBlockNotFound
	}
	return block, nil
}

func (m *mockBlockStore) PutBlock(block *blockstore.Block) error {
	m.blocks[block.Slot] = block
	if block.Slot > m.latestSlot {
		m.latestSlot = block.Slot
	}
	return nil
}

func (m *mockBlockStore) HasBlock(slot uint64) bool {
	_, ok := m.blocks[slot]
	return ok
}

func (m *mockBlockStore) DeleteBlock(slot uint64) error {
	delete(m.blocks, slot)
	return nil
}

func (m *mockBlockStore) GetSlotMeta(slot uint64) (*blockstore.SlotMeta, error) {
	block, ok := m.blocks[slot]
	if !ok {
		return nil, blockstore.ErrSlotNotFound
	}
	return &blockstore.SlotMeta{
		Slot:              block.Slot,
		ParentSlot:        block.ParentSlot,
		Blockhash:         block.Blockhash,
		PreviousBlockhash: block.PreviousBlockhash,
		TransactionCount:  uint64(len(block.Transactions)),
		IsComplete:        true,
	}, nil
}

func (m *mockBlockStore) PutSlotMeta(meta *blockstore.SlotMeta) error {
	return nil
}

func (m *mockBlockStore) SetCommitment(slot uint64, commitment blockstore.CommitmentLevel) error {
	return nil
}

func (m *mockBlockStore) GetTransaction(signature types.Signature) (*blockstore.Transaction, error) {
	for _, block := range m.blocks {
		for i := range block.Transactions {
			if block.Transactions[i].Signature == signature {
				return &block.Transactions[i], nil
			}
		}
	}
	return nil, blockstore.ErrTransactionNotFound
}

func (m *mockBlockStore) GetTransactionStatus(signature types.Signature) (*blockstore.TransactionStatus, error) {
	return nil, blockstore.ErrTransactionNotFound
}

func (m *mockBlockStore) GetSignaturesForAddress(address types.Pubkey, opts *blockstore.SignatureQueryOptions) ([]blockstore.SignatureInfo, error) {
	return nil, nil
}

func (m *mockBlockStore) GetLatestSlot() uint64       { return m.latestSlot }
func (m *mockBlockStore) GetFinalizedSlot() uint64    { return m.finalizedSlot }
func (m *mockBlockStore) GetConfirmedSlot() uint64    { return m.confirmedSlot }
func (m *mockBlockStore) GetOldestSlot() uint64       { return 0 }
func (m *mockBlockStore) SetLatestSlot(slot uint64) error { m.latestSlot = slot; return nil }
func (m *mockBlockStore) SetFinalizedSlot(slot uint64) error { m.finalizedSlot = slot; return nil }
func (m *mockBlockStore) SetConfirmedSlot(slot uint64) error { m.confirmedSlot = slot; return nil }
func (m *mockBlockStore) IsRoot(slot uint64) bool     { return false }
func (m *mockBlockStore) SetRoot(slot uint64) error   { return nil }
func (m *mockBlockStore) GetRoots(start, end uint64) ([]uint64, error) { return nil, nil }
func (m *mockBlockStore) Prune(keepSlots uint64) (uint64, error) { return 0, nil }
func (m *mockBlockStore) GetStats() (*blockstore.Stats, error) { return &blockstore.Stats{}, nil }
func (m *mockBlockStore) Sync() error                 { return nil }
func (m *mockBlockStore) Close() error                { return nil }

// Verify mockBlockStore implements Store interface
var _ blockstore.Store = (*mockBlockStore)(nil)

// Helper functions for creating test data

func createTestPubkey(seed byte) types.Pubkey {
	var pubkey types.Pubkey
	for i := range pubkey {
		pubkey[i] = seed
	}
	return pubkey
}

func createTestSignature(seed byte) types.Signature {
	var sig types.Signature
	for i := range sig {
		sig[i] = seed
	}
	return sig
}

func createTestHash(seed byte) types.Hash {
	var hash types.Hash
	for i := range hash {
		hash[i] = seed
	}
	return hash
}

// createTransferInstruction creates a System Program transfer instruction.
func createTransferInstruction(amount uint64) []byte {
	data := make([]byte, 12) // 4 bytes instruction + 8 bytes amount
	binary.LittleEndian.PutUint32(data[0:4], 2) // Transfer instruction = 2
	binary.LittleEndian.PutUint64(data[4:12], amount)
	return data
}

// createCreateAccountInstruction creates a System Program CreateAccount instruction.
func createCreateAccountInstruction(lamports, space uint64, owner types.Pubkey) []byte {
	data := make([]byte, 52) // 4 bytes instruction + 8 bytes lamports + 8 bytes space + 32 bytes owner
	binary.LittleEndian.PutUint32(data[0:4], 0) // CreateAccount instruction = 0
	binary.LittleEndian.PutUint64(data[4:12], lamports)
	binary.LittleEndian.PutUint64(data[12:20], space)
	copy(data[20:52], owner[:])
	return data
}

// TestNewTransactionExecutor tests executor creation.
func TestNewTransactionExecutor(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	executor := NewTransactionExecutor(db)

	if executor == nil {
		t.Fatal("expected non-nil executor")
	}

	if executor.accounts == nil {
		t.Error("executor accounts DB is nil")
	}

	if executor.systemProcessor == nil {
		t.Error("executor system processor is nil")
	}
}

// TestExecuteSystemTransfer tests executing a simple SOL transfer.
func TestExecuteSystemTransfer(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	// Create test accounts
	senderPubkey := createTestPubkey(1)
	recipientPubkey := createTestPubkey(2)

	// Fund sender account
	senderAccount := &accounts.Account{
		Lamports: 1_000_000_000, // 1 SOL
		Owner:    types.Pubkey{}, // System Program
	}
	err := db.SetAccount(senderPubkey, senderAccount)
	if err != nil {
		t.Fatalf("failed to set sender account: %v", err)
	}

	// Create recipient account (empty)
	recipientAccount := &accounts.Account{
		Lamports: 0,
		Owner:    types.Pubkey{}, // System Program
	}
	err = db.SetAccount(recipientPubkey, recipientAccount)
	if err != nil {
		t.Fatalf("failed to set recipient account: %v", err)
	}

	executor := NewTransactionExecutor(db)

	// Create transfer transaction
	transferAmount := uint64(500_000_000) // 0.5 SOL
	tx := &blockstore.Transaction{
		Signature:  createTestSignature(1),
		Signatures: []types.Signature{createTestSignature(1)},
		Message: blockstore.TransactionMessage{
			AccountKeys: []types.Pubkey{senderPubkey, recipientPubkey, types.Pubkey{}}, // sender, recipient, system program
			Header: blockstore.MessageHeader{
				NumRequiredSignatures:       1,
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: 1, // System program is readonly
			},
			Instructions: []blockstore.Instruction{
				{
					ProgramIDIndex: 2, // System program
					AccountIndexes: []uint8{0, 1}, // sender, recipient
					Data:           createTransferInstruction(transferAmount),
				},
			},
		},
		Slot: 1,
	}

	result, err := executor.Execute(tx)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Verify balances
	updatedSender, err := db.GetAccount(senderPubkey)
	if err != nil {
		t.Fatalf("failed to get sender account: %v", err)
	}

	updatedRecipient, err := db.GetAccount(recipientPubkey)
	if err != nil {
		t.Fatalf("failed to get recipient account: %v", err)
	}

	expectedSenderBalance := uint64(500_000_000)
	expectedRecipientBalance := uint64(500_000_000)

	if updatedSender.Lamports != expectedSenderBalance {
		t.Errorf("sender balance: expected %d, got %d", expectedSenderBalance, updatedSender.Lamports)
	}

	if updatedRecipient.Lamports != expectedRecipientBalance {
		t.Errorf("recipient balance: expected %d, got %d", expectedRecipientBalance, updatedRecipient.Lamports)
	}

	// Verify modified accounts were tracked
	if len(result.ModifiedAccounts) != 2 {
		t.Errorf("expected 2 modified accounts, got %d", len(result.ModifiedAccounts))
	}
}

// TestExecuteCreateAccount tests account creation instruction.
func TestExecuteCreateAccount(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	// Create funding account
	funderPubkey := createTestPubkey(1)
	newAccountPubkey := createTestPubkey(2)
	ownerPubkey := createTestPubkey(3) // Program that will own the new account

	funderAccount := &accounts.Account{
		Lamports: 10_000_000_000, // 10 SOL
		Owner:    types.Pubkey{}, // System Program
	}
	err := db.SetAccount(funderPubkey, funderAccount)
	if err != nil {
		t.Fatalf("failed to set funder account: %v", err)
	}

	executor := NewTransactionExecutor(db)

	// Create account with 1 SOL and 100 bytes of space
	createLamports := uint64(1_000_000_000)
	createSpace := uint64(100)

	tx := &blockstore.Transaction{
		Signature:  createTestSignature(1),
		Signatures: []types.Signature{createTestSignature(1), createTestSignature(2)},
		Message: blockstore.TransactionMessage{
			AccountKeys: []types.Pubkey{funderPubkey, newAccountPubkey, types.Pubkey{}},
			Header: blockstore.MessageHeader{
				NumRequiredSignatures:       2, // Both funder and new account must sign
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: 1,
			},
			Instructions: []blockstore.Instruction{
				{
					ProgramIDIndex: 2,
					AccountIndexes: []uint8{0, 1},
					Data:           createCreateAccountInstruction(createLamports, createSpace, ownerPubkey),
				},
			},
		},
		Slot: 1,
	}

	result, err := executor.Execute(tx)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Verify funder balance decreased
	updatedFunder, err := db.GetAccount(funderPubkey)
	if err != nil {
		t.Fatalf("failed to get funder account: %v", err)
	}

	expectedFunderBalance := uint64(9_000_000_000) // 10 - 1 SOL
	if updatedFunder.Lamports != expectedFunderBalance {
		t.Errorf("funder balance: expected %d, got %d", expectedFunderBalance, updatedFunder.Lamports)
	}

	// Verify new account was created
	newAccount, err := db.GetAccount(newAccountPubkey)
	if err != nil {
		t.Fatalf("failed to get new account: %v", err)
	}

	if newAccount.Lamports != createLamports {
		t.Errorf("new account lamports: expected %d, got %d", createLamports, newAccount.Lamports)
	}

	if uint64(len(newAccount.Data)) != createSpace {
		t.Errorf("new account data size: expected %d, got %d", createSpace, len(newAccount.Data))
	}

	if newAccount.Owner != ownerPubkey {
		t.Errorf("new account owner: expected %s, got %s", ownerPubkey.String(), newAccount.Owner.String())
	}
}

// TestExecuteFailedTransaction tests handling of failed transactions.
func TestExecuteFailedTransaction(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	// Create sender with insufficient funds
	senderPubkey := createTestPubkey(1)
	recipientPubkey := createTestPubkey(2)

	senderAccount := &accounts.Account{
		Lamports: 100_000, // Very small balance
		Owner:    types.Pubkey{},
	}
	err := db.SetAccount(senderPubkey, senderAccount)
	if err != nil {
		t.Fatalf("failed to set sender account: %v", err)
	}

	executor := NewTransactionExecutor(db)

	// Try to transfer more than available
	transferAmount := uint64(1_000_000_000) // 1 SOL (more than available)
	tx := &blockstore.Transaction{
		Signature:  createTestSignature(1),
		Signatures: []types.Signature{createTestSignature(1)},
		Message: blockstore.TransactionMessage{
			AccountKeys: []types.Pubkey{senderPubkey, recipientPubkey, types.Pubkey{}},
			Header: blockstore.MessageHeader{
				NumRequiredSignatures:       1,
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: 1,
			},
			Instructions: []blockstore.Instruction{
				{
					ProgramIDIndex: 2,
					AccountIndexes: []uint8{0, 1},
					Data:           createTransferInstruction(transferAmount),
				},
			},
		},
		Slot: 1,
	}

	result, err := executor.Execute(tx)
	if err != nil {
		t.Fatalf("execute returned unexpected error: %v", err)
	}

	// Transaction should fail, not error
	if result.Success {
		t.Error("expected transaction to fail due to insufficient funds")
	}

	if result.Error == "" {
		t.Error("expected error message for failed transaction")
	}

	// Verify sender balance unchanged
	updatedSender, err := db.GetAccount(senderPubkey)
	if err != nil {
		t.Fatalf("failed to get sender account: %v", err)
	}

	if updatedSender.Lamports != 100_000 {
		t.Errorf("sender balance should be unchanged: expected 100000, got %d", updatedSender.Lamports)
	}
}

// TestReplaySlot tests replaying a complete slot with multiple transactions.
func TestReplaySlot(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	blockStore := newMockBlockStore()

	// Create test accounts
	account1 := createTestPubkey(1)
	account2 := createTestPubkey(2)
	account3 := createTestPubkey(3)

	// Fund accounts
	db.SetAccount(account1, &accounts.Account{Lamports: 5_000_000_000, Owner: types.Pubkey{}})
	db.SetAccount(account2, &accounts.Account{Lamports: 5_000_000_000, Owner: types.Pubkey{}})

	// Create block with multiple transactions
	blockhash := createTestHash(100)
	parentBlockhash := createTestHash(99)

	block := &blockstore.Block{
		Slot:              1,
		ParentSlot:        0,
		Blockhash:         blockhash,
		PreviousBlockhash: parentBlockhash,
		Transactions: []blockstore.Transaction{
			{
				Signature:  createTestSignature(1),
				Signatures: []types.Signature{createTestSignature(1)},
				Message: blockstore.TransactionMessage{
					AccountKeys: []types.Pubkey{account1, account2, types.Pubkey{}},
					Header: blockstore.MessageHeader{
						NumRequiredSignatures:       1,
						NumReadonlySignedAccounts:   0,
						NumReadonlyUnsignedAccounts: 1,
					},
					Instructions: []blockstore.Instruction{
						{
							ProgramIDIndex: 2,
							AccountIndexes: []uint8{0, 1},
							Data:           createTransferInstruction(1_000_000_000),
						},
					},
				},
				Slot: 1,
			},
			{
				Signature:  createTestSignature(2),
				Signatures: []types.Signature{createTestSignature(2)},
				Message: blockstore.TransactionMessage{
					AccountKeys: []types.Pubkey{account2, account3, types.Pubkey{}},
					Header: blockstore.MessageHeader{
						NumRequiredSignatures:       1,
						NumReadonlySignedAccounts:   0,
						NumReadonlyUnsignedAccounts: 1,
					},
					Instructions: []blockstore.Instruction{
						{
							ProgramIDIndex: 2,
							AccountIndexes: []uint8{0, 1},
							Data:           createTransferInstruction(500_000_000),
						},
					},
				},
				Slot: 1,
			},
		},
	}

	err := blockStore.PutBlock(block)
	if err != nil {
		t.Fatalf("failed to put block: %v", err)
	}

	// Create replayer with initial state
	config := DefaultConfig()
	config.SkipSignatureVerification = true
	replayer := New(blockStore, db, config)

	// Initialize with parent blockhash (slot 0)
	err = replayer.Initialize(0, parentBlockhash, types.Hash{})
	if err != nil {
		t.Fatalf("failed to initialize replayer: %v", err)
	}

	// Replay slot 1
	result, err := replayer.ReplaySlot(1)
	if err != nil {
		t.Fatalf("replay slot failed: %v", err)
	}

	// Verify result
	if result.Slot != 1 {
		t.Errorf("result slot: expected 1, got %d", result.Slot)
	}

	if len(result.Transactions) != 2 {
		t.Errorf("expected 2 transaction results, got %d", len(result.Transactions))
	}

	// Check all transactions succeeded
	for i, txResult := range result.Transactions {
		if !txResult.Success {
			t.Errorf("transaction %d failed: %s", i, txResult.Error)
		}
	}

	// Verify final balances
	acc1, _ := db.GetAccount(account1)
	acc2, _ := db.GetAccount(account2)
	acc3, _ := db.GetAccount(account3)

	// account1: 5 SOL - 1 SOL = 4 SOL
	if acc1.Lamports != 4_000_000_000 {
		t.Errorf("account1 balance: expected 4000000000, got %d", acc1.Lamports)
	}

	// account2: 5 SOL + 1 SOL - 0.5 SOL = 5.5 SOL
	if acc2.Lamports != 5_500_000_000 {
		t.Errorf("account2 balance: expected 5500000000, got %d", acc2.Lamports)
	}

	// account3: 0 + 0.5 SOL = 0.5 SOL
	if acc3.Lamports != 500_000_000 {
		t.Errorf("account3 balance: expected 500000000, got %d", acc3.Lamports)
	}

	// Verify replayer state updated
	if replayer.CurrentSlot() != 1 {
		t.Errorf("current slot: expected 1, got %d", replayer.CurrentSlot())
	}

	if replayer.CurrentBlockhash() != blockhash {
		t.Errorf("current blockhash mismatch")
	}
}

// TestReplaySlotValidation tests slot validation (parent linkage, etc.).
func TestReplaySlotValidation(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	blockStore := newMockBlockStore()

	config := DefaultConfig()
	config.SkipSignatureVerification = true
	replayer := New(blockStore, db, config)

	// Test 1: Missing block
	t.Run("MissingBlock", func(t *testing.T) {
		err := replayer.Initialize(0, createTestHash(1), types.Hash{})
		if err != nil {
			t.Fatalf("initialize failed: %v", err)
		}

		_, err = replayer.ReplaySlot(999)
		if err == nil {
			t.Error("expected error for missing block")
		}
	})

	// Test 2: Already processed slot
	t.Run("AlreadyProcessed", func(t *testing.T) {
		// Create and add a block
		block := &blockstore.Block{
			Slot:              5,
			ParentSlot:        4,
			Blockhash:         createTestHash(5),
			PreviousBlockhash: createTestHash(4),
			Transactions:      []blockstore.Transaction{},
		}
		blockStore.PutBlock(block)

		// Initialize replayer at slot 10 (past slot 5)
		err := replayer.Initialize(10, createTestHash(10), types.Hash{})
		if err != nil {
			t.Fatalf("initialize failed: %v", err)
		}

		_, err = replayer.ReplaySlot(5)
		if err == nil {
			t.Error("expected error for already processed slot")
		}
	})

	// Test 3: Parent blockhash mismatch
	t.Run("ParentBlockhashMismatch", func(t *testing.T) {
		parentHash := createTestHash(100)
		wrongParentHash := createTestHash(200)

		// Create block with specific previous blockhash
		block := &blockstore.Block{
			Slot:              101,
			ParentSlot:        100,
			Blockhash:         createTestHash(101),
			PreviousBlockhash: wrongParentHash, // This won't match replayer's current hash
			Transactions:      []blockstore.Transaction{},
		}
		blockStore.PutBlock(block)

		// Initialize replayer with different current blockhash
		err := replayer.Initialize(100, parentHash, types.Hash{})
		if err != nil {
			t.Fatalf("initialize failed: %v", err)
		}

		_, err = replayer.ReplaySlot(101)
		if err == nil {
			t.Error("expected error for parent blockhash mismatch")
		}
	})

	// Test 4: Valid slot replay
	t.Run("ValidSlotReplay", func(t *testing.T) {
		parentHash := createTestHash(50)

		block := &blockstore.Block{
			Slot:              51,
			ParentSlot:        50,
			Blockhash:         createTestHash(51),
			PreviousBlockhash: parentHash, // Matches replayer's current hash
			Transactions:      []blockstore.Transaction{},
		}
		blockStore.PutBlock(block)

		err := replayer.Initialize(50, parentHash, types.Hash{})
		if err != nil {
			t.Fatalf("initialize failed: %v", err)
		}

		result, err := replayer.ReplaySlot(51)
		if err != nil {
			t.Errorf("unexpected error for valid slot: %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})
}

// TestComputeBankHash tests bank hash computation.
func TestComputeBankHash(t *testing.T) {
	// Test the bank hash computation function
	parentBankhash := createTestHash(1)
	accountsDeltaHash := createTestHash(2)
	numSigs := uint64(5)
	blockhash := createTestHash(3)

	bankHash := computeBankHashFromComponents(parentBankhash, accountsDeltaHash, numSigs, blockhash)

	// Bank hash should not be zero
	if bankHash.IsZero() {
		t.Error("bank hash should not be zero")
	}

	// Same inputs should produce same output (deterministic)
	bankHash2 := computeBankHashFromComponents(parentBankhash, accountsDeltaHash, numSigs, blockhash)
	if bankHash != bankHash2 {
		t.Error("bank hash should be deterministic")
	}

	// Different inputs should produce different outputs
	differentParent := createTestHash(99)
	bankHash3 := computeBankHashFromComponents(differentParent, accountsDeltaHash, numSigs, blockhash)
	if bankHash == bankHash3 {
		t.Error("different inputs should produce different bank hashes")
	}

	// Different signature count should produce different hash
	bankHash4 := computeBankHashFromComponents(parentBankhash, accountsDeltaHash, numSigs+1, blockhash)
	if bankHash == bankHash4 {
		t.Error("different signature count should produce different bank hash")
	}
}

// TestIsAccountWritable tests the writable account detection.
func TestIsAccountWritable(t *testing.T) {
	testCases := []struct {
		name                   string
		index                  int
		numSigners             int
		numReadonlySigned      int
		numReadonlyUnsigned    int
		total                  int
		expectedWritable       bool
	}{
		{
			name:                "First signer (writable)",
			index:               0,
			numSigners:          2,
			numReadonlySigned:   0,
			numReadonlyUnsigned: 1,
			total:               5,
			expectedWritable:    true,
		},
		{
			name:                "Second signer (writable)",
			index:               1,
			numSigners:          2,
			numReadonlySigned:   0,
			numReadonlyUnsigned: 1,
			total:               5,
			expectedWritable:    true,
		},
		{
			name:                "Readonly signed account",
			index:               1,
			numSigners:          2,
			numReadonlySigned:   1,
			numReadonlyUnsigned: 1,
			total:               5,
			expectedWritable:    false,
		},
		{
			name:                "First non-signer (writable)",
			index:               2,
			numSigners:          2,
			numReadonlySigned:   0,
			numReadonlyUnsigned: 1,
			total:               5,
			expectedWritable:    true,
		},
		{
			name:                "Readonly unsigned account (program)",
			index:               4,
			numSigners:          2,
			numReadonlySigned:   0,
			numReadonlyUnsigned: 1,
			total:               5,
			expectedWritable:    false,
		},
		{
			name:                "System program position",
			index:               2,
			numSigners:          1,
			numReadonlySigned:   0,
			numReadonlyUnsigned: 1,
			total:               3,
			expectedWritable:    false,
		},
		{
			name:                "Sender in transfer",
			index:               0,
			numSigners:          1,
			numReadonlySigned:   0,
			numReadonlyUnsigned: 1,
			total:               3,
			expectedWritable:    true,
		},
		{
			name:                "Recipient in transfer",
			index:               1,
			numSigners:          1,
			numReadonlySigned:   0,
			numReadonlyUnsigned: 1,
			total:               3,
			expectedWritable:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isAccountWritable(tc.index, tc.numSigners, tc.numReadonlySigned, tc.numReadonlyUnsigned, tc.total)
			if result != tc.expectedWritable {
				t.Errorf("expected writable=%v, got %v", tc.expectedWritable, result)
			}
		})
	}
}

// TestReplayRange tests replaying a range of slots.
func TestReplayRange(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	blockStore := newMockBlockStore()

	// Create accounts
	account1 := createTestPubkey(1)
	account2 := createTestPubkey(2)
	db.SetAccount(account1, &accounts.Account{Lamports: 10_000_000_000, Owner: types.Pubkey{}})

	// Create a series of blocks
	previousHash := createTestHash(0)
	for slot := uint64(1); slot <= 5; slot++ {
		blockhash := createTestHash(byte(slot))
		block := &blockstore.Block{
			Slot:              slot,
			ParentSlot:        slot - 1,
			Blockhash:         blockhash,
			PreviousBlockhash: previousHash,
			Transactions: []blockstore.Transaction{
				{
					Signature:  createTestSignature(byte(slot)),
					Signatures: []types.Signature{createTestSignature(byte(slot))},
					Message: blockstore.TransactionMessage{
						AccountKeys: []types.Pubkey{account1, account2, types.Pubkey{}},
						Header: blockstore.MessageHeader{
							NumRequiredSignatures:       1,
							NumReadonlySignedAccounts:   0,
							NumReadonlyUnsignedAccounts: 1,
						},
						Instructions: []blockstore.Instruction{
							{
								ProgramIDIndex: 2,
								AccountIndexes: []uint8{0, 1},
								Data:           createTransferInstruction(100_000_000), // 0.1 SOL per slot
							},
						},
					},
					Slot: slot,
				},
			},
		}
		blockStore.PutBlock(block)
		previousHash = blockhash
	}

	config := DefaultConfig()
	config.SkipSignatureVerification = true
	replayer := New(blockStore, db, config)

	// Initialize at slot 0
	err := replayer.Initialize(0, createTestHash(0), types.Hash{})
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}

	// Replay slots 1-5
	results, err := replayer.ReplayRange(1, 5)
	if err != nil {
		t.Fatalf("replay range failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}

	// Verify each slot was replayed
	for i, result := range results {
		expectedSlot := uint64(i + 1)
		if result.Slot != expectedSlot {
			t.Errorf("result %d: expected slot %d, got %d", i, expectedSlot, result.Slot)
		}
		if len(result.Transactions) != 1 {
			t.Errorf("result %d: expected 1 transaction, got %d", i, len(result.Transactions))
		}
	}

	// Verify final state
	if replayer.CurrentSlot() != 5 {
		t.Errorf("current slot: expected 5, got %d", replayer.CurrentSlot())
	}

	// Verify final balances (5 * 0.1 SOL transferred)
	acc1, _ := db.GetAccount(account1)
	acc2, _ := db.GetAccount(account2)

	expectedAcc1 := uint64(10_000_000_000 - 5*100_000_000) // 9.5 SOL
	expectedAcc2 := uint64(5 * 100_000_000)                // 0.5 SOL

	if acc1.Lamports != expectedAcc1 {
		t.Errorf("account1 balance: expected %d, got %d", expectedAcc1, acc1.Lamports)
	}
	if acc2.Lamports != expectedAcc2 {
		t.Errorf("account2 balance: expected %d, got %d", expectedAcc2, acc2.Lamports)
	}
}

// TestReplayRangeValidation tests validation of replay range parameters.
func TestReplayRangeValidation(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	blockStore := newMockBlockStore()
	config := DefaultConfig()
	replayer := New(blockStore, db, config)

	// Test: end < start should fail
	t.Run("InvalidRange", func(t *testing.T) {
		_, err := replayer.ReplayRange(10, 5)
		if err == nil {
			t.Error("expected error for invalid range (end < start)")
		}
	})

	// Test: range too large
	t.Run("RangeTooLarge", func(t *testing.T) {
		_, err := replayer.ReplayRange(0, MaxSlotsPerReplay+1)
		if err == nil {
			t.Error("expected error for range exceeding maximum")
		}
	})
}

// TestReplayerCallbacks tests that callbacks are invoked correctly.
func TestReplayerCallbacks(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	blockStore := newMockBlockStore()

	// Create test accounts and block
	account1 := createTestPubkey(1)
	account2 := createTestPubkey(2)
	db.SetAccount(account1, &accounts.Account{Lamports: 1_000_000_000, Owner: types.Pubkey{}})

	block := &blockstore.Block{
		Slot:              1,
		ParentSlot:        0,
		Blockhash:         createTestHash(1),
		PreviousBlockhash: createTestHash(0),
		Transactions: []blockstore.Transaction{
			{
				Signature:  createTestSignature(1),
				Signatures: []types.Signature{createTestSignature(1)},
				Message: blockstore.TransactionMessage{
					AccountKeys: []types.Pubkey{account1, account2, types.Pubkey{}},
					Header: blockstore.MessageHeader{
						NumRequiredSignatures:       1,
						NumReadonlySignedAccounts:   0,
						NumReadonlyUnsignedAccounts: 1,
					},
					Instructions: []blockstore.Instruction{
						{
							ProgramIDIndex: 2,
							AccountIndexes: []uint8{0, 1},
							Data:           createTransferInstruction(100_000),
						},
					},
				},
				Slot: 1,
			},
		},
	}
	blockStore.PutBlock(block)

	// Track callback invocations
	var slotCompleteCalled bool
	var completedSlot uint64
	var completedBankHash types.Hash

	var txCompleteCalled bool
	var completedTxSig types.Signature
	var completedTxSuccess bool

	config := DefaultConfig()
	config.SkipSignatureVerification = true
	config.OnSlotComplete = func(slot uint64, bankHash types.Hash) {
		slotCompleteCalled = true
		completedSlot = slot
		completedBankHash = bankHash
	}
	config.OnTransactionComplete = func(sig types.Signature, success bool, logs []string) {
		txCompleteCalled = true
		completedTxSig = sig
		completedTxSuccess = success
	}

	replayer := New(blockStore, db, config)
	replayer.Initialize(0, createTestHash(0), types.Hash{})

	_, err := replayer.ReplaySlot(1)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}

	// Verify slot callback
	if !slotCompleteCalled {
		t.Error("OnSlotComplete callback was not called")
	}
	if completedSlot != 1 {
		t.Errorf("completed slot: expected 1, got %d", completedSlot)
	}
	if completedBankHash.IsZero() {
		t.Error("completed bank hash should not be zero")
	}

	// Verify transaction callback
	if !txCompleteCalled {
		t.Error("OnTransactionComplete callback was not called")
	}
	if completedTxSig != createTestSignature(1) {
		t.Error("completed transaction signature mismatch")
	}
	if !completedTxSuccess {
		t.Error("transaction should have succeeded")
	}
}

// TestReplayerStats tests the Stats() method.
func TestReplayerStats(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	blockStore := newMockBlockStore()

	// Create test accounts
	account1 := createTestPubkey(1)
	account2 := createTestPubkey(2)
	db.SetAccount(account1, &accounts.Account{Lamports: 1_000_000_000, Owner: types.Pubkey{}})
	db.SetAccount(account2, &accounts.Account{Lamports: 1_000_000_000, Owner: types.Pubkey{}})

	config := DefaultConfig()
	replayer := New(blockStore, db, config)

	// Check initial stats
	stats := replayer.Stats()
	if stats.CurrentSlot != 0 {
		t.Errorf("initial current slot: expected 0, got %d", stats.CurrentSlot)
	}
	if stats.AccountsCount != 2 {
		t.Errorf("accounts count: expected 2, got %d", stats.AccountsCount)
	}

	// After initialization
	replayer.Initialize(100, createTestHash(100), types.Hash{})
	stats = replayer.Stats()
	if stats.CurrentSlot != 100 {
		t.Errorf("current slot after init: expected 100, got %d", stats.CurrentSlot)
	}
}

// TestExecutorInvalidProgram tests handling of unknown programs.
func TestExecutorInvalidProgram(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	executor := NewTransactionExecutor(db)

	// Create a transaction with unknown program
	unknownProgram := createTestPubkey(99)
	account1 := createTestPubkey(1)
	db.SetAccount(account1, &accounts.Account{Lamports: 1_000_000_000, Owner: types.Pubkey{}})

	tx := &blockstore.Transaction{
		Signature:  createTestSignature(1),
		Signatures: []types.Signature{createTestSignature(1)},
		Message: blockstore.TransactionMessage{
			AccountKeys: []types.Pubkey{account1, unknownProgram},
			Header: blockstore.MessageHeader{
				NumRequiredSignatures:       1,
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: 1,
			},
			Instructions: []blockstore.Instruction{
				{
					ProgramIDIndex: 1, // Unknown program
					AccountIndexes: []uint8{0},
					Data:           []byte{0x01, 0x02, 0x03},
				},
			},
		},
		Slot: 1,
	}

	result, err := executor.Execute(tx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	// BPF programs are skipped, so this should succeed
	if !result.Success {
		t.Errorf("expected success (BPF programs are skipped), got error: %s", result.Error)
	}
}

// TestExecutorInvalidAccountIndex tests handling of invalid account indexes.
func TestExecutorInvalidAccountIndex(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	executor := NewTransactionExecutor(db)

	account1 := createTestPubkey(1)
	db.SetAccount(account1, &accounts.Account{Lamports: 1_000_000_000, Owner: types.Pubkey{}})

	// Transaction with invalid account index in instruction
	tx := &blockstore.Transaction{
		Signature:  createTestSignature(1),
		Signatures: []types.Signature{createTestSignature(1)},
		Message: blockstore.TransactionMessage{
			AccountKeys: []types.Pubkey{account1, types.Pubkey{}},
			Header: blockstore.MessageHeader{
				NumRequiredSignatures:       1,
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: 1,
			},
			Instructions: []blockstore.Instruction{
				{
					ProgramIDIndex: 1, // System program
					AccountIndexes: []uint8{0, 99}, // Invalid index 99
					Data:           createTransferInstruction(100),
				},
			},
		},
		Slot: 1,
	}

	result, err := executor.Execute(tx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	if result.Success {
		t.Error("expected failure for invalid account index")
	}
}

// TestExecutorInvalidProgramIndex tests handling of invalid program ID index.
func TestExecutorInvalidProgramIndex(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	executor := NewTransactionExecutor(db)

	account1 := createTestPubkey(1)
	db.SetAccount(account1, &accounts.Account{Lamports: 1_000_000_000, Owner: types.Pubkey{}})

	// Transaction with invalid program ID index
	tx := &blockstore.Transaction{
		Signature:  createTestSignature(1),
		Signatures: []types.Signature{createTestSignature(1)},
		Message: blockstore.TransactionMessage{
			AccountKeys: []types.Pubkey{account1},
			Header: blockstore.MessageHeader{
				NumRequiredSignatures:       1,
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: 0,
			},
			Instructions: []blockstore.Instruction{
				{
					ProgramIDIndex: 99, // Invalid program index
					AccountIndexes: []uint8{0},
					Data:           []byte{},
				},
			},
		},
		Slot: 1,
	}

	result, err := executor.Execute(tx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	if result.Success {
		t.Error("expected failure for invalid program ID index")
	}
}

// TestTransferZeroAmount tests transferring zero lamports.
func TestTransferZeroAmount(t *testing.T) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	senderPubkey := createTestPubkey(1)
	recipientPubkey := createTestPubkey(2)

	// Give recipient some initial balance so it's not a zero account
	db.SetAccount(senderPubkey, &accounts.Account{Lamports: 1_000_000_000, Owner: types.Pubkey{}})
	db.SetAccount(recipientPubkey, &accounts.Account{Lamports: 100, Owner: types.Pubkey{}})

	executor := NewTransactionExecutor(db)

	tx := &blockstore.Transaction{
		Signature:  createTestSignature(1),
		Signatures: []types.Signature{createTestSignature(1)},
		Message: blockstore.TransactionMessage{
			AccountKeys: []types.Pubkey{senderPubkey, recipientPubkey, types.Pubkey{}},
			Header: blockstore.MessageHeader{
				NumRequiredSignatures:       1,
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: 1,
			},
			Instructions: []blockstore.Instruction{
				{
					ProgramIDIndex: 2,
					AccountIndexes: []uint8{0, 1},
					Data:           createTransferInstruction(0), // Zero transfer
				},
			},
		},
		Slot: 1,
	}

	result, err := executor.Execute(tx)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	// Zero transfer should succeed
	if !result.Success {
		t.Errorf("zero transfer should succeed, got error: %s", result.Error)
	}

	// Verify balances unchanged
	sender, err := db.GetAccount(senderPubkey)
	if err != nil {
		t.Fatalf("failed to get sender account: %v", err)
	}
	recipient, err := db.GetAccount(recipientPubkey)
	if err != nil {
		t.Fatalf("failed to get recipient account: %v", err)
	}

	if sender.Lamports != 1_000_000_000 {
		t.Errorf("sender balance should be unchanged: expected 1000000000, got %d", sender.Lamports)
	}
	if recipient.Lamports != 100 {
		t.Errorf("recipient balance should be unchanged: expected 100, got %d", recipient.Lamports)
	}
}

// BenchmarkExecuteTransfer benchmarks transfer execution.
func BenchmarkExecuteTransfer(b *testing.B) {
	db := accounts.NewMemoryDB()
	defer db.Close()

	senderPubkey := createTestPubkey(1)
	recipientPubkey := createTestPubkey(2)

	db.SetAccount(senderPubkey, &accounts.Account{Lamports: uint64(b.N) * 1000, Owner: types.Pubkey{}})
	db.SetAccount(recipientPubkey, &accounts.Account{Lamports: 0, Owner: types.Pubkey{}})

	executor := NewTransactionExecutor(db)

	tx := &blockstore.Transaction{
		Signature:  createTestSignature(1),
		Signatures: []types.Signature{createTestSignature(1)},
		Message: blockstore.TransactionMessage{
			AccountKeys: []types.Pubkey{senderPubkey, recipientPubkey, types.Pubkey{}},
			Header: blockstore.MessageHeader{
				NumRequiredSignatures:       1,
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: 1,
			},
			Instructions: []blockstore.Instruction{
				{
					ProgramIDIndex: 2,
					AccountIndexes: []uint8{0, 1},
					Data:           createTransferInstruction(1000),
				},
			},
		},
		Slot: 1,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Reset balances for each iteration
		db.SetAccount(senderPubkey, &accounts.Account{Lamports: 1000, Owner: types.Pubkey{}})
		db.SetAccount(recipientPubkey, &accounts.Account{Lamports: 0, Owner: types.Pubkey{}})

		executor.Execute(tx)
	}
}

// BenchmarkComputeBankHash benchmarks bank hash computation.
func BenchmarkComputeBankHash(b *testing.B) {
	parentBankhash := createTestHash(1)
	accountsDeltaHash := createTestHash(2)
	numSigs := uint64(100)
	blockhash := createTestHash(3)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		computeBankHashFromComponents(parentBankhash, accountsDeltaHash, numSigs, blockhash)
	}
}
