// Package rpcfetch provides a block fetcher that uses JSON-RPC to fetch blocks
// from Solana validators, serving as an alternative to the Geyser gRPC client.
package rpcfetch

import (
	"context"
	"sync"
	"time"
)

// Endpoint represents an RPC endpoint with health tracking.
type Endpoint struct {
	URL         string
	Healthy     bool
	LastError   error
	LastSuccess time.Time
	Latency     time.Duration
}

// Pool defines the interface for an RPC endpoint pool.
// The actual implementation will be provided separately.
type Pool interface {
	// GetEndpoint returns a healthy endpoint for making requests.
	// Returns an error if no healthy endpoints are available.
	GetEndpoint(ctx context.Context) (*Endpoint, error)

	// MarkUnhealthy marks an endpoint as unhealthy after a failed request.
	MarkUnhealthy(url string, err error)

	// MarkHealthy marks an endpoint as healthy after a successful request.
	MarkHealthy(url string, latency time.Duration)

	// GetHealthyCount returns the number of currently healthy endpoints.
	GetHealthyCount() int

	// Close releases any resources held by the pool.
	Close() error
}

// SimplePool is a basic implementation of Pool for testing and simple use cases.
// For production, a more sophisticated pool with health checking should be used.
type SimplePool struct {
	endpoints []*Endpoint
	mu        sync.RWMutex
	idx       int
}

// NewSimplePool creates a new SimplePool with the given endpoints.
func NewSimplePool(urls []string) *SimplePool {
	endpoints := make([]*Endpoint, len(urls))
	for i, url := range urls {
		endpoints[i] = &Endpoint{
			URL:     url,
			Healthy: true,
		}
	}
	return &SimplePool{
		endpoints: endpoints,
	}
}

// GetEndpoint returns the next available healthy endpoint using round-robin.
func (p *SimplePool) GetEndpoint(ctx context.Context) (*Endpoint, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Try each endpoint once
	for i := 0; i < len(p.endpoints); i++ {
		idx := (p.idx + i) % len(p.endpoints)
		ep := p.endpoints[idx]
		if ep.Healthy {
			p.idx = (idx + 1) % len(p.endpoints)
			return ep, nil
		}
	}

	// No healthy endpoints, return the first one anyway (may recover)
	if len(p.endpoints) > 0 {
		return p.endpoints[0], nil
	}

	return nil, ErrNoEndpoints
}

// MarkUnhealthy marks an endpoint as unhealthy.
func (p *SimplePool) MarkUnhealthy(url string, err error) {
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
func (p *SimplePool) MarkHealthy(url string, latency time.Duration) {
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
func (p *SimplePool) GetHealthyCount() int {
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

// Close is a no-op for SimplePool.
func (p *SimplePool) Close() error {
	return nil
}
