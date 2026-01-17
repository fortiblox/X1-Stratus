package snapshot

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
)

// TestAppendVecReader tests parsing of append-vec format data.
func TestAppendVecReader(t *testing.T) {
	// Create a mock append-vec with a single account
	buf := &bytes.Buffer{}

	// Write StoredMeta
	writeU64(buf, 1)          // write_version
	writeU64(buf, 10)         // data_len (10 bytes of data)
	writeBytes(buf, testPubkey()) // pubkey (32 bytes)

	// Write AccountMeta
	writeU64(buf, 1000000)     // lamports (0.001 SOL)
	writeU64(buf, 100)         // rent_epoch
	writeBytes(buf, testOwner()) // owner (32 bytes)
	buf.WriteByte(0)           // executable = false

	// Write hash
	writeBytes(buf, make([]byte, 32)) // hash (32 bytes)

	// Write data (10 bytes)
	data := []byte("hello data")
	buf.Write(data)

	// Add padding to align to 8 bytes (10 -> 16)
	buf.Write(make([]byte, 6)) // padding

	// Parse
	reader := NewAppendVecReaderFromReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))

	account, err := reader.ReadAccount()
	if err != nil {
		t.Fatalf("failed to read account: %v", err)
	}

	// Verify fields
	if account.WriteVersion != 1 {
		t.Errorf("expected write_version=1, got %d", account.WriteVersion)
	}
	if account.Lamports != 1000000 {
		t.Errorf("expected lamports=1000000, got %d", account.Lamports)
	}
	if account.RentEpoch != 100 {
		t.Errorf("expected rent_epoch=100, got %d", account.RentEpoch)
	}
	if account.Executable {
		t.Error("expected executable=false")
	}
	if !bytes.Equal(account.Data, data) {
		t.Errorf("expected data=%q, got %q", data, account.Data)
	}
	if account.Pubkey != testPubkeyTyped() {
		t.Errorf("pubkey mismatch")
	}

	// Should be at EOF now
	_, err = reader.ReadAccount()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

// TestAppendVecMultipleAccounts tests parsing multiple accounts.
func TestAppendVecMultipleAccounts(t *testing.T) {
	buf := &bytes.Buffer{}

	// Write 3 accounts with different data sizes
	dataSizes := []int{0, 8, 16}
	for i, dataSize := range dataSizes {
		writeU64(buf, uint64(i+1)) // write_version
		writeU64(buf, uint64(dataSize)) // data_len
		writeBytes(buf, testPubkeyN(i)) // pubkey

		writeU64(buf, uint64((i+1)*100)) // lamports
		writeU64(buf, 0)                  // rent_epoch
		writeBytes(buf, testOwner())      // owner
		if i == 1 {
			buf.WriteByte(1) // executable = true for second account
		} else {
			buf.WriteByte(0) // executable = false
		}

		writeBytes(buf, make([]byte, 32)) // hash

		// Data
		if dataSize > 0 {
			buf.Write(make([]byte, dataSize))
		}
		// No padding needed since all sizes are already 8-byte aligned
	}

	reader := NewAppendVecReaderFromReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))

	count := 0
	for reader.HasMore() {
		account, err := reader.ReadAccount()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to read account %d: %v", count, err)
		}

		if account.WriteVersion != uint64(count+1) {
			t.Errorf("account %d: expected write_version=%d, got %d", count, count+1, account.WriteVersion)
		}
		if len(account.Data) != dataSizes[count] {
			t.Errorf("account %d: expected data_len=%d, got %d", count, dataSizes[count], len(account.Data))
		}
		if count == 1 && !account.Executable {
			t.Errorf("account %d: expected executable=true", count)
		}

		count++
	}

	if count != 3 {
		t.Errorf("expected 3 accounts, got %d", count)
	}
}

// TestAppendVecToAccount tests conversion to accounts.Account.
func TestAppendVecToAccount(t *testing.T) {
	stored := &StoredAccountMeta{
		WriteVersion: 42,
		Pubkey:       testPubkeyTyped(),
		Lamports:     5000000000, // 5 SOL
		RentEpoch:    200,
		Owner:        testOwnerTyped(),
		Executable:   true,
		Hash:         types.Hash{},
		Data:         []byte("program bytecode here"),
	}

	account := stored.ToAccount()

	if account.Lamports != stored.Lamports {
		t.Errorf("lamports mismatch")
	}
	if account.RentEpoch != stored.RentEpoch {
		t.Errorf("rent_epoch mismatch")
	}
	if account.Executable != stored.Executable {
		t.Errorf("executable mismatch")
	}
	if account.Owner != stored.Owner {
		t.Errorf("owner mismatch")
	}
	if !bytes.Equal(account.Data, stored.Data) {
		t.Errorf("data mismatch")
	}

	// Verify it's a copy, not a reference
	stored.Data[0] = 'X'
	if account.Data[0] == 'X' {
		t.Error("Data should be a copy, not a reference")
	}
}

