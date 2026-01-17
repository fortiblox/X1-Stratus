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

// Nonce account data size (80 bytes: 4 version + 4 state + 32 authority + 32 blockhash + 8 fee calculator)
const NonceAccountDataSize = 80

// NonceState represents the state of a nonce account.
type NonceState uint32

const (
	NonceStateUninitialized NonceState = 0
	NonceStateInitialized   NonceState = 1
)

// NonceData represents the data stored in a nonce account.
type NonceData struct {
	Version      uint32   // Always 1 for current version
	State        uint32   // 0 = uninitialized, 1 = initialized
	Authority    [32]byte // Authority authorized to advance nonce
	Blockhash    [32]byte // Stored blockhash (durable transaction nonce)
	FeePerSig    uint64   // Fee calculator (lamports per signature)
}

// Nonce errors.
var (
	ErrNonceNotInitialized     = errors.New("nonce account not initialized")
	ErrNonceAlreadyInitialized = errors.New("nonce account already initialized")
	ErrNonceBlockhashNotExpired = errors.New("nonce blockhash not yet expired")
	ErrNonceUnauthorized       = errors.New("nonce authority mismatch")
)

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

	// GetRecentBlockhash returns the most recent blockhash.
	GetRecentBlockhash() [32]byte

	// GetFeePerSignature returns the current fee per signature.
	GetFeePerSignature() uint64

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
	case InstructionInitializeNonceAccount:
		return p.processInitializeNonceAccount(ctx, data[4:])
	case InstructionAdvanceNonceAccount:
		return p.processAdvanceNonceAccount(ctx, data[4:])
	case InstructionWithdrawNonceAccount:
		return p.processWithdrawNonceAccount(ctx, data[4:])
	case InstructionAuthorizeNonceAccount:
		return p.processAuthorizeNonceAccount(ctx, data[4:])
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

	// Check for overflow on destination
	if to.Lamports > ^uint64(0)-lamports {
		return errors.New("lamport overflow")
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

// processInitializeNonceAccount initializes a nonce account.
func (p *Processor) processInitializeNonceAccount(ctx InvokeContext, data []byte) error {
	// Parse parameters: authority (32)
	if len(data) < 32 {
		return ErrInvalidInstructionData
	}

	var authority [32]byte
	copy(authority[:], data[0:32])

	// Get accounts: [0] = nonce account, [1] = recent blockhashes sysvar, [2] = rent sysvar
	nonceAccount, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// Verify nonce account is writable
	if !nonceAccount.IsWritable {
		return errors.New("nonce account not writable")
	}

	// Verify nonce account is owned by system program
	if nonceAccount.Owner != ProgramID {
		return ErrInvalidAccountOwner
	}

	// Check if already initialized
	if len(nonceAccount.Data) >= 8 && binary.LittleEndian.Uint32(nonceAccount.Data[4:8]) == uint32(NonceStateInitialized) {
		return ErrNonceAlreadyInitialized
	}

	// Ensure account has enough space
	if len(nonceAccount.Data) < NonceAccountDataSize {
		return ErrAccountDataTooSmall
	}

	// Check rent exemption
	rentMinimum := ctx.GetRentMinimum(NonceAccountDataSize)
	if nonceAccount.Lamports < rentMinimum {
		return ErrAccountNotRentExempt
	}

	// Get recent blockhash
	blockhash := ctx.GetRecentBlockhash()
	feePerSig := ctx.GetFeePerSignature()

	// Write nonce data
	binary.LittleEndian.PutUint32(nonceAccount.Data[0:4], 1)                    // Version
	binary.LittleEndian.PutUint32(nonceAccount.Data[4:8], uint32(NonceStateInitialized)) // State
	copy(nonceAccount.Data[8:40], authority[:])                                 // Authority
	copy(nonceAccount.Data[40:72], blockhash[:])                                // Blockhash
	binary.LittleEndian.PutUint64(nonceAccount.Data[72:80], feePerSig)          // Fee per signature

	ctx.Log("InitializeNonceAccount: success")
	return nil
}

// processAdvanceNonceAccount advances the nonce to a new blockhash.
func (p *Processor) processAdvanceNonceAccount(ctx InvokeContext, data []byte) error {
	// Get accounts: [0] = nonce account, [1] = recent blockhashes sysvar, [2] = nonce authority
	nonceAccount, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	authority, err := ctx.GetAccount(2)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// Authority must be signer
	if !authority.IsSigner {
		return ErrMissingRequiredSignature
	}

	// Verify nonce account is writable
	if !nonceAccount.IsWritable {
		return errors.New("nonce account not writable")
	}

	// Check if initialized
	if len(nonceAccount.Data) < NonceAccountDataSize {
		return ErrNonceNotInitialized
	}
	if binary.LittleEndian.Uint32(nonceAccount.Data[4:8]) != uint32(NonceStateInitialized) {
		return ErrNonceNotInitialized
	}

	// Verify authority
	var storedAuthority [32]byte
	copy(storedAuthority[:], nonceAccount.Data[8:40])
	if storedAuthority != authority.Key {
		return ErrNonceUnauthorized
	}

	// Get new blockhash
	newBlockhash := ctx.GetRecentBlockhash()

	// Check that new blockhash is different from stored one
	var storedBlockhash [32]byte
	copy(storedBlockhash[:], nonceAccount.Data[40:72])
	if storedBlockhash == newBlockhash {
		return ErrNonceBlockhashNotExpired
	}

	// Update blockhash and fee
	feePerSig := ctx.GetFeePerSignature()
	copy(nonceAccount.Data[40:72], newBlockhash[:])
	binary.LittleEndian.PutUint64(nonceAccount.Data[72:80], feePerSig)

	ctx.Log("AdvanceNonceAccount: success")
	return nil
}

// processWithdrawNonceAccount withdraws lamports from a nonce account.
func (p *Processor) processWithdrawNonceAccount(ctx InvokeContext, data []byte) error {
	// Parse parameters: lamports (8)
	if len(data) < 8 {
		return ErrInvalidInstructionData
	}

	lamports := binary.LittleEndian.Uint64(data[0:8])

	// Get accounts: [0] = nonce account, [1] = destination, [2] = recent blockhashes, [3] = rent, [4] = authority
	nonceAccount, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	destination, err := ctx.GetAccount(1)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	authority, err := ctx.GetAccount(4)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// Authority must be signer
	if !authority.IsSigner {
		return ErrMissingRequiredSignature
	}

	// Verify accounts are writable
	if !nonceAccount.IsWritable || !destination.IsWritable {
		return errors.New("accounts not writable")
	}

	// Check if initialized
	if len(nonceAccount.Data) < NonceAccountDataSize {
		return ErrNonceNotInitialized
	}
	if binary.LittleEndian.Uint32(nonceAccount.Data[4:8]) != uint32(NonceStateInitialized) {
		return ErrNonceNotInitialized
	}

	// Verify authority
	var storedAuthority [32]byte
	copy(storedAuthority[:], nonceAccount.Data[8:40])
	if storedAuthority != authority.Key {
		return ErrNonceUnauthorized
	}

	// Check balance
	if nonceAccount.Lamports < lamports {
		return ErrInsufficientFunds
	}

	// If withdrawing all lamports, check that we can close the account
	// Otherwise, check rent exemption
	remaining := nonceAccount.Lamports - lamports
	if remaining > 0 {
		rentMinimum := ctx.GetRentMinimum(NonceAccountDataSize)
		if remaining < rentMinimum {
			return ErrAccountNotRentExempt
		}
	}

	// Check for overflow
	if destination.Lamports > ^uint64(0)-lamports {
		return errors.New("lamport overflow")
	}

	// Execute withdrawal
	nonceAccount.Lamports -= lamports
	destination.Lamports += lamports

	// If account is now empty, mark as uninitialized
	if nonceAccount.Lamports == 0 {
		binary.LittleEndian.PutUint32(nonceAccount.Data[4:8], uint32(NonceStateUninitialized))
	}

	ctx.Log("WithdrawNonceAccount: success")
	return nil
}

// processAuthorizeNonceAccount changes the authority of a nonce account.
func (p *Processor) processAuthorizeNonceAccount(ctx InvokeContext, data []byte) error {
	// Parse parameters: new_authority (32)
	if len(data) < 32 {
		return ErrInvalidInstructionData
	}

	var newAuthority [32]byte
	copy(newAuthority[:], data[0:32])

	// Get accounts: [0] = nonce account, [1] = current authority
	nonceAccount, err := ctx.GetAccount(0)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	authority, err := ctx.GetAccount(1)
	if err != nil {
		return ErrNotEnoughAccountKeys
	}

	// Authority must be signer
	if !authority.IsSigner {
		return ErrMissingRequiredSignature
	}

	// Verify nonce account is writable
	if !nonceAccount.IsWritable {
		return errors.New("nonce account not writable")
	}

	// Check if initialized
	if len(nonceAccount.Data) < NonceAccountDataSize {
		return ErrNonceNotInitialized
	}
	if binary.LittleEndian.Uint32(nonceAccount.Data[4:8]) != uint32(NonceStateInitialized) {
		return ErrNonceNotInitialized
	}

	// Verify current authority
	var storedAuthority [32]byte
	copy(storedAuthority[:], nonceAccount.Data[8:40])
	if storedAuthority != authority.Key {
		return ErrNonceUnauthorized
	}

	// Update authority
	copy(nonceAccount.Data[8:40], newAuthority[:])

	ctx.Log("AuthorizeNonceAccount: success")
	return nil
}
