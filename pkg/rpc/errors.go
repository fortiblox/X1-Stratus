package rpc

import (
	"fmt"
)

// JSON-RPC 2.0 standard error codes.
const (
	// ParseError indicates invalid JSON was received.
	ParseError = -32700

	// InvalidRequest indicates the JSON sent is not a valid Request object.
	InvalidRequest = -32600

	// MethodNotFound indicates the method does not exist.
	MethodNotFound = -32601

	// InvalidParams indicates invalid method parameters.
	InvalidParams = -32602

	// InternalError indicates an internal JSON-RPC error.
	InternalError = -32603
)

// Solana-specific error codes.
const (
	// BlockCleanedUp indicates the block was cleaned up (pruned).
	BlockCleanedUp = -32001

	// SendTransactionPreflightFailure indicates preflight simulation failed.
	SendTransactionPreflightFailure = -32002

	// TransactionSignatureVerificationFailure indicates signature verification failed.
	TransactionSignatureVerificationFailure = -32003

	// BlockNotAvailable indicates the block is not available.
	BlockNotAvailable = -32004

	// NodeUnhealthy indicates the node is unhealthy.
	NodeUnhealthy = -32005

	// TransactionPrecompileVerificationFailure indicates precompile verification failed.
	TransactionPrecompileVerificationFailure = -32006

	// SlotSkipped indicates the slot was skipped.
	SlotSkipped = -32007

	// NoSnapshot indicates no snapshot is available.
	NoSnapshot = -32008

	// LongTermStorageSlotSkipped indicates the slot was skipped in long-term storage.
	LongTermStorageSlotSkipped = -32009

	// KeyExcludedFromSecondaryIndex indicates key excluded from secondary index.
	KeyExcludedFromSecondaryIndex = -32010

	// TransactionHistoryNotAvailable indicates transaction history not available.
	TransactionHistoryNotAvailable = -32011

	// ScanError indicates a scan/iteration error.
	ScanError = -32012

	// TransactionSignatureLenMismatch indicates signature length mismatch.
	TransactionSignatureLenMismatch = -32013

	// BlockStatusNotAvailableYet indicates block status not yet available.
	BlockStatusNotAvailableYet = -32014

	// UnsupportedTransactionVersion indicates unsupported transaction version.
	UnsupportedTransactionVersion = -32015

	// MinContextSlotNotReached indicates min context slot not yet reached.
	MinContextSlotNotReached = -32016
)

// Common error messages.
var (
	ErrParseError              = NewRPCError(ParseError, "Parse error")
	ErrInvalidRequest          = NewRPCError(InvalidRequest, "Invalid Request")
	ErrMethodNotFound          = NewRPCError(MethodNotFound, "Method not found")
	ErrInvalidParams           = NewRPCError(InvalidParams, "Invalid params")
	ErrInternalError           = NewRPCError(InternalError, "Internal error")
	ErrBlockCleanedUp          = NewRPCError(BlockCleanedUp, "Block cleaned up, does not exist on this node")
	ErrBlockNotAvailable       = NewRPCError(BlockNotAvailable, "Block not available for slot")
	ErrNodeUnhealthy           = NewRPCError(NodeUnhealthy, "Node is unhealthy")
	ErrSlotSkipped             = NewRPCError(SlotSkipped, "Slot was skipped, or missing due to ledger jump to recent snapshot")
	ErrNoSnapshot              = NewRPCError(NoSnapshot, "No snapshot available")
	ErrMinContextSlotNotReached = NewRPCError(MinContextSlotNotReached, "Minimum context slot has not been reached")
)

// NewRPCError creates a new RPC error.
func NewRPCError(code int, message string) *RPCError {
	return &RPCError{
		Code:    code,
		Message: message,
	}
}

