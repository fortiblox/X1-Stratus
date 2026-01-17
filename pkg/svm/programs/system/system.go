// Package system implements the Solana System Program.
//
// The System Program is responsible for:
// - Creating new accounts
// - Transferring lamports
// - Assigning account ownership
// - Allocating account space
// - Creating accounts with seeds (PDAs)
package system

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

// System Program address (all zeros means native system program).
var ProgramID = [32]byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// Instruction discriminants.
const (
	InstructionCreateAccount = iota
	InstructionAssign
	InstructionTransfer
	InstructionCreateAccountWithSeed
	InstructionAdvanceNonceAccount
	InstructionWithdrawNonceAccount
	InstructionInitializeNonceAccount
	InstructionAuthorizeNonceAccount
	InstructionAllocate
	InstructionAllocateWithSeed
	InstructionAssignWithSeed
	InstructionTransferWithSeed
	InstructionUpgradeNonceAccount
)

// Error types.
var (
	ErrInvalidInstructionData   = errors.New("invalid instruction data")
	ErrInsufficientFunds        = errors.New("insufficient funds")
	ErrAccountAlreadyInUse      = errors.New("account already in use")
	ErrNotEnoughAccountKeys     = errors.New("not enough account keys")
	ErrInvalidAccountOwner      = errors.New("invalid account owner")
	ErrAccountNotRentExempt     = errors.New("account not rent exempt")
	ErrMissingRequiredSignature = errors.New("missing required signature")
	ErrAccountDataTooSmall      = errors.New("account data too small")
	ErrAccountDataTooLarge      = errors.New("account data too large")
	ErrInvalidSeed              = errors.New("invalid seed")
)

// Maximum account data size.
const MaxAccountDataSize = 10 * 1024 * 1024 // 10 MB

// AccountMeta describes an account in an instruction.
type AccountMeta struct {
	Pubkey     [32]byte
	IsSigner   bool
	IsWritable bool
}

// AccountInfo holds account data during execution.
type AccountInfo struct {
	Key        [32]byte
	Owner      [32]byte
	Lamports   uint64
	Data       []byte
	Executable bool
	RentEpoch  uint64
	IsSigner   bool
	IsWritable bool
}

// InvokeContext provides context for program execution.
type InvokeContext interface {
	// GetAccount returns the account at the given index.
	GetAccount(index int) (*AccountInfo, error)

	// GetRentMinimum returns the rent-exempt minimum for given data size.
	GetRentMinimum(dataLen uint64) uint64

	// Log records a log message.
	Log(msg string)
}

// Processor executes System Program instructions.
type Processor struct{}

// NewProcessor creates a new System Program processor.
func NewProcessor() *Processor {
	return &Processor{}
}

// Process executes a System Program instruction.
func (p *Processor) Process(ctx InvokeContext, data []byte) error {
	if len(data) < 4 {
		return ErrInvalidInstructionData
	}

	instruction := binary.LittleEndian.Uint32(data[:4])

	switch instruction {
	case InstructionCreateAccount:
		return p.processCreateAccount(ctx, data[4:])
	case InstructionAssign:
		return p.processAssign(ctx, data[4:])
	case InstructionTransfer:
		return p.processTransfer(ctx, data[4:])
	case InstructionAllocate:
		return p.processAllocate(ctx, data[4:])
	case InstructionCreateAccountWithSeed:
		return p.processCreateAccountWithSeed(ctx, data[4:])
	case InstructionAllocateWithSeed:
		return p.processAllocateWithSeed(ctx, data[4:])
	case InstructionAssignWithSeed:
		return p.processAssignWithSeed(ctx, data[4:])
	case InstructionTransferWithSeed:
		return p.processTransferWithSeed(ctx, data[4:])
	default:
		return ErrInvalidInstructionData
	}
}

// CreateAccountParams for CreateAccount instruction.
type CreateAccountParams struct {
	Lamports uint64
	Space    uint64
	Owner    [32]byte
}

// processCreateAccount creates a new account.
func (p *Processor) processCreateAccount(ctx InvokeContext, data []byte) error {
	// Parse parameters: lamports (8) + space (8) + owner (32)
	if len(data) < 48 {
		return ErrInvalidInstructionData
	}

	params := CreateAccountParams{
		Lamports: binary.LittleEndian.Uint64(data[0:8]),
		Space:    binary.LittleEndian.Uint64(data[8:16]),
	}
	copy(params.Owner[:], data[16:48])

	// Validate space
	if params.Space > MaxAccountDataSize {
		return ErrAccountDataTooLarge
	}

	// Get accounts: [0] = funding account, [1] = new account
	funder, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	newAccount, err := ctx.GetAccount(1)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// Verify signatures
	if !funder.IsSigner {
		return ErrMissingRequiredSignature
	}
	if !newAccount.IsSigner {
		return ErrMissingRequiredSignature
	}

	// Verify funding account has enough lamports
	if funder.Lamports < params.Lamports {
		return ErrInsufficientFunds
	}

	// Verify new account is empty (owned by system program and no data)
	if newAccount.Owner != ProgramID || len(newAccount.Data) > 0 || newAccount.Lamports > 0 {
		return ErrAccountAlreadyInUse
	}

	// Check rent exemption
	rentMinimum := ctx.GetRentMinimum(params.Space)
	if params.Lamports < rentMinimum {
		return ErrAccountNotRentExempt
	}

	// Execute transfer
	funder.Lamports -= params.Lamports
	newAccount.Lamports = params.Lamports

	// Allocate space
	newAccount.Data = make([]byte, params.Space)

	// Assign owner
	newAccount.Owner = params.Owner

	ctx.Log("CreateAccount: success")
	return nil
}

