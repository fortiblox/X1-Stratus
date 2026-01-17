package accounts

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

func TestAccountSerialization(t *testing.T) {
	owner, _ := types.PubkeyFromBase58("11111111111111111111111111111111")
	account := &Account{
		Lamports:   1000000000, // 1 SOL
		Data:       []byte("test data"),
		Owner:      owner,
		Executable: false,
		RentEpoch:  100,
	}

	// Serialize
	data := account.Serialize()

	// Deserialize
	restored, err := DeserializeAccount(data)
	if err != nil {
		t.Fatalf("Failed to deserialize: %v", err)
	}

	// Verify fields
	if restored.Lamports != account.Lamports {
		t.Errorf("Lamports mismatch: got %d, want %d", restored.Lamports, account.Lamports)
	}
	if !bytes.Equal(restored.Data, account.Data) {
		t.Errorf("Data mismatch: got %v, want %v", restored.Data, account.Data)
	}
	if restored.Owner != account.Owner {
		t.Errorf("Owner mismatch: got %v, want %v", restored.Owner, account.Owner)
	}
	if restored.Executable != account.Executable {
		t.Errorf("Executable mismatch: got %v, want %v", restored.Executable, account.Executable)
	}
	if restored.RentEpoch != account.RentEpoch {
		t.Errorf("RentEpoch mismatch: got %d, want %d", restored.RentEpoch, account.RentEpoch)
	}
}

func TestMemoryDB(t *testing.T) {
	db := NewMemoryDB()
	defer db.Close()

	pubkey, _ := types.PubkeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	owner, _ := types.PubkeyFromBase58("11111111111111111111111111111111")

	account := &Account{
		Lamports:   500000000,
		Data:       []byte("account data"),
		Owner:      owner,
		Executable: true,
		RentEpoch:  0,
	}

	// Test SetAccount
	if err := db.SetAccount(pubkey, account); err != nil {
		t.Fatalf("SetAccount failed: %v", err)
	}

	// Test HasAccount
	exists, err := db.HasAccount(pubkey)
	if err != nil {
		t.Fatalf("HasAccount failed: %v", err)
	}
	if !exists {
		t.Error("Account should exist")
	}

	// Test GetAccount
	retrieved, err := db.GetAccount(pubkey)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if retrieved.Lamports != account.Lamports {
		t.Errorf("Retrieved account lamports mismatch")
	}

	// Test AccountsCount
	count, err := db.AccountsCount()
	if err != nil {
		t.Fatalf("AccountsCount failed: %v", err)
	}
	if count != 1 {
		t.Errorf("AccountsCount: got %d, want 1", count)
	}

	// Test DeleteAccount
	if err := db.DeleteAccount(pubkey); err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}

	exists, _ = db.HasAccount(pubkey)
	if exists {
		t.Error("Account should not exist after deletion")
	}

	// Test slot
	if err := db.SetSlot(100); err != nil {
		t.Fatalf("SetSlot failed: %v", err)
	}
	if db.GetSlot() != 100 {
		t.Errorf("GetSlot: got %d, want 100", db.GetSlot())
	}
}

func TestAccountHash(t *testing.T) {
	pubkey, _ := types.PubkeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	owner, _ := types.PubkeyFromBase58("11111111111111111111111111111111")

	account := &Account{
		Lamports:   1000000000,
		Data:       []byte("test"),
		Owner:      owner,
		Executable: false,
		RentEpoch:  0,
	}

	hash1 := ComputeAccountHash(pubkey, account)
	hash2 := ComputeAccountHash(pubkey, account)

	// Same input should produce same hash
	if hash1 != hash2 {
		t.Error("Same account should produce same hash")
	}

	// Different data should produce different hash
	account.Lamports = 2000000000
	hash3 := ComputeAccountHash(pubkey, account)
	if hash1 == hash3 {
		t.Error("Different account should produce different hash")
	}
}

func TestMerkleRoot(t *testing.T) {
	// Empty
	hash := ComputeMerkleRoot(nil)
	if !hash.IsZero() {
		t.Error("Empty merkle root should be zero")
	}

	// Single hash
	h1 := types.ComputeHash([]byte("test1"))
	root1 := ComputeMerkleRoot([]types.Hash{h1})
	if root1.IsZero() {
		t.Error("Single element merkle root should not be zero")
	}

	// Multiple hashes
	h2 := types.ComputeHash([]byte("test2"))
	h3 := types.ComputeHash([]byte("test3"))
	root2 := ComputeMerkleRoot([]types.Hash{h1, h2, h3})
	if root2.IsZero() {
		t.Error("Multiple element merkle root should not be zero")
	}

	// Order matters
	root3 := ComputeMerkleRoot([]types.Hash{h3, h2, h1})
	if root2 == root3 {
		t.Error("Different order should produce different merkle root")
	}
}

