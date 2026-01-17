// Package rpcfetch provides a block fetcher that uses JSON-RPC to fetch blocks
// from Solana validators, serving as an alternative to the Geyser gRPC client.
//
// The rpcfetch package is designed to be a drop-in replacement for the geyser
// package when gRPC/Geyser access is not available. It provides the same
// channel-based interface for receiving blocks.
//
// # Architecture
//
// The package consists of three main components:
//
//   - Pool: Manages multiple RPC endpoints with health tracking
//   - RPCClient: Handles JSON-RPC communication with retry logic
//   - Fetcher: Orchestrates slot polling and block fetching
//
// # Usage
//
// Basic usage with a single endpoint:
//
//	pool := rpcfetch.NewSimplePool([]string{"https://api.mainnet-beta.solana.com"})
//	config := rpcfetch.DefaultConfig()
//
//	fetcher, err := rpcfetch.NewFetcher(pool, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	ctx := context.Background()
//	if err := fetcher.Connect(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer fetcher.Close()
//
//	for block := range fetcher.Blocks() {
//	    fmt.Printf("Received block %d with %d transactions\n",
//	        block.Slot, len(block.Transactions))
//	}
//
// # Integration with Node
//
// The Fetcher outputs *geyser.Block on its Blocks() channel, making it
// compatible with the existing node block ingestion loop:
//
//	// In node initialization, replace geyser.Client with rpcfetch.Fetcher:
//	// n.geyser = geyserClient
//
//	pool := rpcfetch.NewSimplePool(endpoints)
//	fetcher, err := rpcfetch.NewFetcher(pool, rpcfetch.DefaultConfig())
//	// Use fetcher.Blocks() the same way as geyser.Client.Blocks()
//
// # Configuration
//
// Key configuration options:
//
//   - PollInterval: How often to check for new slots (default: 400ms)
//   - RequestTimeout: Timeout for individual RPC requests (default: 30s)
//   - MaxRetries: Number of retries for failed requests (default: 3)
//   - Commitment: Commitment level for queries (default: "confirmed")
//   - FromSlot: Starting slot (default: current network slot)
//
// # Skipped Slots
//
// Solana has slot leaders that may fail to produce blocks, resulting in
// skipped slots. The fetcher handles this automatically by detecting
// null block responses and moving to the next slot.
//
// # Error Handling
//
// The fetcher distinguishes between:
//
//   - Transient errors: Retried automatically with exponential backoff
//   - Skipped slots: Handled silently by moving to the next slot
//   - Fatal errors: Reported via the OnDisconnect callback
//
// Use IsSlotSkipped() to check if an error indicates a skipped slot:
//
//	if rpcfetch.IsSlotSkipped(err) {
//	    // Normal condition, slot had no block
//	}
//
// # RPC Pool Interface
//
// The Pool interface allows custom endpoint management implementations.
// The package provides SimplePool for basic round-robin selection.
// For production use, implement a Pool with:
//
//   - Health checking
//   - Latency-based routing
//   - Rate limiting
//   - Failover logic
//
// # Limitations
//
// Compared to Geyser gRPC streaming:
//
//   - Higher latency due to polling
//   - More RPC calls (one per block)
//   - No real-time slot status updates
//   - No entry-level (PoH) data
//
// The rpcfetch package is best suited for:
//
//   - Development and testing
//   - Backup data source
//   - Environments without Geyser access
package rpcfetch
