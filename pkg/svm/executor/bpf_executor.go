// Package executor implements the BPF program executor for the SVM.
//
// This package provides the runtime environment for executing sBPF programs:
// - Program loading from account data
// - VM execution with proper memory layout
// - Syscall dispatch
// - Account data serialization/deserialization
// - CPI handling
package executor

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
	"github.com/fortiblox/X1-Stratus/pkg/svm"
	"github.com/fortiblox/X1-Stratus/pkg/svm/loader"
	"github.com/fortiblox/X1-Stratus/pkg/svm/sbpf"
	"github.com/fortiblox/X1-Stratus/pkg/svm/syscall"
)

// BPF executor errors.
var (
	ErrProgramNotFound       = errors.New("program not found")
	ErrProgramNotExecutable  = errors.New("program not executable")
	ErrProgramLoadFailed     = errors.New("program load failed")
	ErrExecutionFailed       = errors.New("execution failed")
	ErrInvalidAccountData    = errors.New("invalid account data")
	ErrAccountAccessDenied   = errors.New("account access denied")
	ErrInstructionTooLarge   = errors.New("instruction data too large")
)

// Maximum sizes.
const (
	MaxInstructionDataSize = 10 * 1024        // 10 KB max instruction data
	MaxAccountDataSize     = 10 * 1024 * 1024 // 10 MB max account data
)

// BPFExecutor executes sBPF programs.
type BPFExecutor struct {
	// loader parses ELF files.
	loader *loader.Loader

	// accountsDB provides account access.
	accountsDB accounts.DB

	// programCache caches loaded programs.
	programCache map[types.Pubkey]*loader.Executable
}

// NewBPFExecutor creates a new BPF executor.
func NewBPFExecutor(db accounts.DB) *BPFExecutor {
	return &BPFExecutor{
		loader:       loader.NewLoader(),
		accountsDB:   db,
		programCache: make(map[types.Pubkey]*loader.Executable),
	}
}

// ExecuteInstruction executes a single instruction.
func (e *BPFExecutor) ExecuteInstruction(
	programID types.Pubkey,
	accounts []*AccountInfo,
	data []byte,
	computeLimit uint64,
) (*ExecutionResult, error) {
	// Validate instruction data size
	if len(data) > MaxInstructionDataSize {
		return nil, ErrInstructionTooLarge
	}

	// Load program
	program, err := e.loadProgram(programID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProgramLoadFailed, err)
	}

	// Create execution context
	ctx := newExecutionContext(e.accountsDB, programID, accounts)

	// Create syscall registry
	registry := syscall.NewRegistry(ctx)
	syscall.AddPDASyscalls(registry, ctx)
	syscall.AddCPISyscalls(registry, ctx)

	// Serialize input data
	inputData, err := serializeInput(programID, accounts, data)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize input: %w", err)
	}

	// Create VM
	vm := sbpf.NewInterpreter(
		program.ToProgram(),
		inputData,
		sbpf.InterpreterOpts{
			HeapSize: uint64(svm.HeapSizeDefault),
			MaxCU:    computeLimit,
			Syscalls: registry.Lookup(),
			Context:  ctx,
		},
	)

	// Execute
	r0, execErr := vm.Run()

	// Get result
	result := &ExecutionResult{
		Success:          execErr == nil && r0 == 0,
		ReturnValue:      r0,
		ComputeUnitsUsed: computeLimit - vm.ComputeMeter().Remaining(),
		Logs:             ctx.logs,
		ReturnData:       ctx.returnData,
	}

	if execErr != nil {
		result.Error = execErr.Error()
	} else if r0 != 0 {
		result.Error = fmt.Sprintf("program returned error code %d", r0)
	}

	// Deserialize output if successful
	if result.Success {
		if err := deserializeOutput(inputData, accounts); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("failed to deserialize output: %v", err)
		} else {
			result.ModifiedAccounts = findModifiedAccounts(accounts)
		}
	}

	return result, nil
}

