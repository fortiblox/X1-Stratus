// Package loader implements the sBPF ELF loader for Solana programs.
//
// This package parses ELF files containing sBPF bytecode and prepares them
// for execution in the sBPF virtual machine. It handles:
// - ELF header validation
// - Section parsing (.text, .rodata, .data, .bss)
// - Relocation processing
// - Function symbol extraction
//
// The loader follows Solana's sBPF ELF format which extends standard ELF64
// with Solana-specific conventions for program deployment.
package loader

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"

	"github.com/fortiblox/X1-Stratus/pkg/svm/sbpf"
	"github.com/fortiblox/X1-Stratus/pkg/svm/syscall"
)

// ELF magic bytes.
var elfMagic = []byte{0x7f, 'E', 'L', 'F'}

// ELF class.
const (
	elfClass64 = 2 // 64-bit
)

// ELF data encoding.
const (
	elfDataLSB = 1 // Little endian
)

// ELF machine type.
const (
	elfMachineBPF    = 247 // eBPF
	elfMachineSBPF   = 263 // sBPF (Solana BPF)
)

// ELF type.
const (
	elfTypeExec = 2 // Executable
	elfTypeDyn  = 3 // Shared object (also used by Solana)
)

// Section types.
const (
	shtNull     = 0  // Null section
	shtProgbits = 1  // Program data
	shtSymtab   = 2  // Symbol table
	shtStrtab   = 3  // String table
	shtRela     = 4  // Relocation with addend
	shtNobits   = 8  // .bss (no data in file)
	shtRel      = 9  // Relocation without addend
	shtDynsym   = 11 // Dynamic symbol table
)

// Section flags.
const (
	shfWrite     = 0x1 // Writable
	shfAlloc     = 0x2 // Occupies memory during execution
	shfExecInstr = 0x4 // Executable
)

// Symbol binding.
const (
	stbLocal  = 0
	stbGlobal = 1
	stbWeak   = 2
)

// Symbol type.
const (
	sttNotype  = 0
	sttObject  = 1
	sttFunc    = 2
	sttSection = 3
	sttFile    = 4
)

// Relocation types for sBPF.
const (
	rBPF64_64     = 1  // 64-bit relocation
	rBPF64_32     = 10 // 32-bit relocation
	rBPFRelative  = 8  // Relative relocation
)

// ELF errors.
var (
	ErrInvalidELF         = errors.New("invalid ELF file")
	ErrUnsupportedClass   = errors.New("unsupported ELF class (expected 64-bit)")
	ErrUnsupportedEndian  = errors.New("unsupported endianness (expected little-endian)")
	ErrUnsupportedMachine = errors.New("unsupported machine type (expected BPF/sBPF)")
	ErrNoTextSection      = errors.New("no .text section found")
	ErrInvalidSection     = errors.New("invalid section")
	ErrInvalidSymbol      = errors.New("invalid symbol")
	ErrRelocationFailed   = errors.New("relocation failed")
	ErrTooLarge           = errors.New("ELF file too large")
)

// Maximum sizes.
const (
	MaxELFSize        = 10 * 1024 * 1024 // 10 MB max ELF size
	MaxSections       = 256              // Max number of sections
	MaxSymbols        = 100000           // Max number of symbols
	MaxRelocations    = 100000           // Max number of relocations
	MaxInstructions   = 1000000          // Max number of instructions
)

// ELFHeader represents the ELF64 header.
type ELFHeader struct {
	Magic      [4]byte
	Class      uint8
	Data       uint8
	Version    uint8
	OSABI      uint8
	ABIVersion uint8
	Pad        [7]byte
	Type       uint16
	Machine    uint16
	Version2   uint32
	Entry      uint64
	PHOff      uint64
	SHOff      uint64
	Flags      uint32
	EHSize     uint16
	PHEntSize  uint16
	PHNum      uint16
	SHEntSize  uint16
	SHNum      uint16
	SHStrNdx   uint16
}

