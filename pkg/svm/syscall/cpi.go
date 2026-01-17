// Package syscall implements Cross-Program Invocation (CPI) syscalls.
//
// CPI allows Solana programs to invoke other programs. This is implemented
// through the sol_invoke_signed syscall, which:
// - Takes an instruction struct with program ID, accounts, and data
// - Optionally includes signer seeds for PDA signing
// - Executes the target program with the provided accounts
// - Returns success/failure to the caller
package syscall

import (
	"errors"

	"github.com/fortiblox/X1-Stratus/pkg/svm/sbpf"
)

// CPI constants.
const (
	MaxCPIDepth            = 4                // Maximum CPI nesting depth
	MaxCPIInstructionSize  = 10 * 1024        // Maximum CPI instruction data size
	MaxCPIAccountInfos     = 128              // Maximum account infos per CPI
	MaxCPISignerSeeds      = 16               // Maximum signer seeds
	MaxCPISignerSeedLength = 32               // Maximum length per seed

	// Compute costs for CPI
	CUCPIBaseInvoke       = uint64(1000)      // Base cost for invoke
	CUCPIPerAccount       = uint64(10)        // Cost per account in CPI
	CUCPIPerDataByte      = uint64(1)         // Cost per data byte
)

// CPI errors.
var (
	ErrCPIDepthExceeded        = errors.New("CPI depth exceeded")
	ErrCPIInvalidInstruction   = errors.New("invalid CPI instruction")
	ErrCPIInvalidAccountInfo   = errors.New("invalid CPI account info")
	ErrCPITooManyAccounts      = errors.New("too many accounts in CPI")
	ErrCPITooManySignerSeeds   = errors.New("too many signer seeds")
	ErrCPISeedTooLong          = errors.New("signer seed too long")
	ErrCPIDataTooLarge         = errors.New("CPI instruction data too large")
	ErrCPIPrivilegeEscalation  = errors.New("CPI privilege escalation")
	ErrCPIProgramNotExecutable = errors.New("CPI program not executable")
	ErrCPIAccountMismatch      = errors.New("CPI account mismatch")
)

// CPIContext provides context for CPI execution.
type CPIContext interface {
	InvokeContext

	// GetStackHeight returns the current CPI stack height.
	GetStackHeight() uint64

	// InvokeProgram executes a program via CPI.
	// programID: the target program to invoke
	// accounts: account infos to pass to the program
	// data: instruction data
	// seeds: signer seeds for PDA signing (can be nil)
	InvokeProgram(programID [32]byte, accounts []CPIAccountMeta, data []byte, seeds [][]byte) error

	// GetAccountMeta returns metadata about an account.
	GetAccountMeta(index int) (*CPIAccountMeta, error)

	// TranslateAccount translates an account from the caller's view.
	TranslateAccount(index int) (*CPIAccountInfo, error)
}

// CPIAccountMeta describes an account in a CPI instruction.
type CPIAccountMeta struct {
	Pubkey     [32]byte
	IsSigner   bool
	IsWritable bool
}

// CPIAccountInfo contains full account info for CPI.
type CPIAccountInfo struct {
	Key        [32]byte
	Owner      [32]byte
	Lamports   uint64
	Data       []byte
	Executable bool
	RentEpoch  uint64
	IsSigner   bool
	IsWritable bool
}

// CPIInstruction represents a CPI instruction to be invoked.
type CPIInstruction struct {
	ProgramID [32]byte
	Accounts  []CPIAccountMeta
	Data      []byte
}

