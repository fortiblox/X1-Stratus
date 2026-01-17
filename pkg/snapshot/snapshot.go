package snapshot

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
)

// Snapshot filename patterns.
var (
	// Full snapshot: snapshot-SLOT-HASH.tar.zst or snapshot-SLOT-HASH.tar
	fullSnapshotPattern = regexp.MustCompile(`^snapshot-(\d+)-([a-zA-Z0-9]+)\.(tar\.zst|tar)$`)

	// Incremental snapshot: incremental-snapshot-BASESLOT-SLOT-HASH.tar.zst
	incrementalSnapshotPattern = regexp.MustCompile(`^incremental-snapshot-(\d+)-(\d+)-([a-zA-Z0-9]+)\.(tar\.zst|tar)$`)
)

// FindSnapshots discovers available snapshots in a directory.
// Returns snapshots sorted by slot (newest first).
func FindSnapshots(dir string) ([]SnapshotInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var snapshots []SnapshotInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Try full snapshot pattern
		if matches := fullSnapshotPattern.FindStringSubmatch(name); matches != nil {
			slot, _ := strconv.ParseUint(matches[1], 10, 64)
			info, err := entry.Info()
			if err != nil {
				continue
			}
			snapshots = append(snapshots, SnapshotInfo{
				Path:         filepath.Join(dir, name),
				Slot:         slot,
				Hash:         matches[2],
				IsCompressed: strings.HasSuffix(name, ".zst"),
				Size:         info.Size(),
			})
			continue
		}

		// Try incremental snapshot pattern
		if matches := incrementalSnapshotPattern.FindStringSubmatch(name); matches != nil {
			slot, _ := strconv.ParseUint(matches[2], 10, 64)
			info, err := entry.Info()
			if err != nil {
				continue
			}
			snapshots = append(snapshots, SnapshotInfo{
				Path:         filepath.Join(dir, name),
				Slot:         slot,
				Hash:         matches[3],
				IsCompressed: strings.HasSuffix(name, ".zst"),
				Size:         info.Size(),
			})
		}
	}

	// Sort by slot (newest first)
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Slot > snapshots[j].Slot
	})

	return snapshots, nil
}

// FindLatestSnapshot finds the most recent snapshot in a directory.
func FindLatestSnapshot(dir string) (*SnapshotInfo, error) {
	snapshots, err := FindSnapshots(dir)
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, ErrSnapshotNotFound
	}
	return &snapshots[0], nil
}

// LoadSnapshot loads a Solana snapshot into the accounts database.
func LoadSnapshot(path string, accountsDB accounts.DB) (*SnapshotResult, error) {
	loader, err := NewSnapshotLoader(path)
	if err != nil {
		return nil, err
	}
	defer loader.Close()

	return loader.Load(accountsDB)
}

// SnapshotLoader handles loading snapshots.
type SnapshotLoader struct {
	path         string
	isCompressed bool
	file         *os.File
	zstdDecoder  *zstd.Decoder
	tarReader    *tar.Reader
	bankFields   *BankFields
	accountFiles map[string][]byte // filename -> data for in-memory processing
}

// NewSnapshotLoader creates a new snapshot loader.
func NewSnapshotLoader(path string) (*SnapshotLoader, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("open snapshot: %w", err)
	}

	isCompressed := strings.HasSuffix(path, ".zst")

	loader := &SnapshotLoader{
		path:         path,
		isCompressed: isCompressed,
		file:         file,
		accountFiles: make(map[string][]byte),
	}

	// Set up decompression if needed
	var reader io.Reader = file
	if isCompressed {
		decoder, err := zstd.NewReader(file)
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("%w: %v", ErrDecompressionFailed, err)
		}
		loader.zstdDecoder = decoder
		reader = decoder
	}

	loader.tarReader = tar.NewReader(reader)

	return loader, nil
}

