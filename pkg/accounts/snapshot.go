// Package accounts provides snapshot creation and loading for accounts database.
package accounts

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// Snapshot file format version.
const snapshotVersion uint32 = 1

// Snapshot file magic bytes for format validation.
var snapshotMagic = []byte{'X', '1', 'S', 'N'} // X1 Snapshot

// SnapshotHeader contains metadata about a snapshot.
type SnapshotHeader struct {
	// Version is the snapshot format version.
	Version uint32

	// Slot is the slot at which the snapshot was taken.
	Slot uint64

	// AccountsCount is the number of accounts in the snapshot.
	AccountsCount uint64

	// AccountsHash is the hash of all accounts at this slot.
	AccountsHash types.Hash
}

// SnapshotWriter writes accounts to a snapshot file.
// Snapshot format:
//   - Magic (4 bytes): "X1SN"
//   - Version (4 bytes, little-endian)
//   - Slot (8 bytes, little-endian)
//   - AccountsCount (8 bytes, little-endian)
//   - AccountsHash (32 bytes)
//   - Accounts data (gzip compressed):
//   - For each account:
//   - Pubkey (32 bytes)
//   - AccountSize (4 bytes, little-endian)
//   - AccountData (variable, serialized account)
type SnapshotWriter struct {
	file     *os.File
	gzWriter *gzip.Writer
	writer   *bufio.Writer
	header   SnapshotHeader
	count    uint64
}

// NewSnapshotWriter creates a new snapshot writer.
func NewSnapshotWriter(path string, slot uint64, accountsHash types.Hash) (*SnapshotWriter, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create snapshot directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create snapshot file: %w", err)
	}

	sw := &SnapshotWriter{
		file: file,
		header: SnapshotHeader{
			Version:      snapshotVersion,
			Slot:         slot,
			AccountsHash: accountsHash,
		},
	}

	// Write placeholder header (will be rewritten at close)
	if err := sw.writeHeader(); err != nil {
		file.Close()
		os.Remove(path)
		return nil, err
	}

	// Initialize gzip writer for account data
	sw.gzWriter = gzip.NewWriter(file)
	sw.writer = bufio.NewWriter(sw.gzWriter)

	return sw, nil
}

// writeHeader writes the snapshot header.
func (sw *SnapshotWriter) writeHeader() error {
	// Magic
	if _, err := sw.file.Write(snapshotMagic); err != nil {
		return err
	}

	buf := make([]byte, 52) // 4 + 8 + 8 + 32
	offset := 0

	// Version
	binary.LittleEndian.PutUint32(buf[offset:], sw.header.Version)
	offset += 4

	// Slot
	binary.LittleEndian.PutUint64(buf[offset:], sw.header.Slot)
	offset += 8

	// AccountsCount
	binary.LittleEndian.PutUint64(buf[offset:], sw.header.AccountsCount)
	offset += 8

	// AccountsHash
	copy(buf[offset:], sw.header.AccountsHash[:])

	_, err := sw.file.Write(buf)
	return err
}

// WriteAccount writes a single account to the snapshot.
func (sw *SnapshotWriter) WriteAccount(pubkey types.Pubkey, account *Account) error {
	// Write pubkey
	if _, err := sw.writer.Write(pubkey[:]); err != nil {
		return err
	}

	// Serialize account
	data := account.Serialize()

	// Write size
	sizeBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(sizeBuf, uint32(len(data)))
	if _, err := sw.writer.Write(sizeBuf); err != nil {
		return err
	}

	// Write account data
	if _, err := sw.writer.Write(data); err != nil {
		return err
	}

	sw.count++
	return nil
}

// Close finalizes and closes the snapshot.
func (sw *SnapshotWriter) Close() error {
	// Flush buffers
	if err := sw.writer.Flush(); err != nil {
		return err
	}
	if err := sw.gzWriter.Close(); err != nil {
		return err
	}

	// Update header with final count
	sw.header.AccountsCount = sw.count
	if _, err := sw.file.Seek(0, 0); err != nil {
		return err
	}
	if err := sw.writeHeader(); err != nil {
		return err
	}

	return sw.file.Close()
}

// SnapshotReader reads accounts from a snapshot file.
type SnapshotReader struct {
	file     *os.File
	gzReader *gzip.Reader
	reader   *bufio.Reader
	Header   SnapshotHeader
	read     uint64
}

// OpenSnapshot opens a snapshot file for reading.
func OpenSnapshot(path string) (*SnapshotReader, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("open snapshot: %w", err)
	}

	sr := &SnapshotReader{
		file: file,
	}

	// Read and validate header
	if err := sr.readHeader(); err != nil {
		file.Close()
		return nil, err
	}

	// Initialize gzip reader
	sr.gzReader, err = gzip.NewReader(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("init gzip reader: %w", err)
	}
	sr.reader = bufio.NewReader(sr.gzReader)

	return sr, nil
}