// registerCPI registers CPI-related syscalls.
func (r *Registry) registerCPI(ctx InvokeContext) {
	cpiCtx, hasCPI := ctx.(CPIContext)

	// sol_invoke_signed - invoke a program with signed PDAs
	r.register("sol_invoke_signed_c", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		if !hasCPI {
			return 1, nil // Return error if CPI not supported
		}

		// r1 = instruction pointer
		// r2 = account_infos pointer
		// r3 = account_infos_len
		// r4 = signer_seeds pointer (can be null)
		// r5 = signer_seeds_len

		// Check stack height
		if cpiCtx.GetStackHeight() >= MaxCPIDepth {
			return uint64(ErrCPIDepthExceeded.Error()[0]), ErrCPIDepthExceeded
		}

		// Read instruction struct
		// Solana C struct layout:
		// struct SolInstruction {
		//     SolPubkey* program_id;       // 8 bytes (pointer)
		//     SolAccountMeta* accounts;    // 8 bytes (pointer)
		//     uint64_t account_len;        // 8 bytes
		//     uint8_t* data;               // 8 bytes (pointer)
		//     uint64_t data_len;           // 8 bytes
		// };

		programIDPtr, err := vm.Read64(r1)
		if err != nil {
			return 1, err
		}
		accountsPtr, err := vm.Read64(r1 + 8)
		if err != nil {
			return 1, err
		}
		accountsLen, err := vm.Read64(r1 + 16)
		if err != nil {
			return 1, err
		}
		dataPtr, err := vm.Read64(r1 + 24)
		if err != nil {
			return 1, err
		}
		dataLen, err := vm.Read64(r1 + 32)
		if err != nil {
			return 1, err
		}

		// Validate sizes
		if accountsLen > MaxCPIAccountInfos {
			return 1, ErrCPITooManyAccounts
		}
		if dataLen > MaxCPIInstructionSize {
			return 1, ErrCPIDataTooLarge
		}

		// Compute cost
		cost := CUCPIBaseInvoke + CUCPIPerAccount*accountsLen + CUCPIPerDataByte*dataLen
		if err := ctx.ConsumeCU(cost); err != nil {
			return 1, err
		}

		// Read program ID
		var programID [32]byte
		programIDBytes := make([]byte, 32)
		if err := vm.Read(programIDPtr, programIDBytes); err != nil {
			return 1, err
		}
		copy(programID[:], programIDBytes)

		// Read accounts
		accounts := make([]CPIAccountMeta, accountsLen)
		for i := uint64(0); i < accountsLen; i++ {
			// SolAccountMeta struct:
			// struct SolAccountMeta {
			//     SolPubkey* pubkey;    // 8 bytes
			//     bool is_writable;     // 1 byte
			//     bool is_signer;       // 1 byte
			// };
			metaOffset := accountsPtr + i*10 // 8 + 1 + 1 = 10 bytes per meta

			pubkeyPtr, err := vm.Read64(metaOffset)
			if err != nil {
				return 1, err
			}

			pubkeyBytes := make([]byte, 32)
			if err := vm.Read(pubkeyPtr, pubkeyBytes); err != nil {
				return 1, err
			}
			copy(accounts[i].Pubkey[:], pubkeyBytes)

			isWritable, err := vm.Read8(metaOffset + 8)
			if err != nil {
				return 1, err
			}
			accounts[i].IsWritable = isWritable != 0

			isSigner, err := vm.Read8(metaOffset + 9)
			if err != nil {
				return 1, err
			}
			accounts[i].IsSigner = isSigner != 0
		}

		// Read instruction data
		data := make([]byte, dataLen)
		if dataLen > 0 {
			if err := vm.Read(dataPtr, data); err != nil {
				return 1, err
			}
		}

		// Read signer seeds if present
		var seeds [][]byte
		if r4 != 0 && r5 > 0 {
			if r5 > MaxCPISignerSeeds {
				return 1, ErrCPITooManySignerSeeds
			}

			seeds = make([][]byte, r5)
			for i := uint64(0); i < r5; i++ {
				// Each seed is (ptr, len) pair
				seedPtr, err := vm.Read64(r4 + i*16)
				if err != nil {
					return 1, err
				}
				seedLen, err := vm.Read64(r4 + i*16 + 8)
				if err != nil {
					return 1, err
				}

				if seedLen > MaxCPISignerSeedLength {
					return 1, ErrCPISeedTooLong
				}

				seed := make([]byte, seedLen)
				if seedLen > 0 {
					if err := vm.Read(seedPtr, seed); err != nil {
						return 1, err
					}
				}
				seeds[i] = seed
			}
		}

		// Execute CPI
		if err := cpiCtx.InvokeProgram(programID, accounts, data, seeds); err != nil {
			// CPI failures return error code in r0
			return 1, nil
		}

		return 0, nil
	})

	// sol_invoke_signed_rust - Rust ABI version
	r.register("sol_invoke_signed_rust", func(vm sbpf.VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
		// Rust ABI has slightly different layout but similar semantics
		// For now, delegate to C version after translating
		return r.syscalls[murmur3Hash("sol_invoke_signed_c")].Invoke(vm, r1, r2, r3, r4, r5)
	})
}

// AddCPISyscalls adds CPI syscalls to a registry.
func AddCPISyscalls(r *Registry, ctx InvokeContext) {
	r.registerCPI(ctx)
}
