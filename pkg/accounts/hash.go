// Package accounts provides hash computation for accounts state verification.
package accounts

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// HashComputer provides accounts hash computation capabilities.
// It implements the hash algorithms used by Solana for state verification.
//
// # Solana Hash Algorithms
//
// Solana uses several types of hashes for accounts:
//
// 1. Account Hash: SHA256 of individual account fields
//    - Used to verify account integrity
//    - Computed as: SHA256(lamports || rent_epoch || data || executable || owner || pubkey)
//
// 2. Accounts Delta Hash: Merkle root of accounts modified in a block
//    - Used in BankHash computation
//    - 16-ary Merkle tree with accounts sorted by pubkey
//
// 3. Epoch Accounts Hash: Merkle root of ALL accounts
//    - Computed once per epoch for full state verification
//    - Same algorithm as delta hash but includes all accounts
//
// 4. Bank Hash: Final block hash
//    - BankHash = SHA256(parent_bankhash || accounts_delta_hash || num_sigs || blockhash)
//
// # Merkle Tree Structure
//
// Solana uses a 16-ary Merkle tree:
// - Leaf nodes: SHA256(0x00 || account_hash)
// - Internal nodes: SHA256(0x01 || child0 || child1 || ... || child15)
// - Empty children use a zero hash
//
// For simplicity, this implementation uses a binary Merkle tree which is
// functionally equivalent but may produce different hashes than Solana.
// A full-compatibility implementation would need the 16-ary tree.
type HashComputer struct {
	db DB
}

// NewHashComputer creates a new hash computer with the given database.
func NewHashComputer(db DB) *HashComputer {
	return &HashComputer{db: db}
}

// ComputeAccountHash computes the hash of a single account.
// This follows Solana's account hash algorithm:
// SHA256(lamports || rent_epoch || data || executable || owner || pubkey)
//
// The order and format matches Solana's implementation for hash compatibility.
func ComputeAccountHash(pubkey types.Pubkey, account *Account) types.Hash {
	// Pre-calculate buffer size
	// lamports (8) + rent_epoch (8) + data + executable (1) + owner (32) + pubkey (32)
	size := 8 + 8 + len(account.Data) + 1 + 32 + 32
	buf := make([]byte, size)
	offset := 0

	// Lamports (little-endian, 8 bytes)
	binary.LittleEndian.PutUint64(buf[offset:], account.Lamports)
	offset += 8

	// RentEpoch (little-endian, 8 bytes)
	binary.LittleEndian.PutUint64(buf[offset:], account.RentEpoch)
	offset += 8

	// Data
	copy(buf[offset:], account.Data)
	offset += len(account.Data)

	// Executable (1 byte)
	if account.Executable {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}
	offset += 1

	// Owner (32 bytes)
	copy(buf[offset:], account.Owner[:])
	offset += 32

	// Pubkey (32 bytes)
	copy(buf[offset:], pubkey[:])

	return sha256.Sum256(buf)
}

// ComputeAccountHash computes the hash of an account via the HashComputer.
func (h *HashComputer) ComputeAccountHash(pubkey types.Pubkey, account *Account) types.Hash {
	return ComputeAccountHash(pubkey, account)
}

// AccountHashEntry pairs a pubkey with its hash for sorting and merkle computation.
type AccountHashEntry struct {
	Pubkey types.Pubkey
	Hash   types.Hash
}

// ComputeAccountsHash computes the accounts hash (Epoch Accounts Hash).
// This iterates over all accounts, computes their hashes, sorts by pubkey,
// and computes a Merkle root.
func (h *HashComputer) ComputeAccountsHash() (types.Hash, error) {
	// Collect all account hashes
	var entries []AccountHashEntry

	// Use BadgerDB's iterator if available
	if bdb, ok := h.db.(*BadgerDB); ok {
		err := bdb.IterateAccounts(func(pubkey types.Pubkey, account *Account) error {
			hash := ComputeAccountHash(pubkey, account)
			entries = append(entries, AccountHashEntry{
				Pubkey: pubkey,
				Hash:   hash,
			})
			return nil
		})
		if err != nil {
			return types.Hash{}, err
		}
	} else if mdb, ok := h.db.(*MemoryDB); ok {
		// For MemoryDB, iterate manually
		for pubkey, account := range mdb.GetAllAccounts() {
			hash := ComputeAccountHash(pubkey, account)
			entries = append(entries, AccountHashEntry{
				Pubkey: pubkey,
				Hash:   hash,
			})
		}
	} else {
		return types.Hash{}, ErrNotImplemented
	}

	// Sort by pubkey
	sort.Slice(entries, func(i, j int) bool {
		return comparePubkeys(entries[i].Pubkey, entries[j].Pubkey) < 0
	})

	// Extract hashes in sorted order
	hashes := make([]types.Hash, len(entries))
	for i, entry := range entries {
		hashes[i] = entry.Hash
	}

	// Compute Merkle root
	return ComputeMerkleRoot(hashes), nil
}

