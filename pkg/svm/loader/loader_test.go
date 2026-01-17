package loader

import (
	"testing"
)

// TestMurmur3Hash tests the murmur3 hash function used for syscall identification.
func TestMurmur3Hash(t *testing.T) {
	// Test that the hash function is deterministic and produces non-zero values
	tests := []string{
		"sol_log_",
		"sol_sha256",
		"abort",
		"sol_invoke_signed_c",
	}

	for _, test := range tests {
		result1 := murmur3Hash(test)
		result2 := murmur3Hash(test)

		// Hash should be consistent
		if result1 != result2 {
			t.Errorf("murmur3Hash(%q) not consistent: got 0x%08x and 0x%08x", test, result1, result2)
		}

		// Hash should be non-zero
		if result1 == 0 {
			t.Errorf("murmur3Hash(%q) returned 0", test)
		}
	}

	// Different inputs should produce different outputs
	hash1 := murmur3Hash("sol_log_")
	hash2 := murmur3Hash("sol_sha256")
	if hash1 == hash2 {
		t.Error("Different inputs produced same hash")
	}
}

// TestParseHeader tests ELF header parsing.
func TestParseHeader(t *testing.T) {
	// Minimal ELF64 header for BPF
	header := make([]byte, 64)
	// Magic
	copy(header[0:4], elfMagic)
	// Class: 64-bit
	header[4] = elfClass64
	// Data: little-endian
	header[5] = elfDataLSB
	// Version
	header[6] = 1
	// Type: Executable
	header[16] = 2
	header[17] = 0
	// Machine: BPF
	header[18] = 247
	header[19] = 0

	parsed, err := parseHeader(header)
	if err != nil {
		t.Fatalf("parseHeader failed: %v", err)
	}

	if parsed.Class != elfClass64 {
		t.Errorf("Class = %d, want %d", parsed.Class, elfClass64)
	}

	if parsed.Data != elfDataLSB {
		t.Errorf("Data = %d, want %d", parsed.Data, elfDataLSB)
	}

	if parsed.Machine != elfMachineBPF {
		t.Errorf("Machine = %d, want %d", parsed.Machine, elfMachineBPF)
	}
}

// TestValidateHeader tests header validation.
func TestValidateHeader(t *testing.T) {
	tests := []struct {
		name    string
		header  *ELFHeader
		wantErr bool
	}{
		{
			name: "valid BPF",
			header: &ELFHeader{
				Class:   elfClass64,
				Data:    elfDataLSB,
				Machine: elfMachineBPF,
				Type:    elfTypeExec,
			},
			wantErr: false,
		},
		{
			name: "valid sBPF",
			header: &ELFHeader{
				Class:   elfClass64,
				Data:    elfDataLSB,
				Machine: elfMachineSBPF,
				Type:    elfTypeDyn,
			},
			wantErr: false,
		},
		{
			name: "invalid class",
			header: &ELFHeader{
				Class:   1, // 32-bit
				Data:    elfDataLSB,
				Machine: elfMachineBPF,
				Type:    elfTypeExec,
			},
			wantErr: true,
		},
		{
			name: "invalid endianness",
			header: &ELFHeader{
				Class:   elfClass64,
				Data:    2, // Big-endian
				Machine: elfMachineBPF,
				Type:    elfTypeExec,
			},
			wantErr: true,
		},
		{
			name: "invalid machine",
			header: &ELFHeader{
				Class:   elfClass64,
				Data:    elfDataLSB,
				Machine: 62, // x86-64
				Type:    elfTypeExec,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHeader(tt.header)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateHeader() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestLoadInvalidELF tests loading invalid ELF files.
func TestLoadInvalidELF(t *testing.T) {
	loader := NewLoader()

	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too short", []byte{0x7f, 'E', 'L', 'F'}},
		{"wrong magic", make([]byte, 64)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loader.Load(tt.data)
			if err == nil {
				t.Error("Load() expected error, got nil")
			}
		})
	}
}

// TestGetSymbolName tests symbol name extraction.
func TestGetSymbolName(t *testing.T) {
	strtab := []byte("\x00hello\x00world\x00")

	tests := []struct {
		offset   uint32
		expected string
	}{
		{0, ""},
		{1, "hello"},
		{7, "world"},
	}

	for _, tt := range tests {
		result := getSymbolName(strtab, tt.offset)
		if result != tt.expected {
			t.Errorf("getSymbolName(strtab, %d) = %q, want %q", tt.offset, result, tt.expected)
		}
	}
}

// TestExecutableToProgram tests converting Executable to Program.
func TestExecutableToProgram(t *testing.T) {
	exec := &Executable{
		Text:      []uint64{0x12345678, 0x9abcdef0},
		RO:        []byte{1, 2, 3, 4},
		Entry:     1,
		Functions: map[uint32]uint64{0x100: 5},
	}

	prog := exec.ToProgram()

	if len(prog.Text) != len(exec.Text) {
		t.Errorf("Text length = %d, want %d", len(prog.Text), len(exec.Text))
	}

	if len(prog.RO) != len(exec.RO) {
		t.Errorf("RO length = %d, want %d", len(prog.RO), len(exec.RO))
	}

	if prog.Entry != exec.Entry {
		t.Errorf("Entry = %d, want %d", prog.Entry, exec.Entry)
	}

	if len(prog.Functions) != len(exec.Functions) {
		t.Errorf("Functions length = %d, want %d", len(prog.Functions), len(exec.Functions))
	}
}
