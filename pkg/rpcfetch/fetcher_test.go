package rpcfetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// mockRPCServer creates a mock RPC server for testing.
func mockRPCServer(t *testing.T, handler func(method string, params []interface{}) (interface{}, error)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			JSONRPC string        `json:"jsonrpc"`
			ID      int           `json:"id"`
			Method  string        `json:"method"`
			Params  []interface{} `json:"params"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		result, err := handler(req.Method, req.Params)

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
		}

		if err != nil {
			resp["error"] = map[string]interface{}{
				"code":    -32000,
				"message": err.Error(),
			}
		} else {
			resp["result"] = result
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestSimplePool(t *testing.T) {
	urls := []string{"http://localhost:8899", "http://localhost:8900"}
	pool := NewSimplePool(urls)

	// Test GetEndpoint returns endpoints
	ctx := context.Background()
	ep1, err := pool.GetEndpoint(ctx)
	if err != nil {
		t.Fatalf("GetEndpoint failed: %v", err)
	}
	if ep1.URL != urls[0] {
		t.Errorf("Expected first endpoint, got %s", ep1.URL)
	}

	ep2, err := pool.GetEndpoint(ctx)
	if err != nil {
		t.Fatalf("GetEndpoint failed: %v", err)
	}
	if ep2.URL != urls[1] {
		t.Errorf("Expected second endpoint, got %s", ep2.URL)
	}

	// Test round-robin
	ep3, err := pool.GetEndpoint(ctx)
	if err != nil {
		t.Fatalf("GetEndpoint failed: %v", err)
	}
	if ep3.URL != urls[0] {
		t.Errorf("Expected first endpoint again, got %s", ep3.URL)
	}

	// Test MarkUnhealthy
	pool.MarkUnhealthy(urls[0], ErrRequestTimeout)
	if pool.GetHealthyCount() != 1 {
		t.Errorf("Expected 1 healthy endpoint, got %d", pool.GetHealthyCount())
	}

	ep4, err := pool.GetEndpoint(ctx)
	if err != nil {
		t.Fatalf("GetEndpoint failed: %v", err)
	}
	if ep4.URL != urls[1] {
		t.Errorf("Expected healthy endpoint, got %s", ep4.URL)
	}

	// Test MarkHealthy
	pool.MarkHealthy(urls[0], 10*time.Millisecond)
	if pool.GetHealthyCount() != 2 {
		t.Errorf("Expected 2 healthy endpoints, got %d", pool.GetHealthyCount())
	}
}

func TestRPCClient_GetSlot(t *testing.T) {
	expectedSlot := uint64(123456789)

	server := mockRPCServer(t, func(method string, params []interface{}) (interface{}, error) {
		if method != "getSlot" {
			t.Errorf("Unexpected method: %s", method)
		}
		return expectedSlot, nil
	})
	defer server.Close()

	pool := NewSimplePool([]string{server.URL})
	client := NewRPCClient(pool, 10*time.Second)

	slot, err := client.GetSlot(context.Background(), "confirmed")
	if err != nil {
		t.Fatalf("GetSlot failed: %v", err)
	}
	if slot != expectedSlot {
		t.Errorf("Expected slot %d, got %d", expectedSlot, slot)
	}
}

func TestRPCClient_GetBlock(t *testing.T) {
	slot := uint64(100)
	blockResp := map[string]interface{}{
		"blockhash":         "4sGjMW1sUnHzSxGspuhpqLDx6wiyjNtZAMdL4VZHirmd",
		"previousBlockhash": "4sGjMW1sUnHzSxGspuhpqLDx6wiyjNtZAMdL4VZHirmd",
		"parentSlot":        float64(99),
		"blockTime":         float64(1234567890),
		"blockHeight":       float64(100),
		"transactions": []interface{}{
			map[string]interface{}{
				"transaction": map[string]interface{}{
					"signatures": []interface{}{
						"5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW",
					},
					"message": map[string]interface{}{
						"accountKeys": []interface{}{
							"11111111111111111111111111111111",
							"Vote111111111111111111111111111111111111111",
						},
						"recentBlockhash": "4sGjMW1sUnHzSxGspuhpqLDx6wiyjNtZAMdL4VZHirmd",
						"instructions": []interface{}{
							map[string]interface{}{
								"programIdIndex": float64(1),
								"accounts":       []interface{}{float64(0)},
								"data":           "3Bxs411Dtc7pkFQj",
							},
						},
					},
				},
				"meta": map[string]interface{}{
					"fee":          float64(5000),
					"preBalances":  []interface{}{float64(1000000000)},
					"postBalances": []interface{}{float64(999995000)},
					"logMessages":  []interface{}{"Program log: Hello"},
				},
			},
		},
		"rewards": []interface{}{},
	}

	server := mockRPCServer(t, func(method string, params []interface{}) (interface{}, error) {
		if method != "getBlock" {
			t.Errorf("Unexpected method: %s", method)
		}
		return blockResp, nil
	})
	defer server.Close()

	pool := NewSimplePool([]string{server.URL})
	client := NewRPCClient(pool, 10*time.Second)

	block, err := client.GetBlock(context.Background(), slot)
	if err != nil {
		t.Fatalf("GetBlock failed: %v", err)
	}

	if block.Slot != slot {
		t.Errorf("Expected slot %d, got %d", slot, block.Slot)
	}
	if block.ParentSlot != 99 {
		t.Errorf("Expected parent slot 99, got %d", block.ParentSlot)
	}
	if len(block.Transactions) != 1 {
		t.Errorf("Expected 1 transaction, got %d", len(block.Transactions))
	}
	if len(block.Transactions[0].Signatures) != 1 {
		t.Errorf("Expected 1 signature, got %d", len(block.Transactions[0].Signatures))
	}
}

func TestRPCClient_GetBlock_SkippedSlot(t *testing.T) {
	server := mockRPCServer(t, func(method string, params []interface{}) (interface{}, error) {
		// Return nil for skipped slot
		return nil, nil
	})
	defer server.Close()

	pool := NewSimplePool([]string{server.URL})
	client := NewRPCClient(pool, 10*time.Second)

	_, err := client.GetBlock(context.Background(), 100)
	if err != ErrSlotSkipped {
		t.Errorf("Expected ErrSlotSkipped, got %v", err)
	}
}

func TestFetcher_Connect(t *testing.T) {
	var slotCalls atomic.Int32
	currentSlot := uint64(100)

	server := mockRPCServer(t, func(method string, params []interface{}) (interface{}, error) {
		switch method {
		case "getSlot":
			slotCalls.Add(1)
			return currentSlot, nil
		case "getBlock":
			slot := uint64(params[0].(float64))
			return map[string]interface{}{
				"blockhash":         "4sGjMW1sUnHzSxGspuhpqLDx6wiyjNtZAMdL4VZHirmd",
				"previousBlockhash": "4sGjMW1sUnHzSxGspuhpqLDx6wiyjNtZAMdL4VZHirmd",
				"parentSlot":        float64(slot - 1),
				"blockTime":         float64(1234567890),
				"blockHeight":       float64(slot),
				"transactions":      []interface{}{},
				"rewards":           []interface{}{},
			}, nil
		}
		return nil, nil
	})
	defer server.Close()

	pool := NewSimplePool([]string{server.URL})
	config := DefaultConfig()
	config.PollInterval = 50 * time.Millisecond

	fetcher, err := NewFetcher(pool, config)
	if err != nil {
		t.Fatalf("NewFetcher failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := fetcher.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Wait for at least one block
	select {
	case block := <-fetcher.Blocks():
		if block.Slot != currentSlot {
			t.Errorf("Expected slot %d, got %d", currentSlot, block.Slot)
		}
	case <-ctx.Done():
		t.Error("Timeout waiting for block")
	}

	// Verify getSlot was called
	if slotCalls.Load() < 1 {
		t.Error("getSlot was not called")
	}

	if err := fetcher.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestFetcher_HandleSkippedSlots(t *testing.T) {
	// Slots 100 and 102 exist, 101 is skipped
	server := mockRPCServer(t, func(method string, params []interface{}) (interface{}, error) {
		switch method {
		case "getSlot":
			return uint64(102), nil
		case "getBlock":
			slot := uint64(params[0].(float64))
			if slot == 101 {
				// Slot 101 is skipped
				return nil, nil
			}
			return map[string]interface{}{
				"blockhash":         "4sGjMW1sUnHzSxGspuhpqLDx6wiyjNtZAMdL4VZHirmd",
				"previousBlockhash": "4sGjMW1sUnHzSxGspuhpqLDx6wiyjNtZAMdL4VZHirmd",
				"parentSlot":        float64(slot - 1),
				"blockTime":         float64(1234567890),
				"blockHeight":       float64(slot),
				"transactions":      []interface{}{},
				"rewards":           []interface{}{},
			}, nil
		}
		return nil, nil
	})
	defer server.Close()

	pool := NewSimplePool([]string{server.URL})
	config := DefaultConfig()
	config.PollInterval = 50 * time.Millisecond
	startSlot := uint64(100)
	config.FromSlot = &startSlot

	fetcher, err := NewFetcher(pool, config)
	if err != nil {
		t.Fatalf("NewFetcher failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := fetcher.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Should receive slots 100 and 102 (101 skipped)
	receivedSlots := make([]uint64, 0)
	for len(receivedSlots) < 2 {
		select {
		case block := <-fetcher.Blocks():
			receivedSlots = append(receivedSlots, block.Slot)
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for blocks, received: %v", receivedSlots)
		}
	}

	if receivedSlots[0] != 100 {
		t.Errorf("Expected first slot 100, got %d", receivedSlots[0])
	}
	if receivedSlots[1] != 102 {
		t.Errorf("Expected second slot 102, got %d", receivedSlots[1])
	}

	if err := fetcher.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestFetcher_Health(t *testing.T) {
	server := mockRPCServer(t, func(method string, params []interface{}) (interface{}, error) {
		return uint64(100), nil
	})
	defer server.Close()

	pool := NewSimplePool([]string{server.URL})
	config := DefaultConfig()

	fetcher, err := NewFetcher(pool, config)
	if err != nil {
		t.Fatalf("NewFetcher failed: %v", err)
	}

	// Before connect
	health := fetcher.Health()
	if health.Connected {
		t.Error("Expected not connected before Connect()")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := fetcher.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// After connect
	health = fetcher.Health()
	if !health.Connected {
		t.Error("Expected connected after Connect()")
	}
	if health.Provider != server.URL {
		t.Errorf("Expected provider %s, got %s", server.URL, health.Provider)
	}

	fetcher.Close()
}

func TestConfig_WithDefaults(t *testing.T) {
	config := Config{}
	config = config.WithDefaults()

	if config.PollInterval != DefaultPollInterval {
		t.Errorf("Expected PollInterval %v, got %v", DefaultPollInterval, config.PollInterval)
	}
	if config.RequestTimeout != DefaultRequestTimeout {
		t.Errorf("Expected RequestTimeout %v, got %v", DefaultRequestTimeout, config.RequestTimeout)
	}
	if config.BlockChannelSize != DefaultBlockChannelSize {
		t.Errorf("Expected BlockChannelSize %d, got %d", DefaultBlockChannelSize, config.BlockChannelSize)
	}
	if config.Commitment != "confirmed" {
		t.Errorf("Expected Commitment 'confirmed', got %s", config.Commitment)
	}
	if config.MaxRetries != DefaultMaxRetries {
		t.Errorf("Expected MaxRetries %d, got %d", DefaultMaxRetries, config.MaxRetries)
	}
}

func TestIsSlotSkipped(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrSlotSkipped", ErrSlotSkipped, true},
		{"RPC error -32009", &RPCError{Code: -32009, Message: "Slot skipped"}, true},
		{"RPC error -32007", &RPCError{Code: -32007, Message: "Slot skipped"}, true},
		{"RPC error -32004", &RPCError{Code: -32004, Message: "Block not available"}, true},
		{"Other RPC error", &RPCError{Code: -32000, Message: "Other error"}, false},
		{"Regular error", ErrRequestTimeout, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSlotSkipped(tt.err); got != tt.expected {
				t.Errorf("IsSlotSkipped(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}