// processAssign changes the owner of an account.
func (p *Processor) processAssign(ctx InvokeContext, data []byte) error {
	// Parse parameters: owner (32)
	if len(data) < 32 {
		return ErrInvalidInstructionData
	}

	var newOwner [32]byte
	copy(newOwner[:], data[0:32])

	// Get account
	account, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// Must be signer
	if !account.IsSigner {
		return ErrMissingRequiredSignature
	}

	// Must be owned by system program
	if account.Owner != ProgramID {
		return ErrInvalidAccountOwner
	}

	// Assign new owner
	account.Owner = newOwner

	ctx.Log("Assign: success")
	return nil
}

// TransferParams for Transfer instruction.
type TransferParams struct {
	Lamports uint64
}

// processTransfer transfers lamports between accounts.
func (p *Processor) processTransfer(ctx InvokeContext, data []byte) error {
	// Parse parameters: lamports (8)
	if len(data) < 8 {
		return ErrInvalidInstructionData
	}

	lamports := binary.LittleEndian.Uint64(data[0:8])

	// Get accounts: [0] = from, [1] = to
	from, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	to, err := ctx.GetAccount(1)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// From must be signer
	if !from.IsSigner {
		return ErrMissingRequiredSignature
	}

	// Verify accounts are writable
	if !from.IsWritable {
		return errors.New("source account not writable")
	}
	if !to.IsWritable {
		return errors.New("destination account not writable")
	}

	// Check balance
	if from.Lamports < lamports {
		return ErrInsufficientFunds
	}

	// Check for overflow on destination
	if to.Lamports > ^uint64(0)-lamports {
		return errors.New("lamport overflow")
	}

	// Execute transfer
	from.Lamports -= lamports
	to.Lamports += lamports

	ctx.Log("Transfer: success")
	return nil
}

// processAllocate allocates space in an account.
func (p *Processor) processAllocate(ctx InvokeContext, data []byte) error {
	// Parse parameters: space (8)
	if len(data) < 8 {
		return ErrInvalidInstructionData
	}

	space := binary.LittleEndian.Uint64(data[0:8])

	// Validate space
	if space > MaxAccountDataSize {
		return ErrAccountDataTooLarge
	}

	// Get account
	account, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// Must be signer
	if !account.IsSigner {
		return ErrMissingRequiredSignature
	}

	// Must be owned by system program
	if account.Owner != ProgramID {
		return ErrInvalidAccountOwner
	}

	// Cannot shrink account
	if uint64(len(account.Data)) > space {
		return ErrAccountDataTooSmall
	}

	// Allocate space
	if uint64(len(account.Data)) < space {
		newData := make([]byte, space)
		copy(newData, account.Data)
		account.Data = newData
	}

	ctx.Log("Allocate: success")
	return nil
}

// processCreateAccountWithSeed creates an account with a seed-derived address.
func (p *Processor) processCreateAccountWithSeed(ctx InvokeContext, data []byte) error {
	// Parse parameters:
	// base (32) + seed_len (8) + seed (variable) + lamports (8) + space (8) + owner (32)
	if len(data) < 48 {
		return ErrInvalidInstructionData
	}

	var base [32]byte
	copy(base[:], data[0:32])

	seedLen := binary.LittleEndian.Uint64(data[32:40])
	if seedLen > 32 || len(data) < int(48+seedLen) {
		return ErrInvalidSeed
	}

	seed := data[40 : 40+seedLen]
	offset := 40 + seedLen

	if len(data) < int(offset+48) {
		return ErrInvalidInstructionData
	}

	lamports := binary.LittleEndian.Uint64(data[offset : offset+8])
	space := binary.LittleEndian.Uint64(data[offset+8 : offset+16])
	var owner [32]byte
	copy(owner[:], data[offset+16:offset+48])

	// Validate
	if space > MaxAccountDataSize {
		return ErrAccountDataTooLarge
	}

	// Get accounts: [0] = funding, [1] = created account, [2] = base (optional if same as funding)
	funder, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	newAccount, err := ctx.GetAccount(1)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// Verify signatures
	if !funder.IsSigner {
		return ErrMissingRequiredSignature
	}

	// Verify balance
	if funder.Lamports < lamports {
		return ErrInsufficientFunds
	}

	// Verify new account is unused
	if newAccount.Owner != ProgramID || len(newAccount.Data) > 0 || newAccount.Lamports > 0 {
		return ErrAccountAlreadyInUse
	}

	// Verify address derivation
	expectedAddr := createWithSeedAddress(base, string(seed), owner)
	if expectedAddr != newAccount.Key {
		return errors.New("create address with seed mismatch")
	}

	// Check rent exemption
	rentMinimum := ctx.GetRentMinimum(space)
	if lamports < rentMinimum {
		return ErrAccountNotRentExempt
	}

	// Execute
	funder.Lamports -= lamports
	newAccount.Lamports = lamports
	newAccount.Data = make([]byte, space)
	newAccount.Owner = owner

	ctx.Log("CreateAccountWithSeed: success")
	return nil
}