// loadProgram loads a program from the accounts database.
func (e *BPFExecutor) loadProgram(programID types.Pubkey) (*loader.Executable, error) {
	// Check cache
	if exec, ok := e.programCache[programID]; ok {
		return exec, nil
	}

	// Load program account
	programAccount, err := e.accountsDB.GetAccount(programID)
	if err != nil {
		return nil, ErrProgramNotFound
	}

	// Verify it's executable
	if !programAccount.Executable {
		return nil, ErrProgramNotExecutable
	}

	// Load ELF
	exec, err := e.loader.Load(programAccount.Data)
	if err != nil {
		return nil, fmt.Errorf("ELF load failed: %w", err)
	}

	// Cache
	e.programCache[programID] = exec

	return exec, nil
}

// ClearCache clears the program cache.
func (e *BPFExecutor) ClearCache() {
	e.programCache = make(map[types.Pubkey]*loader.Executable)
}

// AccountInfo holds account information for execution.
type AccountInfo struct {
	// Key is the account public key.
	Key types.Pubkey

	// Owner is the program that owns this account.
	Owner types.Pubkey

	// Lamports is the account balance.
	Lamports uint64

	// Data is the account data.
	Data []byte

	// Executable indicates if this is a program account.
	Executable bool

	// RentEpoch is the rent epoch.
	RentEpoch uint64

	// IsSigner indicates if this account signed the transaction.
	IsSigner bool

	// IsWritable indicates if this account can be modified.
	IsWritable bool

	// originalData stores the original data for change detection.
	originalData []byte

	// originalLamports stores the original lamports.
	originalLamports uint64
}

// MarkOriginal marks the current state as original for change detection.
func (a *AccountInfo) MarkOriginal() {
	a.originalData = make([]byte, len(a.Data))
	copy(a.originalData, a.Data)
	a.originalLamports = a.Lamports
}

// IsModified returns true if the account has been modified.
func (a *AccountInfo) IsModified() bool {
	if a.Lamports != a.originalLamports {
		return true
	}
	if len(a.Data) != len(a.originalData) {
		return true
	}
	for i := range a.Data {
		if a.Data[i] != a.originalData[i] {
			return true
		}
	}
	return false
}

// ExecutionResult contains the result of BPF execution.
type ExecutionResult struct {
	// Success indicates if execution succeeded.
	Success bool

	// ReturnValue is the value in r0 at exit.
	ReturnValue uint64

	// Error contains the error message if execution failed.
	Error string

	// ComputeUnitsUsed is the compute units consumed.
	ComputeUnitsUsed uint64

	// Logs contains program log messages.
	Logs []string

	// ReturnData is the program return data.
	ReturnData []byte

	// ModifiedAccounts contains the pubkeys of modified accounts.
	ModifiedAccounts []types.Pubkey
}

// executionContext implements syscall.InvokeContext.
type executionContext struct {
	db           accounts.DB
	programID    [32]byte
	callerID     [32]byte
	hasCallerID  bool
	logs         []string
	returnData   []byte
	returnDataID [32]byte
	stackHeight  uint64
	accounts     []*AccountInfo
	cuConsumed   uint64
	cuLimit      uint64
}

func newExecutionContext(db accounts.DB, programID types.Pubkey, accounts []*AccountInfo) *executionContext {
	var pid [32]byte
	copy(pid[:], programID[:])

	return &executionContext{
		db:          db,
		programID:   pid,
		logs:        make([]string, 0),
		stackHeight: 1,
		accounts:    accounts,
		cuLimit:     svm.CUMax,
	}
}

// Implement syscall.InvokeContext

func (c *executionContext) Log(msg string) {
	c.logs = append(c.logs, msg)
}

func (c *executionContext) LogData(data [][]byte) {
	// Format as hex-encoded log
	for _, d := range data {
		c.logs = append(c.logs, fmt.Sprintf("Program data: %x", d))
	}
}

