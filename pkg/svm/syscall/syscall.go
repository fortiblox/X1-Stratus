// Package syscall implements Solana syscalls for the sBPF VM.
//
// Syscalls are host functions callable from sBPF programs. Each syscall
// is identified by a hash of its name (murmur3). Arguments are passed in
// registers r1-r5, and the return value is placed in r0.
package syscall

import (
	"crypto/sha256"
	"errors"

	"github.com/fortiblox/X1-Stratus/pkg/svm/sbpf"
	"github.com/zeebo/blake3"
	"golang.org/x/crypto/sha3"
)

// Syscall errors.
var (
	ErrInvalidPointer     = errors.New("invalid pointer")
	ErrInvalidLength      = errors.New("invalid length")
	ErrAccessViolation    = errors.New("access violation")
	ErrComputeExceeded    = errors.New("compute budget exceeded")
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrMaxInstructionData = errors.New("max instruction data size exceeded")
)

// Compute costs for syscalls.
const (
	CUSyscallBase      = uint64(100)
	CULogBase          = uint64(100)
	CULogPerByte       = uint64(1)
	CULogPubkey        = uint64(100)
	CULog64            = uint64(100)
	CUMemOpBase        = uint64(10)
	CUMemOpPerByte     = uint64(1)
	CUSha256Base       = uint64(85)
	CUSha256PerByte    = uint64(1)
	CUKeccak256Base    = uint64(85)
	CUKeccak256PerByte = uint64(1)
	CUBlake3Base       = uint64(85)
	CUBlake3PerByte    = uint64(1)
	CUCreatePDA        = uint64(1500)
	CUFindPDA          = uint64(1500)
)

// Maximum sizes.
const (
	MaxLogMsgLen          = 10000             // Maximum log message length
	MaxReturnData         = 1024              // Maximum return data size
	MaxCPIInstructionData = 10240             // Maximum CPI instruction data
	MaxMemOpSize          = 10 * 1024 * 1024  // Maximum memory operation size (10 MB)
)

// InvokeContext provides execution context to syscalls.
type InvokeContext interface {
	// Logging
	Log(msg string)
	LogData(data [][]byte)

	// Return data
	SetReturnData(programID [32]byte, data []byte) error
	GetReturnData() (programID [32]byte, data []byte)

	// Compute metering
	ConsumeCU(cost uint64) error
	RemainingCU() uint64

	// Program info
	GetProgramID() [32]byte
	GetCallerProgramID() ([32]byte, bool)
}

// Registry holds all registered syscalls.
type Registry struct {
	syscalls map[uint32]sbpf.Syscall
}

// NewRegistry creates a new syscall registry with all standard syscalls.
func NewRegistry(ctx InvokeContext) *Registry {
	r := &Registry{
		syscalls: make(map[uint32]sbpf.Syscall),
	}

	// Register all syscalls
	r.registerLogging(ctx)
	r.registerMemory(ctx)
	r.registerCrypto(ctx)
	r.registerMisc(ctx)

	return r
}

// Get returns a syscall by its hash.
func (r *Registry) Get(hash uint32) (sbpf.Syscall, bool) {
	sc, ok := r.syscalls[hash]
	return sc, ok
}

// Lookup returns the registry lookup function.
func (r *Registry) Lookup() sbpf.SyscallRegistry {
	return func(hash uint32) (sbpf.Syscall, bool) {
		return r.Get(hash)
	}
}

// register adds a syscall to the registry.
func (r *Registry) register(name string, fn sbpf.SyscallFunc) {
	hash := murmur3Hash(name)
	r.syscalls[hash] = fn
}