// processAllocateWithSeed allocates space in a seed-derived account.
func (p *Processor) processAllocateWithSeed(ctx InvokeContext, data []byte) error {
	// Parse: base (32) + seed_len (8) + seed + space (8) + owner (32)
	if len(data) < 48 {
		return ErrInvalidInstructionData
	}

	var base [32]byte
	copy(base[:], data[0:32])

	seedLen := binary.LittleEndian.Uint64(data[32:40])
	if seedLen > 32 || len(data) < int(48+seedLen) {
		return ErrInvalidSeed
	}

	seed := data[40 : 40+seedLen]
	offset := 40 + seedLen

	if len(data) < int(offset+40) {
		return ErrInvalidInstructionData
	}

	space := binary.LittleEndian.Uint64(data[offset : offset+8])
	var owner [32]byte
	copy(owner[:], data[offset+8:offset+40])

	if space > MaxAccountDataSize {
		return ErrAccountDataTooLarge
	}

	// Get account
	account, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// Verify address
	expectedAddr := createWithSeedAddress(base, string(seed), owner)
	if expectedAddr != account.Key {
		return errors.New("allocate address with seed mismatch")
	}

	// Must be owned by system program
	if account.Owner != ProgramID {
		return ErrInvalidAccountOwner
	}

	// Allocate
	if uint64(len(account.Data)) < space {
		newData := make([]byte, space)
		copy(newData, account.Data)
		account.Data = newData
	}

	account.Owner = owner

	ctx.Log("AllocateWithSeed: success")
	return nil
}

// processAssignWithSeed assigns owner to a seed-derived account.
func (p *Processor) processAssignWithSeed(ctx InvokeContext, data []byte) error {
	// Parse: base (32) + seed_len (8) + seed + owner (32)
	if len(data) < 40 {
		return ErrInvalidInstructionData
	}

	var base [32]byte
	copy(base[:], data[0:32])

	seedLen := binary.LittleEndian.Uint64(data[32:40])
	if seedLen > 32 || len(data) < int(72+seedLen) {
		return ErrInvalidSeed
	}

	seed := data[40 : 40+seedLen]
	offset := 40 + seedLen

	var owner [32]byte
	copy(owner[:], data[offset:offset+32])

	// Get account
	account, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// Verify address
	expectedAddr := createWithSeedAddress(base, string(seed), owner)
	if expectedAddr != account.Key {
		return errors.New("assign address with seed mismatch")
	}

	// Must be owned by system program
	if account.Owner != ProgramID {
		return ErrInvalidAccountOwner
	}

	account.Owner = owner

	ctx.Log("AssignWithSeed: success")
	return nil
}

// processTransferWithSeed transfers from a seed-derived account.
func (p *Processor) processTransferWithSeed(ctx InvokeContext, data []byte) error {
	// Parse: lamports (8) + from_seed_len (8) + from_seed + from_owner (32)
	if len(data) < 16 {
		return ErrInvalidInstructionData
	}

	lamports := binary.LittleEndian.Uint64(data[0:8])
	seedLen := binary.LittleEndian.Uint64(data[8:16])

	if seedLen > 32 || len(data) < int(48+seedLen) {
		return ErrInvalidSeed
	}

	seed := data[16 : 16+seedLen]
	offset := 16 + seedLen

	var fromOwner [32]byte
	copy(fromOwner[:], data[offset:offset+32])

	// Get accounts: [0] = from, [1] = base, [2] = to
	from, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	base, err := ctx.GetAccount(1)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	to, err := ctx.GetAccount(2)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// Base must be signer
	if !base.IsSigner {
		return ErrMissingRequiredSignature
	}

	// Verify from address
	expectedAddr := createWithSeedAddress(base.Key, string(seed), fromOwner)
	if expectedAddr != from.Key {
		return errors.New("transfer address with seed mismatch")
	}

	// Check balance
	if from.Lamports < lamports {
		return ErrInsufficientFunds
	}

	// Execute
	from.Lamports -= lamports
	to.Lamports += lamports

	ctx.Log("TransferWithSeed: success")
	return nil
}

// createWithSeedAddress derives an address from base + seed + owner.
func createWithSeedAddress(base [32]byte, seed string, owner [32]byte) [32]byte {
	// SHA256(base + seed + owner)
	h := sha256.New()
	h.Write(base[:])
	h.Write([]byte(seed))
	h.Write(owner[:])

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}