// readHeader reads and validates the snapshot header.
func (sr *SnapshotReader) readHeader() error {
	// Read magic
	magic := make([]byte, 4)
	if _, err := io.ReadFull(sr.file, magic); err != nil {
		return fmt.Errorf("read magic: %w", err)
	}
	if string(magic) != string(snapshotMagic) {
		return fmt.Errorf("invalid snapshot magic: %s", magic)
	}

	// Read header fields
	buf := make([]byte, 52)
	if _, err := io.ReadFull(sr.file, buf); err != nil {
		return fmt.Errorf("read header: %w", err)
	}

	offset := 0

	// Version
	sr.Header.Version = binary.LittleEndian.Uint32(buf[offset:])
	offset += 4
	if sr.Header.Version != snapshotVersion {
		return fmt.Errorf("unsupported snapshot version: %d", sr.Header.Version)
	}

	// Slot
	sr.Header.Slot = binary.LittleEndian.Uint64(buf[offset:])
	offset += 8

	// AccountsCount
	sr.Header.AccountsCount = binary.LittleEndian.Uint64(buf[offset:])
	offset += 8

	// AccountsHash
	copy(sr.Header.AccountsHash[:], buf[offset:])

	return nil
}

// ReadAccount reads the next account from the snapshot.
// Returns io.EOF when all accounts have been read.
func (sr *SnapshotReader) ReadAccount() (types.Pubkey, *Account, error) {
	if sr.read >= sr.Header.AccountsCount {
		return types.Pubkey{}, nil, io.EOF
	}

	// Read pubkey
	var pubkey types.Pubkey
	if _, err := io.ReadFull(sr.reader, pubkey[:]); err != nil {
		return types.Pubkey{}, nil, fmt.Errorf("read pubkey: %w", err)
	}

	// Read size
	sizeBuf := make([]byte, 4)
	if _, err := io.ReadFull(sr.reader, sizeBuf); err != nil {
		return types.Pubkey{}, nil, fmt.Errorf("read size: %w", err)
	}
	size := binary.LittleEndian.Uint32(sizeBuf)

	// Validate size to prevent unbounded allocation (max 10MB account data + overhead)
	const maxAccountSerializedSize = 10*1024*1024 + 100
	if size > maxAccountSerializedSize {
		return types.Pubkey{}, nil, fmt.Errorf("account size %d exceeds maximum %d", size, maxAccountSerializedSize)
	}

	// Read account data
	data := make([]byte, size)
	if _, err := io.ReadFull(sr.reader, data); err != nil {
		return types.Pubkey{}, nil, fmt.Errorf("read account data: %w", err)
	}

	// Deserialize account
	account, err := DeserializeAccount(data)
	if err != nil {
		return types.Pubkey{}, nil, fmt.Errorf("deserialize account: %w", err)
	}

	sr.read++
	return pubkey, account, nil
}

// Close closes the snapshot reader.
func (sr *SnapshotReader) Close() error {
	if sr.gzReader != nil {
		sr.gzReader.Close()
	}
	return sr.file.Close()
}

// CreateSnapshot creates a snapshot from a BadgerDB database.
func (b *BadgerDB) CreateSnapshot(path string) error {
	if b.closed.Load() {
		return ErrClosed
	}

	// Compute accounts hash
	hasher := NewHashComputer(b)
	accountsHash, err := hasher.ComputeAccountsHash()
	if err != nil {
		return fmt.Errorf("compute accounts hash: %w", err)
	}

	// Create snapshot writer
	writer, err := NewSnapshotWriter(path, b.GetSlot(), accountsHash)
	if err != nil {
		return err
	}
	defer writer.Close()

	// Write all accounts
	err = b.IterateAccounts(func(pubkey types.Pubkey, account *Account) error {
		return writer.WriteAccount(pubkey, account)
	})
	if err != nil {
		return fmt.Errorf("write accounts: %w", err)
	}

	return nil
}