// registerLogging registers logging syscalls.
func (r *Registry) registerLogging(ctx InvokeContext) {
	// sol_log_ - log a message
	r.register("sol_log_", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		msgLen := r2
		if msgLen > MaxLogMsgLen {
			msgLen = MaxLogMsgLen
		}

		// Compute cost
		cost := CULogBase + CULogPerByte*msgLen
		if err := ctx.ConsumeCU(cost); err != nil {
			return 0, err
		}

		// Read message from VM memory
		msg := make([]byte, msgLen)
		if err := vm.Read(r1, msg); err != nil {
			return 0, err
		}

		ctx.Log(string(msg))
		return 0, nil
	})

	// sol_log_64_ - log 5 integers
	r.register("sol_log_64_", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		if err := ctx.ConsumeCU(CULog64); err != nil {
			return 0, err
		}

		ctx.LogData([][]byte{
			uint64ToBytes(r1),
			uint64ToBytes(r2),
			uint64ToBytes(r3),
			uint64ToBytes(r4),
			uint64ToBytes(r5),
		})
		return 0, nil
	})

	// sol_log_pubkey - log a pubkey
	r.register("sol_log_pubkey", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		if err := ctx.ConsumeCU(CULogPubkey); err != nil {
			return 0, err
		}

		pubkey := make([]byte, 32)
		if err := vm.Read(r1, pubkey); err != nil {
			return 0, err
		}

		ctx.LogData([][]byte{pubkey})
		return 0, nil
	})

	// sol_log_compute_units_ - log remaining compute units
	r.register("sol_log_compute_units_", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		if err := ctx.ConsumeCU(CUSyscallBase); err != nil {
			return 0, err
		}

		remaining := ctx.RemainingCU()
		ctx.LogData([][]byte{uint64ToBytes(remaining)})
		return 0, nil
	})

	// sol_log_data - log arbitrary data slices
	r.register("sol_log_data", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		// r1 = pointer to array of (ptr, len) pairs
		// r2 = number of data slices
		numSlices := r2
		if numSlices == 0 || numSlices > 100 {
			return 0, ErrInvalidArgument
		}

		if err := ctx.ConsumeCU(CULogBase); err != nil {
			return 0, err
		}

		var data [][]byte
		for i := uint64(0); i < numSlices; i++ {
			// Read (ptr, len) pair
			ptr, err := vm.Read64(r1 + i*16)
			if err != nil {
				return 0, err
			}
			length, err := vm.Read64(r1 + i*16 + 8)
			if err != nil {
				return 0, err
			}

			if length > MaxLogMsgLen {
				return 0, ErrInvalidLength
			}

			// Consume CU for data
			if err := ctx.ConsumeCU(CULogPerByte * length); err != nil {
				return 0, err
			}

			// Read data
			slice := make([]byte, length)
			if err := vm.Read(ptr, slice); err != nil {
				return 0, err
			}
			data = append(data, slice)
		}

		ctx.LogData(data)
		return 0, nil
	})
}