// SectionHeader represents an ELF64 section header.
type SectionHeader struct {
	Name      uint32
	Type      uint32
	Flags     uint64
	Addr      uint64
	Offset    uint64
	Size      uint64
	Link      uint32
	Info      uint32
	AddrAlign uint64
	EntSize   uint64
}

// Symbol represents an ELF64 symbol.
type Symbol struct {
	Name  uint32
	Info  uint8
	Other uint8
	Shndx uint16
	Value uint64
	Size  uint64
}

// Rela represents an ELF64 relocation with addend.
type Rela struct {
	Offset uint64
	Info   uint64
	Addend int64
}

// Executable represents a loaded sBPF program ready for execution.
type Executable struct {
	// Text contains the program instructions.
	Text []uint64

	// RO contains read-only data (.rodata + .data.rel.ro).
	RO []byte

	// Entry is the entry point (instruction index).
	Entry uint64

	// Functions maps function hashes to their entry points.
	Functions map[uint32]uint64

	// Syscalls contains the syscall name hashes referenced by the program.
	Syscalls []uint32
}

// ToProgram converts the executable to an sbpf.Program.
func (e *Executable) ToProgram() *sbpf.Program {
	return &sbpf.Program{
		Text:      e.Text,
		RO:        e.RO,
		Entry:     e.Entry,
		Functions: e.Functions,
	}
}

// Loader loads sBPF programs from ELF files.
type Loader struct {
	// Hasher for computing function hashes.
	hasher hash.Hash32
}

// NewLoader creates a new ELF loader.
func NewLoader() *Loader {
	return &Loader{}
}

// Load parses an ELF file and returns an executable.
func (l *Loader) Load(data []byte) (*Executable, error) {
	if len(data) > MaxELFSize {
		return nil, ErrTooLarge
	}

	if len(data) < 64 {
		return nil, ErrInvalidELF
	}

	// Parse ELF header
	header, err := parseHeader(data)
	if err != nil {
		return nil, err
	}

	// Validate header
	if err := validateHeader(header); err != nil {
		return nil, err
	}

	// Parse section headers
	sections, err := parseSectionHeaders(data, header)
	if err != nil {
		return nil, err
	}

	// Get section names
	sectionNames, err := getSectionNames(data, sections, header.SHStrNdx)
	if err != nil {
		return nil, err
	}

	// Find important sections
	textSection := findSection(sections, sectionNames, ".text")
	rodataSection := findSection(sections, sectionNames, ".rodata")
	symtabSection := findSection(sections, sectionNames, ".symtab")
	strtabSection := findSection(sections, sectionNames, ".strtab")
	dynsymSection := findSection(sections, sectionNames, ".dynsym")
	dynstrSection := findSection(sections, sectionNames, ".dynstr")
	relaTextSection := findSection(sections, sectionNames, ".rel.text")
	relaDynSection := findSection(sections, sectionNames, ".rel.dyn")

	if textSection == nil {
		return nil, ErrNoTextSection
	}

	// Extract text section
	text, err := extractText(data, textSection)
	if err != nil {
		return nil, err
	}

	// Extract read-only data
	var rodata []byte
	if rodataSection != nil {
		rodata, err = extractSection(data, rodataSection)
		if err != nil {
			return nil, err
		}
	}

	// Parse symbols
	var symbols []Symbol
	var symbolStrings []byte
	if symtabSection != nil && strtabSection != nil {
		symbols, err = parseSymbols(data, symtabSection)
		if err != nil {
			return nil, err
		}
		symbolStrings, err = extractSection(data, strtabSection)
		if err != nil {
			return nil, err
		}
	} else if dynsymSection != nil && dynstrSection != nil {
		symbols, err = parseSymbols(data, dynsymSection)
		if err != nil {
			return nil, err
		}
		symbolStrings, err = extractSection(data, dynstrSection)
		if err != nil {
			return nil, err
		}
	}

	// Build function registry
	functions := make(map[uint32]uint64)
	syscalls := make([]uint32, 0)

	for _, sym := range symbols {
		if sym.Info&0xf == sttFunc && sym.Shndx != 0 {
			name := getSymbolName(symbolStrings, sym.Name)
			if name != "" {
				hash := murmur3Hash(name)
				// Function addresses are instruction indices
				functions[hash] = sym.Value / 8
			}
		}
	}

	// Process relocations
	if relaTextSection != nil {
		err = processRelocations(data, relaTextSection, text, symbols, symbolStrings, &syscalls)
		if err != nil {
			return nil, err
		}
	}
	if relaDynSection != nil {
		err = processRelocations(data, relaDynSection, text, symbols, symbolStrings, &syscalls)
		if err != nil {
			return nil, err
		}
	}

	// Calculate entry point
	entry := header.Entry / 8 // Convert byte offset to instruction index

	// Adjust for virtual address offset
	if textSection.Addr > 0 {
		entry = (header.Entry - textSection.Addr) / 8
	}

	return &Executable{
		Text:      text,
		RO:        rodata,
		Entry:     entry,
		Functions: functions,
		Syscalls:  syscalls,
	}, nil
}

