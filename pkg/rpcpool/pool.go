// Package rpcpool provides an RPC endpoint pool manager with health checking.
//
// The pool maintains a set of RPC endpoints and periodically health-checks
// each endpoint by comparing its slot to reference endpoints. Endpoints that
// fall behind the reference slot by more than a configurable threshold are
// marked unhealthy and excluded from the pool until they recover.
//
// Usage:
//
//	pool := rpcpool.NewPool([]string{"https://rpc.mainnet.x1.xyz"}, 50)
//	pool.AddEndpoint("https://my-rpc-1.example.com")
//	pool.AddEndpoint("https://my-rpc-2.example.com")
//	pool.Start(ctx)
//
//	endpoint, err := pool.GetHealthy()
//	if err != nil {
//	    // No healthy endpoints available
//	}
package rpcpool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Pool errors.
var (
	ErrNoHealthyEndpoints = errors.New("no healthy endpoints available")
	ErrPoolClosed         = errors.New("pool is closed")
	ErrEmptyReferenceURLs = errors.New("reference URLs cannot be empty")
)

// Default configuration values.
const (
	DefaultSlotThreshold     = uint64(50)
	DefaultHealthCheckPeriod = 30 * time.Second
	DefaultRequestTimeout    = 10 * time.Second
)

// endpointState represents the health state of an endpoint.
type endpointState struct {
	url       string
	healthy   atomic.Bool
	lastSlot  atomic.Uint64
	lastCheck atomic.Int64 // Unix nano timestamp
	failCount atomic.Int32
}

// Pool manages a pool of RPC endpoints with health checking.
//
// It maintains a list of RPC endpoints and periodically checks their health
// by comparing their slot to reference endpoints. Endpoints that are more
// than the configured threshold slots behind are marked unhealthy.
type Pool struct {
	// Configuration
	referenceURLs []string
	threshold     uint64

	// Endpoint management
	endpoints []*endpointState
	mu        sync.RWMutex

	// Round-robin selection
	nextIndex atomic.Uint64

	// Reference slot cache
	referenceSlot atomic.Uint64

	// Health check configuration
	healthCheckPeriod time.Duration
	requestTimeout    time.Duration

	// HTTP client for RPC requests
	client *http.Client

	// Lifecycle management
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started atomic.Bool
	closed  atomic.Bool

	// Callbacks
	onHealthChange func(url string, healthy bool, slot uint64)
}