// registerMemory registers memory syscalls.
func (r *Registry) registerMemory(ctx InvokeContext) {
	// sol_memcpy_ - copy memory
	r.register("sol_memcpy_", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		dst, src, n := r1, r2, r3

		if n == 0 {
			return 0, nil
		}

		// Validate size to prevent DoS via memory exhaustion
		if n > MaxMemOpSize {
			return 0, ErrInvalidLength
		}

		// Check for compute cost overflow
		if n > (^uint64(0)-CUMemOpBase)/CUMemOpPerByte {
			return 0, ErrComputeExceeded
		}
		cost := CUMemOpBase + CUMemOpPerByte*n
		if err := ctx.ConsumeCU(cost); err != nil {
			return 0, err
		}

		// Read source
		data := make([]byte, n)
		if err := vm.Read(src, data); err != nil {
			return 0, err
		}

		// Write to destination
		if err := vm.Write(dst, data); err != nil {
			return 0, err
		}

		return 0, nil
	})

	// sol_memmove_ - move memory (handles overlapping regions)
	r.register("sol_memmove_", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		dst, src, n := r1, r2, r3

		if n == 0 {
			return 0, nil
		}

		// Validate size to prevent DoS
		if n > MaxMemOpSize {
			return 0, ErrInvalidLength
		}

		// Check for compute cost overflow
		if n > (^uint64(0)-CUMemOpBase)/CUMemOpPerByte {
			return 0, ErrComputeExceeded
		}
		cost := CUMemOpBase + CUMemOpPerByte*n
		if err := ctx.ConsumeCU(cost); err != nil {
			return 0, err
		}

		// Read source first (handles overlap)
		data := make([]byte, n)
		if err := vm.Read(src, data); err != nil {
			return 0, err
		}

		// Write to destination
		if err := vm.Write(dst, data); err != nil {
			return 0, err
		}

		return 0, nil
	})

	// sol_memset_ - set memory to a value
	r.register("sol_memset_", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		dst, val, n := r1, uint8(r2), r3

		if n == 0 {
			return 0, nil
		}

		// Validate size to prevent DoS
		if n > MaxMemOpSize {
			return 0, ErrInvalidLength
		}

		// Check for compute cost overflow
		if n > (^uint64(0)-CUMemOpBase)/CUMemOpPerByte {
			return 0, ErrComputeExceeded
		}
		cost := CUMemOpBase + CUMemOpPerByte*n
		if err := ctx.ConsumeCU(cost); err != nil {
			return 0, err
		}

		// Create filled buffer
		data := make([]byte, n)
		for i := range data {
			data[i] = val
		}

		// Write to destination
		if err := vm.Write(dst, data); err != nil {
			return 0, err
		}

		return 0, nil
	})

	// sol_memcmp_ - compare memory
	r.register("sol_memcmp_", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		addr1, addr2, n, resultAddr := r1, r2, r3, r4

		// Validate length to prevent unbounded allocation
		if n > MaxMemOpSize {
			return 0, ErrInvalidLength
		}

		cost := CUMemOpBase + CUMemOpPerByte*n
		if err := ctx.ConsumeCU(cost); err != nil {
			return 0, err
		}

		if n == 0 {
			// Write 0 result
			if err := vm.Write32(resultAddr, 0); err != nil {
				return 0, err
			}
			return 0, nil
		}

		// Read both memory regions
		data1 := make([]byte, n)
		data2 := make([]byte, n)
		if err := vm.Read(addr1, data1); err != nil {
			return 0, err
		}
		if err := vm.Read(addr2, data2); err != nil {
			return 0, err
		}

		// Compare
		var result int32
		for i := uint64(0); i < n; i++ {
			if data1[i] != data2[i] {
				result = int32(data1[i]) - int32(data2[i])
				break
			}
		}

		// Write result
		if err := vm.Write32(resultAddr, uint32(result)); err != nil {
			return 0, err
		}

		return 0, nil
	})

	// sol_alloc_free_ - heap allocator (simplified bump allocator)
	r.register("sol_alloc_free_", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		size := r1
		// freeAddr := r2 // Free is no-op in bump allocator

		if err := ctx.ConsumeCU(CUSyscallBase); err != nil {
			return 0, err
		}

		if size == 0 {
			return 0, nil
		}

		// Align to 8 bytes
		size = (size + 7) &^ 7

		// Check heap capacity
		currentHeap := vm.HeapSize()
		heapMax := vm.HeapMax()

		if currentHeap+size > heapMax {
			return 0, nil // Return 0 (null) on allocation failure
		}

		// Expand heap
		vm.UpdateHeapSize(currentHeap + size)

		// Return pointer to allocated region
		return sbpf.VaddrHeap + currentHeap, nil
	})
}