// parseHeader parses the ELF header.
func parseHeader(data []byte) (*ELFHeader, error) {
	if len(data) < 64 {
		return nil, ErrInvalidELF
	}

	// Check magic
	if !bytes.Equal(data[0:4], elfMagic) {
		return nil, ErrInvalidELF
	}

	header := &ELFHeader{}
	copy(header.Magic[:], data[0:4])
	header.Class = data[4]
	header.Data = data[5]
	header.Version = data[6]
	header.OSABI = data[7]
	header.ABIVersion = data[8]

	// Rest of header (little-endian)
	header.Type = binary.LittleEndian.Uint16(data[16:18])
	header.Machine = binary.LittleEndian.Uint16(data[18:20])
	header.Version2 = binary.LittleEndian.Uint32(data[20:24])
	header.Entry = binary.LittleEndian.Uint64(data[24:32])
	header.PHOff = binary.LittleEndian.Uint64(data[32:40])
	header.SHOff = binary.LittleEndian.Uint64(data[40:48])
	header.Flags = binary.LittleEndian.Uint32(data[48:52])
	header.EHSize = binary.LittleEndian.Uint16(data[52:54])
	header.PHEntSize = binary.LittleEndian.Uint16(data[54:56])
	header.PHNum = binary.LittleEndian.Uint16(data[56:58])
	header.SHEntSize = binary.LittleEndian.Uint16(data[58:60])
	header.SHNum = binary.LittleEndian.Uint16(data[60:62])
	header.SHStrNdx = binary.LittleEndian.Uint16(data[62:64])

	return header, nil
}

// validateHeader validates the ELF header.
func validateHeader(h *ELFHeader) error {
	if h.Class != elfClass64 {
		return ErrUnsupportedClass
	}

	if h.Data != elfDataLSB {
		return ErrUnsupportedEndian
	}

	if h.Machine != elfMachineBPF && h.Machine != elfMachineSBPF {
		return ErrUnsupportedMachine
	}

	if h.Type != elfTypeExec && h.Type != elfTypeDyn {
		return fmt.Errorf("%w: unsupported ELF type %d", ErrInvalidELF, h.Type)
	}

	return nil
}

// parseSectionHeaders parses section headers.
func parseSectionHeaders(data []byte, header *ELFHeader) ([]SectionHeader, error) {
	if header.SHNum == 0 {
		return nil, nil
	}

	if header.SHNum > MaxSections {
		return nil, fmt.Errorf("%w: too many sections", ErrInvalidELF)
	}

	offset := header.SHOff
	size := uint64(header.SHEntSize) * uint64(header.SHNum)

	if offset+size > uint64(len(data)) {
		return nil, ErrInvalidELF
	}

	sections := make([]SectionHeader, header.SHNum)
	for i := uint16(0); i < header.SHNum; i++ {
		off := offset + uint64(i)*uint64(header.SHEntSize)
		sec := &sections[i]
		sec.Name = binary.LittleEndian.Uint32(data[off : off+4])
		sec.Type = binary.LittleEndian.Uint32(data[off+4 : off+8])
		sec.Flags = binary.LittleEndian.Uint64(data[off+8 : off+16])
		sec.Addr = binary.LittleEndian.Uint64(data[off+16 : off+24])
		sec.Offset = binary.LittleEndian.Uint64(data[off+24 : off+32])
		sec.Size = binary.LittleEndian.Uint64(data[off+32 : off+40])
		sec.Link = binary.LittleEndian.Uint32(data[off+40 : off+44])
		sec.Info = binary.LittleEndian.Uint32(data[off+44 : off+48])
		sec.AddrAlign = binary.LittleEndian.Uint64(data[off+48 : off+56])
		sec.EntSize = binary.LittleEndian.Uint64(data[off+56 : off+64])
	}

	return sections, nil
}

