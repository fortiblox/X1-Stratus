package rpcfetch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fortiblox/X1-Stratus/pkg/geyser"
)

// Default configuration values.
const (
	// DefaultPollInterval is the default interval for polling new slots.
	// Solana slot time is ~400ms.
	DefaultPollInterval = 400 * time.Millisecond

	// DefaultRequestTimeout is the default timeout for RPC requests.
	DefaultRequestTimeout = 30 * time.Second

	// DefaultBlockChannelSize is the default buffer size for the block channel.
	DefaultBlockChannelSize = 100

	// DefaultMaxRetries is the default number of retries for failed requests.
	DefaultMaxRetries = 3

	// DefaultRetryDelay is the initial delay between retries.
	DefaultRetryDelay = 100 * time.Millisecond

	// DefaultMaxRetryDelay is the maximum delay between retries.
	DefaultMaxRetryDelay = 5 * time.Second

	// DefaultStaleTimeout is how long without updates before connection is stale.
	DefaultStaleTimeout = 60 * time.Second
)

// Config holds configuration for the Fetcher.
type Config struct {
	// PollInterval is the interval for polling new slots.
	// Defaults to 400ms (Solana slot time).
	PollInterval time.Duration

	// RequestTimeout is the timeout for individual RPC requests.
	RequestTimeout time.Duration

	// BlockChannelSize is the buffer size for the block output channel.
	BlockChannelSize int

	// Commitment is the commitment level for slot queries.
	// Should be "confirmed" or "finalized".
	Commitment string

	// FromSlot is the starting slot. If 0, starts from current slot.
	FromSlot *uint64

	// MaxRetries is the number of retries for failed block fetches.
	MaxRetries int

	// RetryDelay is the initial delay between retries.
	RetryDelay time.Duration

	// MaxRetryDelay is the maximum delay between retries.
	MaxRetryDelay time.Duration

	// StaleTimeout is how long without updates before reconnection.
	StaleTimeout time.Duration

	// OnBlock is called for each received block (optional).
	OnBlock func(*geyser.Block)

	// OnConnect is called when fetching starts.
	OnConnect func()

	// OnDisconnect is called when fetching stops due to error.
	OnDisconnect func(error)

	// OnReconnect is called after successful recovery.
	OnReconnect func(attempt int)
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		PollInterval:     DefaultPollInterval,
		RequestTimeout:   DefaultRequestTimeout,
		BlockChannelSize: DefaultBlockChannelSize,
		Commitment:       "confirmed",
		MaxRetries:       DefaultMaxRetries,
		RetryDelay:       DefaultRetryDelay,
		MaxRetryDelay:    DefaultMaxRetryDelay,
		StaleTimeout:     DefaultStaleTimeout,
	}
}

// WithDefaults applies default values for any unset fields.
func (c Config) WithDefaults() Config {
	defaults := DefaultConfig()

	if c.PollInterval == 0 {
		c.PollInterval = defaults.PollInterval
	}
	if c.RequestTimeout == 0 {
		c.RequestTimeout = defaults.RequestTimeout
	}
	if c.BlockChannelSize == 0 {
		c.BlockChannelSize = defaults.BlockChannelSize
	}
	if c.Commitment == "" {
		c.Commitment = defaults.Commitment
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaults.MaxRetries
	}
	if c.RetryDelay == 0 {
		c.RetryDelay = defaults.RetryDelay
	}
	if c.MaxRetryDelay == 0 {
		c.MaxRetryDelay = defaults.MaxRetryDelay
	}
	if c.StaleTimeout == 0 {
		c.StaleTimeout = defaults.StaleTimeout
	}

	return c
}

// Fetcher fetches blocks from Solana RPC endpoints.
// It provides the same interface as geyser.Client for seamless integration.
type Fetcher struct {
	config Config
	client *RPCClient
	pool   Pool

	// Output channel for blocks
	blocks chan *geyser.Block

	// State tracking
	mu              sync.RWMutex
	running         atomic.Bool
	closed          atomic.Bool
	currentSlot     atomic.Uint64
	latestSlot      atomic.Uint64
	lastUpdate      atomic.Int64
	reconnectCount  atomic.Int32
	lastError       error
	lastErrorMu     sync.RWMutex

	// Context and synchronization
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewFetcher creates a new block fetcher with the given pool and configuration.
func NewFetcher(pool Pool, config Config) (*Fetcher, error) {
	config = config.WithDefaults()

	if pool == nil {
		return nil, ErrNoEndpoints
	}

	client := NewRPCClient(pool, config.RequestTimeout)

	return &Fetcher{
		config: config,
		client: client,
		pool:   pool,
		blocks: make(chan *geyser.Block, config.BlockChannelSize),
	}, nil
}

// Connect starts the block fetching process.
// This mimics the geyser.Client.Connect interface.
func (f *Fetcher) Connect(ctx context.Context) error {
	if f.closed.Load() {
		return ErrClosed
	}
	if f.running.Load() {
		return ErrAlreadyRunning
	}

	// Create cancellable context
	f.ctx, f.cancel = context.WithCancel(ctx)
	f.running.Store(true)
	f.lastUpdate.Store(time.Now().UnixNano())

	// Determine starting slot
	var startSlot uint64
	if f.config.FromSlot != nil {
		startSlot = *f.config.FromSlot
	} else {
		// Get current slot from the network
		slot, err := f.client.GetSlot(f.ctx, f.config.Commitment)
		if err != nil {
			f.running.Store(false)
			return fmt.Errorf("get initial slot: %w", err)
		}
		startSlot = slot
	}

	f.currentSlot.Store(startSlot)
	f.latestSlot.Store(startSlot)

	// Start the fetching loops
	f.wg.Add(2)
	go f.slotPollingLoop()
	go f.blockFetchingLoop()

	// Call connect callback
	if f.config.OnConnect != nil {
		f.config.OnConnect()
	}

	return nil
}

// slotPollingLoop continuously polls for the latest slot.
func (f *Fetcher) slotPollingLoop() {
	defer f.wg.Done()

	ticker := time.NewTicker(f.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			slot, err := f.client.GetSlot(f.ctx, f.config.Commitment)
			if err != nil {
				f.setLastError(err)
				// Don't stop on transient errors
				continue
			}

			// Update latest slot if newer
			for {
				current := f.latestSlot.Load()
				if slot <= current {
					break
				}
				if f.latestSlot.CompareAndSwap(current, slot) {
					break
				}
			}

			f.lastUpdate.Store(time.Now().UnixNano())
		}
	}
}

