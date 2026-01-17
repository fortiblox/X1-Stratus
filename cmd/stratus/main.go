// X1-Stratus: Lightweight Verification Node for X1 Blockchain
//
// This is the main entry point for X1-Stratus, a lightweight block verification
// node that uses RPC-based block fetching with dynamic validator pool discovery.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/fortiblox/X1-Stratus/pkg/gossip"
	"github.com/fortiblox/X1-Stratus/pkg/rpcfetch"
)

// Version information
var (
	Version   = "0.1.0"
	GitCommit = "dev"
)

// Configuration flags
var (
	dataDir       = flag.String("data-dir", "/mnt/x1-stratus", "Data directory for blockstore and accounts")
	logLevel      = flag.String("log-level", "info", "Log level: debug, info, warn, error")
	rpcAddr       = flag.String("rpc-addr", ":8899", "RPC server listen address")
	enableRPC     = flag.Bool("enable-rpc", false, "Enable JSON-RPC server")
	commitment    = flag.String("commitment", "confirmed", "Commitment level: processed, confirmed, finalized")
	slotThreshold = flag.Uint64("slot-threshold", 50, "Max slots behind before marking endpoint unhealthy")
	startSlot     = flag.Uint64("start-slot", 0, "Starting slot (0 = current)")
	showVersion   = flag.Bool("version", false, "Print version and exit")
)

// Reference RPC endpoints (source of truth for current slot)
var referenceEndpoints = []string{
	"https://rpc.mainnet.x1.xyz",
	"https://entrypoint0.mainnet.x1.xyz",
	"https://entrypoint1.mainnet.x1.xyz",
	"https://entrypoint2.mainnet.x1.xyz",
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("X1-Stratus %s (%s)\n", Version, GitCommit)
		os.Exit(0)
	}

	// Setup logging
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("Starting X1-Stratus %s", Version)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Discover validators on the network
	log.Println("Discovering validators on X1 network...")
	endpoints, err := discoverEndpoints(ctx)
	if err != nil {
		log.Printf("Warning: validator discovery failed: %v", err)
		log.Println("Falling back to reference endpoints only")
		endpoints = referenceEndpoints
	}

	log.Printf("Using %d RPC endpoints", len(endpoints))
	for i, ep := range endpoints {
		if i < 5 {
			log.Printf("  - %s", ep)
		} else if i == 5 {
			log.Printf("  ... and %d more", len(endpoints)-5)
			break
		}
	}

	// Create a discovery function that can be called for periodic re-discovery
	discoveryFunc := func(ctx context.Context) ([]string, error) {
		return discoverEndpoints(ctx)
	}

	// Create the RPC pool with discovered endpoints and discovery function
	pool := NewSmartPool(referenceEndpoints, endpoints, *slotThreshold, discoveryFunc)
	pool.Start(ctx)

	// Create the block fetcher
	fetcherConfig := rpcfetch.DefaultConfig()
	fetcherConfig.Commitment = *commitment
	if *startSlot > 0 {
		fetcherConfig.FromSlot = startSlot
	}
	fetcherConfig.OnConnect = func() {
		log.Println("Block fetcher connected")
	}
	fetcherConfig.OnDisconnect = func(err error) {
		log.Printf("Block fetcher disconnected: %v", err)
	}

	fetcher, err := rpcfetch.NewFetcher(pool, fetcherConfig)
	if err != nil {
		log.Fatalf("Failed to create block fetcher: %v", err)
	}
	defer fetcher.Close()

	// Start fetching blocks
	if err := fetcher.Connect(ctx); err != nil {
		log.Fatalf("Failed to start block fetcher: %v", err)
	}

	log.Println("Starting block verification...")

	// Print status periodically
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				health := fetcher.Health()
				log.Printf("Status: slot=%d, healthy_endpoints=%d, connected=%v",
					health.LastSlot, pool.GetHealthyCount(), health.Connected)
			}
		}
	}()

	// Process blocks
	blocksProcessed := 0
	for {
		select {
		case <-ctx.Done():
			log.Printf("Processed %d blocks total", blocksProcessed)
			log.Println("X1-Stratus stopped")
			return
		case block, ok := <-fetcher.Blocks():
			if !ok {
				log.Println("Block channel closed")
				return
			}
			blocksProcessed++
			if blocksProcessed%100 == 0 {
				log.Printf("Processed %d blocks, current slot: %d", blocksProcessed, block.Slot)
			}
		}
	}
}

