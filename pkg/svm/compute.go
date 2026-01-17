// Package svm implements the Solana Virtual Machine for X1.
package svm

import (
	"errors"
	"sync/atomic"
)

// Compute unit cost constants.
// These match the Solana/Agave reference implementation.
const (
	// Base costs
	CUDefault                    = uint64(200_000)  // Default CU limit per instruction
	CUMax                        = uint64(1_400_000) // Max CU limit per transaction
	CUSyscallBase                = uint64(100)      // Base cost for syscalls
	CUInvokeBase                 = uint64(1_000)    // Base cost for CPI

	// Signature verification
	CUSignatureVerify            = uint64(720)      // Ed25519 signature verification
	CUSecp256k1Recover           = uint64(25_000)   // Secp256k1 recovery

	// Cryptographic operations
	CUSha256Base                 = uint64(85)       // SHA256 base cost
	CUSha256PerByte              = uint64(1)        // SHA256 per byte
	CUKeccak256Base              = uint64(85)       // Keccak256 base cost
	CUKeccak256PerByte           = uint64(1)        // Keccak256 per byte
	CUBlake3Base                 = uint64(85)       // Blake3 base cost
	CUBlake3PerByte              = uint64(1)        // Blake3 per byte

	// Memory operations
	CUMemoryOpBase               = uint64(10)       // Base cost for memory ops
	CUMemoryOpPerByte            = uint64(1)        // Per byte for memory ops

	// Address derivation
	CUCreateProgramAddress       = uint64(1_500)    // create_program_address
	CUFindProgramAddress         = uint64(1_500)    // find_program_address per iteration

	// Curve operations
	CUCurve25519PointValidation  = uint64(5_000)    // Point validation
	CUCurve25519Add              = uint64(5_000)    // Point addition
	CUCurve25519Subtract         = uint64(5_000)    // Point subtraction
	CUCurve25519Multiply         = uint64(10_000)   // Scalar multiplication

	// Account operations
	CUWriteLock                  = uint64(300)      // Write lock cost

	// Heap
	CUHeapCostDefault            = uint64(8)        // Per 32KB heap page

	// Native program defaults
	CUSystemProgramDefault       = uint64(150)      // System program base
	CUVoteProgramDefault         = uint64(2_100)    // Vote program base
	CUStakeProgramDefault        = uint64(750)      // Stake program base
	CUComputeBudgetDefault       = uint64(150)      // Compute budget base
	CUAddressLookupTableDefault  = uint64(750)      // ALT base
	CUBPFLoaderDefault           = uint64(570)      // BPF loader base
)

// Heap size constants.
const (
	HeapSizeDefault = uint32(32 * 1024)  // 32 KB
	HeapSizeMin     = uint32(32 * 1024)  // 32 KB minimum
	HeapSizeMax     = uint32(256 * 1024) // 256 KB maximum
)

// Stack constants.
const (
	StackFrameSize = uint64(4096) // 4 KB per frame
	StackDepthMax  = uint64(64)   // Max call depth
	StackSizeMax   = StackFrameSize * StackDepthMax // 256 KB total
)

// CPI constants.
const (
	CPIDepthMax = uint64(4) // Max CPI nesting depth
)

var (
	// ErrComputeExceeded is returned when compute units are exhausted.
	ErrComputeExceeded = errors.New("compute budget exceeded")

	// ErrComputeInvalidLimit is returned for invalid compute limit.
	ErrComputeInvalidLimit = errors.New("invalid compute unit limit")
)

// ComputeMeter tracks compute unit consumption.
type ComputeMeter struct {
	remaining uint64
	consumed  uint64
	limit     uint64
	disabled  bool
}

// NewComputeMeter creates a new compute meter with the specified limit.
func NewComputeMeter(limit uint64) *ComputeMeter {
	if limit > CUMax {
		limit = CUMax
	}
	return &ComputeMeter{
		remaining: limit,
		consumed:  0,
		limit:     limit,
		disabled:  false,
	}
}

// NewComputeMeterDisabled creates a disabled compute meter (for testing).
func NewComputeMeterDisabled() *ComputeMeter {
	return &ComputeMeter{
		remaining: CUMax,
		consumed:  0,
		limit:     CUMax,
		disabled:  true,
	}
}

// Consume attempts to consume the specified compute units.
// Returns ErrComputeExceeded if insufficient units remain.
func (cm *ComputeMeter) Consume(cost uint64) error {
	if cm.disabled {
		return nil
	}

	for {
		remaining := atomic.LoadUint64(&cm.remaining)
		if remaining < cost {
			atomic.StoreUint64(&cm.remaining, 0)
			return ErrComputeExceeded
		}
		if atomic.CompareAndSwapUint64(&cm.remaining, remaining, remaining-cost) {
			atomic.AddUint64(&cm.consumed, cost)
			return nil
		}
	}
}

// ConsumeChecked consumes compute units, returning the actual amount consumed.
// Does not return error - caller should check remaining.
func (cm *ComputeMeter) ConsumeChecked(cost uint64) uint64 {
	if cm.disabled {
		return cost
	}

	for {
		remaining := atomic.LoadUint64(&cm.remaining)
		actual := cost
		if remaining < cost {
			actual = remaining
		}
		if atomic.CompareAndSwapUint64(&cm.remaining, remaining, remaining-actual) {
			atomic.AddUint64(&cm.consumed, actual)
			return actual
		}
	}
}

// Remaining returns the remaining compute units.
func (cm *ComputeMeter) Remaining() uint64 {
	return atomic.LoadUint64(&cm.remaining)
}

// Consumed returns the total consumed compute units.
func (cm *ComputeMeter) Consumed() uint64 {
	return atomic.LoadUint64(&cm.consumed)
}

// Limit returns the compute unit limit.
func (cm *ComputeMeter) Limit() uint64 {
	return cm.limit
}

// IsExhausted returns true if compute units are exhausted.
func (cm *ComputeMeter) IsExhausted() bool {
	return atomic.LoadUint64(&cm.remaining) == 0
}

// Reset resets the compute meter to its initial state.
func (cm *ComputeMeter) Reset() {
	atomic.StoreUint64(&cm.remaining, cm.limit)
	atomic.StoreUint64(&cm.consumed, 0)
}

// ComputeBudgetLimits contains the parsed compute budget for a transaction.
type ComputeBudgetLimits struct {
	// ComputeUnitLimit is the maximum compute units for the transaction.
	ComputeUnitLimit uint32

	// ComputeUnitPrice is the price in micro-lamports per compute unit.
	ComputeUnitPrice uint64

	// HeapSize is the requested heap size in bytes.
	HeapSize uint32

	// LoadedAccountsBytes is the max bytes for loaded accounts.
	LoadedAccountsBytes uint32
}

// DefaultComputeBudgetLimits returns the default compute budget limits.
func DefaultComputeBudgetLimits() *ComputeBudgetLimits {
	return &ComputeBudgetLimits{
		ComputeUnitLimit:    uint32(CUDefault),
		ComputeUnitPrice:    0,
		HeapSize:            HeapSizeDefault,
		LoadedAccountsBytes: 64 * 1024 * 1024, // 64 MB
	}
}