// ComputeDeltaHash computes the Accounts Delta Hash for a set of modified accounts.
// The pubkeys must be provided in sorted order for deterministic results.
func (h *HashComputer) ComputeDeltaHash(modifiedPubkeys []types.Pubkey) (types.Hash, error) {
	if len(modifiedPubkeys) == 0 {
		return types.Hash{}, nil
	}

	// Compute hash for each modified account
	hashes := make([]types.Hash, 0, len(modifiedPubkeys))

	for _, pubkey := range modifiedPubkeys {
		account, err := h.db.GetAccount(pubkey)
		if err == ErrAccountNotFound {
			// Deleted account - use zero hash
			hashes = append(hashes, types.Hash{})
			continue
		}
		if err != nil {
			return types.Hash{}, err
		}

		hash := ComputeAccountHash(pubkey, account)
		hashes = append(hashes, hash)
	}

	return ComputeMerkleRoot(hashes), nil
}

// ComputeMerkleRoot computes the Merkle root of a list of hashes.
// Uses a binary Merkle tree with SHA256.
//
// Tree structure:
// - Leaf: SHA256(0x00 || hash)
// - Node: SHA256(0x01 || left || right)
// - If odd number of nodes, last node is paired with zero hash
func ComputeMerkleRoot(hashes []types.Hash) types.Hash {
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

	// Build tree bottom-up
	for len(level) > 1 {
		nextLevel := make([]types.Hash, (len(level)+1)/2)

		for i := 0; i < len(level); i += 2 {
			left := level[i]
			var right types.Hash
			if i+1 < len(level) {
				right = level[i+1]
			}
			// right is zero hash if no pair
			nextLevel[i/2] = computeNodeHash(left, right)
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

// computeNodeHash computes the hash of an internal node.
// Node: SHA256(0x01 || left || right)
func computeNodeHash(left, right types.Hash) types.Hash {
	buf := make([]byte, 1+32+32)
	buf[0] = 0x01
	copy(buf[1:], left[:])
	copy(buf[33:], right[:])
	return sha256.Sum256(buf)
}

// BankHashInput contains the inputs for computing a BankHash.
type BankHashInput struct {
	ParentBankHash    types.Hash
	AccountsDeltaHash types.Hash
	NumSignatures     uint64
	Blockhash         types.Hash
}

// ComputeBankHash computes the BankHash for a slot.
// BankHash = SHA256(parent_bankhash || accounts_delta_hash || num_sigs || blockhash)
func ComputeBankHash(input BankHashInput) types.Hash {
	buf := make([]byte, 32+32+8+32)
	offset := 0

	// Parent bank hash
	copy(buf[offset:], input.ParentBankHash[:])
	offset += 32

	// Accounts delta hash
	copy(buf[offset:], input.AccountsDeltaHash[:])
	offset += 32

	// Number of signatures (little-endian)
	binary.LittleEndian.PutUint64(buf[offset:], input.NumSignatures)
	offset += 8

	// Blockhash
	copy(buf[offset:], input.Blockhash[:])

	return sha256.Sum256(buf)
}

// comparePubkeys compares two pubkeys lexicographically.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
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

// SortPubkeys sorts a slice of pubkeys in ascending order.
func SortPubkeys(pubkeys []types.Pubkey) {
	sort.Slice(pubkeys, func(i, j int) bool {
		return comparePubkeys(pubkeys[i], pubkeys[j]) < 0
	})
}

// HashableMemoryDB wraps MemoryDB with hash computation capabilities.
type HashableMemoryDB struct {
	*MemoryDB
	hasher *HashComputer
}

// NewHashableMemoryDB creates a new hashable in-memory database.
func NewHashableMemoryDB() *HashableMemoryDB {
	mdb := NewMemoryDB()
	return &HashableMemoryDB{
		MemoryDB: mdb,
		hasher:   NewHashComputer(mdb),
	}
}

// ComputeAccountHash computes the hash of an account.
func (h *HashableMemoryDB) ComputeAccountHash(pubkey types.Pubkey, account *Account) types.Hash {
	return ComputeAccountHash(pubkey, account)
}

// ComputeAccountsHash computes the accounts hash.
func (h *HashableMemoryDB) ComputeAccountsHash() (types.Hash, error) {
	return h.hasher.ComputeAccountsHash()
}

// ComputeDeltaHash computes the delta hash.
func (h *HashableMemoryDB) ComputeDeltaHash(modifiedPubkeys []types.Pubkey) (types.Hash, error) {
	return h.hasher.ComputeDeltaHash(modifiedPubkeys)
}

// Verify that HashableMemoryDB implements HashableDB interface.
var _ HashableDB = (*HashableMemoryDB)(nil)
