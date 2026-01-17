package rpcfetch

import (
	"errors"
	"fmt"
)

// Package errors.
var (
	// ErrNoEndpoints is returned when no RPC endpoints are available.
	ErrNoEndpoints = errors.New("no RPC endpoints available")

	// ErrClosed is returned when operating on a closed fetcher.
	ErrClosed = errors.New("fetcher is closed")

	// ErrAlreadyRunning is returned when Connect is called on a running fetcher.
	ErrAlreadyRunning = errors.New("fetcher is already running")

	// ErrSlotSkipped is returned when a slot has no block (was skipped).
	ErrSlotSkipped = errors.New("slot was skipped")

	// ErrBlockNotFound is returned when a block cannot be found.
	ErrBlockNotFound = errors.New("block not found")

	// ErrRequestTimeout is returned when an RPC request times out.
	ErrRequestTimeout = errors.New("request timeout")
)

// RPCError represents a JSON-RPC error response.
type RPCError struct {
	Code    int
	Message string
}

// Error implements the error interface.
func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// IsSlotSkipped returns true if the error indicates a skipped slot.
func IsSlotSkipped(err error) bool {
	if errors.Is(err, ErrSlotSkipped) {
		return true
	}

	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		// Common RPC error codes for missing/skipped slots:
		// -32009: Slot was skipped, or missing in long-term storage
		// -32007: Slot was skipped
		// -32004: Block not available for slot
		switch rpcErr.Code {
		case -32009, -32007, -32004:
			return true
		}
	}

	return false
}

// IsRetryable returns true if the error is likely transient and worth retrying.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry slot skipped errors
	if IsSlotSkipped(err) {
		return false
	}

	// Don't retry closed errors
	if errors.Is(err, ErrClosed) {
		return false
	}

	// Most other errors are potentially retryable
	return true
}
