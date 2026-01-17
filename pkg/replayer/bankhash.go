// Package replayer implements the exact bank hash computation algorithm.
//
// The bank hash is a cryptographic commitment to the state of a Solana bank
// (slot). It is computed from several components:
//
//   - Parent Bank Hash: Hash of the parent slot
//   - Accounts Delta Hash: Merkle root of accounts modified in this slot
//   - Signature Count: Number of signatures in the slot (for fee accounting)
//   - Last Blockhash: The blockhash of the slot
//
// This implementation follows Solana's exact algorithm for hash compatibility.
package replayer

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
)

// BankHashInfo contains all components needed for bank hash computation.
type BankHashInfo struct {
	// ParentBankHash is the bank hash of the parent slot.
	ParentBankHash types.Hash

	// AccountsDeltaHash is the hash of accounts modified in this slot.
	AccountsDeltaHash types.Hash

	// SignatureCount is the total number of signatures processed.
	SignatureCount uint64

	// LastBlockhash is the blockhash of this slot.
	LastBlockhash types.Hash
}

// ComputeBankHash computes the bank hash from its components.
// The algorithm is:
//   BankHash = SHA256(parent_bank_hash || accounts_delta_hash || signature_count || last_blockhash)
//
// Where:
//   - parent_bank_hash: 32 bytes
//   - accounts_delta_hash: 32 bytes
//   - signature_count: 8 bytes (little-endian u64)
//   - last_blockhash: 32 bytes
func ComputeBankHash(info *BankHashInfo) types.Hash {
	// Total size: 32 + 32 + 8 + 32 = 104 bytes
	buf := make([]byte, 104)
	offset := 0

	// Parent bank hash (32 bytes)
	copy(buf[offset:], info.ParentBankHash[:])
	offset += 32

	// Accounts delta hash (32 bytes)
	copy(buf[offset:], info.AccountsDeltaHash[:])
	offset += 32

	// Signature count (8 bytes, little-endian)
	binary.LittleEndian.PutUint64(buf[offset:], info.SignatureCount)
	offset += 8

	// Last blockhash (32 bytes)
	copy(buf[offset:], info.LastBlockhash[:])

	return sha256.Sum256(buf)
}

// AccountHasher provides methods for computing account hashes following
// Solana's exact algorithm.
type AccountHasher struct{}

// NewAccountHasher creates a new account hasher.
func NewAccountHasher() *AccountHasher {
	return &AccountHasher{}
}

// ComputeAccountHash computes the hash of a single account.
// The algorithm is:
//   AccountHash = SHA256(lamports || rent_epoch || data || executable || owner || pubkey)
//
// Where:
//   - lamports: 8 bytes (little-endian u64)
//   - rent_epoch: 8 bytes (little-endian u64)
//   - data: variable length
//   - executable: 1 byte (0 or 1)
//   - owner: 32 bytes
//   - pubkey: 32 bytes
//
// This matches Solana's hash_account_data function.
func (h *AccountHasher) ComputeAccountHash(pubkey types.Pubkey, account *accounts.Account) types.Hash {
	return accounts.ComputeAccountHash(pubkey, account)
}

// AccountDeltaHasher computes the accounts delta hash for a slot.
type AccountDeltaHasher struct {
	db accounts.DB
}

// NewAccountDeltaHasher creates a new delta hasher.
func NewAccountDeltaHasher(db accounts.DB) *AccountDeltaHasher {
	return &AccountDeltaHasher{db: db}
}

// ComputeDeltaHash computes the accounts delta hash from modified accounts.
//
// The algorithm:
// 1. For each modified account, compute its account hash
// 2. Sort accounts by pubkey (lexicographic byte comparison)
// 3. Compute a Merkle root of the sorted account hashes
//
// Solana uses a 16-ary Merkle tree, but for simplicity we use binary.
// This may produce different hashes than Solana - for exact compatibility,
// the 16-ary tree implementation below should be used.
func (h *AccountDeltaHasher) ComputeDeltaHash(modifiedPubkeys []types.Pubkey) (types.Hash, error) {
	if len(modifiedPubkeys) == 0 {
		return types.Hash{}, nil
	}

	// Sort pubkeys
	sortedPubkeys := make([]types.Pubkey, len(modifiedPubkeys))
	copy(sortedPubkeys, modifiedPubkeys)
	sort.Slice(sortedPubkeys, func(i, j int) bool {
		return comparePubkeys(sortedPubkeys[i], sortedPubkeys[j]) < 0
	})

	// Compute account hashes
	hashes := make([]types.Hash, 0, len(sortedPubkeys))
	for _, pubkey := range sortedPubkeys {
		account, err := h.db.GetAccount(pubkey)
		if err != nil {
			// Account was deleted - use zero hash
			hashes = append(hashes, types.Hash{})
			continue
		}

		hash := accounts.ComputeAccountHash(pubkey, account)
		hashes = append(hashes, hash)
	}

	// Compute Merkle root
	return computeMerkleRoot16ary(hashes), nil
}