// registerCrypto registers cryptographic syscalls.
func (r *Registry) registerCrypto(ctx InvokeContext) {
	// sol_sha256 - SHA256 hash
	r.register("sol_sha256", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		// r1 = pointer to array of (ptr, len) pairs
		// r2 = number of slices
		// r3 = result pointer (32 bytes)
		numSlices := r2
		resultAddr := r3

		if numSlices > 100 {
			return 0, ErrInvalidArgument
		}

		if err := ctx.ConsumeCU(CUSha256Base); err != nil {
			return 0, err
		}

		h := sha256.New()

		for i := uint64(0); i < numSlices; i++ {
			ptr, err := vm.Read64(r1 + i*16)
			if err != nil {
				return 0, err
			}
			length, err := vm.Read64(r1 + i*16 + 8)
			if err != nil {
				return 0, err
			}

			// Validate length to prevent unbounded allocation
			if length > MaxMemOpSize {
				return 0, ErrInvalidLength
			}

			if err := ctx.ConsumeCU(CUSha256PerByte * length); err != nil {
				return 0, err
			}

			data := make([]byte, length)
			if err := vm.Read(ptr, data); err != nil {
				return 0, err
			}
			h.Write(data)
		}

		result := h.Sum(nil)
		if err := vm.Write(resultAddr, result); err != nil {
			return 0, err
		}

		return 0, nil
	})

	// sol_keccak256 - Keccak256 hash
	r.register("sol_keccak256", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		// r1 = pointer to array of (ptr, len) pairs
		// r2 = number of slices
		// r3 = result pointer (32 bytes)
		numSlices := r2
		resultAddr := r3

		if numSlices > 100 {
			return 0, ErrInvalidArgument
		}

		if err := ctx.ConsumeCU(CUKeccak256Base); err != nil {
			return 0, err
		}

		h := sha3.NewLegacyKeccak256()

		for i := uint64(0); i < numSlices; i++ {
			ptr, err := vm.Read64(r1 + i*16)
			if err != nil {
				return 0, err
			}
			length, err := vm.Read64(r1 + i*16 + 8)
			if err != nil {
				return 0, err
			}

			// Validate size
			if length > MaxMemOpSize {
				return 0, ErrInvalidLength
			}

			if err := ctx.ConsumeCU(CUKeccak256PerByte * length); err != nil {
				return 0, err
			}

			data := make([]byte, length)
			if err := vm.Read(ptr, data); err != nil {
				return 0, err
			}
			h.Write(data)
		}

		result := h.Sum(nil)
		if err := vm.Write(resultAddr, result); err != nil {
			return 0, err
		}

		return 0, nil
	})

	// sol_blake3 - Blake3 hash
	r.register("sol_blake3", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		// r1 = pointer to array of (ptr, len) pairs
		// r2 = number of slices
		// r3 = result pointer (32 bytes)
		numSlices := r2
		resultAddr := r3

		if numSlices > 100 {
			return 0, ErrInvalidArgument
		}

		if err := ctx.ConsumeCU(CUBlake3Base); err != nil {
			return 0, err
		}

		h := blake3.New()

		for i := uint64(0); i < numSlices; i++ {
			ptr, err := vm.Read64(r1 + i*16)
			if err != nil {
				return 0, err
			}
			length, err := vm.Read64(r1 + i*16 + 8)
			if err != nil {
				return 0, err
			}

			// Validate size
			if length > MaxMemOpSize {
				return 0, ErrInvalidLength
			}

			if err := ctx.ConsumeCU(CUBlake3PerByte * length); err != nil {
				return 0, err
			}

			data := make([]byte, length)
			if err := vm.Read(ptr, data); err != nil {
				return 0, err
			}
			h.Write(data)
		}

		result := make([]byte, 32)
		h.Sum(result[:0])
		if err := vm.Write(resultAddr, result); err != nil {
			return 0, err
		}

		return 0, nil
	})
}