// TestBincodeReader tests bincode deserialization helpers.
func TestBincodeReader(t *testing.T) {
	buf := &bytes.Buffer{}

	// Write test data
	writeU64(buf, 0x0102030405060708) // u64
	writeU64(buf, 0x1122334455667788) // u64 (for i64)
	buf.WriteByte(1)                   // bool (true)
	buf.WriteByte(0)                   // bool (false)

	// Float64 (pi)
	var piBits uint64 = 0x400921FB54442D18 // pi in IEEE 754
	writeU64(buf, piBits)

	br := NewBincodeReader(buf)

	// Test ReadU64
	v1, err := br.ReadU64()
	if err != nil {
		t.Fatalf("ReadU64 failed: %v", err)
	}
	if v1 != 0x0102030405060708 {
		t.Errorf("ReadU64: expected 0x0102030405060708, got 0x%x", v1)
	}

	// Test ReadI64
	v2, err := br.ReadI64()
	if err != nil {
		t.Fatalf("ReadI64 failed: %v", err)
	}
	if uint64(v2) != 0x1122334455667788 {
		t.Errorf("ReadI64: unexpected value")
	}

	// Test ReadBool (true)
	b1, err := br.ReadBool()
	if err != nil {
		t.Fatalf("ReadBool failed: %v", err)
	}
	if !b1 {
		t.Error("ReadBool: expected true")
	}

	// Test ReadBool (false)
	b2, err := br.ReadBool()
	if err != nil {
		t.Fatalf("ReadBool failed: %v", err)
	}
	if b2 {
		t.Error("ReadBool: expected false")
	}

	// Test ReadF64
	f1, err := br.ReadF64()
	if err != nil {
		t.Fatalf("ReadF64 failed: %v", err)
	}
	if f1 < 3.14 || f1 > 3.15 {
		t.Errorf("ReadF64: expected ~pi, got %f", f1)
	}
}

// TestFindSnapshots tests snapshot discovery.
func TestFindSnapshots(t *testing.T) {
	// Create temp directory with mock snapshot files
	tmpDir := t.TempDir()

	files := []string{
		"snapshot-100-abc123def.tar.zst",
		"snapshot-200-xyz789.tar",
		"snapshot-50-old123.tar.zst",
		"incremental-snapshot-100-150-inc456.tar.zst",
		"random-file.txt",
		"snapshot-invalid.tar.zst",
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	snapshots, err := FindSnapshots(tmpDir)
	if err != nil {
		t.Fatalf("FindSnapshots failed: %v", err)
	}

	// Should find 4 valid snapshots (3 full + 1 incremental)
	if len(snapshots) != 4 {
		t.Errorf("expected 4 snapshots, got %d", len(snapshots))
		for _, s := range snapshots {
			t.Logf("  found: slot=%d path=%s", s.Slot, s.Path)
		}
	}

	// Should be sorted by slot (newest first)
	if len(snapshots) >= 2 && snapshots[0].Slot < snapshots[1].Slot {
		t.Error("snapshots should be sorted by slot (newest first)")
	}

	// Check first (newest) snapshot
	if len(snapshots) > 0 {
		newest := snapshots[0]
		if newest.Slot != 200 {
			t.Errorf("expected newest slot=200, got %d", newest.Slot)
		}
	}
}

// TestFindLatestSnapshot tests finding the newest snapshot.
func TestFindLatestSnapshot(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some snapshot files
	files := []struct {
		name string
		slot uint64
	}{
		{"snapshot-100-abc.tar.zst", 100},
		{"snapshot-500-def.tar.zst", 500},
		{"snapshot-250-ghi.tar", 250},
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f.name)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	latest, err := FindLatestSnapshot(tmpDir)
	if err != nil {
		t.Fatalf("FindLatestSnapshot failed: %v", err)
	}

	if latest.Slot != 500 {
		t.Errorf("expected latest slot=500, got %d", latest.Slot)
	}
}

// TestFindSnapshotsEmpty tests with empty directory.
func TestFindSnapshotsEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	snapshots, err := FindSnapshots(tmpDir)
	if err != nil {
		t.Fatalf("FindSnapshots failed: %v", err)
	}

	if len(snapshots) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snapshots))
	}
}

// TestLoadSnapshotIntegration tests the full snapshot loading workflow.
func TestLoadSnapshotIntegration(t *testing.T) {
	// Create a mock snapshot archive
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "snapshot-100-test123.tar")

	// Create the tar archive with mock data
	if err := createMockSnapshot(snapshotPath); err != nil {
		t.Fatalf("failed to create mock snapshot: %v", err)
	}

	// Create in-memory accounts DB
	accountsDB := accounts.NewMemoryDB()

	// Load snapshot
	result, err := LoadSnapshot(snapshotPath, accountsDB)
	if err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}

	// Verify results
	if result.AccountsCount == 0 {
		t.Error("expected accounts to be loaded")
	}

	// Verify accounts are in DB
	count, err := accountsDB.AccountsCount()
	if err != nil {
		t.Fatalf("AccountsCount failed: %v", err)
	}
	if count != result.AccountsCount {
		t.Errorf("expected %d accounts in DB, got %d", result.AccountsCount, count)
	}
}