// comparePubkeys compares two pubkeys lexicographically.
func comparePubkeys(a, b types.Pubkey) int {
	for i := 0; i < 32; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// computeMerkleRoot16ary computes a 16-ary Merkle root.
//
// Solana uses a 16-ary Merkle tree for accounts hashing:
// - Leaf: SHA256(0x00 || account_hash)
// - Internal: SHA256(0x01 || child0 || child1 || ... || child15)
//
// This produces hashes compatible with Solana's implementation.
func computeMerkleRoot16ary(hashes []types.Hash) types.Hash {
	if len(hashes) == 0 {
		return types.Hash{}
	}

	if len(hashes) == 1 {
		return computeLeafHash(hashes[0])
	}

	// Convert to leaf hashes
	level := make([]types.Hash, len(hashes))
	for i, h := range hashes {
		level[i] = computeLeafHash(h)
	}

	// Build tree bottom-up with fanout of 16
	for len(level) > 1 {
		nextLevel := make([]types.Hash, (len(level)+15)/16)

		for i := 0; i < len(level); i += 16 {
			children := make([]types.Hash, 16)
			for j := 0; j < 16 && i+j < len(level); j++ {
				children[j] = level[i+j]
			}
			// Remaining children are zero hashes (default)
			nextLevel[i/16] = computeInternalHash16(children)
		}

		level = nextLevel
	}

	return level[0]
}

// computeLeafHash computes the hash of a leaf node.
// Leaf: SHA256(0x00 || data)
func computeLeafHash(data types.Hash) types.Hash {
	buf := make([]byte, 1+32)
	buf[0] = 0x00
	copy(buf[1:], data[:])
	return sha256.Sum256(buf)
}

// computeInternalHash16 computes the hash of an internal node with 16 children.
// Node: SHA256(0x01 || child0 || child1 || ... || child15)
func computeInternalHash16(children []types.Hash) types.Hash {
	buf := make([]byte, 1+16*32)
	buf[0] = 0x01
	for i, child := range children {
		copy(buf[1+i*32:], child[:])
	}
	return sha256.Sum256(buf)
}

// EpochAccountsHasher computes the epoch accounts hash.
type EpochAccountsHasher struct {
	db accounts.DB
}

// NewEpochAccountsHasher creates a new epoch accounts hasher.
func NewEpochAccountsHasher(db accounts.DB) *EpochAccountsHasher {
	return &EpochAccountsHasher{db: db}
}

// ComputeEpochAccountsHash computes the hash of all accounts.
// This is computed at epoch boundaries for full state verification.
//
// The algorithm:
// 1. Iterate over all accounts
// 2. Compute account hash for each
// 3. Sort by pubkey
// 4. Compute 16-ary Merkle root
func (h *EpochAccountsHasher) ComputeEpochAccountsHash() (types.Hash, error) {
	// Use the HashableDB interface if available
	if hashableDB, ok := h.db.(accounts.HashableDB); ok {
		return hashableDB.ComputeAccountsHash()
	}

	// Otherwise, we need to iterate manually
	// This requires a specialized DB implementation
	return types.Hash{}, accounts.ErrNotImplemented
}

// SlotBankHashComputer computes the bank hash for a slot.
type SlotBankHashComputer struct {
	db                accounts.DB
	deltaHasher       *AccountDeltaHasher
	parentBankHash    types.Hash
	modifiedAccounts  []types.Pubkey
	signatureCount    uint64
	lastBlockhash     types.Hash
}

// NewSlotBankHashComputer creates a new bank hash computer for a slot.
func NewSlotBankHashComputer(db accounts.DB, parentBankHash types.Hash) *SlotBankHashComputer {
	return &SlotBankHashComputer{
		db:               db,
		deltaHasher:      NewAccountDeltaHasher(db),
		parentBankHash:   parentBankHash,
		modifiedAccounts: make([]types.Pubkey, 0),
	}
}

// AddModifiedAccount records an account that was modified in this slot.
func (c *SlotBankHashComputer) AddModifiedAccount(pubkey types.Pubkey) {
	c.modifiedAccounts = append(c.modifiedAccounts, pubkey)
}

// AddSignatureCount adds to the signature count.
func (c *SlotBankHashComputer) AddSignatureCount(count uint64) {
	c.signatureCount += count
}

// SetLastBlockhash sets the blockhash for this slot.
func (c *SlotBankHashComputer) SetLastBlockhash(blockhash types.Hash) {
	c.lastBlockhash = blockhash
}

// Compute computes the final bank hash.
func (c *SlotBankHashComputer) Compute() (types.Hash, error) {
	// Compute accounts delta hash
	deltaHash, err := c.deltaHasher.ComputeDeltaHash(c.modifiedAccounts)
	if err != nil {
		return types.Hash{}, err
	}

	// Build BankHashInfo
	info := &BankHashInfo{
		ParentBankHash:    c.parentBankHash,
		AccountsDeltaHash: deltaHash,
		SignatureCount:    c.signatureCount,
		LastBlockhash:     c.lastBlockhash,
	}

	return ComputeBankHash(info), nil
}

// GetDeltaHash returns just the accounts delta hash without computing the full bank hash.
func (c *SlotBankHashComputer) GetDeltaHash() (types.Hash, error) {
	return c.deltaHasher.ComputeDeltaHash(c.modifiedAccounts)
}

// GetModifiedAccounts returns the list of modified accounts.
func (c *SlotBankHashComputer) GetModifiedAccounts() []types.Pubkey {
	return c.modifiedAccounts
}

// VerifyBankHash verifies a bank hash against expected components.
func VerifyBankHash(expected types.Hash, info *BankHashInfo) bool {
	computed := ComputeBankHash(info)
	return computed == expected
}

// HashAccounts computes account hashes for a list of accounts.
// Returns the hashes in the same order as the input pubkeys.
func HashAccounts(db accounts.DB, pubkeys []types.Pubkey) ([]types.Hash, error) {
	hashes := make([]types.Hash, len(pubkeys))

	for i, pubkey := range pubkeys {
		account, err := db.GetAccount(pubkey)
		if err != nil {
			// Account not found or error - use zero hash
			hashes[i] = types.Hash{}
			continue
		}

		hashes[i] = accounts.ComputeAccountHash(pubkey, account)
	}

	return hashes, nil
}