// blockFetchingLoop fetches blocks as new slots become available.
func (f *Fetcher) blockFetchingLoop() {
	defer f.wg.Done()

	// Small delay to allow slot polling to start
	time.Sleep(50 * time.Millisecond)

	nextSlot := f.currentSlot.Load()

	for {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		latestSlot := f.latestSlot.Load()

		// Wait if we're caught up
		if nextSlot > latestSlot {
			select {
			case <-f.ctx.Done():
				return
			case <-time.After(f.config.PollInterval / 2):
				continue
			}
		}

		// Fetch the block
		block, err := f.fetchBlockWithRetry(f.ctx, nextSlot)
		if err != nil {
			if errors.Is(err, ErrSlotSkipped) {
				// Slot was skipped (no block), move to next
				nextSlot++
				continue
			}

			f.setLastError(err)

			// Check if context was cancelled
			if f.ctx.Err() != nil {
				return
			}

			// For other errors, wait and retry same slot
			select {
			case <-f.ctx.Done():
				return
			case <-time.After(f.config.RetryDelay):
				continue
			}
		}

		// Successfully fetched block
		f.currentSlot.Store(block.Slot)
		f.lastUpdate.Store(time.Now().UnixNano())

		// Call callback if configured
		if f.config.OnBlock != nil {
			f.config.OnBlock(block)
		}

		// Send to channel (non-blocking with potential drop of oldest)
		select {
		case f.blocks <- block:
		default:
			// Channel full, drop oldest and add new
			select {
			case <-f.blocks:
			default:
			}
			f.blocks <- block
		}

		nextSlot++
	}
}

// fetchBlockWithRetry fetches a block with retry logic.
func (f *Fetcher) fetchBlockWithRetry(ctx context.Context, slot uint64) (*geyser.Block, error) {
	var lastErr error
	delay := f.config.RetryDelay

	for attempt := 0; attempt <= f.config.MaxRetries; attempt++ {
		// Check context before each attempt
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		block, err := f.client.GetBlock(ctx, slot)
		if err == nil {
			return block, nil
		}

		// Check if slot was skipped (not an error condition)
		if errors.Is(err, ErrSlotSkipped) {
			return nil, ErrSlotSkipped
		}

		// Check for RPC errors that indicate slot doesn't exist
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) {
			// -32009: Slot was skipped or not in ledger
			// -32007: Slot was skipped
			if rpcErr.Code == -32009 || rpcErr.Code == -32007 {
				return nil, ErrSlotSkipped
			}
		}

		lastErr = err

		// Wait before retry with exponential backoff
		if attempt < f.config.MaxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			delay = min(delay*2, f.config.MaxRetryDelay)
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", f.config.MaxRetries+1, lastErr)
}

// Blocks returns the channel for receiving block updates.
// This matches the geyser.Client.Blocks() interface.
func (f *Fetcher) Blocks() <-chan *geyser.Block {
	return f.blocks
}

// Health returns the current health status.
func (f *Fetcher) Health() geyser.ClientHealth {
	lastUpdate := time.Unix(0, f.lastUpdate.Load())
	latency := time.Since(lastUpdate)

	var provider string
	if endpoint, err := f.pool.GetEndpoint(context.Background()); err == nil {
		provider = endpoint.URL
	}

	return geyser.ClientHealth{
		Connected:      f.running.Load() && !f.closed.Load(),
		LastSlot:       f.currentSlot.Load(),
		LastUpdate:     lastUpdate,
		Provider:       provider,
		Latency:        latency,
		ReconnectCount: int(f.reconnectCount.Load()),
		LastError:      f.getLastError(),
	}
}

// Close stops the fetcher and releases resources.
func (f *Fetcher) Close() error {
	if f.closed.Swap(true) {
		return ErrClosed
	}

	// Cancel context to stop goroutines
	if f.cancel != nil {
		f.cancel()
	}

	// Wait for goroutines to finish
	f.wg.Wait()

	// Close the blocks channel
	close(f.blocks)

	// Close the pool
	if f.pool != nil {
		f.pool.Close()
	}

	f.running.Store(false)
	return nil
}

// CurrentSlot returns the last fetched slot.
func (f *Fetcher) CurrentSlot() uint64 {
	return f.currentSlot.Load()
}

// LatestSlot returns the latest known slot from the network.
func (f *Fetcher) LatestSlot() uint64 {
	return f.latestSlot.Load()
}

// IsRunning returns whether the fetcher is currently running.
func (f *Fetcher) IsRunning() bool {
	return f.running.Load() && !f.closed.Load()
}

// setLastError safely sets the last error.
func (f *Fetcher) setLastError(err error) {
	f.lastErrorMu.Lock()
	f.lastError = err
	f.lastErrorMu.Unlock()
}

// getLastError safely gets the last error.
func (f *Fetcher) getLastError() error {
	f.lastErrorMu.RLock()
	defer f.lastErrorMu.RUnlock()
	return f.lastError
}

// min returns the minimum of two durations.
func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