// NewRPCErrorWithData creates a new RPC error with additional data.
func NewRPCErrorWithData(code int, message string, data interface{}) *RPCError {
	return &RPCError{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// Error implements the error interface.
func (e *RPCError) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("RPC error %d: %s (data: %v)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// InvalidParamsError creates an invalid params error with a custom message.
func InvalidParamsError(msg string) *RPCError {
	return NewRPCError(InvalidParams, msg)
}

// InvalidParamsErrorf creates an invalid params error with a formatted message.
func InvalidParamsErrorf(format string, args ...interface{}) *RPCError {
	return NewRPCError(InvalidParams, fmt.Sprintf(format, args...))
}

// InternalServerError creates an internal server error with a custom message.
func InternalServerError(msg string) *RPCError {
	return NewRPCError(InternalError, msg)
}

// InternalServerErrorf creates an internal server error with a formatted message.
func InternalServerErrorf(format string, args ...interface{}) *RPCError {
	return NewRPCError(InternalError, fmt.Sprintf(format, args...))
}

// AccountNotFoundError creates an error for account not found.
func AccountNotFoundError() *RPCError {
	return NewRPCError(InvalidParams, "Account not found")
}

// TransactionNotFoundError creates an error for transaction not found.
func TransactionNotFoundError() *RPCError {
	return NewRPCError(InvalidParams, "Transaction not found")
}

// BlockNotFoundError creates an error for block not found.
func BlockNotFoundError(slot uint64) *RPCError {
	return NewRPCErrorWithData(BlockNotAvailable,
		fmt.Sprintf("Block not available for slot %d", slot),
		map[string]uint64{"slot": slot})
}

// SlotSkippedError creates an error for skipped slot.
func SlotSkippedError(slot uint64) *RPCError {
	return NewRPCErrorWithData(SlotSkipped,
		fmt.Sprintf("Slot %d was skipped, or missing due to ledger jump to recent snapshot", slot),
		map[string]uint64{"slot": slot})
}

// MinContextSlotError creates an error for min context slot not reached.
func MinContextSlotError(minSlot, currentSlot uint64) *RPCError {
	return NewRPCErrorWithData(MinContextSlotNotReached,
		fmt.Sprintf("Minimum context slot %d has not been reached, current slot is %d", minSlot, currentSlot),
		map[string]uint64{"minSlot": minSlot, "currentSlot": currentSlot})
}

// UnsupportedTransactionVersionError creates an error for unsupported transaction version.
func UnsupportedTransactionVersionError(version int) *RPCError {
	return NewRPCErrorWithData(UnsupportedTransactionVersion,
		fmt.Sprintf("Transaction version %d is not supported", version),
		map[string]int{"version": version})
}

// TransactionError represents a transaction execution error.
type TransactionErrorCode int

const (
	// TransactionErrorAccountInUse indicates account in use.
	TransactionErrorAccountInUse TransactionErrorCode = iota

	// TransactionErrorAccountLoadedTwice indicates account loaded twice.
	TransactionErrorAccountLoadedTwice

	// TransactionErrorAccountNotFound indicates account not found.
	TransactionErrorAccountNotFound

	// TransactionErrorProgramAccountNotFound indicates program account not found.
	TransactionErrorProgramAccountNotFound

	// TransactionErrorInsufficientFundsForFee indicates insufficient funds for fee.
	TransactionErrorInsufficientFundsForFee

	// TransactionErrorInvalidAccountForFee indicates invalid account for fee.
	TransactionErrorInvalidAccountForFee

	// TransactionErrorAlreadyProcessed indicates already processed.
	TransactionErrorAlreadyProcessed

	// TransactionErrorBlockhashNotFound indicates blockhash not found.
	TransactionErrorBlockhashNotFound

	// TransactionErrorInstructionError indicates instruction error.
	TransactionErrorInstructionError

	// TransactionErrorCallChainTooDeep indicates call chain too deep.
	TransactionErrorCallChainTooDeep

	// TransactionErrorMissingSignatureForFee indicates missing signature for fee.
	TransactionErrorMissingSignatureForFee

	// TransactionErrorInvalidAccountIndex indicates invalid account index.
	TransactionErrorInvalidAccountIndex

	// TransactionErrorSignatureFailure indicates signature failure.
	TransactionErrorSignatureFailure

	// TransactionErrorInvalidProgramForExecution indicates invalid program for execution.
	TransactionErrorInvalidProgramForExecution

	// TransactionErrorSanitizeFailure indicates sanitize failure.
	TransactionErrorSanitizeFailure

	// TransactionErrorClusterMaintenance indicates cluster maintenance.
	TransactionErrorClusterMaintenance

	// TransactionErrorAccountBorrowOutstanding indicates account borrow outstanding.
	TransactionErrorAccountBorrowOutstanding

	// TransactionErrorWouldExceedMaxBlockCostLimit indicates would exceed max block cost limit.
	TransactionErrorWouldExceedMaxBlockCostLimit

	// TransactionErrorUnsupportedVersion indicates unsupported version.
	TransactionErrorUnsupportedVersion

	// TransactionErrorInvalidWritableAccount indicates invalid writable account.
	TransactionErrorInvalidWritableAccount

	// TransactionErrorWouldExceedMaxAccountCostLimit indicates would exceed max account cost limit.
	TransactionErrorWouldExceedMaxAccountCostLimit

	// TransactionErrorWouldExceedAccountDataBlockLimit indicates would exceed account data block limit.
	TransactionErrorWouldExceedAccountDataBlockLimit

	// TransactionErrorTooManyAccountLocks indicates too many account locks.
	TransactionErrorTooManyAccountLocks

	// TransactionErrorAddressLookupTableNotFound indicates address lookup table not found.
	TransactionErrorAddressLookupTableNotFound

	// TransactionErrorInvalidAddressLookupTableOwner indicates invalid address lookup table owner.
	TransactionErrorInvalidAddressLookupTableOwner

	// TransactionErrorInvalidAddressLookupTableData indicates invalid address lookup table data.
	TransactionErrorInvalidAddressLookupTableData

	// TransactionErrorInvalidAddressLookupTableIndex indicates invalid address lookup table index.
	TransactionErrorInvalidAddressLookupTableIndex

	// TransactionErrorInvalidRentPayingAccount indicates invalid rent paying account.
	TransactionErrorInvalidRentPayingAccount

	// TransactionErrorWouldExceedMaxVoteCostLimit indicates would exceed max vote cost limit.
	TransactionErrorWouldExceedMaxVoteCostLimit

	// TransactionErrorWouldExceedAccountDataTotalLimit indicates would exceed account data total limit.
	TransactionErrorWouldExceedAccountDataTotalLimit

	// TransactionErrorDuplicateInstruction indicates duplicate instruction.
	TransactionErrorDuplicateInstruction

	// TransactionErrorInsufficientFundsForRent indicates insufficient funds for rent.
	TransactionErrorInsufficientFundsForRent

	// TransactionErrorMaxLoadedAccountsDataSizeExceeded indicates max loaded accounts data size exceeded.
	TransactionErrorMaxLoadedAccountsDataSizeExceeded

	// TransactionErrorInvalidLoadedAccountsDataSizeLimit indicates invalid loaded accounts data size limit.
	TransactionErrorInvalidLoadedAccountsDataSizeLimit

	// TransactionErrorResanitizationNeeded indicates resanitization needed.
	TransactionErrorResanitizationNeeded

	// TransactionErrorProgramExecutionTemporarilyRestricted indicates program execution temporarily restricted.
	TransactionErrorProgramExecutionTemporarilyRestricted

	// TransactionErrorUnbalancedTransaction indicates unbalanced transaction.
	TransactionErrorUnbalancedTransaction
)

// String returns the string representation of the transaction error code.
func (e TransactionErrorCode) String() string {
	switch e {
	case TransactionErrorAccountInUse:
		return "AccountInUse"
	case TransactionErrorAccountLoadedTwice:
		return "AccountLoadedTwice"
	case TransactionErrorAccountNotFound:
		return "AccountNotFound"
	case TransactionErrorProgramAccountNotFound:
		return "ProgramAccountNotFound"
	case TransactionErrorInsufficientFundsForFee:
		return "InsufficientFundsForFee"
	case TransactionErrorInvalidAccountForFee:
		return "InvalidAccountForFee"
	case TransactionErrorAlreadyProcessed:
		return "AlreadyProcessed"
	case TransactionErrorBlockhashNotFound:
		return "BlockhashNotFound"
	case TransactionErrorInstructionError:
		return "InstructionError"
	case TransactionErrorCallChainTooDeep:
		return "CallChainTooDeep"
	case TransactionErrorMissingSignatureForFee:
		return "MissingSignatureForFee"
	case TransactionErrorInvalidAccountIndex:
		return "InvalidAccountIndex"
	case TransactionErrorSignatureFailure:
		return "SignatureFailure"
	case TransactionErrorInvalidProgramForExecution:
		return "InvalidProgramForExecution"
	case TransactionErrorSanitizeFailure:
		return "SanitizeFailure"
	case TransactionErrorClusterMaintenance:
		return "ClusterMaintenance"
	default:
		return "Unknown"
	}
}
