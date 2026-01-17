// Package replayer implements the transaction executor.
package replayer

import (
	"errors"
	"fmt"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
	"github.com/fortiblox/X1-Stratus/pkg/blockstore"
	"github.com/fortiblox/X1-Stratus/pkg/svm"
	sysProgram "github.com/fortiblox/X1-Stratus/pkg/svm/programs/system"
)

// TransactionExecutor executes individual transactions.
type TransactionExecutor struct {
	accounts accounts.DB

	// Native program processors
	systemProcessor *sysProgram.Processor
}

// NewTransactionExecutor creates a new transaction executor.
func NewTransactionExecutor(accts accounts.DB) *TransactionExecutor {
	return &TransactionExecutor{
		accounts:        accts,
		systemProcessor: sysProgram.NewProcessor(),
	}
}

// ExecutionResult contains the result of transaction execution.
type ExecutionResult struct {
	Success          bool
	Error            string
	ComputeUnitsUsed uint64
	Logs             []string
	ModifiedAccounts []types.Pubkey
}

// Execute executes a transaction and updates account state.
func (e *TransactionExecutor) Execute(tx *blockstore.Transaction) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Logs:             make([]string, 0),
		ModifiedAccounts: make([]types.Pubkey, 0),
	}

	// Create compute meter
	computeLimit := svm.CUDefault
	if tx.Meta != nil && tx.Meta.ComputeUnitsConsumed > 0 {
		// Use the limit from metadata if available
		computeLimit = tx.Meta.ComputeUnitsConsumed + 10000 // Add buffer
	}
	computeMeter := svm.NewComputeMeter(computeLimit)

	// Load accounts
	accountInfos, err := e.loadAccounts(tx)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to load accounts: %v", err)
		// Even on load failure, fee payer may be charged
		if len(tx.Message.AccountKeys) > 0 {
			result.ModifiedAccounts = append(result.ModifiedAccounts, tx.Message.AccountKeys[0])
		}
		return result, nil
	}

	// Execute each instruction
	var execErr error
	for i, instruction := range tx.Message.Instructions {
		err := e.executeInstruction(tx, &instruction, accountInfos, computeMeter, result)
		if err != nil {
			execErr = fmt.Errorf("instruction %d failed: %v", i, err)
			break
		}
	}

	// Handle execution failure - only fee payer is modified (for fee deduction)
	if execErr != nil {
		result.Success = false
		result.Error = execErr.Error()
		result.ComputeUnitsUsed = computeLimit - computeMeter.Remaining()
		// For failed transactions, only the fee payer (first account) is modified
		if len(accountInfos) > 0 {
			result.ModifiedAccounts = append(result.ModifiedAccounts, accountInfos[0].key)
		}
		return result, nil
	}

	// Save modified accounts (only for successful transactions)
	for _, info := range accountInfos {
		if info.modified {
			account := &accounts.Account{
				Lamports:   info.account.Lamports,
				Data:       info.account.Data,
				Owner:      info.account.Owner,
				Executable: info.account.Executable,
				RentEpoch:  info.account.RentEpoch,
			}
			if err := e.accounts.SetAccount(info.key, account); err != nil {
				return nil, fmt.Errorf("failed to save account %s: %w", info.key.String(), err)
			}
			result.ModifiedAccounts = append(result.ModifiedAccounts, info.key)
		}
	}

	result.Success = true
	result.ComputeUnitsUsed = computeLimit - computeMeter.Remaining()
	return result, nil
}

// accountInfo holds account data during execution.
type accountInfo struct {
	key      types.Pubkey
	account  *sysProgram.AccountInfo
	modified bool
}

// loadAccounts loads all accounts referenced by a transaction.
func (e *TransactionExecutor) loadAccounts(tx *blockstore.Transaction) ([]*accountInfo, error) {
	infos := make([]*accountInfo, len(tx.Message.AccountKeys))

	header := tx.Message.Header
	numSigners := int(header.NumRequiredSignatures)
	numReadonlySigned := int(header.NumReadonlySignedAccounts)
	numReadonlyUnsigned := int(header.NumReadonlyUnsignedAccounts)
	totalAccounts := len(tx.Message.AccountKeys)

	for i, key := range tx.Message.AccountKeys {
		// Determine account properties from position and header
		isSigner := i < numSigners
		isWritable := isAccountWritable(i, numSigners, numReadonlySigned, numReadonlyUnsigned, totalAccounts)

		// Load from database
		var accData *accounts.Account
		dbAccount, err := e.accounts.GetAccount(key)
		if err != nil {
			if errors.Is(err, accounts.ErrAccountNotFound) {
				// Create empty account
				accData = &accounts.Account{
					Owner: types.Pubkey{}, // System program
				}
			} else {
				return nil, fmt.Errorf("failed to load account %s: %w", key.String(), err)
			}
		} else {
			accData = dbAccount
		}

		// Convert to execution account info
		infos[i] = &accountInfo{
			key: key,
			account: &sysProgram.AccountInfo{
				Key:        toArray32(key),
				Owner:      toArray32(accData.Owner),
				Lamports:   accData.Lamports,
				Data:       accData.Data,
				Executable: accData.Executable,
				RentEpoch:  accData.RentEpoch,
				IsSigner:   isSigner,
				IsWritable: isWritable,
			},
			modified: false,
		}
	}

	return infos, nil
}

