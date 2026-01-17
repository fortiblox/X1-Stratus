package rpcpool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockRPCServer creates a test server that responds to getSlot requests.
func mockRPCServer(slot uint64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  float64(slot),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

// mockRPCServerWithDelay creates a test server that responds after a delay.
func mockRPCServerWithDelay(slot uint64, delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)

		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  float64(slot),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

// mockRPCServerError creates a test server that returns an error.
func mockRPCServerError() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    -32000,
				Message: "Node is unhealthy",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

// mockRPCServerDynamic creates a test server with a mutable slot value.
func mockRPCServerDynamic(slot *atomic.Uint64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  float64(slot.Load()),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestNewPool(t *testing.T) {
	tests := []struct {
		name          string
		referenceURLs []string
		threshold     uint64
		wantRefs      int
		wantThreshold uint64
	}{
		{
			name:          "default values",
			referenceURLs: nil,
			threshold:     0,
			wantRefs:      4, // Default reference endpoints
			wantThreshold: DefaultSlotThreshold,
		},
		{
			name:          "custom values",
			referenceURLs: []string{"https://example.com"},
			threshold:     100,
			wantRefs:      1,
			wantThreshold: 100,
		},
		{
			name:          "empty refs uses defaults",
			referenceURLs: []string{},
			threshold:     50,
			wantRefs:      4,
			wantThreshold: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewPool(tt.referenceURLs, tt.threshold)
			if len(pool.referenceURLs) != tt.wantRefs {
				t.Errorf("got %d reference URLs, want %d", len(pool.referenceURLs), tt.wantRefs)
			}
			if pool.threshold != tt.wantThreshold {
				t.Errorf("got threshold %d, want %d", pool.threshold, tt.wantThreshold)
			}
		})
	}
}

func TestAddEndpoint(t *testing.T) {
	pool := NewPool([]string{"https://ref.example.com"}, 50)

	// Add first endpoint
	pool.AddEndpoint("https://rpc1.example.com")
	if pool.TotalCount() != 1 {
		t.Errorf("got %d endpoints, want 1", pool.TotalCount())
	}

	// Add second endpoint
	pool.AddEndpoint("https://rpc2.example.com")
	if pool.TotalCount() != 2 {
		t.Errorf("got %d endpoints, want 2", pool.TotalCount())
	}

	// Add duplicate - should not increase count
	pool.AddEndpoint("https://rpc1.example.com")
	if pool.TotalCount() != 2 {
		t.Errorf("got %d endpoints after duplicate, want 2", pool.TotalCount())
	}
}

func TestAddEndpoints(t *testing.T) {
	pool := NewPool([]string{"https://ref.example.com"}, 50)

	urls := []string{
		"https://rpc1.example.com",
		"https://rpc2.example.com",
		"https://rpc3.example.com",
	}
	pool.AddEndpoints(urls)

	if pool.TotalCount() != 3 {
		t.Errorf("got %d endpoints, want 3", pool.TotalCount())
	}
}

func TestRemoveEndpoint(t *testing.T) {
	pool := NewPool([]string{"https://ref.example.com"}, 50)
	pool.AddEndpoint("https://rpc1.example.com")
	pool.AddEndpoint("https://rpc2.example.com")

	pool.RemoveEndpoint("https://rpc1.example.com")
	if pool.TotalCount() != 1 {
		t.Errorf("got %d endpoints, want 1", pool.TotalCount())
	}

	// Remove non-existent endpoint - should not error
	pool.RemoveEndpoint("https://nonexistent.example.com")
	if pool.TotalCount() != 1 {
		t.Errorf("got %d endpoints, want 1", pool.TotalCount())
	}
}

func TestGetHealthy(t *testing.T) {
	refServer := mockRPCServer(1000)
	defer refServer.Close()

	pool := NewPool([]string{refServer.URL}, 50)

	// Test with no endpoints
	_, err := pool.GetHealthy()
	if err != ErrNoHealthyEndpoints {
		t.Errorf("expected ErrNoHealthyEndpoints, got %v", err)
	}

	// Add healthy endpoints
	server1 := mockRPCServer(1000)
	defer server1.Close()
	server2 := mockRPCServer(999)
	defer server2.Close()

	pool.AddEndpoint(server1.URL)
	pool.AddEndpoint(server2.URL)

	// Endpoints are healthy by default
	url, err := pool.GetHealthy()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if url != server1.URL && url != server2.URL {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestGetHealthyRoundRobin(t *testing.T) {
	refServer := mockRPCServer(1000)
	defer refServer.Close()

	pool := NewPool([]string{refServer.URL}, 50)

	server1 := mockRPCServer(1000)
	defer server1.Close()
	server2 := mockRPCServer(1000)
	defer server2.Close()

	pool.AddEndpoint(server1.URL)
	pool.AddEndpoint(server2.URL)

	// Get multiple healthy endpoints - should round-robin
	seen := make(map[string]int)
	for i := 0; i < 10; i++ {
		url, err := pool.GetHealthy()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		seen[url]++
	}

	// Both endpoints should be selected roughly equally
	if seen[server1.URL] == 0 || seen[server2.URL] == 0 {
		t.Errorf("round-robin not working: %v", seen)
	}
}

func TestGetReferenceSlot(t *testing.T) {
	// Create reference servers with different slots
	server1 := mockRPCServer(1000)
	defer server1.Close()
	server2 := mockRPCServer(1100)
	defer server2.Close()
	server3 := mockRPCServer(1050)
	defer server3.Close()

	pool := NewPool([]string{server1.URL, server2.URL, server3.URL}, 50)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.ctx = ctx

	slot, err := pool.GetReferenceSlot()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should return the highest slot
	if slot != 1100 {
		t.Errorf("got slot %d, want 1100", slot)
	}
}

func TestHealthCheck(t *testing.T) {
	refServer := mockRPCServer(1000)
	defer refServer.Close()

	pool := NewPool([]string{refServer.URL}, 50)
	pool.SetHealthCheckPeriod(100 * time.Millisecond)

	// Add healthy endpoint
	healthyServer := mockRPCServer(990)
	defer healthyServer.Close()

	// Add unhealthy endpoint (too far behind)
	unhealthyServer := mockRPCServer(900)
	defer unhealthyServer.Close()

	pool.AddEndpoint(healthyServer.URL)
	pool.AddEndpoint(unhealthyServer.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool.Start(ctx)
	defer pool.Stop()

	// Wait for health check to complete
	time.Sleep(200 * time.Millisecond)

	// Check healthy count
	if pool.HealthyCount() != 1 {
		t.Errorf("got %d healthy endpoints, want 1", pool.HealthyCount())
	}

	// Verify the healthy endpoint is reachable
	url, err := pool.GetHealthy()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if url != healthyServer.URL {
		t.Errorf("got URL %s, want %s", url, healthyServer.URL)
	}
}

func TestHealthCheckRecovery(t *testing.T) {
	// Create reference server
	refSlot := &atomic.Uint64{}
	refSlot.Store(1000)
	refServer := mockRPCServerDynamic(refSlot)
	defer refServer.Close()

	pool := NewPool([]string{refServer.URL}, 50)
	pool.SetHealthCheckPeriod(100 * time.Millisecond)

	// Create endpoint that starts unhealthy
	epSlot := &atomic.Uint64{}
	epSlot.Store(900) // 100 slots behind - unhealthy
	epServer := mockRPCServerDynamic(epSlot)
	defer epServer.Close()

	// Track health changes
	var healthChanges []struct {
		healthy bool
		slot    uint64
	}
	var mu sync.Mutex
	pool.SetOnHealthChange(func(url string, healthy bool, slot uint64) {
		mu.Lock()
		healthChanges = append(healthChanges, struct {
			healthy bool
			slot    uint64
		}{healthy, slot})
		mu.Unlock()
	})

	pool.AddEndpoint(epServer.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool.Start(ctx)
	defer pool.Stop()

	// Wait for initial health check
	time.Sleep(200 * time.Millisecond)

	// Endpoint should be unhealthy
	if pool.HealthyCount() != 0 {
		t.Errorf("got %d healthy endpoints, want 0", pool.HealthyCount())
	}

	// Simulate endpoint catching up
	epSlot.Store(995) // Within threshold

	// Wait for next health check
	time.Sleep(200 * time.Millisecond)

	// Endpoint should now be healthy
	if pool.HealthyCount() != 1 {
		t.Errorf("got %d healthy endpoints, want 1", pool.HealthyCount())
	}

	// Verify health change callbacks were called
	mu.Lock()
	if len(healthChanges) < 2 {
		t.Errorf("expected at least 2 health changes, got %d", len(healthChanges))
	}
	mu.Unlock()
}

func TestHealthCheckFailure(t *testing.T) {
	refServer := mockRPCServer(1000)
	defer refServer.Close()

	pool := NewPool([]string{refServer.URL}, 50)
	pool.SetHealthCheckPeriod(50 * time.Millisecond)

	// Add endpoint that returns errors
	errorServer := mockRPCServerError()
	defer errorServer.Close()

	pool.AddEndpoint(errorServer.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool.Start(ctx)
	defer pool.Stop()

	// Wait for multiple health checks (need 3 failures)
	time.Sleep(300 * time.Millisecond)

	// Endpoint should be marked unhealthy after 3 consecutive failures
	if pool.HealthyCount() != 0 {
		t.Errorf("got %d healthy endpoints, want 0", pool.HealthyCount())
	}
}

func TestEndpointStatus(t *testing.T) {
	refServer := mockRPCServer(1000)
	defer refServer.Close()

	pool := NewPool([]string{refServer.URL}, 50)

	server1 := mockRPCServer(990)
	defer server1.Close()
	server2 := mockRPCServer(900)
	defer server2.Close()

	pool.AddEndpoint(server1.URL)
	pool.AddEndpoint(server2.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool.Start(ctx)
	defer pool.Stop()

	// Wait for health check
	time.Sleep(200 * time.Millisecond)

	statuses := pool.EndpointStatus()
	if len(statuses) != 2 {
		t.Errorf("got %d statuses, want 2", len(statuses))
	}

	// Find each endpoint status
	for _, status := range statuses {
		if status.URL == server1.URL {
			if !status.Healthy {
				t.Errorf("server1 should be healthy")
			}
			if status.Slot != 990 {
				t.Errorf("server1 slot = %d, want 990", status.Slot)
			}
		} else if status.URL == server2.URL {
			if status.Healthy {
				t.Errorf("server2 should be unhealthy")
			}
			if status.Slot != 900 {
				t.Errorf("server2 slot = %d, want 900", status.Slot)
			}
		}
	}
}

func TestPoolClosed(t *testing.T) {
	pool := NewPool([]string{"https://ref.example.com"}, 50)
	pool.AddEndpoint("https://rpc.example.com")

	pool.Stop()

	_, err := pool.GetHealthy()
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}

	_, err = pool.GetReferenceSlot()
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	refServer := mockRPCServer(1000)
	defer refServer.Close()

	pool := NewPool([]string{refServer.URL}, 50)

	// Add several endpoints
	servers := make([]*httptest.Server, 5)
	for i := 0; i < 5; i++ {
		servers[i] = mockRPCServer(uint64(990 + i))
		defer servers[i].Close()
		pool.AddEndpoint(servers[i].URL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool.Start(ctx)
	defer pool.Stop()

	// Wait for initial health check
	time.Sleep(200 * time.Millisecond)

	// Concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = pool.GetHealthy()
			_ = pool.HealthyCount()
			_ = pool.TotalCount()
			_ = pool.EndpointStatus()
		}()
	}

	wg.Wait()
}

func TestSetHealthCheckPeriod(t *testing.T) {
	pool := NewPool([]string{"https://ref.example.com"}, 50)
	pool.SetHealthCheckPeriod(5 * time.Second)

	if pool.healthCheckPeriod != 5*time.Second {
		t.Errorf("got health check period %v, want 5s", pool.healthCheckPeriod)
	}
}

func TestSetRequestTimeout(t *testing.T) {
	pool := NewPool([]string{"https://ref.example.com"}, 50)
	pool.SetRequestTimeout(15 * time.Second)

	if pool.requestTimeout != 15*time.Second {
		t.Errorf("got request timeout %v, want 15s", pool.requestTimeout)
	}
}

func TestSlowEndpoint(t *testing.T) {
	refServer := mockRPCServer(1000)
	defer refServer.Close()

	pool := NewPool([]string{refServer.URL}, 50)
	pool.SetRequestTimeout(100 * time.Millisecond)
	pool.SetHealthCheckPeriod(50 * time.Millisecond)

	// Create slow endpoint
	slowServer := mockRPCServerWithDelay(990, 500*time.Millisecond)
	defer slowServer.Close()

	// Create fast endpoint
	fastServer := mockRPCServer(990)
	defer fastServer.Close()

	pool.AddEndpoint(slowServer.URL)
	pool.AddEndpoint(fastServer.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool.Start(ctx)
	defer pool.Stop()

	// Wait for health checks
	time.Sleep(500 * time.Millisecond)

	// Slow endpoint should be unhealthy (timeout), fast should be healthy
	if pool.HealthyCount() != 1 {
		t.Errorf("got %d healthy endpoints, want 1", pool.HealthyCount())
	}
}