// registerMisc registers miscellaneous syscalls.
func (r *Registry) registerMisc(ctx InvokeContext) {
	// abort - terminate execution
	r.register("abort", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		return 0, errors.New("program aborted")
	})

	// panic_ - panic with message
	r.register("sol_panic_", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		// r1 = filename ptr, r2 = filename len
		// r3 = line number, r4 = column
		fileLen := r2
		if fileLen > 256 {
			fileLen = 256
		}

		filename := make([]byte, fileLen)
		if err := vm.Read(r1, filename); err != nil {
			return 0, errors.New("program panicked")
		}

		return 0, errors.New("program panicked at " + string(filename))
	})

	// sol_set_return_data - set return data
	r.register("sol_set_return_data", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		dataAddr, dataLen := r1, r2

		if err := ctx.ConsumeCU(CUSyscallBase); err != nil {
			return 0, err
		}

		if dataLen > MaxReturnData {
			return 0, ErrMaxInstructionData
		}

		data := make([]byte, dataLen)
		if err := vm.Read(dataAddr, data); err != nil {
			return 0, err
		}

		programID := ctx.GetProgramID()
		if err := ctx.SetReturnData(programID, data); err != nil {
			return 0, err
		}

		return 0, nil
	})

	// sol_get_return_data - get return data from last CPI
	r.register("sol_get_return_data", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		dstAddr, maxLen, programIDAddr := r1, r2, r3

		if err := ctx.ConsumeCU(CUSyscallBase); err != nil {
			return 0, err
		}

		programID, data := ctx.GetReturnData()

		// Copy data
		copyLen := uint64(len(data))
		if copyLen > maxLen {
			copyLen = maxLen
		}

		if copyLen > 0 {
			if err := vm.Write(dstAddr, data[:copyLen]); err != nil {
				return 0, err
			}
		}

		// Copy program ID
		if err := vm.Write(programIDAddr, programID[:]); err != nil {
			return 0, err
		}

		return uint64(len(data)), nil
	})

	// sol_get_stack_height - get current CPI stack height
	r.register("sol_get_stack_height", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		if err := ctx.ConsumeCU(CUSyscallBase); err != nil {
			return 0, err
		}
		// Return 1 for now (single program execution)
		return 1, nil
	})
}

// Murmur3Hash computes the murmur3 hash of a syscall name.
// This is the standard murmur3 hash used by Solana for syscall identification.
func Murmur3Hash(name string) uint32 {
	return murmur3Hash(name)
}

// murmur3Hash computes the murmur3 hash of a syscall name.
// This is a simplified version for syscall name hashing.
func murmur3Hash(name string) uint32 {
	const (
		c1 = 0xcc9e2d51
		c2 = 0x1b873593
	)

	data := []byte(name)
	h1 := uint32(0)
	length := len(data)

	// Process 4-byte chunks
	nblocks := length / 4
	for i := 0; i < nblocks; i++ {
		k1 := uint32(data[i*4]) |
			uint32(data[i*4+1])<<8 |
			uint32(data[i*4+2])<<16 |
			uint32(data[i*4+3])<<24

		k1 *= c1
		k1 = (k1 << 15) | (k1 >> 17)
		k1 *= c2

		h1 ^= k1
		h1 = (h1 << 13) | (h1 >> 19)
		h1 = h1*5 + 0xe6546b64
	}

	// Process remaining bytes
	tail := data[nblocks*4:]
	var k1 uint32
	switch len(tail) {
	case 3:
		k1 ^= uint32(tail[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint32(tail[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint32(tail[0])
		k1 *= c1
		k1 = (k1 << 15) | (k1 >> 17)
		k1 *= c2
		h1 ^= k1
	}

	// Finalization
	h1 ^= uint32(length)
	h1 ^= h1 >> 16
	h1 *= 0x85ebca6b
	h1 ^= h1 >> 13
	h1 *= 0xc2b2ae35
	h1 ^= h1 >> 16

	return h1
}

// uint64ToBytes converts a uint64 to little-endian bytes.
func uint64ToBytes(v uint64) []byte {
	return []byte{
		byte(v),
		byte(v >> 8),
		byte(v >> 16),
		byte(v >> 24),
		byte(v >> 32),
		byte(v >> 40),
		byte(v >> 48),
		byte(v >> 56),
	}
}