// getSectionNames extracts section names from the string table.
func getSectionNames(data []byte, sections []SectionHeader, shstrndx uint16) ([]string, error) {
	if shstrndx >= uint16(len(sections)) {
		return nil, ErrInvalidSection
	}

	strtab := &sections[shstrndx]
	if strtab.Offset+strtab.Size > uint64(len(data)) {
		return nil, ErrInvalidSection
	}

	strtabData := data[strtab.Offset : strtab.Offset+strtab.Size]
	names := make([]string, len(sections))

	for i, sec := range sections {
		if sec.Name < uint32(len(strtabData)) {
			end := bytes.IndexByte(strtabData[sec.Name:], 0)
			if end == -1 {
				end = len(strtabData) - int(sec.Name)
			}
			names[i] = string(strtabData[sec.Name : sec.Name+uint32(end)])
		}
	}

	return names, nil
}

// findSection finds a section by name.
func findSection(sections []SectionHeader, names []string, name string) *SectionHeader {
	for i, n := range names {
		if n == name {
			return &sections[i]
		}
	}
	return nil
}

// extractSection extracts section data.
func extractSection(data []byte, section *SectionHeader) ([]byte, error) {
	if section.Type == shtNobits {
		// .bss section - return zero-filled buffer
		return make([]byte, section.Size), nil
	}

	if section.Offset+section.Size > uint64(len(data)) {
		return nil, ErrInvalidSection
	}

	result := make([]byte, section.Size)
	copy(result, data[section.Offset:section.Offset+section.Size])
	return result, nil
}

// extractText extracts and parses the text section.
func extractText(data []byte, section *SectionHeader) ([]uint64, error) {
	textData, err := extractSection(data, section)
	if err != nil {
		return nil, err
	}

	if len(textData)%8 != 0 {
		return nil, fmt.Errorf("%w: text section not aligned", ErrInvalidSection)
	}

	numInstructions := len(textData) / 8
	if numInstructions > MaxInstructions {
		return nil, fmt.Errorf("%w: too many instructions", ErrTooLarge)
	}

	text := make([]uint64, numInstructions)
	for i := 0; i < numInstructions; i++ {
		text[i] = binary.LittleEndian.Uint64(textData[i*8 : (i+1)*8])
	}

	return text, nil
}

// parseSymbols parses the symbol table.
func parseSymbols(data []byte, section *SectionHeader) ([]Symbol, error) {
	if section.EntSize == 0 {
		section.EntSize = 24 // Standard ELF64 symbol size
	}

	numSymbols := section.Size / section.EntSize
	if numSymbols > MaxSymbols {
		return nil, fmt.Errorf("%w: too many symbols", ErrInvalidELF)
	}

	if section.Offset+section.Size > uint64(len(data)) {
		return nil, ErrInvalidSection
	}

	symbols := make([]Symbol, numSymbols)
	for i := uint64(0); i < numSymbols; i++ {
		off := section.Offset + i*section.EntSize
		sym := &symbols[i]
		sym.Name = binary.LittleEndian.Uint32(data[off : off+4])
		sym.Info = data[off+4]
		sym.Other = data[off+5]
		sym.Shndx = binary.LittleEndian.Uint16(data[off+6 : off+8])
		sym.Value = binary.LittleEndian.Uint64(data[off+8 : off+16])
		sym.Size = binary.LittleEndian.Uint64(data[off+16 : off+24])
	}

	return symbols, nil
}