// Close closes the snapshot loader.
func (l *SnapshotLoader) Close() error {
	if l.zstdDecoder != nil {
		l.zstdDecoder.Close()
	}
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Load loads the snapshot into the accounts database.
func (l *SnapshotLoader) Load(accountsDB accounts.DB) (*SnapshotResult, error) {
	result := &SnapshotResult{}

	// First pass: read metadata and collect account file references
	if err := l.scanArchive(); err != nil {
		return nil, fmt.Errorf("scan archive: %w", err)
	}

	// Extract bank fields info into result
	if l.bankFields != nil {
		result.Slot = l.bankFields.Slot
		result.ParentSlot = l.bankFields.ParentSlot
		result.BlockHeight = l.bankFields.BlockHeight
		result.Blockhash = l.bankFields.Blockhash
		result.Epoch = l.bankFields.Epoch
		result.Capitalization = l.bankFields.Capitalization
		result.AccountsHash = l.bankFields.AccountsHash
	}

	// Second pass: load accounts from append-vec files
	var totalLamports uint64
	var accountCount uint64

	for filename, data := range l.accountFiles {
		// Only process append-vec files (SLOT.OFFSET format)
		if !isAppendVecFilename(filename) {
			continue
		}

		reader := NewAppendVecReaderFromReader(bytes.NewReader(data), int64(len(data)))

		err := IterateAppendVecReader(reader, func(storedAccount *StoredAccountMeta) error {
			// Skip accounts with zero lamports (deleted accounts)
			if storedAccount.Lamports == 0 && len(storedAccount.Data) == 0 {
				return nil
			}

			// Convert to accounts.Account and store
			account := storedAccount.ToAccount()
			if err := accountsDB.SetAccount(storedAccount.Pubkey, account); err != nil {
				return fmt.Errorf("set account %s: %w", storedAccount.Pubkey.String(), err)
			}

			accountCount++
			totalLamports += storedAccount.Lamports
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("load accounts from %s: %w", filename, err)
		}
	}

	result.AccountsCount = accountCount
	result.Capitalization = totalLamports

	// Update slot in accounts DB
	if result.Slot > 0 {
		if err := accountsDB.SetSlot(result.Slot); err != nil {
			return nil, fmt.Errorf("set slot: %w", err)
		}
	}

	// Commit changes
	if err := accountsDB.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return result, nil
}

// scanArchive scans the tar archive to find and parse metadata.
func (l *SnapshotLoader) scanArchive() error {
	for {
		header, err := l.tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		// Read file content
		data := make([]byte, header.Size)
		if _, err := io.ReadFull(l.tarReader, data); err != nil {
			return fmt.Errorf("read %s: %w", header.Name, err)
		}

		// Determine file type and handle accordingly
		name := filepath.Base(header.Name)
		dir := filepath.Dir(header.Name)

		// Check for bank fields (snapshots/SLOT/SLOT file)
		if strings.Contains(dir, "snapshots/") && !strings.Contains(name, ".") {
			// This might be the bank fields file
			if l.bankFields == nil {
				fields, err := ParseBankFields(bytes.NewReader(data))
				if err != nil {
					// Try partial parsing
					fields, err = ParseBankFieldsPartial(bytes.NewReader(data), header.Size)
					if err != nil {
						// Continue without bank fields - not fatal
						continue
					}
				}
				l.bankFields = fields
			}
		}

		// Check for account files (accounts/ directory, SLOT.OFFSET format)
		if strings.Contains(header.Name, "accounts/") && isAppendVecFilename(name) {
			l.accountFiles[name] = data
		}
	}

	return nil
}

// isAppendVecFilename checks if a filename matches the SLOT.OFFSET append-vec format.
func isAppendVecFilename(name string) bool {
	parts := strings.Split(name, ".")
	if len(parts) != 2 {
		return false
	}
	// Both parts should be numeric
	_, err1 := strconv.ParseUint(parts[0], 10, 64)
	_, err2 := strconv.ParseUint(parts[1], 10, 64)
	return err1 == nil && err2 == nil
}

// GetSnapshotInfo extracts metadata from a snapshot without fully loading it.
func GetSnapshotInfo(path string) (*SnapshotInfo, error) {
	// Parse filename for basic info
	name := filepath.Base(path)

	var slot uint64
	var hash string
	var isCompressed bool

	if matches := fullSnapshotPattern.FindStringSubmatch(name); matches != nil {
		slot, _ = strconv.ParseUint(matches[1], 10, 64)
		hash = matches[2]
		isCompressed = strings.HasSuffix(name, ".zst")
	} else if matches := incrementalSnapshotPattern.FindStringSubmatch(name); matches != nil {
		slot, _ = strconv.ParseUint(matches[2], 10, 64)
		hash = matches[3]
		isCompressed = strings.HasSuffix(name, ".zst")
	} else {
		return nil, fmt.Errorf("%w: unrecognized filename format", ErrInvalidSnapshot)
	}

	// Get file size
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrSnapshotNotFound
		}
		return nil, err
	}

	return &SnapshotInfo{
		Path:         path,
		Slot:         slot,
		Hash:         hash,
		IsCompressed: isCompressed,
		Size:         info.Size(),
	}, nil
}

// GetBankFields extracts bank fields from a snapshot.
func GetBankFields(path string) (*BankFields, error) {
	loader, err := NewSnapshotLoader(path)
	if err != nil {
		return nil, err
	}
	defer loader.Close()

	// Scan to find bank fields
	for {
		header, err := loader.tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar header: %w", err)
		}

		// Check for bank fields (snapshots/SLOT/SLOT file)
		dir := filepath.Dir(header.Name)
		name := filepath.Base(header.Name)

		if strings.Contains(dir, "snapshots/") && !strings.Contains(name, ".") {
			data := make([]byte, header.Size)
			if _, err := io.ReadFull(loader.tarReader, data); err != nil {
				continue
			}

			fields, err := ParseBankFields(bytes.NewReader(data))
			if err != nil {
				fields, err = ParseBankFieldsPartial(bytes.NewReader(data), header.Size)
				if err != nil {
					continue
				}
			}
			return fields, nil
		}

		// Skip file content
		if _, err := io.CopyN(io.Discard, loader.tarReader, header.Size); err != nil {
			return nil, err
		}
	}

	return nil, ErrMissingManifest
}

