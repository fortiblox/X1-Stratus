package gossip

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if len(cfg.Entrypoints) != 3 {
		t.Errorf("expected 3 default entrypoints, got %d", len(cfg.Entrypoints))
	}

	if cfg.Timeout != DefaultTimeout {
		t.Errorf("expected timeout %v, got %v", DefaultTimeout, cfg.Timeout)
	}

	if cfg.PullInterval != DefaultPullInterval {
		t.Errorf("expected pull interval %v, got %v", DefaultPullInterval, cfg.PullInterval)
	}

	if cfg.MaxNodes != DefaultMaxNodes {
		t.Errorf("expected max nodes %d, got %d", DefaultMaxNodes, cfg.MaxNodes)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "empty config without RPC only",
			config:  Config{},
			wantErr: true,
		},
		{
			name: "valid config with entrypoints",
			config: Config{
				Entrypoints:  []string{"localhost:8001"},
				Timeout:      10 * time.Second,
				PullInterval: 5 * time.Second,
				BufferSize:   65535,
				RPCTimeout:   10 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "RPC only without endpoint",
			config: Config{
				UseRPCOnly:   true,
				Timeout:      10 * time.Second,
				PullInterval: 5 * time.Second,
				BufferSize:   65535,
			},
			wantErr: true,
		},
		{
			name: "valid RPC only config",
			config: Config{
				UseRPCOnly:   true,
				RPCEndpoint:  "http://localhost:8899",
				Timeout:      10 * time.Second,
				PullInterval: 5 * time.Second,
				BufferSize:   65535,
				RPCTimeout:   10 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "invalid timeout",
			config: Config{
				Entrypoints:  []string{"localhost:8001"},
				Timeout:      0,
				PullInterval: 5 * time.Second,
				BufferSize:   65535,
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

func TestConfigBuilder(t *testing.T) {
	cfg, err := NewConfigBuilder().
		Entrypoints("entrypoint1:8001", "entrypoint2:8001").
		Timeout(60 * time.Second).
		MaxNodes(500).
		ShredVersion(100).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(cfg.Entrypoints) != 2 {
		t.Errorf("expected 2 entrypoints, got %d", len(cfg.Entrypoints))
	}

	if cfg.Timeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", cfg.Timeout)
	}

	if cfg.MaxNodes != 500 {
		t.Errorf("expected max nodes 500, got %d", cfg.MaxNodes)
	}

	if cfg.ShredVersion != 100 {
		t.Errorf("expected shred version 100, got %d", cfg.ShredVersion)
	}
}

func TestNewClient(t *testing.T) {
	cfg := DefaultConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if client.codec == nil {
		t.Error("expected codec to be initialized")
	}

	if client.nodes == nil {
		t.Error("expected nodes map to be initialized")
	}

	// Check that ephemeral keypair was generated
	if client.publicKey.IsZero() {
		t.Error("expected public key to be generated")
	}

	if client.privateKey == nil {
		t.Error("expected private key to be generated")
	}
}

func TestNodeHasRPC(t *testing.T) {
	tests := []struct {
		name   string
		node   Node
		hasRPC bool
	}{
		{
			name:   "node without RPC",
			node:   Node{},
			hasRPC: false,
		},
		{
			name: "node with RPC",
			node: Node{
				RPCAddr: "127.0.0.1:8899",
			},
			hasRPC: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.HasRPC(); got != tt.hasRPC {
				t.Errorf("HasRPC() = %v, want %v", got, tt.hasRPC)
			}
		})
	}
}

func TestNodeRPCURL(t *testing.T) {
	tests := []struct {
		name     string
		rpcAddr  string
		expected string
	}{
		{
			name:     "empty address",
			rpcAddr:  "",
			expected: "",
		},
		{
			name:     "valid address",
			rpcAddr:  "127.0.0.1:8899",
			expected: "http://127.0.0.1:8899",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := Node{RPCAddr: tt.rpcAddr}
			if got := node.RPCURL(); got != tt.expected {
				t.Errorf("RPCURL() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSocketAddrString(t *testing.T) {
	tests := []struct {
		name     string
		addr     SocketAddr
		expected string
	}{
		{
			name:     "empty address",
			addr:     SocketAddr{},
			expected: "",
		},
		{
			name: "IPv4 address",
			addr: SocketAddr{
				IP:   net.IPv4(192, 168, 1, 1),
				Port: 8001,
			},
			expected: "192.168.1.1:8001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	v := Version{
		Major: 1,
		Minor: 18,
		Patch: 23,
	}

	expected := "1.18.23"
	if got := v.String(); got != expected {
		t.Errorf("Version.String() = %v, want %v", got, expected)
	}
}

func TestDiscoveryResultRPCNodes(t *testing.T) {
	result := &DiscoveryResult{
		Nodes: []*Node{
			{RPCAddr: ""},
			{RPCAddr: "127.0.0.1:8899"},
			{RPCAddr: ""},
			{RPCAddr: "127.0.0.2:8899"},
		},
	}

	rpcNodes := result.RPCNodes()
	if len(rpcNodes) != 2 {
		t.Errorf("expected 2 RPC nodes, got %d", len(rpcNodes))
	}

	if result.NodeCount() != 4 {
		t.Errorf("expected total nodes 4, got %d", result.NodeCount())
	}

	if result.RPCNodeCount() != 2 {
		t.Errorf("expected RPC node count 2, got %d", result.RPCNodeCount())
	}
}

func TestGetClusterNodes(t *testing.T) {
	// Create a mock RPC server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json")
		}

		// Return mock response
		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": []map[string]interface{}{
				{
					"pubkey":       "4Nd1mBQtrMJVYVfKf2PJy9NZUZdTAsp7D4xWLs4gDB4T",
					"gossip":       "192.168.1.1:8001",
					"tpu":          "192.168.1.1:8003",
					"rpc":          "192.168.1.1:8899",
					"version":      "1.18.23",
					"featureSet":   uint32(123456),
					"shredVersion": uint16(50000),
				},
				{
					"pubkey": "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d",
					"gossip": "192.168.1.2:8001",
					// No RPC
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nodes, err := GetClusterNodes(ctx, server.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("GetClusterNodes() error = %v", err)
	}

	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}

	// Check first node
	if nodes[0].RPCAddr != "192.168.1.1:8899" {
		t.Errorf("expected RPC addr 192.168.1.1:8899, got %s", nodes[0].RPCAddr)
	}

	if nodes[0].Version != "1.18.23" {
		t.Errorf("expected version 1.18.23, got %s", nodes[0].Version)
	}

	// Check second node doesn't have RPC
	if nodes[1].RPCAddr != "" {
		t.Errorf("expected empty RPC addr for second node, got %s", nodes[1].RPCAddr)
	}
}

func TestGetClusterNodesError(t *testing.T) {
	// Create a mock RPC server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"error": map[string]interface{}{
				"code":    -32600,
				"message": "Invalid request",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := GetClusterNodes(ctx, server.URL, 5*time.Second)
	if err == nil {
		t.Error("expected error from RPC, got nil")
	}
}

func TestRPCDiscoverer(t *testing.T) {
	// Create a mock RPC server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": []map[string]interface{}{
				{
					"pubkey": "4Nd1mBQtrMJVYVfKf2PJy9NZUZdTAsp7D4xWLs4gDB4T",
					"rpc":    "192.168.1.1:8899",
				},
				{
					"pubkey": "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d",
					"rpc":    "192.168.1.2:8899",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	discoverer := NewRPCDiscoverer(server.URL).WithTimeout(5 * time.Second)

	// Test Discover
	nodes, err := discoverer.Discover(ctx)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}

	// Test DiscoverRPC
	rpcNodes, err := discoverer.DiscoverRPC(ctx)
	if err != nil {
		t.Fatalf("DiscoverRPC() error = %v", err)
	}

	if len(rpcNodes) != 2 {
		t.Errorf("expected 2 RPC nodes, got %d", len(rpcNodes))
	}

	// Test GetRPCEndpoints
	endpoints, err := discoverer.GetRPCEndpoints(ctx)
	if err != nil {
		t.Fatalf("GetRPCEndpoints() error = %v", err)
	}

	if len(endpoints) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(endpoints))
	}

	expectedEndpoints := []string{"http://192.168.1.1:8899", "http://192.168.1.2:8899"}
	for i, ep := range endpoints {
		if ep != expectedEndpoints[i] {
			t.Errorf("endpoint %d: expected %s, got %s", i, expectedEndpoints[i], ep)
		}
	}
}

func TestContactInfoToNode(t *testing.T) {
	pubkey, _ := types.PubkeyFromBase58("4Nd1mBQtrMJVYVfKf2PJy9NZUZdTAsp7D4xWLs4gDB4T")
	wallClock := uint64(time.Now().UnixMilli())

	info := &ContactInfo{
		Pubkey:       pubkey,
		WallClock:    wallClock,
		ShredVersion: 50000,
		Version: Version{
			Major:      1,
			Minor:      18,
			Patch:      23,
			FeatureSet: 123456,
		},
		Gossip: SocketAddr{
			IP:   net.IPv4(192, 168, 1, 1),
			Port: 8001,
		},
		RPC: SocketAddr{
			IP:   net.IPv4(192, 168, 1, 1),
			Port: 8899,
		},
		TPU: SocketAddr{
			IP:   net.IPv4(192, 168, 1, 1),
			Port: 8003,
		},
	}

	node := ContactInfoToNode(info)

	if node.Identity != pubkey {
		t.Error("identity mismatch")
	}

	if node.ShredVersion != 50000 {
		t.Errorf("expected shred version 50000, got %d", node.ShredVersion)
	}

	if node.Version != "1.18.23" {
		t.Errorf("expected version 1.18.23, got %s", node.Version)
	}

	if node.RPCAddr != "192.168.1.1:8899" {
		t.Errorf("expected RPC addr 192.168.1.1:8899, got %s", node.RPCAddr)
	}

	if node.GossipAddr == nil || node.GossipAddr.Port != 8001 {
		t.Error("gossip addr not set correctly")
	}

	if node.TPUAddr == nil || node.TPUAddr.Port != 8003 {
		t.Error("TPU addr not set correctly")
	}
}

func TestCodecEncodeDecode(t *testing.T) {
	codec := NewCodec()

	// Test ping message encoding/decoding
	pubkey, _ := types.PubkeyFromBase58("4Nd1mBQtrMJVYVfKf2PJy9NZUZdTAsp7D4xWLs4gDB4T")
	token := types.ComputeHash([]byte("test token"))

	ping := &PingMessage{
		From:      pubkey,
		Token:     token,
		Signature: types.Signature{},
	}

	encoded, err := codec.EncodePingMessage(ping)
	if err != nil {
		t.Fatalf("EncodePingMessage() error = %v", err)
	}

	msgType, decoded, err := codec.DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if msgType != MessageTypePingMessage {
		t.Errorf("expected message type %d, got %d", MessageTypePingMessage, msgType)
	}

	decodedPing, ok := decoded.(*PingMessage)
	if !ok {
		t.Fatalf("expected *PingMessage, got %T", decoded)
	}

	if decodedPing.From != pubkey {
		t.Error("pubkey mismatch")
	}

	if decodedPing.Token != token {
		t.Error("token mismatch")
	}
}

func TestGossipMessageTypeString(t *testing.T) {
	tests := []struct {
		msgType  GossipMessageType
		expected string
	}{
		{MessageTypePullRequest, "PullRequest"},
		{MessageTypePullResponse, "PullResponse"},
		{MessageTypePushMessage, "PushMessage"},
		{MessageTypePruneMessage, "PruneMessage"},
		{MessageTypePingMessage, "PingMessage"},
		{MessageTypePongMessage, "PongMessage"},
		{GossipMessageType(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.msgType.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClientClose(t *testing.T) {
	cfg := DefaultConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// Close should not error
	if err := client.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Second close should be no-op
	if err := client.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}

	// Discover should fail after close
	ctx := context.Background()
	_, err = client.Discover(ctx)
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func TestNetworkPresets(t *testing.T) {
	// X1 Mainnet
	x1Main := X1MainnetConfig()
	if len(x1Main.Entrypoints) != 3 {
		t.Errorf("X1 mainnet: expected 3 entrypoints, got %d", len(x1Main.Entrypoints))
	}

	// X1 Testnet
	x1Test := X1TestnetConfig()
	if len(x1Test.Entrypoints) != 3 {
		t.Errorf("X1 testnet: expected 3 entrypoints, got %d", len(x1Test.Entrypoints))
	}

	// X1 Devnet
	x1Dev := X1DevnetConfig()
	if len(x1Dev.Entrypoints) != 2 {
		t.Errorf("X1 devnet: expected 2 entrypoints, got %d", len(x1Dev.Entrypoints))
	}

	// Solana Mainnet
	solMain := SolanaMainnetConfig()
	if len(solMain.Entrypoints) != 5 {
		t.Errorf("Solana mainnet: expected 5 entrypoints, got %d", len(solMain.Entrypoints))
	}
}
