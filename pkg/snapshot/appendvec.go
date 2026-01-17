package snapshot

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
)

// AppendVec alignment requirement.
const appendVecAlignment = 8

// Account storage sizes.
const (
	// StoredMetaSize is the size of StoredMeta (write_version + data_len + pubkey).
	StoredMetaSize = 8 + 8 + 32

	// AccountMetaSize is the size of AccountMeta (lamports + rent_epoch + owner + executable).
	AccountMetaSize = 8 + 8 + 32 + 1

	// HashSize is the size of the account hash.
	AccountHashSize = 32

	// MinAccountSize is the minimum size of a stored account entry.
	MinAccountSize = StoredMetaSize + AccountMetaSize + AccountHashSize
)

// Maximum account data size (10 MB).
const MaxAccountDataSize = 10 * 1024 * 1024

// AppendVecReader reads accounts from a Solana append-vec file.
type AppendVecReader struct {
	file     *os.File
	reader   io.Reader
	size     int64
	position int64
}

// NewAppendVecReader creates a new reader for an append-vec file.
func NewAppendVecReader(path string) (*AppendVecReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open append-vec: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat append-vec: %w", err)
	}

	return &AppendVecReader{
		file:     file,
		reader:   file,
		size:     info.Size(),
		position: 0,
	}, nil
}

// NewAppendVecReaderFromReader creates a reader from an io.Reader with known size.
func NewAppendVecReaderFromReader(r io.Reader, size int64) *AppendVecReader {
	return &AppendVecReader{
		reader:   r,
		size:     size,
		position: 0,
	}
}

// Close closes the underlying file if opened from a path.
func (r *AppendVecReader) Close() error {
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

// Size returns the total size of the append-vec.
func (r *AppendVecReader) Size() int64 {
	return r.size
}

// Position returns the current read position.
func (r *AppendVecReader) Position() int64 {
	return r.position
}

// HasMore returns true if there are more accounts to read.
func (r *AppendVecReader) HasMore() bool {
	return r.position < r.size
}

// ReadAccount reads the next account from the append-vec.
// Returns io.EOF when no more accounts are available.
func (r *AppendVecReader) ReadAccount() (*StoredAccountMeta, error) {
	// Check if we have enough space for minimum account
	if r.position+MinAccountSize > r.size {
		return nil, io.EOF
	}

	account := &StoredAccountMeta{}

	// Read StoredMeta
	// write_version: u64
	writeVersion, err := r.readU64()
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("read write_version: %w", err)
	}
	account.WriteVersion = writeVersion

	// data_len: u64
	dataLen, err := r.readU64()
	if err != nil {
		return nil, fmt.Errorf("read data_len: %w", err)
	}

	// Validate data length
	if dataLen > MaxAccountDataSize {
		return nil, fmt.Errorf("%w: data_len %d exceeds maximum %d", ErrCorruptedData, dataLen, MaxAccountDataSize)
	}

	// pubkey: [32]byte
	pubkey, err := r.readPubkey()
	if err != nil {
		return nil, fmt.Errorf("read pubkey: %w", err)
	}
	account.Pubkey = pubkey

	// Read AccountMeta
	// lamports: u64
	lamports, err := r.readU64()
	if err != nil {
		return nil, fmt.Errorf("read lamports: %w", err)
	}
	account.Lamports = lamports

	// rent_epoch: u64
	rentEpoch, err := r.readU64()
	if err != nil {
		return nil, fmt.Errorf("read rent_epoch: %w", err)
	}
	account.RentEpoch = rentEpoch

	// owner: [32]byte
	owner, err := r.readPubkey()
	if err != nil {
		return nil, fmt.Errorf("read owner: %w", err)
	}
	account.Owner = owner

	// executable: bool (1 byte)
	execByte, err := r.readByte()
	if err != nil {
		return nil, fmt.Errorf("read executable: %w", err)
	}
	account.Executable = execByte != 0

	// Read hash: [32]byte
	hash, err := r.readHash()
	if err != nil {
		return nil, fmt.Errorf("read hash: %w", err)
	}
	account.Hash = hash

	// Read data
	data := make([]byte, dataLen)
	if dataLen > 0 {
		if _, err := io.ReadFull(r.reader, data); err != nil {
			return nil, fmt.Errorf("read data: %w", err)
		}
		r.position += int64(dataLen)
	}
	account.Data = data

	// Skip padding to align to 8 bytes
	alignedLen := alignUp(int64(dataLen), appendVecAlignment)
	padding := alignedLen - int64(dataLen)
	if padding > 0 {
		if _, err := io.CopyN(io.Discard, r.reader, padding); err != nil {
			return nil, fmt.Errorf("skip padding: %w", err)
		}
		r.position += padding
	}

	return account, nil
}