func (c *executionContext) SetReturnData(programID [32]byte, data []byte) error {
	if len(data) > syscall.MaxReturnData {
		return errors.New("return data too large")
	}
	c.returnData = make([]byte, len(data))
	copy(c.returnData, data)
	c.returnDataID = programID
	return nil
}

func (c *executionContext) GetReturnData() (programID [32]byte, data []byte) {
	return c.returnDataID, c.returnData
}

func (c *executionContext) ConsumeCU(cost uint64) error {
	if c.cuConsumed+cost > c.cuLimit {
		return svm.ErrComputeExceeded
	}
	c.cuConsumed += cost
	return nil
}

func (c *executionContext) RemainingCU() uint64 {
	if c.cuConsumed >= c.cuLimit {
		return 0
	}
	return c.cuLimit - c.cuConsumed
}

func (c *executionContext) GetProgramID() [32]byte {
	return c.programID
}

func (c *executionContext) GetCallerProgramID() ([32]byte, bool) {
	return c.callerID, c.hasCallerID
}

// Implement syscall.CPIContext

func (c *executionContext) GetStackHeight() uint64 {
	return c.stackHeight
}

func (c *executionContext) InvokeProgram(programID [32]byte, accounts []syscall.CPIAccountMeta, data []byte, seeds [][]byte) error {
	// CPI implementation would go here
	// For now, just log and return success
	c.Log(fmt.Sprintf("CPI to program %x with %d accounts", programID[:8], len(accounts)))
	return nil
}

func (c *executionContext) GetAccountMeta(index int) (*syscall.CPIAccountMeta, error) {
	if index >= len(c.accounts) {
		return nil, errors.New("account index out of bounds")
	}
	acc := c.accounts[index]
	return &syscall.CPIAccountMeta{
		Pubkey:     [32]byte(acc.Key),
		IsSigner:   acc.IsSigner,
		IsWritable: acc.IsWritable,
	}, nil
}

func (c *executionContext) TranslateAccount(index int) (*syscall.CPIAccountInfo, error) {
	if index >= len(c.accounts) {
		return nil, errors.New("account index out of bounds")
	}
	acc := c.accounts[index]
	return &syscall.CPIAccountInfo{
		Key:        [32]byte(acc.Key),
		Owner:      [32]byte(acc.Owner),
		Lamports:   acc.Lamports,
		Data:       acc.Data,
		Executable: acc.Executable,
		RentEpoch:  acc.RentEpoch,
		IsSigner:   acc.IsSigner,
		IsWritable: acc.IsWritable,
	}, nil
}