// TestIsAppendVecFilename tests the append-vec filename detection.
func TestIsAppendVecFilename(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"100.0", true},
		{"12345678.999", true},
		{"0.0", true},
		{"100", false},
		{"abc.def", false},
		{"100.abc", false},
		{"abc.100", false},
		{"version", false},
		{"status_cache", false},
		{"account_paths.txt", false},
	}

	for _, tc := range tests {
		result := isAppendVecFilename(tc.name)
		if result != tc.expected {
			t.Errorf("isAppendVecFilename(%q) = %v, expected %v", tc.name, result, tc.expected)
		}
	}
}

// TestAlignUp tests the alignment function.
func TestAlignUp(t *testing.T) {
	tests := []struct {
		n         int64
		alignment int64
		expected  int64
	}{
		{0, 8, 0},
		{1, 8, 8},
		{7, 8, 8},
		{8, 8, 8},
		{9, 8, 16},
		{15, 8, 16},
		{16, 8, 16},
		{17, 8, 24},
	}

	for _, tc := range tests {
		result := alignUp(tc.n, tc.alignment)
		if result != tc.expected {
			t.Errorf("alignUp(%d, %d) = %d, expected %d", tc.n, tc.alignment, result, tc.expected)
		}
	}
}

// Helper functions

func writeU64(buf *bytes.Buffer, v uint64) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	buf.Write(b)
}

func writeBytes(buf *bytes.Buffer, data []byte) {
	buf.Write(data)
}

func testPubkey() []byte {
	return bytes.Repeat([]byte{0x11}, 32)
}

func testPubkeyTyped() types.Pubkey {
	var pk types.Pubkey
	copy(pk[:], testPubkey())
	return pk
}

func testPubkeyN(n int) []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(n + 1)
	}
	return b
}

func testOwner() []byte {
	return bytes.Repeat([]byte{0x22}, 32)
}

func testOwnerTyped() types.Pubkey {
	var pk types.Pubkey
	copy(pk[:], testOwner())
	return pk
}

// createMockSnapshot creates a minimal tar archive that looks like a Solana snapshot.
func createMockSnapshot(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	tw := newTarWriter(file)
	defer tw.close()

	// Add a mock append-vec file
	appendVecData := createMockAppendVec(3) // 3 accounts
	if err := tw.addFile("accounts/100.0", appendVecData); err != nil {
		return err
	}

	return nil
}

// createMockAppendVec creates mock append-vec data with the specified number of accounts.
func createMockAppendVec(numAccounts int) []byte {
	buf := &bytes.Buffer{}

	for i := 0; i < numAccounts; i++ {
		// StoredMeta
		writeU64(buf, uint64(i+1)) // write_version
		writeU64(buf, 8)           // data_len
		writeBytes(buf, testPubkeyN(i)) // pubkey

		// AccountMeta
		writeU64(buf, uint64((i+1)*1000000)) // lamports
		writeU64(buf, 0)                      // rent_epoch
		writeBytes(buf, testOwner())          // owner
		buf.WriteByte(0)                      // executable

		// Hash
		writeBytes(buf, make([]byte, 32))

		// Data (8 bytes, already aligned)
		buf.Write(make([]byte, 8))
	}

	return buf.Bytes()
}

// Simple tar writer helper
type tarWriterHelper struct {
	file *os.File
}

func newTarWriter(f *os.File) *tarWriterHelper {
	return &tarWriterHelper{file: f}
}

func (tw *tarWriterHelper) addFile(name string, data []byte) error {
	// Write tar header
	header := make([]byte, 512)
	copy(header[0:100], []byte(name))

	// Size in octal (at offset 124, 11 bytes + null)
	sizeOctal := []byte(octalString(int64(len(data)), 11))
	copy(header[124:135], sizeOctal)

	// Mode
	copy(header[100:107], []byte("0000644"))

	// Type flag (regular file)
	header[156] = '0'

	// Compute checksum
	copy(header[148:156], []byte("        "))
	var sum int64
	for _, b := range header {
		sum += int64(b)
	}
	copy(header[148:155], []byte(octalString(sum, 6)))
	header[155] = ' '

	if _, err := tw.file.Write(header); err != nil {
		return err
	}

	// Write file data
	if _, err := tw.file.Write(data); err != nil {
		return err
	}

	// Pad to 512 bytes
	padding := 512 - (len(data) % 512)
	if padding < 512 {
		if _, err := tw.file.Write(make([]byte, padding)); err != nil {
			return err
		}
	}

	return nil
}

func (tw *tarWriterHelper) close() error {
	// Write two empty blocks for tar EOF
	_, err := tw.file.Write(make([]byte, 1024))
	return err
}

func octalString(n int64, width int) string {
	s := make([]byte, width)
	for i := width - 1; i >= 0 && n > 0; i-- {
		s[i] = byte('0' + (n & 7))
		n >>= 3
	}
	for i := 0; i < width; i++ {
		if s[i] == 0 {
			s[i] = '0'
		}
	}
	return string(s)
}