// discoverEndpoints discovers RPC endpoints from the X1 network.
func discoverEndpoints(ctx context.Context) ([]string, error) {
	// Try RPC-based discovery first (simpler, works when UDP blocked)
	endpoints, err := gossip.QuickDiscoverRPCEndpoints(ctx, referenceEndpoints[0])
	if err == nil && len(endpoints) > 0 {
		return endpoints, nil
	}

	// Fallback to gossip protocol
	cfg := gossip.X1MainnetConfig()
	cfg.Timeout = 30 * time.Second
	cfg.RPCEndpoint = referenceEndpoints[0]

	client, err := gossip.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create gossip client: %w", err)
	}
	defer client.Close()

	result, err := client.Discover(ctx)
	if err != nil {
		return nil, fmt.Errorf("gossip discovery: %w", err)
	}

	// Extract RPC endpoints
	var rpcEndpoints []string
	for _, node := range result.RPCNodes() {
		if url := node.RPCURL(); url != "" {
			rpcEndpoints = append(rpcEndpoints, url)
		}
	}

	if len(rpcEndpoints) == 0 {
		return nil, fmt.Errorf("no RPC endpoints discovered")
	}

	return rpcEndpoints, nil
}

// DiscoveryFunc is a function type for discovering endpoints.
type DiscoveryFunc func(context.Context) ([]string, error)

// SmartPool implements rpcfetch.Pool with dynamic health checking.
type SmartPool struct {
	referenceURLs []string
	endpoints     []*rpcfetch.Endpoint
	threshold     uint64

	currentSlot uint64
	mu          sync.RWMutex
	idx         int
	closed      bool

	// Re-discovery configuration
	rediscoverInterval time.Duration
	discoveryFunc      DiscoveryFunc
	endpointSet        map[string]bool // For fast deduplication

	ctx    context.Context
	cancel context.CancelFunc
}

// NewSmartPool creates a pool with reference endpoints for slot truth.
func NewSmartPool(referenceURLs, endpoints []string, threshold uint64, discoveryFunc DiscoveryFunc) *SmartPool {
	pool := &SmartPool{
		referenceURLs:      referenceURLs,
		threshold:          threshold,
		endpoints:          make([]*rpcfetch.Endpoint, 0, len(endpoints)),
		endpointSet:        make(map[string]bool),
		rediscoverInterval: 5 * time.Minute, // Default: re-discover every 5 minutes
		discoveryFunc:      discoveryFunc,
	}

	// Add all endpoints (deduplicated)
	for _, url := range endpoints {
		if !pool.endpointSet[url] {
			pool.endpointSet[url] = true
			pool.endpoints = append(pool.endpoints, &rpcfetch.Endpoint{
				URL:     url,
				Healthy: true,
			})
		}
	}

	return pool
}

// SetRediscoverInterval sets the interval for periodic re-discovery.
func (p *SmartPool) SetRediscoverInterval(interval time.Duration) {
	p.rediscoverInterval = interval
}

// Start begins periodic health checking and re-discovery.
func (p *SmartPool) Start(ctx context.Context) {
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Health check goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		// Initial health check
		p.healthCheck()

		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				p.healthCheck()
			}
		}
	}()

	// Re-discovery goroutine (runs independently, doesn't block health checks or fetching)
	if p.discoveryFunc != nil && p.rediscoverInterval > 0 {
		go func() {
			ticker := time.NewTicker(p.rediscoverInterval)
			defer ticker.Stop()

			for {
				select {
				case <-p.ctx.Done():
					return
				case <-ticker.C:
					p.rediscoverEndpoints()
				}
			}
		}()
	}
}