func TestHashableMemoryDB(t *testing.T) {
	db := NewHashableMemoryDB()
	defer db.Close()

	// Add some accounts
	owner, _ := types.PubkeyFromBase58("11111111111111111111111111111111")

	pubkey1, _ := types.PubkeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	account1 := &Account{
		Lamports: 1000000000,
		Data:     []byte("data1"),
		Owner:    owner,
	}
	db.SetAccount(pubkey1, account1)

	pubkey2, _ := types.PubkeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")
	account2 := &Account{
		Lamports: 2000000000,
		Data:     []byte("data2"),
		Owner:    owner,
	}
	db.SetAccount(pubkey2, account2)

	// Compute accounts hash
	hash, err := db.ComputeAccountsHash()
	if err != nil {
		t.Fatalf("ComputeAccountsHash failed: %v", err)
	}
	if hash.IsZero() {
		t.Error("Accounts hash should not be zero")
	}

	// Compute delta hash
	SortPubkeys([]types.Pubkey{pubkey1, pubkey2})
	deltaHash, err := db.ComputeDeltaHash([]types.Pubkey{pubkey1})
	if err != nil {
		t.Fatalf("ComputeDeltaHash failed: %v", err)
	}
	if deltaHash.IsZero() {
		t.Error("Delta hash should not be zero")
	}
}

func TestBankHash(t *testing.T) {
	input := BankHashInput{
		ParentBankHash:    types.ComputeHash([]byte("parent")),
		AccountsDeltaHash: types.ComputeHash([]byte("delta")),
		NumSignatures:     100,
		Blockhash:         types.ComputeHash([]byte("block")),
	}

	bankHash := ComputeBankHash(input)
	if bankHash.IsZero() {
		t.Error("Bank hash should not be zero")
	}

	// Same input should produce same hash
	bankHash2 := ComputeBankHash(input)
	if bankHash != bankHash2 {
		t.Error("Same input should produce same bank hash")
	}

	// Different input should produce different hash
	input.NumSignatures = 200
	bankHash3 := ComputeBankHash(input)
	if bankHash == bankHash3 {
		t.Error("Different input should produce different bank hash")
	}
}

func TestSnapshotMemoryDB(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "snapshot-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create source database
	srcDB := NewMemoryDB()
	owner, _ := types.PubkeyFromBase58("11111111111111111111111111111111")

	pubkey1, _ := types.PubkeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	srcDB.SetAccount(pubkey1, &Account{
		Lamports: 1000000000,
		Data:     []byte("account1 data"),
		Owner:    owner,
	})

	pubkey2, _ := types.PubkeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")
	srcDB.SetAccount(pubkey2, &Account{
		Lamports: 2000000000,
		Data:     []byte("account2 data with more content"),
		Owner:    owner,
	})

	srcDB.SetSlot(12345)

	// Create snapshot
	snapshotPath := filepath.Join(tmpDir, "test.x1snap")
	if err := srcDB.CreateSnapshot(snapshotPath); err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	// Verify snapshot exists
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Fatal("Snapshot file should exist")
	}

	// Get snapshot header
	header, err := GetSnapshotHeader(snapshotPath)
	if err != nil {
		t.Fatalf("GetSnapshotHeader failed: %v", err)
	}
	if header.Slot != 12345 {
		t.Errorf("Snapshot slot: got %d, want 12345", header.Slot)
	}
	if header.AccountsCount != 2 {
		t.Errorf("Snapshot accounts count: got %d, want 2", header.AccountsCount)
	}

	// Load snapshot into new database
	dstDB := NewMemoryDB()
	if err := dstDB.LoadSnapshot(snapshotPath); err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}

	// Verify loaded state
	if dstDB.GetSlot() != 12345 {
		t.Errorf("Loaded slot: got %d, want 12345", dstDB.GetSlot())
	}

	count, _ := dstDB.AccountsCount()
	if count != 2 {
		t.Errorf("Loaded accounts count: got %d, want 2", count)
	}

	// Verify account data
	acc1, err := dstDB.GetAccount(pubkey1)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acc1.Lamports != 1000000000 {
		t.Errorf("Account1 lamports: got %d, want 1000000000", acc1.Lamports)
	}
	if !bytes.Equal(acc1.Data, []byte("account1 data")) {
		t.Errorf("Account1 data mismatch")
	}
}

func TestSortPubkeys(t *testing.T) {
	pk1, _ := types.PubkeyFromBase58("11111111111111111111111111111111")
	pk2, _ := types.PubkeyFromBase58("22222222222222222222222222222222")
	pk3, _ := types.PubkeyFromBase58("33333333333333333333333333333333")

	pubkeys := []types.Pubkey{pk3, pk1, pk2}
	SortPubkeys(pubkeys)

	// After sorting, pk1 should be first (smallest)
	if pubkeys[0] != pk1 || pubkeys[1] != pk2 || pubkeys[2] != pk3 {
		t.Error("Pubkeys not properly sorted")
	}
}

func TestAccountClone(t *testing.T) {
	owner, _ := types.PubkeyFromBase58("11111111111111111111111111111111")
	original := &Account{
		Lamports:   1000,
		Data:       []byte("original"),
		Owner:      owner,
		Executable: true,
		RentEpoch:  5,
	}

	cloned := original.Clone()

	// Verify clone matches
	if cloned.Lamports != original.Lamports {
		t.Error("Clone lamports mismatch")
	}

	// Verify data is independent
	original.Data[0] = 'X'
	if cloned.Data[0] == 'X' {
		t.Error("Clone data should be independent")
	}

	// Verify nil clone
	var nilAccount *Account
	if nilAccount.Clone() != nil {
		t.Error("Clone of nil should be nil")
	}
}

func TestAccountIsZero(t *testing.T) {
	account := &Account{}
	if !account.IsZero() {
		t.Error("Empty account should be zero")
	}

	account.Lamports = 1
	if account.IsZero() {
		t.Error("Account with lamports should not be zero")
	}

	account.Lamports = 0
	account.Data = []byte("data")
	if account.IsZero() {
		t.Error("Account with data should not be zero")
	}
}