// LoadSnapshot loads state from a snapshot file.
func (b *BadgerDB) LoadSnapshot(path string) error {
	if b.closed.Load() {
		return ErrClosed
	}

	// Open snapshot
	reader, err := OpenSnapshot(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Use batch writer for efficiency
	batch := b.NewBatchWriter()
	batchSize := 0
	maxBatchSize := 1000

	// Read all accounts
	for {
		pubkey, account, err := reader.ReadAccount()
		if err == io.EOF {
			break
		}
		if err != nil {
			batch.Cancel()
			return fmt.Errorf("read account: %w", err)
		}

		if err := batch.SetAccount(pubkey, account); err != nil {
			batch.Cancel()
			return fmt.Errorf("set account: %w", err)
		}

		batchSize++
		if batchSize >= maxBatchSize {
			if err := batch.Flush(); err != nil {
				return fmt.Errorf("flush batch: %w", err)
			}
			batch = b.NewBatchWriter()
			batchSize = 0
		}
	}

	// Flush remaining accounts
	if batchSize > 0 {
		if err := batch.Flush(); err != nil {
			return fmt.Errorf("flush final batch: %w", err)
		}
	}

	// Update slot
	if err := b.SetSlot(reader.Header.Slot); err != nil {
		return fmt.Errorf("set slot: %w", err)
	}

	// Verify accounts hash
	hasher := NewHashComputer(b)
	computedHash, err := hasher.ComputeAccountsHash()
	if err != nil {
		return fmt.Errorf("compute hash: %w", err)
	}
	if computedHash != reader.Header.AccountsHash {
		return fmt.Errorf("accounts hash mismatch: expected %s, got %s",
			reader.Header.AccountsHash.String(), computedHash.String())
	}

	return nil
}

// GetSnapshotSlot returns the slot of a snapshot file without fully loading it.
func (b *BadgerDB) GetSnapshotSlot(path string) (uint64, error) {
	reader, err := OpenSnapshot(path)
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	return reader.Header.Slot, nil
}

// GetSnapshotHeader returns the full header of a snapshot file.
func GetSnapshotHeader(path string) (*SnapshotHeader, error) {
	reader, err := OpenSnapshot(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return &reader.Header, nil
}

// SnapshotFilename returns the standard filename for a snapshot.
// Format: snapshot-{slot}-{hash}.x1snap
func SnapshotFilename(slot uint64, hash types.Hash) string {
	return fmt.Sprintf("snapshot-%d-%s.x1snap", slot, hash.String()[:16])
}

// ParseSnapshotFilename extracts the slot from a snapshot filename.
func ParseSnapshotFilename(filename string) (uint64, error) {
	var slot uint64
	var hashPart string
	_, err := fmt.Sscanf(filename, "snapshot-%d-%s.x1snap", &slot, &hashPart)
	if err != nil {
		return 0, fmt.Errorf("invalid snapshot filename: %s", filename)
	}
	return slot, nil
}

// Verify that BadgerDB implements SnapshotableDB interface.
var _ SnapshotableDB = (*BadgerDB)(nil)

// CreateSnapshot creates a snapshot from a MemoryDB (for testing).
func (m *MemoryDB) CreateSnapshot(path string) error {
	if m.closed {
		return ErrClosed
	}

	// Compute accounts hash
	hmdb := &HashableMemoryDB{MemoryDB: m, hasher: NewHashComputer(m)}
	accountsHash, err := hmdb.ComputeAccountsHash()
	if err != nil {
		return fmt.Errorf("compute accounts hash: %w", err)
	}

	// Create snapshot writer
	writer, err := NewSnapshotWriter(path, m.GetSlot(), accountsHash)
	if err != nil {
		return err
	}
	defer writer.Close()

	// Write all accounts
	for pubkey, account := range m.accounts {
		if err := writer.WriteAccount(pubkey, account); err != nil {
			return fmt.Errorf("write account: %w", err)
		}
	}

	return nil
}

// LoadSnapshot loads state from a snapshot into a MemoryDB.
func (m *MemoryDB) LoadSnapshot(path string) error {
	if m.closed {
		return ErrClosed
	}

	// Open snapshot
	reader, err := OpenSnapshot(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Clear existing accounts
	m.accounts = make(map[types.Pubkey]*Account)

	// Read all accounts
	for {
		pubkey, account, err := reader.ReadAccount()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read account: %w", err)
		}

		m.accounts[pubkey] = account
	}

	// Update slot
	m.slot = reader.Header.Slot

	// Verify accounts hash
	hmdb := &HashableMemoryDB{MemoryDB: m, hasher: NewHashComputer(m)}
	computedHash, err := hmdb.ComputeAccountsHash()
	if err != nil {
		return fmt.Errorf("compute hash: %w", err)
	}
	if computedHash != reader.Header.AccountsHash {
		return fmt.Errorf("accounts hash mismatch: expected %s, got %s",
			reader.Header.AccountsHash.String(), computedHash.String())
	}

	return nil
}

// GetSnapshotSlot returns the slot of a snapshot file.
func (m *MemoryDB) GetSnapshotSlot(path string) (uint64, error) {
	reader, err := OpenSnapshot(path)
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	return reader.Header.Slot, nil
}

// Verify that MemoryDB implements SnapshotableDB interface.
var _ SnapshotableDB = (*MemoryDB)(nil)