// serializeInput serializes the input data for the VM.
// This follows Solana's input format:
//
// Layout:
// - num_accounts (8 bytes, u64)
// - For each account:
//   - duplicate marker (1 byte, 0xff if duplicate, else account index)
//   - is_signer (1 byte)
//   - is_writable (1 byte)
//   - executable (1 byte)
//   - padding (4 bytes)
//   - key (32 bytes)
//   - owner (32 bytes)
//   - lamports (8 bytes, u64)
//   - data_len (8 bytes, u64)
//   - data (data_len bytes)
//   - padding to 8-byte alignment
//   - rent_epoch (8 bytes, u64)
// - instruction_data_len (8 bytes, u64)
// - instruction_data
// - program_id (32 bytes)
func serializeInput(programID types.Pubkey, accounts []*AccountInfo, data []byte) ([]byte, error) {
	// Calculate total size
	size := 8 // num_accounts

	for _, acc := range accounts {
		// Mark original state for change detection
		acc.MarkOriginal()

		accountSize := 1 + 1 + 1 + 1 + 4 + 32 + 32 + 8 + 8 + len(acc.Data)
		// Pad to 8-byte alignment
		padding := (8 - (len(acc.Data) % 8)) % 8
		accountSize += padding
		accountSize += 8 // rent_epoch
		size += accountSize
	}

	size += 8 + len(data) + 32 // instruction data + program_id

	buf := make([]byte, size)
	offset := 0

	// Number of accounts
	binary.LittleEndian.PutUint64(buf[offset:], uint64(len(accounts)))
	offset += 8

	// Serialize each account
	for i, acc := range accounts {
		// Duplicate marker
		buf[offset] = byte(i) // Non-duplicate
		offset++

		// is_signer
		if acc.IsSigner {
			buf[offset] = 1
		}
		offset++

		// is_writable
		if acc.IsWritable {
			buf[offset] = 1
		}
		offset++

		// executable
		if acc.Executable {
			buf[offset] = 1
		}
		offset++

		// Padding
		offset += 4

		// Key
		copy(buf[offset:], acc.Key[:])
		offset += 32

		// Owner
		copy(buf[offset:], acc.Owner[:])
		offset += 32

		// Lamports
		binary.LittleEndian.PutUint64(buf[offset:], acc.Lamports)
		offset += 8

		// Data length
		binary.LittleEndian.PutUint64(buf[offset:], uint64(len(acc.Data)))
		offset += 8

		// Data
		copy(buf[offset:], acc.Data)
		offset += len(acc.Data)

		// Padding to 8-byte alignment
		padding := (8 - (len(acc.Data) % 8)) % 8
		offset += padding

		// Rent epoch
		binary.LittleEndian.PutUint64(buf[offset:], acc.RentEpoch)
		offset += 8
	}

	// Instruction data length
	binary.LittleEndian.PutUint64(buf[offset:], uint64(len(data)))
	offset += 8

	// Instruction data
	copy(buf[offset:], data)
	offset += len(data)

	// Program ID
	copy(buf[offset:], programID[:])

	return buf, nil
}

// deserializeOutput deserializes the output from the VM and updates accounts.
func deserializeOutput(inputData []byte, accounts []*AccountInfo) error {
	offset := 8 // Skip num_accounts

	for _, acc := range accounts {
		if !acc.IsWritable {
			// Skip non-writable accounts
			offset += 1 + 1 + 1 + 1 + 4 + 32 + 32 + 8 + 8 + len(acc.Data)
			padding := (8 - (len(acc.Data) % 8)) % 8
			offset += padding + 8
			continue
		}

		// Skip to lamports field
		offset += 1 + 1 + 1 + 1 + 4 + 32 + 32

		// Read updated lamports
		if offset+8 > len(inputData) {
			return ErrInvalidAccountData
		}
		acc.Lamports = binary.LittleEndian.Uint64(inputData[offset:])
		offset += 8

		// Read data length
		if offset+8 > len(inputData) {
			return ErrInvalidAccountData
		}
		dataLen := binary.LittleEndian.Uint64(inputData[offset:])
		offset += 8

		// Read data
		if offset+int(dataLen) > len(inputData) {
			return ErrInvalidAccountData
		}

		// Update account data if size changed
		if uint64(len(acc.Data)) != dataLen {
			acc.Data = make([]byte, dataLen)
		}
		copy(acc.Data, inputData[offset:offset+int(dataLen)])
		offset += int(dataLen)

		// Skip padding
		padding := (8 - (int(dataLen) % 8)) % 8
		offset += padding

		// Read rent epoch
		if offset+8 > len(inputData) {
			return ErrInvalidAccountData
		}
		acc.RentEpoch = binary.LittleEndian.Uint64(inputData[offset:])
		offset += 8
	}

	return nil
}

// findModifiedAccounts returns the pubkeys of modified accounts.
func findModifiedAccounts(accounts []*AccountInfo) []types.Pubkey {
	modified := make([]types.Pubkey, 0)
	for _, acc := range accounts {
		if acc.IsWritable && acc.IsModified() {
			modified = append(modified, acc.Key)
		}
	}
	return modified
}