// getSymbolName gets a symbol name from the string table.
func getSymbolName(strtab []byte, nameOffset uint32) string {
	if nameOffset >= uint32(len(strtab)) {
		return ""
	}
	end := bytes.IndexByte(strtab[nameOffset:], 0)
	if end == -1 {
		end = len(strtab) - int(nameOffset)
	}
	return string(strtab[nameOffset : nameOffset+uint32(end)])
}

// processRelocations processes relocations in the text section.
func processRelocations(data []byte, section *SectionHeader, text []uint64, symbols []Symbol, strtab []byte, syscalls *[]uint32) error {
	if section.EntSize == 0 {
		section.EntSize = 24 // Standard ELF64 Rela size
	}

	numRelas := section.Size / section.EntSize
	if numRelas > MaxRelocations {
		return fmt.Errorf("%w: too many relocations", ErrInvalidELF)
	}

	if section.Offset+section.Size > uint64(len(data)) {
		return ErrInvalidSection
	}

	for i := uint64(0); i < numRelas; i++ {
		off := section.Offset + i*section.EntSize

		var rela Rela
		rela.Offset = binary.LittleEndian.Uint64(data[off : off+8])
		rela.Info = binary.LittleEndian.Uint64(data[off+8 : off+16])
		if section.EntSize >= 24 {
			rela.Addend = int64(binary.LittleEndian.Uint64(data[off+16 : off+24]))
		}

		symIdx := rela.Info >> 32
		relType := uint32(rela.Info & 0xffffffff)

		if symIdx >= uint64(len(symbols)) {
			continue // Skip invalid symbol references
		}

		sym := &symbols[symIdx]
		symName := getSymbolName(strtab, sym.Name)

		// Calculate instruction index
		insIdx := rela.Offset / 8
		if insIdx >= uint64(len(text)) {
			continue
		}

		switch relType {
		case rBPF64_32:
			// 32-bit immediate relocation - typically for function calls
			// The immediate field in the call instruction needs to be patched
			hash := murmur3Hash(symName)

			// Check if this is a syscall reference
			if sym.Shndx == 0 { // External symbol
				*syscalls = append(*syscalls, hash)
			}

			// Patch the immediate field (bits 32-63 of the instruction)
			ins := text[insIdx]
			ins = (ins & 0x00000000FFFFFFFF) | (uint64(hash) << 32)
			text[insIdx] = ins

		case rBPF64_64:
			// 64-bit immediate relocation (lddw instruction)
			// This uses two instruction slots
			if insIdx+1 >= uint64(len(text)) {
				continue
			}

			// Calculate target address
			targetAddr := uint64(sym.Value) + uint64(rela.Addend)

			// Patch both instruction slots
			// First slot: lower 32 bits in imm field
			ins1 := text[insIdx]
			ins1 = (ins1 & 0x00000000FFFFFFFF) | (uint64(uint32(targetAddr)) << 32)
			text[insIdx] = ins1

			// Second slot: upper 32 bits in imm field
			ins2 := text[insIdx+1]
			ins2 = (ins2 & 0x00000000FFFFFFFF) | (uint64(uint32(targetAddr>>32)) << 32)
			text[insIdx+1] = ins2

		case rBPFRelative:
			// Relative relocation
			relativeAddr := int64(insIdx*8) + rela.Addend

			ins := text[insIdx]
			ins = (ins & 0x00000000FFFFFFFF) | (uint64(int32(relativeAddr)) << 32)
			text[insIdx] = ins
		}
	}

	return nil
}

// murmur3Hash computes the murmur3 hash of a string.
// This is the same hash function used by Solana for syscall name hashing.
func murmur3Hash(name string) uint32 {
	return syscall.Murmur3Hash(name)
}

// LoadFromBytes is a convenience function to load an ELF from bytes.
func LoadFromBytes(data []byte) (*Executable, error) {
	loader := NewLoader()
	return loader.Load(data)
}
