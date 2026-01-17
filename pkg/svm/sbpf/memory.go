// Package sbpf implements memory operations for the sBPF VM.
package sbpf

import (
	"encoding/binary"
	"fmt"
)

// Memory access methods for the interpreter.

// Translate converts a virtual address to a memory slice.
func (ip *Interpreter) Translate(addr uint64, size uint64, write bool) ([]byte, error) {
	hi := addr >> 32
	lo := addr & 0xFFFFFFFF

	// Check for integer overflow in address calculation
	if size > 0 && lo > ^uint64(0)-size {
		return nil, fmt.Errorf("%w: address overflow at 0x%x (size %d)", ErrInvalidMemoryAccess, addr, size)
	}
	end := lo + size

	switch hi {
	case VaddrProgram >> 32:
		// Program/RO segment - read only
		if write {
			return nil, fmt.Errorf("%w: write to read-only program segment at 0x%x", ErrInvalidMemoryAccess, addr)
		}
		roLen := uint64(len(ip.ro))
		if end > roLen || lo > end {
			return nil, fmt.Errorf("%w: read beyond program segment at 0x%x (size %d, max %d)", ErrInvalidMemoryAccess, addr, size, roLen)
		}
		return ip.ro[lo:end], nil

	case VaddrStack >> 32:
		// Stack segment
		mem := ip.stack.GetFrame(uint32(lo))
		if mem == nil || uint64(len(mem)) < size {
			return nil, fmt.Errorf("%w: stack access at 0x%x (size %d)", ErrInvalidMemoryAccess, addr, size)
		}
		return mem[:size], nil

	case VaddrHeap >> 32:
		// Heap segment
		heapLen := uint64(len(ip.heap))
		if end > heapLen || lo > end {
			return nil, fmt.Errorf("%w: heap access at 0x%x (size %d, heap size %d)", ErrInvalidMemoryAccess, addr, size, heapLen)
		}
		return ip.heap[lo:end], nil

	case VaddrInput >> 32:
		// Input segment - read only
		if write {
			return nil, fmt.Errorf("%w: write to read-only input segment at 0x%x", ErrInvalidMemoryAccess, addr)
		}
		inputLen := uint64(len(ip.input))
		if end > inputLen || lo > end {
			return nil, fmt.Errorf("%w: read beyond input segment at 0x%x (size %d, max %d)", ErrInvalidMemoryAccess, addr, size, inputLen)
		}
		return ip.input[lo:end], nil

	default:
		return nil, fmt.Errorf("%w: unmapped region at 0x%x", ErrInvalidMemoryAccess, addr)
	}
}

// Read reads bytes from virtual memory.
func (ip *Interpreter) Read(addr uint64, p []byte) error {
	mem, err := ip.Translate(addr, uint64(len(p)), false)
	if err != nil {
		return err
	}
	copy(p, mem)
	return nil
}

// Read8 reads a byte from virtual memory.
func (ip *Interpreter) Read8(addr uint64) (uint8, error) {
	mem, err := ip.Translate(addr, 1, false)
	if err != nil {
		return 0, err
	}
	return mem[0], nil
}

// Read16 reads a 16-bit value from virtual memory (little-endian).
func (ip *Interpreter) Read16(addr uint64) (uint16, error) {
	mem, err := ip.Translate(addr, 2, false)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(mem), nil
}

// Read32 reads a 32-bit value from virtual memory (little-endian).
func (ip *Interpreter) Read32(addr uint64) (uint32, error) {
	mem, err := ip.Translate(addr, 4, false)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(mem), nil
}

// Read64 reads a 64-bit value from virtual memory (little-endian).
func (ip *Interpreter) Read64(addr uint64) (uint64, error) {
	mem, err := ip.Translate(addr, 8, false)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(mem), nil
}

// Write writes bytes to virtual memory.
func (ip *Interpreter) Write(addr uint64, p []byte) error {
	mem, err := ip.Translate(addr, uint64(len(p)), true)
	if err != nil {
		return err
	}
	copy(mem, p)
	return nil
}

// Write8 writes a byte to virtual memory.
func (ip *Interpreter) Write8(addr uint64, x uint8) error {
	mem, err := ip.Translate(addr, 1, true)
	if err != nil {
		return err
	}
	mem[0] = x
	return nil
}

// Write16 writes a 16-bit value to virtual memory (little-endian).
func (ip *Interpreter) Write16(addr uint64, x uint16) error {
	mem, err := ip.Translate(addr, 2, true)
	if err != nil {
		return err
	}
	binary.LittleEndian.PutUint16(mem, x)
	return nil
}

// Write32 writes a 32-bit value to virtual memory (little-endian).
func (ip *Interpreter) Write32(addr uint64, x uint32) error {
	mem, err := ip.Translate(addr, 4, true)
	if err != nil {
		return err
	}
	binary.LittleEndian.PutUint32(mem, x)
	return nil
}

// Write64 writes a 64-bit value to virtual memory (little-endian).
func (ip *Interpreter) Write64(addr uint64, x uint64) error {
	mem, err := ip.Translate(addr, 8, true)
	if err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(mem, x)
	return nil
}