// healthCheck queries reference endpoints and updates endpoint health.
func (p *SmartPool) healthCheck() {
	// Get reference slot
	refSlot, err := p.getReferenceSlot()
	if err != nil {
		log.Printf("Failed to get reference slot: %v", err)
		return
	}

	p.mu.Lock()
	p.currentSlot = refSlot
	p.mu.Unlock()

	// Check each endpoint concurrently
	var wg sync.WaitGroup
	for _, ep := range p.endpoints {
		wg.Add(1)
		go func(endpoint *rpcfetch.Endpoint) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
			defer cancel()

			slot, err := getSlot(ctx, endpoint.URL)
			if err != nil {
				endpoint.Healthy = false
				endpoint.LastError = err
				return
			}

			// Check if within threshold
			if refSlot > slot && (refSlot-slot) > p.threshold {
				endpoint.Healthy = false
				endpoint.LastError = fmt.Errorf("slot %d is %d behind reference %d", slot, refSlot-slot, refSlot)
			} else {
				endpoint.Healthy = true
				endpoint.LastError = nil
				endpoint.LastSuccess = time.Now()
			}
		}(ep)
	}
	wg.Wait()
}

// rediscoverEndpoints calls the discovery function and adds any new endpoints to the pool.
func (p *SmartPool) rediscoverEndpoints() {
	if p.discoveryFunc == nil {
		return
	}

	// Create a timeout context for discovery
	ctx, cancel := context.WithTimeout(p.ctx, 60*time.Second)
	defer cancel()

	log.Println("Starting periodic endpoint re-discovery...")

	// Call the discovery function
	discoveredEndpoints, err := p.discoveryFunc(ctx)
	if err != nil {
		log.Printf("Endpoint re-discovery failed: %v", err)
		return
	}

	// Add new endpoints (with proper locking)
	p.mu.Lock()
	newCount := 0
	for _, url := range discoveredEndpoints {
		if !p.endpointSet[url] {
			p.endpointSet[url] = true
			p.endpoints = append(p.endpoints, &rpcfetch.Endpoint{
				URL:     url,
				Healthy: true, // Assume healthy until proven otherwise
			})
			newCount++
		}
	}
	totalCount := len(p.endpoints)
	p.mu.Unlock()

	if newCount > 0 {
		log.Printf("Re-discovered %d new endpoints, pool now has %d endpoints", newCount, totalCount)
	} else {
		log.Printf("Re-discovery complete, no new endpoints found (pool has %d endpoints)", totalCount)
	}
}

// getReferenceSlot gets the current slot from reference endpoints.
func (p *SmartPool) getReferenceSlot() (uint64, error) {
	for _, url := range p.referenceURLs {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		slot, err := getSlot(ctx, url)
		cancel()
		if err == nil {
			return slot, nil
		}
	}
	return 0, fmt.Errorf("all reference endpoints failed")
}

// GetEndpoint returns a healthy endpoint.
func (p *SmartPool) GetEndpoint(ctx context.Context) (*rpcfetch.Endpoint, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, fmt.Errorf("pool closed")
	}

	// Try each endpoint with round-robin
	for i := 0; i < len(p.endpoints); i++ {
		idx := (p.idx + i) % len(p.endpoints)
		ep := p.endpoints[idx]
		if ep.Healthy {
			p.idx = (idx + 1) % len(p.endpoints)
			return ep, nil
		}
	}

	// No healthy endpoints, return first one (may recover)
	if len(p.endpoints) > 0 {
		return p.endpoints[0], nil
	}

	return nil, fmt.Errorf("no endpoints available")
}

// MarkUnhealthy marks an endpoint as unhealthy.
func (p *SmartPool) MarkUnhealthy(url string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, ep := range p.endpoints {
		if ep.URL == url {
			ep.Healthy = false
			ep.LastError = err
			return
		}
	}
}

// MarkHealthy marks an endpoint as healthy.
func (p *SmartPool) MarkHealthy(url string, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, ep := range p.endpoints {
		if ep.URL == url {
			ep.Healthy = true
			ep.LastSuccess = time.Now()
			ep.Latency = latency
			ep.LastError = nil
			return
		}
	}
}

// GetHealthyCount returns the number of healthy endpoints.
func (p *SmartPool) GetHealthyCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, ep := range p.endpoints {
		if ep.Healthy {
			count++
		}
	}
	return count
}

// Close stops the pool.
func (p *SmartPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// getSlot queries an RPC endpoint for the current slot.
func getSlot(ctx context.Context, url string) (uint64, error) {
	reqBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"getSlot"}`)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Result uint64 `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	return result.Result, nil
}