// readU64 reads a little-endian uint64.
func (r *AppendVecReader) readU64() (uint64, error) {
	buf := make([]byte, 8)
	if _, err := io.ReadFull(r.reader, buf); err != nil {
		return 0, err
	}
	r.position += 8
	return binary.LittleEndian.Uint64(buf), nil
}

// readPubkey reads a 32-byte pubkey.
func (r *AppendVecReader) readPubkey() (types.Pubkey, error) {
	var pubkey types.Pubkey
	if _, err := io.ReadFull(r.reader, pubkey[:]); err != nil {
		return pubkey, err
	}
	r.position += 32
	return pubkey, nil
}

// readHash reads a 32-byte hash.
func (r *AppendVecReader) readHash() (types.Hash, error) {
	var hash types.Hash
	if _, err := io.ReadFull(r.reader, hash[:]); err != nil {
		return hash, err
	}
	r.position += 32
	return hash, nil
}

// readByte reads a single byte.
func (r *AppendVecReader) readByte() (byte, error) {
	buf := make([]byte, 1)
	if _, err := io.ReadFull(r.reader, buf); err != nil {
		return 0, err
	}
	r.position += 1
	return buf[0], nil
}

// alignUp aligns n up to the given alignment.
func alignUp(n, alignment int64) int64 {
	return (n + alignment - 1) & ^(alignment - 1)
}

// ToAccount converts StoredAccountMeta to an accounts.Account.
func (s *StoredAccountMeta) ToAccount() *accounts.Account {
	data := make([]byte, len(s.Data))
	copy(data, s.Data)
	return &accounts.Account{
		Lamports:   s.Lamports,
		Data:       data,
		Owner:      s.Owner,
		Executable: s.Executable,
		RentEpoch:  s.RentEpoch,
	}
}

// ParseAppendVec parses all accounts from an append-vec file.
func ParseAppendVec(path string) ([]*StoredAccountMeta, error) {
	reader, err := NewAppendVecReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var accounts []*StoredAccountMeta
	for reader.HasMore() {
		account, err := reader.ReadAccount()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read account at position %d: %w", reader.Position(), err)
		}
		accounts = append(accounts, account)
	}

	return accounts, nil
}

// IterateAppendVec iterates over accounts in an append-vec, calling fn for each.
// If fn returns an error, iteration stops and the error is returned.
func IterateAppendVec(path string, fn func(*StoredAccountMeta) error) error {
	reader, err := NewAppendVecReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	for reader.HasMore() {
		account, err := reader.ReadAccount()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read account at position %d: %w", reader.Position(), err)
		}
		if err := fn(account); err != nil {
			return err
		}
	}

	return nil
}

// IterateAppendVecReader iterates over accounts using an existing reader.
func IterateAppendVecReader(reader *AppendVecReader, fn func(*StoredAccountMeta) error) error {
	for reader.HasMore() {
		account, err := reader.ReadAccount()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read account at position %d: %w", reader.Position(), err)
		}
		if err := fn(account); err != nil {
			return err
		}
	}
	return nil
}

// AppendVecStats contains statistics about an append-vec.
type AppendVecStats struct {
	// AccountCount is the number of accounts.
	AccountCount uint64

	// TotalLamports is the total lamports across all accounts.
	TotalLamports uint64

	// TotalDataSize is the total data size.
	TotalDataSize uint64

	// ExecutableCount is the number of executable accounts.
	ExecutableCount uint64

	// ZeroLamportCount is the number of accounts with zero lamports.
	ZeroLamportCount uint64
}

// GetAppendVecStats computes statistics for an append-vec.
func GetAppendVecStats(path string) (*AppendVecStats, error) {
	stats := &AppendVecStats{}

	err := IterateAppendVec(path, func(account *StoredAccountMeta) error {
		stats.AccountCount++
		stats.TotalLamports += account.Lamports
		stats.TotalDataSize += uint64(len(account.Data))
		if account.Executable {
			stats.ExecutableCount++
		}
		if account.Lamports == 0 {
			stats.ZeroLamportCount++
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return stats, nil
}