// NewPool creates a new RPC endpoint pool.
//
// Parameters:
//   - referenceURLs: List of trusted RPC endpoints to use as slot truth sources.
//     The default reference endpoints are rpc.mainnet.x1.xyz and
//     entrypoint0/1/2.mainnet.x1.xyz.
//   - threshold: Maximum number of slots an endpoint can be behind before
//     being marked unhealthy. Default is 50.
//
// The pool is not started until Start() is called.
func NewPool(referenceURLs []string, threshold uint64) *Pool {
	if len(referenceURLs) == 0 {
		// Use default reference endpoints
		referenceURLs = []string{
			"https://rpc.mainnet.x1.xyz",
			"https://entrypoint0.mainnet.x1.xyz",
			"https://entrypoint1.mainnet.x1.xyz",
			"https://entrypoint2.mainnet.x1.xyz",
		}
	}

	if threshold == 0 {
		threshold = DefaultSlotThreshold
	}

	return &Pool{
		referenceURLs:     referenceURLs,
		threshold:         threshold,
		endpoints:         make([]*endpointState, 0),
		healthCheckPeriod: DefaultHealthCheckPeriod,
		requestTimeout:    DefaultRequestTimeout,
		client: &http.Client{
			Timeout: DefaultRequestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// SetHealthCheckPeriod sets the interval between health checks.
// Must be called before Start().
func (p *Pool) SetHealthCheckPeriod(period time.Duration) {
	p.healthCheckPeriod = period
}

// SetRequestTimeout sets the timeout for RPC requests.
// Must be called before Start().
func (p *Pool) SetRequestTimeout(timeout time.Duration) {
	p.requestTimeout = timeout
	p.client.Timeout = timeout
}

// SetOnHealthChange sets a callback that is invoked when an endpoint's
// health status changes.
func (p *Pool) SetOnHealthChange(callback func(url string, healthy bool, slot uint64)) {
	p.onHealthChange = callback
}

// AddEndpoint adds a single RPC endpoint to the pool.
// The endpoint is initially marked as healthy and will be checked
// on the next health check cycle.
func (p *Pool) AddEndpoint(url string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if endpoint already exists
	for _, ep := range p.endpoints {
		if ep.url == url {
			return
		}
	}

	ep := &endpointState{
		url: url,
	}
	ep.healthy.Store(true) // Assume healthy until proven otherwise
	p.endpoints = append(p.endpoints, ep)
}

// AddEndpoints adds multiple RPC endpoints to the pool.
func (p *Pool) AddEndpoints(urls []string) {
	for _, url := range urls {
		p.AddEndpoint(url)
	}
}

// RemoveEndpoint removes an endpoint from the pool.
func (p *Pool) RemoveEndpoint(url string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, ep := range p.endpoints {
		if ep.url == url {
			p.endpoints = append(p.endpoints[:i], p.endpoints[i+1:]...)
			return
		}
	}
}

// GetHealthy returns a healthy endpoint URL using round-robin selection.
// Returns ErrNoHealthyEndpoints if no healthy endpoints are available.
func (p *Pool) GetHealthy() (string, error) {
	if p.closed.Load() {
		return "", ErrPoolClosed
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.endpoints) == 0 {
		return "", ErrNoHealthyEndpoints
	}

	// Collect healthy endpoints
	var healthy []*endpointState
	for _, ep := range p.endpoints {
		if ep.healthy.Load() {
			healthy = append(healthy, ep)
		}
	}

	if len(healthy) == 0 {
		return "", ErrNoHealthyEndpoints
	}

	// Round-robin selection among healthy endpoints
	idx := p.nextIndex.Add(1) % uint64(len(healthy))
	return healthy[idx].url, nil
}

// GetHealthyRandom returns a random healthy endpoint URL.
// Returns ErrNoHealthyEndpoints if no healthy endpoints are available.
func (p *Pool) GetHealthyRandom() (string, error) {
	if p.closed.Load() {
		return "", ErrPoolClosed
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.endpoints) == 0 {
		return "", ErrNoHealthyEndpoints
	}

	// Collect healthy endpoints
	var healthy []*endpointState
	for _, ep := range p.endpoints {
		if ep.healthy.Load() {
			healthy = append(healthy, ep)
		}
	}

	if len(healthy) == 0 {
		return "", ErrNoHealthyEndpoints
	}

	// Random selection
	idx := rand.Intn(len(healthy))
	return healthy[idx].url, nil
}

// GetReferenceSlot returns the current reference slot from trusted endpoints.
// This queries the reference endpoints and returns the highest slot found.
func (p *Pool) GetReferenceSlot() (uint64, error) {
	if p.closed.Load() {
		return 0, ErrPoolClosed
	}

	slot, err := p.fetchReferenceSlot()
	if err != nil {
		return 0, err
	}

	p.referenceSlot.Store(slot)
	return slot, nil
}

// HealthyCount returns the number of currently healthy endpoints.
func (p *Pool) HealthyCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, ep := range p.endpoints {
		if ep.healthy.Load() {
			count++
		}
	}
	return count
}

// TotalCount returns the total number of endpoints in the pool.
func (p *Pool) TotalCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.endpoints)
}

// Start begins the health check loop.
// This method spawns a goroutine that periodically checks all endpoints
// and blocks until the context is cancelled.
func (p *Pool) Start(ctx context.Context) {
	if p.started.Swap(true) {
		return // Already started
	}

	p.ctx, p.cancel = context.WithCancel(ctx)

	// Perform initial health check
	p.performHealthCheck()

	// Start the health check loop
	p.wg.Add(1)
	go p.healthCheckLoop()
}

// Stop stops the health check loop and releases resources.
func (p *Pool) Stop() {
	if p.closed.Swap(true) {
		return // Already closed
	}

	if p.cancel != nil {
		p.cancel()
	}

	p.wg.Wait()
}

// healthCheckLoop periodically performs health checks on all endpoints.
func (p *Pool) healthCheckLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.healthCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.performHealthCheck()
		}
	}
}

// performHealthCheck checks the health of all endpoints.
func (p *Pool) performHealthCheck() {
	// First, get the reference slot
	refSlot, err := p.fetchReferenceSlot()
	if err != nil {
		// If we can't get reference slot, keep existing health states
		return
	}
	p.referenceSlot.Store(refSlot)

	// Get snapshot of endpoints
	p.mu.RLock()
	endpoints := make([]*endpointState, len(p.endpoints))
	copy(endpoints, p.endpoints)
	p.mu.RUnlock()

	// Check each endpoint concurrently
	var wg sync.WaitGroup
	for _, ep := range endpoints {
		wg.Add(1)
		go func(ep *endpointState) {
			defer wg.Done()
			p.checkEndpoint(ep, refSlot)
		}(ep)
	}
	wg.Wait()
}

