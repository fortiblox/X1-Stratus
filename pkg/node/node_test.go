package node

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DataDir != "./data" {
		t.Errorf("expected DataDir './data', got %q", cfg.DataDir)
	}
	if cfg.Commitment != "confirmed" {
		t.Errorf("expected Commitment 'confirmed', got %q", cfg.Commitment)
	}
	if !cfg.GeyserUseTLS {
		t.Error("expected GeyserUseTLS to be true")
	}
	if !cfg.VerifyBankHash {
		t.Error("expected VerifyBankHash to be true")
	}
	if !cfg.PruneEnabled {
		t.Error("expected PruneEnabled to be true")
	}
	if cfg.MaxSlotLag != 100 {
		t.Errorf("expected MaxSlotLag 100, got %d", cfg.MaxSlotLag)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				DataDir:        "/tmp/test",
				GeyserEndpoint: "grpc.example.com:443",
			},
			wantErr: false,
		},
		{
			name: "missing data dir",
			config: Config{
				DataDir:        "",
				GeyserEndpoint: "grpc.example.com:443",
			},
			wantErr: true,
		},
		{
			name: "missing endpoint",
			config: Config{
				DataDir:        "/tmp/test",
				GeyserEndpoint: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewNode(t *testing.T) {
	// Test with nil config
	_, err := New(nil)
	if err == nil {
		t.Error("expected error with nil config (missing endpoint)")
	}

	// Test with valid config
	cfg := &Config{
		DataDir:        "/tmp/test-node",
		GeyserEndpoint: "grpc.example.com:443",
		GeyserToken:    "test-token",
	}

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node == nil {
		t.Fatal("expected non-nil node")
	}

	// Verify defaults were applied
	if node.config.Commitment != "confirmed" {
		t.Errorf("expected commitment 'confirmed', got %q", node.config.Commitment)
	}
}

func TestParseCommitment(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"processed", 0},
		{"confirmed", 1},
		{"finalized", 2},
		{"invalid", 1}, // defaults to confirmed
		{"", 1},        // defaults to confirmed
	}

	for _, tt := range tests {
		result := parseCommitment(tt.input)
		if int(result) != tt.expected {
			t.Errorf("parseCommitment(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestStatus(t *testing.T) {
	cfg := &Config{
		DataDir:        "/tmp/test-node-status",
		GeyserEndpoint: "grpc.example.com:443",
	}

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Get status before starting
	status := node.Status()

	if status.IsRunning {
		t.Error("expected IsRunning to be false before Start")
	}
	if status.CurrentSlot != 0 {
		t.Errorf("expected CurrentSlot 0, got %d", status.CurrentSlot)
	}
	if status.IsSyncing {
		t.Error("expected IsSyncing to be false before Start")
	}
}

func TestNodeNotRunningErrors(t *testing.T) {
	cfg := &Config{
		DataDir:        "/tmp/test-node-errors",
		GeyserEndpoint: "grpc.example.com:443",
	}

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test Stop before Start
	err = node.Stop()
	if err != ErrNotRunning {
		t.Errorf("expected ErrNotRunning, got %v", err)
	}
}

func TestClientHealth(t *testing.T) {
	h := ClientHealth{
		Connected:      true,
		LastSlot:       12345,
		LastUpdate:     time.Now(),
		Latency:        100 * time.Millisecond,
		ReconnectCount: 2,
	}

	if !h.Connected {
		t.Error("expected Connected to be true")
	}
	if h.LastSlot != 12345 {
		t.Errorf("expected LastSlot 12345, got %d", h.LastSlot)
	}
	if h.ReconnectCount != 2 {
		t.Errorf("expected ReconnectCount 2, got %d", h.ReconnectCount)
	}
}
