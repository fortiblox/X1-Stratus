// Package syscall implements PDA (Program Derived Address) operations.
package syscall

import (
	"crypto/sha256"
	"errors"
	"math/big"

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
	for bump := uint8(255); ; bump-- {
		// Consume CU for each iteration
		if err := ctx.ConsumeCU(CUFindPDA); err != nil {
			return nil, 0, err
		}

		// Create a new slice to avoid modifying the input
		bumpSeed := []byte{bump}
		seedsWithBump := make([][]byte, len(seeds)+1)
		copy(seedsWithBump, seeds)
		seedsWithBump[len(seeds)] = bumpSeed

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
// This implements the full mathematical verification using the curve equation.
//
// Ed25519 uses the twisted Edwards curve: -x^2 + y^2 = 1 + d*x^2*y^2
// where d = -121665/121666 (mod p) and p = 2^255 - 19
//
// A compressed point stores the y-coordinate and the sign of x.
// To verify, we compute x^2 from y and check if it has a valid square root.
func isOnCurve(point []byte) bool {
	if len(point) != 32 {
		return false
	}

	// Field prime p = 2^255 - 19
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))

	// Curve parameter d = -121665/121666 (mod p)
	// d = -121665 * inverse(121666) mod p
	d := new(big.Int).Mul(big.NewInt(-121665), new(big.Int).ModInverse(big.NewInt(121666), p))
	d.Mod(d, p)

	// Extract y-coordinate (little-endian, clear high bit which is sign of x)
	yBytes := make([]byte, 32)
	copy(yBytes, point)
	yBytes[31] &= 0x7F // Clear sign bit

	// Convert to big.Int (little-endian)
	y := new(big.Int)
	for i := 31; i >= 0; i-- {
		y.Lsh(y, 8)
		y.Or(y, big.NewInt(int64(yBytes[i])))
	}

	// Check y is in valid range [0, p)
	if y.Cmp(p) >= 0 {
		return false
	}

	// Compute x^2 from curve equation: -x^2 + y^2 = 1 + d*x^2*y^2
	// Rearranging: x^2 = (y^2 - 1) / (d*y^2 + 1)
	y2 := new(big.Int).Mul(y, y)
	y2.Mod(y2, p)

	// numerator = y^2 - 1
	num := new(big.Int).Sub(y2, big.NewInt(1))
	num.Mod(num, p)

	// denominator = d*y^2 + 1
	den := new(big.Int).Mul(d, y2)
	den.Add(den, big.NewInt(1))
	den.Mod(den, p)

	// x^2 = num * inverse(den) mod p
	denInv := new(big.Int).ModInverse(den, p)
	if denInv == nil {
		return false // No inverse means invalid point
	}
	x2 := new(big.Int).Mul(num, denInv)
	x2.Mod(x2, p)

	// Check if x^2 has a square root in the field
	// Use Euler's criterion: x^2 is a quadratic residue iff x^2^((p-1)/2) = 1 (mod p)
	// Or compute sqrt and verify
	exp := new(big.Int).Sub(p, big.NewInt(1))
	exp.Rsh(exp, 1) // (p-1)/2

	legendre := new(big.Int).Exp(x2, exp, p)

	// If legendre symbol is 1 or x^2 is 0, point is on curve
	return legendre.Cmp(big.NewInt(1)) == 0 || x2.Sign() == 0
}

// AddPDASyscalls adds PDA syscalls to an existing registry.
func AddPDASyscalls(r *Registry, ctx InvokeContext) {
	r.registerPDA(ctx)
}
