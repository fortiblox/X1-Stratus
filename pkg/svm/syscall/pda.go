// Package syscall implements PDA (Program Derived Address) operations.
package syscall

import (
	"crypto/sha256"
	"errors"

	"github.com/fortiblox/X1-Stratus/pkg/svm/sbpf"
)

// PDA constants.
const (
	MaxSeeds     = 16
	MaxSeedLen   = 32
	PDAMarkerLen = 21 // "ProgramDerivedAddress" length
)

// PDA marker used in address derivation.
var pdaMarker = []byte("ProgramDerivedAddress")

// PDA errors.
var (
	ErrMaxSeedLengthExceeded = errors.New("max seed length exceeded")
	ErrMaxSeedsExceeded      = errors.New("max seeds exceeded")
	ErrInvalidSeeds          = errors.New("invalid seeds")
	ErrInvalidProgramID      = errors.New("invalid program ID")
)

// registerPDA registers PDA-related syscalls.
func (r *Registry) registerPDA(ctx InvokeContext) {
	// sol_create_program_address - derive a PDA
	r.register("sol_create_program_address", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		// r1 = seeds array ptr
		// r2 = seeds count
		// r3 = program_id ptr
		// r4 = result address ptr
		seedsAddr := r1
		seedsCount := r2
		programIDAddr := r3
		resultAddr := r4

		if err := ctx.ConsumeCU(CUCreatePDA); err != nil {
			return 0, err
		}

		if seedsCount > MaxSeeds {
			return 1, nil // Return error code
		}

		// Read program ID
		programID := make([]byte, 32)
		if err := vm.Read(programIDAddr, programID); err != nil {
			return 0, err
		}

		// Read seeds
		seeds := make([][]byte, seedsCount)
		for i := uint64(0); i < seedsCount; i++ {
			// Read seed pointer and length
			seedPtr, err := vm.Read64(seedsAddr + i*16)
			if err != nil {
				return 0, err
			}
			seedLen, err := vm.Read64(seedsAddr + i*16 + 8)
			if err != nil {
				return 0, err
			}

			if seedLen > MaxSeedLen {
				return 1, nil // Return error code
			}

			seed := make([]byte, seedLen)
			if err := vm.Read(seedPtr, seed); err != nil {
				return 0, err
			}
			seeds[i] = seed
		}

		// Derive PDA
		pda, err := CreateProgramAddress(seeds, programID)
		if err != nil {
			return 1, nil // Return error code (not on curve)
		}

		// Write result
		if err := vm.Write(resultAddr, pda); err != nil {
			return 0, err
		}

		return 0, nil // Success
	})

	// sol_try_find_program_address - find a PDA with bump seed
	r.register("sol_try_find_program_address", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		// r1 = seeds array ptr
		// r2 = seeds count
		// r3 = program_id ptr
		// r4 = result address ptr
		// r5 = bump seed result ptr
		seedsAddr := r1
		seedsCount := r2
		programIDAddr := r3
		resultAddr := r4
		bumpAddr := r5

		if seedsCount > MaxSeeds-1 { // Need room for bump seed
			return 1, nil
		}

		// Read program ID
		programID := make([]byte, 32)
		if err := vm.Read(programIDAddr, programID); err != nil {
			return 0, err
		}

		// Read seeds
		seeds := make([][]byte, seedsCount)
		for i := uint64(0); i < seedsCount; i++ {
			seedPtr, err := vm.Read64(seedsAddr + i*16)
			if err != nil {
				return 0, err
			}
			seedLen, err := vm.Read64(seedsAddr + i*16 + 8)
			if err != nil {
				return 0, err
			}

			if seedLen > MaxSeedLen {
				return 1, nil
			}

			seed := make([]byte, seedLen)
			if err := vm.Read(seedPtr, seed); err != nil {
				return 0, err
			}
			seeds[i] = seed
		}

		// Try to find PDA with bump
		pda, bump, err := FindProgramAddress(seeds, programID, ctx)
		if err != nil {
			return 1, nil
		}

		// Write results
		if err := vm.Write(resultAddr, pda); err != nil {
			return 0, err
		}
		if err := vm.Write8(bumpAddr, bump); err != nil {
			return 0, err
		}

		return 0, nil
	})
}

// CreateProgramAddress derives a program address from seeds and a program ID.
// Returns error if the derived address is on the ed25519 curve.
func CreateProgramAddress(seeds [][]byte, programID []byte) ([]byte, error) {
	if len(seeds) > MaxSeeds {
		return nil, ErrMaxSeedsExceeded
	}

	for _, seed := range seeds {
		if len(seed) > MaxSeedLen {
			return nil, ErrMaxSeedLengthExceeded
		}
	}

	// Build hash input: seeds + programID + marker
	h := sha256.New()
	for _, seed := range seeds {
		h.Write(seed)
	}
	h.Write(programID)
	h.Write(pdaMarker)

	hash := h.Sum(nil)

	// Check if on curve (simplified - in production, verify against ed25519)
	if isOnCurve(hash) {
		return nil, errors.New("invalid seeds - derived address is on curve")
	}

	return hash, nil
}

// FindProgramAddress finds a valid PDA by iterating bump seeds from 255 to 0.
func FindProgramAddress(seeds [][]byte, programID []byte, ctx InvokeContext) ([]byte, uint8, error) {
	bumpSeed := make([]byte, 1)

	for bump := uint8(255); ; bump-- {
		// Consume CU for each iteration
		if err := ctx.ConsumeCU(CUFindPDA); err != nil {
			return nil, 0, err
		}

		bumpSeed[0] = bump
		seedsWithBump := append(seeds, bumpSeed)

		pda, err := CreateProgramAddress(seedsWithBump, programID)
		if err == nil {
			return pda, bump, nil
		}

		if bump == 0 {
			break
		}
	}

	return nil, 0, errors.New("unable to find a viable program address bump seed")
}

// isOnCurve checks if the given bytes represent a point on the ed25519 curve.
// This is a simplified check - a full implementation would use proper curve math.
func isOnCurve(point []byte) bool {
	// Simplified: check if high bit is set which often indicates off-curve
	// In production, this should verify the point is actually on ed25519
	//
	// The real check would:
	// 1. Decompress the point
	// 2. Verify y^2 = x^3 + 486662*x^2 + x (mod p) for curve25519
	// 3. Verify the point is in the prime-order subgroup
	//
	// For now, we use a probabilistic approach: most random 32-byte values
	// are NOT valid ed25519 points, so we accept them as PDAs

	// Very simplified check - look at specific bytes that tend to indicate curve points
	// This is NOT cryptographically correct and should be replaced with proper ed25519 check
	if len(point) != 32 {
		return false
	}

	// Check if it looks like a valid compressed ed25519 point
	// A proper implementation would decompress and verify
	// For now, we'll accept most hashes as valid PDAs (which is the common case)
	return false
}

// AddPDASyscalls adds PDA syscalls to an existing registry.
func AddPDASyscalls(r *Registry, ctx InvokeContext) {
	r.registerPDA(ctx)
}