// checkEndpoint checks the health of a single endpoint.
func (p *Pool) checkEndpoint(ep *endpointState, refSlot uint64) {
	slot, err := p.fetchSlot(ep.url)
	now := time.Now().UnixNano()
	ep.lastCheck.Store(now)

	if err != nil {
		// Failed to get slot
		failCount := ep.failCount.Add(1)
		if failCount >= 3 {
			// Mark unhealthy after 3 consecutive failures
			wasHealthy := ep.healthy.Swap(false)
			if wasHealthy && p.onHealthChange != nil {
				p.onHealthChange(ep.url, false, ep.lastSlot.Load())
			}
		}
		return
	}

	// Reset fail count on success
	ep.failCount.Store(0)
	ep.lastSlot.Store(slot)

	// Check if slot is within threshold
	var behind uint64
	if refSlot > slot {
		behind = refSlot - slot
	}

	healthy := behind <= p.threshold
	wasHealthy := ep.healthy.Swap(healthy)

	// Notify on health change
	if wasHealthy != healthy && p.onHealthChange != nil {
		p.onHealthChange(ep.url, healthy, slot)
	}
}

// fetchReferenceSlot fetches the current slot from reference endpoints.
// Returns the highest slot found among all reference endpoints.
func (p *Pool) fetchReferenceSlot() (uint64, error) {
	type result struct {
		slot uint64
		err  error
	}

	results := make(chan result, len(p.referenceURLs))
	ctx, cancel := context.WithTimeout(p.ctx, p.requestTimeout)
	defer cancel()

	// Query all reference endpoints concurrently
	for _, url := range p.referenceURLs {
		go func(url string) {
			slot, err := p.fetchSlotWithContext(ctx, url)
			results <- result{slot: slot, err: err}
		}(url)
	}

	// Collect results and find highest slot
	var maxSlot uint64
	var successCount int
	var lastErr error

	for i := 0; i < len(p.referenceURLs); i++ {
		select {
		case r := <-results:
			if r.err != nil {
				lastErr = r.err
				continue
			}
			successCount++
			if r.slot > maxSlot {
				maxSlot = r.slot
			}
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	if successCount == 0 {
		if lastErr != nil {
			return 0, fmt.Errorf("failed to fetch reference slot: %w", lastErr)
		}
		return 0, errors.New("failed to fetch reference slot from any endpoint")
	}

	return maxSlot, nil
}

// fetchSlot fetches the current slot from an RPC endpoint.
func (p *Pool) fetchSlot(url string) (uint64, error) {
	ctx, cancel := context.WithTimeout(p.ctx, p.requestTimeout)
	defer cancel()
	return p.fetchSlotWithContext(ctx, url)
}

// fetchSlotWithContext fetches the current slot from an RPC endpoint with context.
func (p *Pool) fetchSlotWithContext(ctx context.Context, url string) (uint64, error) {
	// Prepare JSON-RPC request
	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "getSlot",
		Params:  []interface{}{},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read response with size limit
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return 0, fmt.Errorf("unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return 0, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
	}

	// Parse slot from result
	slot, ok := rpcResp.Result.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected result type: %T", rpcResp.Result)
	}

	return uint64(slot), nil
}

// EndpointStatus returns the status of all endpoints in the pool.
func (p *Pool) EndpointStatus() []EndpointInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	infos := make([]EndpointInfo, len(p.endpoints))
	for i, ep := range p.endpoints {
		infos[i] = EndpointInfo{
			URL:       ep.url,
			Healthy:   ep.healthy.Load(),
			Slot:      ep.lastSlot.Load(),
			LastCheck: time.Unix(0, ep.lastCheck.Load()),
			FailCount: int(ep.failCount.Load()),
		}
	}
	return infos
}

// EndpointInfo contains status information about an endpoint.
type EndpointInfo struct {
	URL       string
	Healthy   bool
	Slot      uint64
	LastCheck time.Time
	FailCount int
}

// jsonRPCRequest represents a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params,omitempty"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

// jsonRPCError represents a JSON-RPC 2.0 error.
type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