// VerifySnapshotHash verifies the snapshot hash matches the filename.
// This performs a quick validation without loading all accounts.
func VerifySnapshotHash(path string) (bool, error) {
	info, err := GetSnapshotInfo(path)
	if err != nil {
		return false, err
	}

	// Get bank fields to extract accounts hash
	fields, err := GetBankFields(path)
	if err != nil {
		// If we can't get bank fields, we can't verify
		return false, fmt.Errorf("cannot extract bank fields: %w", err)
	}

	// The hash in the filename is typically a truncated version of the accounts hash
	accountsHashStr := fields.AccountsHash.String()
	if len(info.Hash) <= len(accountsHashStr) {
		// Check if the filename hash is a prefix of the accounts hash
		return strings.HasPrefix(accountsHashStr, info.Hash), nil
	}

	return false, nil
}

// StreamingLoader provides a streaming interface for loading very large snapshots.
type StreamingLoader struct {
	loader      *SnapshotLoader
	accountsDB  accounts.DB
	batchSize   int
	onProgress  func(accountsLoaded uint64)
	currentFile string
}

// NewStreamingLoader creates a new streaming snapshot loader.
func NewStreamingLoader(path string, accountsDB accounts.DB) (*StreamingLoader, error) {
	loader, err := NewSnapshotLoader(path)
	if err != nil {
		return nil, err
	}

	return &StreamingLoader{
		loader:     loader,
		accountsDB: accountsDB,
		batchSize:  1000,
	}, nil
}

// SetProgressCallback sets a callback for progress updates.
func (s *StreamingLoader) SetProgressCallback(fn func(accountsLoaded uint64)) {
	s.onProgress = fn
}

// SetBatchSize sets the batch size for account commits.
func (s *StreamingLoader) SetBatchSize(size int) {
	if size > 0 {
		s.batchSize = size
	}
}

// Load performs the streaming load operation.
func (s *StreamingLoader) Load() (*SnapshotResult, error) {
	defer s.loader.Close()

	result := &SnapshotResult{}
	var accountCount uint64
	var totalLamports uint64
	var batchCount int

	for {
		header, err := s.loader.tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar header: %w", err)
		}

		name := filepath.Base(header.Name)
		dir := filepath.Dir(header.Name)

		// Handle bank fields
		if strings.Contains(dir, "snapshots/") && !strings.Contains(name, ".") {
			data := make([]byte, header.Size)
			if _, err := io.ReadFull(s.loader.tarReader, data); err != nil {
				continue
			}

			fields, err := ParseBankFields(bytes.NewReader(data))
			if err != nil {
				fields, err = ParseBankFieldsPartial(bytes.NewReader(data), header.Size)
			}
			if err == nil && s.loader.bankFields == nil {
				s.loader.bankFields = fields
				result.Slot = fields.Slot
				result.ParentSlot = fields.ParentSlot
				result.BlockHeight = fields.BlockHeight
				result.Blockhash = fields.Blockhash
				result.Epoch = fields.Epoch
			}
			continue
		}

		// Handle account files
		if strings.Contains(header.Name, "accounts/") && isAppendVecFilename(name) {
			s.currentFile = name

			// Stream directly from tar without buffering entire file
			reader := NewAppendVecReaderFromReader(
				io.LimitReader(s.loader.tarReader, header.Size),
				header.Size,
			)

			err := IterateAppendVecReader(reader, func(storedAccount *StoredAccountMeta) error {
				if storedAccount.Lamports == 0 && len(storedAccount.Data) == 0 {
					return nil
				}

				account := storedAccount.ToAccount()
				if err := s.accountsDB.SetAccount(storedAccount.Pubkey, account); err != nil {
					return fmt.Errorf("set account: %w", err)
				}

				accountCount++
				totalLamports += storedAccount.Lamports
				batchCount++

				// Commit periodically
				if batchCount >= s.batchSize {
					if err := s.accountsDB.Commit(); err != nil {
						return fmt.Errorf("commit batch: %w", err)
					}
					batchCount = 0

					if s.onProgress != nil {
						s.onProgress(accountCount)
					}
				}

				return nil
			})

			if err != nil {
				return nil, fmt.Errorf("load accounts from %s: %w", name, err)
			}
			continue
		}

		// Skip unneeded files
		if _, err := io.CopyN(io.Discard, s.loader.tarReader, header.Size); err != nil {
			return nil, fmt.Errorf("skip %s: %w", header.Name, err)
		}
	}

	// Final commit
	if err := s.accountsDB.Commit(); err != nil {
		return nil, fmt.Errorf("final commit: %w", err)
	}

	// Update slot
	if result.Slot > 0 {
		if err := s.accountsDB.SetSlot(result.Slot); err != nil {
			return nil, fmt.Errorf("set slot: %w", err)
		}
	}

	result.AccountsCount = accountCount
	result.Capitalization = totalLamports

	if s.onProgress != nil {
		s.onProgress(accountCount)
	}

	return result, nil
}

// ComputeAccountsHash computes the accounts hash after loading.
// This can be used to verify the snapshot was loaded correctly.
func ComputeAccountsHash(accountsDB accounts.HashableDB) (types.Hash, error) {
	return accountsDB.ComputeAccountsHash()
}
