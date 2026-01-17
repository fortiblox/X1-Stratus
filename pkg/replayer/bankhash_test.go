package replayer

import (
	"bytes"
	"testing"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
)

// TestComputeBankHashFromInfo tests the bank hash computation from BankHashInfo.
func TestComputeBankHashFromInfo(t *testing.T) {
	info := &BankHashInfo{
		ParentBankHash:    types.Hash{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		AccountsDeltaHash: types.Hash{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		SignatureCount:    100,
		LastBlockhash:     types.Hash{1, 1, 1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 2, 2, 2, 2, 3, 3, 3, 3, 3, 3, 3, 3, 4, 4, 4, 4, 4, 4, 4, 4},
	}

	// Compute hash
	hash := ComputeBankHash(info)

	// Verify it's not zero
	zeroHash := types.Hash{}
	if hash == zeroHash {
		t.Error("ComputeBankHash returned zero hash")
	}

	// Verify consistency - same input should produce same output
	hash2 := ComputeBankHash(info)
	if hash != hash2 {
		t.Error("ComputeBankHash is not consistent")
	}

	// Verify that changing any component changes the hash
	info2 := *info
	info2.SignatureCount = 101
	hash3 := ComputeBankHash(&info2)
	if hash == hash3 {
		t.Error("Changing signature count should change hash")
	}
}

// TestVerifyBankHash tests bank hash verification.
func TestVerifyBankHash(t *testing.T) {
	info := &BankHashInfo{
		ParentBankHash:    types.Hash{1, 2, 3},
		AccountsDeltaHash: types.Hash{4, 5, 6},
		SignatureCount:    50,
		LastBlockhash:     types.Hash{7, 8, 9},
	}

	expected := ComputeBankHash(info)

	// Should verify correctly
	if !VerifyBankHash(expected, info) {
		t.Error("VerifyBankHash should return true for matching hash")
	}

	// Should fail with wrong hash
	wrong := types.Hash{1}
	if VerifyBankHash(wrong, info) {
		t.Error("VerifyBankHash should return false for non-matching hash")
	}
}

// TestComputeAccountHash tests individual account hashing.
func TestComputeAccountHash(t *testing.T) {
	hasher := NewAccountHasher()

	pubkey := types.Pubkey{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	account := &accounts.Account{
		Lamports:   1000000,
		Data:       []byte{1, 2, 3, 4, 5},
		Owner:      types.Pubkey{},
		Executable: false,
		RentEpoch:  100,
	}

	hash := hasher.ComputeAccountHash(pubkey, account)

	// Verify it's not zero
	zeroHash := types.Hash{}
	if hash == zeroHash {
		t.Error("ComputeAccountHash returned zero hash")
	}

	// Verify consistency
	hash2 := hasher.ComputeAccountHash(pubkey, account)
	if hash != hash2 {
		t.Error("ComputeAccountHash is not consistent")
	}

	// Verify that changing lamports changes the hash
	account2 := *account
	account2.Lamports = 2000000
	hash3 := hasher.ComputeAccountHash(pubkey, &account2)
	if hash == hash3 {
		t.Error("Changing lamports should change hash")
	}

	// Verify that changing data changes the hash
	account3 := *account
	account3.Data = []byte{5, 4, 3, 2, 1}
	hash4 := hasher.ComputeAccountHash(pubkey, &account3)
	if hash == hash4 {
		t.Error("Changing data should change hash")
	}
}

// TestComputeMerkleRoot16ary tests the 16-ary merkle tree computation.
func TestComputeMerkleRoot16ary(t *testing.T) {
	// Empty list
	hash := computeMerkleRoot16ary(nil)
	zeroHash := types.Hash{}
	if hash != zeroHash {
		t.Error("Empty list should produce zero hash")
	}

	// Single element
	single := []types.Hash{{1, 2, 3}}
	hashSingle := computeMerkleRoot16ary(single)
	if hashSingle == zeroHash {
		t.Error("Single element should not produce zero hash")
	}

	// Multiple elements
	multiple := []types.Hash{{1}, {2}, {3}, {4}}
	hashMultiple := computeMerkleRoot16ary(multiple)
	if hashMultiple == zeroHash {
		t.Error("Multiple elements should not produce zero hash")
	}

	// Verify that order matters
	reordered := []types.Hash{{2}, {1}, {3}, {4}}
	hashReordered := computeMerkleRoot16ary(reordered)
	if hashMultiple == hashReordered {
		t.Error("Different order should produce different hash")
	}

	// Verify consistency
	hashMultiple2 := computeMerkleRoot16ary(multiple)
	if hashMultiple != hashMultiple2 {
		t.Error("computeMerkleRoot16ary is not consistent")
	}
}

// TestComputeLeafHash tests leaf hash computation.
func TestComputeLeafHash(t *testing.T) {
	data := types.Hash{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	hash := computeLeafHash(data)

	zeroHash := types.Hash{}
	if hash == zeroHash {
		t.Error("computeLeafHash returned zero hash")
	}

	// Verify consistency
	hash2 := computeLeafHash(data)
	if hash != hash2 {
		t.Error("computeLeafHash is not consistent")
	}

	// Different data should produce different hash
	data2 := types.Hash{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	hash3 := computeLeafHash(data2)
	if hash == hash3 {
		t.Error("Different data should produce different hash")
	}
}

// TestComparePubkeys tests pubkey comparison.
func TestComparePubkeys(t *testing.T) {
	a := types.Pubkey{1, 2, 3}
	b := types.Pubkey{1, 2, 4}
	c := types.Pubkey{1, 2, 3}

	if comparePubkeys(a, b) >= 0 {
		t.Error("a should be less than b")
	}

	if comparePubkeys(b, a) <= 0 {
		t.Error("b should be greater than a")
	}

	if comparePubkeys(a, c) != 0 {
		t.Error("a should equal c")
	}
}

// TestSlotBankHashComputer tests the slot-level bank hash computer.
func TestSlotBankHashComputer(t *testing.T) {
	// Create a memory database
	db := accounts.NewMemoryDB()

	// Add some accounts
	pubkey1 := types.Pubkey{1}
	pubkey2 := types.Pubkey{2}
	account1 := &accounts.Account{Lamports: 1000, Data: []byte{1}}
	account2 := &accounts.Account{Lamports: 2000, Data: []byte{2}}

	db.SetAccount(pubkey1, account1)
	db.SetAccount(pubkey2, account2)

	// Create computer
	parentHash := types.Hash{1, 2, 3}
	computer := NewSlotBankHashComputer(db, parentHash)

	// Add modified accounts
	computer.AddModifiedAccount(pubkey1)
	computer.AddModifiedAccount(pubkey2)

	// Set other parameters
	computer.AddSignatureCount(5)
	computer.SetLastBlockhash(types.Hash{4, 5, 6})

	// Compute
	hash, err := computer.Compute()
	if err != nil {
		t.Fatalf("Compute failed: %v", err)
	}

	// Verify non-zero
	zeroHash := types.Hash{}
	if hash == zeroHash {
		t.Error("Computed hash should not be zero")
	}

	// Verify delta hash
	deltaHash, err := computer.GetDeltaHash()
	if err != nil {
		t.Fatalf("GetDeltaHash failed: %v", err)
	}
	if deltaHash == zeroHash {
		t.Error("Delta hash should not be zero")
	}

	// Verify modified accounts
	modified := computer.GetModifiedAccounts()
	if len(modified) != 2 {
		t.Errorf("Expected 2 modified accounts, got %d", len(modified))
	}
}

// TestHashAccounts tests batch account hashing.
func TestHashAccounts(t *testing.T) {
	db := accounts.NewMemoryDB()

	pubkey1 := types.Pubkey{1}
	pubkey2 := types.Pubkey{2}
	pubkey3 := types.Pubkey{3} // Not in database

	account1 := &accounts.Account{Lamports: 1000}
	account2 := &accounts.Account{Lamports: 2000}

	db.SetAccount(pubkey1, account1)
	db.SetAccount(pubkey2, account2)

	hashes, err := HashAccounts(db, []types.Pubkey{pubkey1, pubkey2, pubkey3})
	if err != nil {
		t.Fatalf("HashAccounts failed: %v", err)
	}

	if len(hashes) != 3 {
		t.Fatalf("Expected 3 hashes, got %d", len(hashes))
	}

	// First two should be non-zero
	zeroHash := types.Hash{}
	if hashes[0] == zeroHash {
		t.Error("Hash 0 should not be zero")
	}
	if hashes[1] == zeroHash {
		t.Error("Hash 1 should not be zero")
	}

	// Third should be zero (account not found)
	if hashes[2] != zeroHash {
		t.Error("Hash 2 should be zero (account not found)")
	}
}

// TestAccountDeltaHasherEmptyList tests delta hash with empty list.
func TestAccountDeltaHasherEmptyList(t *testing.T) {
	db := accounts.NewMemoryDB()
	hasher := NewAccountDeltaHasher(db)

	hash, err := hasher.ComputeDeltaHash(nil)
	if err != nil {
		t.Fatalf("ComputeDeltaHash failed: %v", err)
	}

	zeroHash := types.Hash{}
	if hash != zeroHash {
		t.Error("Empty list should produce zero hash")
	}
}

// TestAccountDeltaHasherWithDeletedAccount tests handling of deleted accounts.
func TestAccountDeltaHasherWithDeletedAccount(t *testing.T) {
	db := accounts.NewMemoryDB()

	// Add one account
	pubkey1 := types.Pubkey{1}
	db.SetAccount(pubkey1, &accounts.Account{Lamports: 1000})

	// Reference a non-existent account (simulating deletion)
	pubkey2 := types.Pubkey{2}

	hasher := NewAccountDeltaHasher(db)
	hash, err := hasher.ComputeDeltaHash([]types.Pubkey{pubkey1, pubkey2})
	if err != nil {
		t.Fatalf("ComputeDeltaHash failed: %v", err)
	}

	zeroHash := types.Hash{}
	if hash == zeroHash {
		t.Error("Hash should not be zero with at least one valid account")
	}
}

// TestBankHashDeterminism tests that bank hash computation is deterministic.
func TestBankHashDeterminism(t *testing.T) {
	info := &BankHashInfo{
		ParentBankHash:    types.Hash{0x11, 0x22, 0x33},
		AccountsDeltaHash: types.Hash{0x44, 0x55, 0x66},
		SignatureCount:    42,
		LastBlockhash:     types.Hash{0x77, 0x88, 0x99},
	}

	// Compute multiple times
	hashes := make([]types.Hash, 100)
	for i := 0; i < 100; i++ {
		hashes[i] = ComputeBankHash(info)
	}

	// All should be equal
	for i := 1; i < 100; i++ {
		if !bytes.Equal(hashes[0][:], hashes[i][:]) {
			t.Errorf("Hash %d differs from hash 0", i)
		}
	}
}