// isAccountWritable determines if an account is writable based on its position.
func isAccountWritable(index, numSigners, numReadonlySigned, numReadonlyUnsigned, total int) bool {
	if index < numSigners {
		// Signer accounts: first (numSigners - numReadonlySigned) are writable
		return index < (numSigners - numReadonlySigned)
	}
	// Non-signer accounts: first (total - numSigners - numReadonlyUnsigned) are writable
	nonSignerIndex := index - numSigners
	numWritableUnsigned := total - numSigners - numReadonlyUnsigned
	return nonSignerIndex < numWritableUnsigned
}

// executeInstruction executes a single instruction.
func (e *TransactionExecutor) executeInstruction(
	tx *blockstore.Transaction,
	instruction *blockstore.Instruction,
	accountInfos []*accountInfo,
	computeMeter *svm.ComputeMeter,
	result *ExecutionResult,
) error {
	// Get program ID
	if int(instruction.ProgramIDIndex) >= len(tx.Message.AccountKeys) {
		return errors.New("invalid program ID index")
	}
	programID := tx.Message.AccountKeys[instruction.ProgramIDIndex]

	// Create instruction context
	ctx := &instructionContext{
		accounts:     make([]*sysProgram.AccountInfo, len(instruction.AccountIndexes)),
		computeMeter: computeMeter,
		logs:         &result.Logs,
	}

	// Map account indexes to account infos
	for i, idx := range instruction.AccountIndexes {
		if int(idx) >= len(accountInfos) {
			return errors.New("invalid account index")
		}
		ctx.accounts[i] = accountInfos[idx].account
	}

	// Execute based on program type
	var err error
	switch {
	case isSystemProgram(programID):
		err = e.executeSystemProgram(ctx, instruction.Data)
	case isComputeBudgetProgram(programID):
		err = e.executeComputeBudget(ctx, instruction.Data, computeMeter)
	default:
		// For BPF programs, we'd need to load and execute the bytecode
		// For now, we trust the transaction metadata for CU consumption
		result.Logs = append(result.Logs, fmt.Sprintf("Skipping BPF program: %s", programID.String()))
		return nil
	}

	if err != nil {
		return err
	}

	// Mark modified accounts
	for i, idx := range instruction.AccountIndexes {
		if ctx.accounts[i].IsWritable {
			accountInfos[idx].modified = true
		}
	}

	return nil
}

// instructionContext provides context for instruction execution.
type instructionContext struct {
	accounts     []*sysProgram.AccountInfo
	computeMeter *svm.ComputeMeter
	logs         *[]string
}

// GetAccount returns the account at the given index.
func (c *instructionContext) GetAccount(index int) (*sysProgram.AccountInfo, error) {
	if index >= len(c.accounts) {
		return nil, errors.New("account index out of bounds")
	}
	return c.accounts[index], nil
}

// GetRentMinimum returns the rent-exempt minimum.
func (c *instructionContext) GetRentMinimum(dataLen uint64) uint64 {
	// Simplified rent calculation: 2 years of rent at 3.48 lamports/byte-epoch
	// Rent exemption = (128 + data_len) * 3.48 * 2 * 365.25 * 2 = (128 + data_len) * 19.055
	return (128 + dataLen) * 6960 / 1000 * 2 // Approximately 2 years rent
}

// GetRecentBlockhash returns a placeholder blockhash.
func (c *instructionContext) GetRecentBlockhash() [32]byte {
	// In production, this would return the actual recent blockhash
	return [32]byte{}
}

// GetFeePerSignature returns the fee per signature.
func (c *instructionContext) GetFeePerSignature() uint64 {
	return 5000 // Standard fee
}

// Log records a log message.
func (c *instructionContext) Log(msg string) {
	*c.logs = append(*c.logs, msg)
}

// executeSystemProgram executes a System Program instruction.
func (e *TransactionExecutor) executeSystemProgram(ctx *instructionContext, data []byte) error {
	return e.systemProcessor.Process(ctx, data)
}

// executeComputeBudget processes compute budget instructions.
func (e *TransactionExecutor) executeComputeBudget(ctx *instructionContext, data []byte, meter *svm.ComputeMeter) error {
	// Compute budget instructions are processed at transaction loading time
	// They don't modify account state, just configure compute limits
	ctx.Log("ComputeBudget: processed")
	return nil
}

// Native program checks

var (
	systemProgramID = types.Pubkey{} // All zeros
	computeBudgetProgramID = mustParsePubkey("ComputeBudget111111111111111111111111111111")
)

func isSystemProgram(id types.Pubkey) bool {
	return id == systemProgramID
}

func isComputeBudgetProgram(id types.Pubkey) bool {
	return id == computeBudgetProgramID
}

func mustParsePubkey(s string) types.Pubkey {
	pubkey, err := types.PubkeyFromBase58(s)
	if err != nil {
		// Use zeros if parsing fails (during init)
		return types.Pubkey{}
	}
	return pubkey
}

func toArray32(p types.Pubkey) [32]byte {
	var arr [32]byte
	copy(arr[:], p[:])
	return arr
}
