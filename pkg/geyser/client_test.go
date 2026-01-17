package geyser

import (
	"os"
	"testing"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// =============================================================================
// TestNewClient - Test client creation with valid/invalid configs
// =============================================================================

func TestNewClient_ValidConfig(t *testing.T) {
	config := Config{
		Endpoint:            "grpc.example.com:443",
		UseTLS:              true,
		Commitment:          CommitmentConfirmed,
		IncludeTransactions: true,
		IncludeEntries:      true,
		BlockChannelSize:    100,
		SlotChannelSize:     500,
		MaxMessageSize:      1024 * 1024,
		KeepaliveTime:       10 * time.Second,
		KeepaliveTimeout:    5 * time.Second,
		ReconnectMinDelay:   1 * time.Second,
		ReconnectMaxDelay:   60 * time.Second,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}

	if client == nil {
		t.Fatal("NewClient() returned nil client")
	}

	// Check that channels are created
	if client.Blocks() == nil {
		t.Error("client.Blocks() returned nil")
	}
	if client.Slots() == nil {
		t.Error("client.Slots() returned nil")
	}
}

func TestNewClient_WithDefaults(t *testing.T) {
	// Minimal config - should get defaults applied
	config := Config{
		Endpoint: "grpc.example.com:443",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}

	// Check defaults were applied by examining the client's config
	health := client.Health()
	if health.Provider != "grpc.example.com:443" {
		t.Errorf("Health().Provider = %v, want %v", health.Provider, "grpc.example.com:443")
	}
}

func TestNewClient_EmptyEndpoint(t *testing.T) {
	config := Config{
		Endpoint: "", // Empty - should fail
	}

	_, err := NewClient(config)
	if err == nil {
		t.Fatal("NewClient() with empty endpoint should return error")
	}
}

func TestNewClient_InvalidBlockChannelSize(t *testing.T) {
	config := Config{
		Endpoint:         "grpc.example.com:443",
		BlockChannelSize: -1, // Invalid
	}

	_, err := NewClient(config)
	if err == nil {
		t.Fatal("NewClient() with negative BlockChannelSize should return error")
	}
}

func TestNewClient_InvalidSlotChannelSize(t *testing.T) {
	config := Config{
		Endpoint:         "grpc.example.com:443",
		BlockChannelSize: 100, // Needs to be valid since checked first with defaults
		SlotChannelSize:  -1,  // Invalid
	}

	_, err := NewClient(config)
	if err == nil {
		t.Fatal("NewClient() with negative SlotChannelSize should return error")
	}
}

// =============================================================================
// TestConfigValidation - Test configuration validation
// =============================================================================

func TestConfigValidation_EmptyEndpoint(t *testing.T) {
	config := Config{}
	err := config.Validate()
	if err != ErrNoEndpoint {
		t.Errorf("Validate() error = %v, want %v", err, ErrNoEndpoint)
	}
}

func TestConfigValidation_InvalidBlockChannelSize(t *testing.T) {
	config := Config{
		Endpoint:         "grpc.example.com:443",
		BlockChannelSize: 0,
		SlotChannelSize:  100,
		MaxMessageSize:   1024,
		KeepaliveTime:    time.Second,
		KeepaliveTimeout: time.Second,
		ReconnectMinDelay: time.Second,
		ReconnectMaxDelay: time.Minute,
	}
	err := config.Validate()
	if err == nil {
		t.Error("Validate() should fail with zero BlockChannelSize")
	}
}

func TestConfigValidation_InvalidSlotChannelSize(t *testing.T) {
	config := Config{
		Endpoint:         "grpc.example.com:443",
		BlockChannelSize: 100,
		SlotChannelSize:  0,
		MaxMessageSize:   1024,
		KeepaliveTime:    time.Second,
		KeepaliveTimeout: time.Second,
		ReconnectMinDelay: time.Second,
		ReconnectMaxDelay: time.Minute,
	}
	err := config.Validate()
	if err == nil {
		t.Error("Validate() should fail with zero SlotChannelSize")
	}
}

func TestConfigValidation_InvalidMaxMessageSize(t *testing.T) {
	config := Config{
		Endpoint:         "grpc.example.com:443",
		BlockChannelSize: 100,
		SlotChannelSize:  100,
		MaxMessageSize:   0,
		KeepaliveTime:    time.Second,
		KeepaliveTimeout: time.Second,
		ReconnectMinDelay: time.Second,
		ReconnectMaxDelay: time.Minute,
	}
	err := config.Validate()
	if err == nil {
		t.Error("Validate() should fail with zero MaxMessageSize")
	}
}

func TestConfigValidation_InvalidKeepaliveTime(t *testing.T) {
	config := Config{
		Endpoint:         "grpc.example.com:443",
		BlockChannelSize: 100,
		SlotChannelSize:  100,
		MaxMessageSize:   1024,
		KeepaliveTime:    0,
		KeepaliveTimeout: time.Second,
		ReconnectMinDelay: time.Second,
		ReconnectMaxDelay: time.Minute,
	}
	err := config.Validate()
	if err == nil {
		t.Error("Validate() should fail with zero KeepaliveTime")
	}
}

func TestConfigValidation_InvalidKeepaliveTimeout(t *testing.T) {
	config := Config{
		Endpoint:         "grpc.example.com:443",
		BlockChannelSize: 100,
		SlotChannelSize:  100,
		MaxMessageSize:   1024,
		KeepaliveTime:    time.Second,
		KeepaliveTimeout: 0,
		ReconnectMinDelay: time.Second,
		ReconnectMaxDelay: time.Minute,
	}
	err := config.Validate()
	if err == nil {
		t.Error("Validate() should fail with zero KeepaliveTimeout")
	}
}

func TestConfigValidation_InvalidReconnectMinDelay(t *testing.T) {
	config := Config{
		Endpoint:         "grpc.example.com:443",
		BlockChannelSize: 100,
		SlotChannelSize:  100,
		MaxMessageSize:   1024,
		KeepaliveTime:    time.Second,
		KeepaliveTimeout: time.Second,
		ReconnectMinDelay: 0,
		ReconnectMaxDelay: time.Minute,
	}
	err := config.Validate()
	if err == nil {
		t.Error("Validate() should fail with zero ReconnectMinDelay")
	}
}

func TestConfigValidation_InvalidReconnectMaxDelay(t *testing.T) {
	config := Config{
		Endpoint:         "grpc.example.com:443",
		BlockChannelSize: 100,
		SlotChannelSize:  100,
		MaxMessageSize:   1024,
		KeepaliveTime:    time.Second,
		KeepaliveTimeout: time.Second,
		ReconnectMinDelay: time.Minute,   // Min > Max
		ReconnectMaxDelay: time.Second,
	}
	err := config.Validate()
	if err == nil {
		t.Error("Validate() should fail when ReconnectMinDelay > ReconnectMaxDelay")
	}
}

func TestConfigValidation_InvalidCommitment(t *testing.T) {
	config := Config{
		Endpoint:         "grpc.example.com:443",
		BlockChannelSize: 100,
		SlotChannelSize:  100,
		MaxMessageSize:   1024,
		KeepaliveTime:    time.Second,
		KeepaliveTimeout: time.Second,
		ReconnectMinDelay: time.Second,
		ReconnectMaxDelay: time.Minute,
		Commitment:       CommitmentLevel(99), // Invalid
	}
	err := config.Validate()
	if err == nil {
		t.Error("Validate() should fail with invalid commitment level")
	}
}

func TestConfigValidation_ValidConfig(t *testing.T) {
	config := Config{
		Endpoint:         "grpc.example.com:443",
		BlockChannelSize: 100,
		SlotChannelSize:  100,
		MaxMessageSize:   1024,
		KeepaliveTime:    time.Second,
		KeepaliveTimeout: time.Second,
		ReconnectMinDelay: time.Second,
		ReconnectMaxDelay: time.Minute,
		Commitment:       CommitmentConfirmed,
	}
	err := config.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

// =============================================================================
// TestConfigBuilder - Test the fluent config builder API
// =============================================================================

func TestConfigBuilder_BasicBuilding(t *testing.T) {
	config, err := NewConfigBuilder().
		Endpoint("grpc.example.com:443").
		Token("my-secret-token").
		UseTLS(true).
		Commitment(CommitmentFinalized).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if config.Endpoint != "grpc.example.com:443" {
		t.Errorf("Endpoint = %v, want %v", config.Endpoint, "grpc.example.com:443")
	}
	if config.Token != "my-secret-token" {
		t.Errorf("Token = %v, want %v", config.Token, "my-secret-token")
	}
	if !config.UseTLS {
		t.Error("UseTLS = false, want true")
	}
	if config.Commitment != CommitmentFinalized {
		t.Errorf("Commitment = %v, want %v", config.Commitment, CommitmentFinalized)
	}
}

func TestConfigBuilder_IncludeOptions(t *testing.T) {
	config, err := NewConfigBuilder().
		Endpoint("grpc.example.com:443").
		IncludeTransactions(true).
		IncludeEntries(false).
		IncludeAccounts(true).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if !config.IncludeTransactions {
		t.Error("IncludeTransactions = false, want true")
	}
	if config.IncludeEntries {
		t.Error("IncludeEntries = true, want false")
	}
	if !config.IncludeAccounts {
		t.Error("IncludeAccounts = false, want true")
	}
}

func TestConfigBuilder_ChannelSizes(t *testing.T) {
	config, err := NewConfigBuilder().
		Endpoint("grpc.example.com:443").
		BlockChannelSize(200).
		SlotChannelSize(1000).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if config.BlockChannelSize != 200 {
		t.Errorf("BlockChannelSize = %v, want %v", config.BlockChannelSize, 200)
	}
	if config.SlotChannelSize != 1000 {
		t.Errorf("SlotChannelSize = %v, want %v", config.SlotChannelSize, 1000)
	}
}

func TestConfigBuilder_FromSlot(t *testing.T) {
	config, err := NewConfigBuilder().
		Endpoint("grpc.example.com:443").
		FromSlot(12345678).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if config.FromSlot == nil {
		t.Fatal("FromSlot is nil, want non-nil")
	}
	if *config.FromSlot != 12345678 {
		t.Errorf("FromSlot = %v, want %v", *config.FromSlot, 12345678)
	}
}

func TestConfigBuilder_ReconnectPolicy(t *testing.T) {
	config, err := NewConfigBuilder().
		Endpoint("grpc.example.com:443").
		ReconnectPolicy(2*time.Second, 120*time.Second, 10).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if config.ReconnectMinDelay != 2*time.Second {
		t.Errorf("ReconnectMinDelay = %v, want %v", config.ReconnectMinDelay, 2*time.Second)
	}
	if config.ReconnectMaxDelay != 120*time.Second {
		t.Errorf("ReconnectMaxDelay = %v, want %v", config.ReconnectMaxDelay, 120*time.Second)
	}
	if config.MaxReconnects != 10 {
		t.Errorf("MaxReconnects = %v, want %v", config.MaxReconnects, 10)
	}
}

func TestConfigBuilder_Headers(t *testing.T) {
	config, err := NewConfigBuilder().
		Endpoint("grpc.example.com:443").
		Header("x-custom-header", "custom-value").
		Header("x-another-header", "another-value").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if config.Headers["x-custom-header"] != "custom-value" {
		t.Errorf("Headers[x-custom-header] = %v, want %v", config.Headers["x-custom-header"], "custom-value")
	}
	if config.Headers["x-another-header"] != "another-value" {
		t.Errorf("Headers[x-another-header] = %v, want %v", config.Headers["x-another-header"], "another-value")
	}
}

func TestConfigBuilder_Callbacks(t *testing.T) {
	var blockCalled, slotCalled, connectCalled bool

	config, err := NewConfigBuilder().
		Endpoint("grpc.example.com:443").
		OnBlock(func(*Block) { blockCalled = true }).
		OnSlot(func(*SlotUpdate) { slotCalled = true }).
		OnConnect(func() { connectCalled = true }).
		OnDisconnect(func(error) {}).
		OnReconnect(func(int) {}).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	// Test callbacks are set
	if config.OnBlock == nil {
		t.Error("OnBlock is nil")
	}
	if config.OnSlot == nil {
		t.Error("OnSlot is nil")
	}
	if config.OnConnect == nil {
		t.Error("OnConnect is nil")
	}
	if config.OnDisconnect == nil {
		t.Error("OnDisconnect is nil")
	}
	if config.OnReconnect == nil {
		t.Error("OnReconnect is nil")
	}

	// Invoke callbacks to test they work
	config.OnBlock(&Block{})
	config.OnSlot(&SlotUpdate{})
	config.OnConnect()

	if !blockCalled {
		t.Error("OnBlock callback was not invoked")
	}
	if !slotCalled {
		t.Error("OnSlot callback was not invoked")
	}
	if !connectCalled {
		t.Error("OnConnect callback was not invoked")
	}
}

func TestConfigBuilder_MissingEndpoint(t *testing.T) {
	_, err := NewConfigBuilder().
		Token("token").
		Build()

	if err == nil {
		t.Error("Build() should fail without endpoint")
	}
}

func TestConfigBuilder_MustBuild_Valid(t *testing.T) {
	// Should not panic
	config := NewConfigBuilder().
		Endpoint("grpc.example.com:443").
		MustBuild()

	if config.Endpoint != "grpc.example.com:443" {
		t.Errorf("Endpoint = %v, want %v", config.Endpoint, "grpc.example.com:443")
	}
}

func TestConfigBuilder_MustBuild_Invalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustBuild() should panic with invalid config")
		}
	}()

	// This should panic
	NewConfigBuilder().MustBuild()
}

// =============================================================================
// TestProviderPresets - Test provider preset configurations
// =============================================================================

func TestProviderPresets_TritonOne(t *testing.T) {
	config, err := NewConfigBuilder().
		ApplyProvider(TritonProvider, "grpc.triton.one:443", "triton-token").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if config.Endpoint != "grpc.triton.one:443" {
		t.Errorf("Endpoint = %v, want %v", config.Endpoint, "grpc.triton.one:443")
	}
	if config.Token != "triton-token" {
		t.Errorf("Token = %v, want %v", config.Token, "triton-token")
	}
	if !config.UseTLS {
		t.Error("UseTLS should be true for TritonProvider")
	}
	if config.Headers[TritonProvider.AuthHeader] != "triton-token" {
		t.Errorf("Headers[%s] = %v, want %v", TritonProvider.AuthHeader, config.Headers[TritonProvider.AuthHeader], "triton-token")
	}
}

func TestProviderPresets_Helius(t *testing.T) {
	config, err := NewConfigBuilder().
		ApplyProvider(HeliusProvider, "grpc.helius.xyz:443", "helius-token").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if config.Endpoint != "grpc.helius.xyz:443" {
		t.Errorf("Endpoint = %v, want %v", config.Endpoint, "grpc.helius.xyz:443")
	}
	if !config.UseTLS {
		t.Error("UseTLS should be true for HeliusProvider")
	}
	if config.Headers[HeliusProvider.AuthHeader] != "helius-token" {
		t.Errorf("Headers[%s] = %v, want %v", HeliusProvider.AuthHeader, config.Headers[HeliusProvider.AuthHeader], "helius-token")
	}
}

func TestProviderPresets_QuickNode(t *testing.T) {
	config, err := NewConfigBuilder().
		ApplyProvider(QuickNodeProvider, "grpc.quicknode.pro:443", "qn-token").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if config.Endpoint != "grpc.quicknode.pro:443" {
		t.Errorf("Endpoint = %v, want %v", config.Endpoint, "grpc.quicknode.pro:443")
	}
	if !config.UseTLS {
		t.Error("UseTLS should be true for QuickNodeProvider")
	}
}

func TestProviderPresets_Chainstack(t *testing.T) {
	config, err := NewConfigBuilder().
		ApplyProvider(ChainstackProvider, "grpc.chainstack.com:443", "chainstack-token").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if config.Endpoint != "grpc.chainstack.com:443" {
		t.Errorf("Endpoint = %v, want %v", config.Endpoint, "grpc.chainstack.com:443")
	}
	if config.Headers["authorization"] != "chainstack-token" {
		t.Errorf("Headers[authorization] = %v, want %v", config.Headers["authorization"], "chainstack-token")
	}
}

func TestProviderPresets_Attributes(t *testing.T) {
	tests := []struct {
		name       string
		provider   ProviderConfig
		wantName   string
		wantAuth   string
		wantTLS    bool
	}{
		{"TritonProvider", TritonProvider, "triton", "x-token", true},
		{"HeliusProvider", HeliusProvider, "helius", "x-token", true},
		{"QuickNodeProvider", QuickNodeProvider, "quicknode", "x-token", true},
		{"ChainstackProvider", ChainstackProvider, "chainstack", "authorization", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.provider.Name != tt.wantName {
				t.Errorf("Name = %v, want %v", tt.provider.Name, tt.wantName)
			}
			if tt.provider.AuthHeader != tt.wantAuth {
				t.Errorf("AuthHeader = %v, want %v", tt.provider.AuthHeader, tt.wantAuth)
			}
			if tt.provider.UseTLS != tt.wantTLS {
				t.Errorf("UseTLS = %v, want %v", tt.provider.UseTLS, tt.wantTLS)
			}
		})
	}
}

// =============================================================================
// TestBlockConversion - Test converting proto messages to Block types
// =============================================================================

func TestConvertBlock_BasicFields(t *testing.T) {
	client := &Client{}

	blockTime := int64(1704067200)
	blockHeight := uint64(123456)

	pb := &blockUpdate{
		Slot:                     100,
		ParentSlot:               99,
		Blockhash:                "4uQeVj5tqViQh7yWWGStvkEG1Zmhx6uasJtWCJziofM", // Valid base58
		ParentBlockhash:          "11111111111111111111111111111111",
		BlockTime:                &unixTimestamp{Timestamp: blockTime},
		BlockHeight:              &blockHeight_{BlockHeight: blockHeight},
		ExecutedTransactionCount: 50,
	}

	block := client.convertBlock(pb)

	if block.Slot != 100 {
		t.Errorf("Slot = %v, want %v", block.Slot, 100)
	}
	if block.ParentSlot != 99 {
		t.Errorf("ParentSlot = %v, want %v", block.ParentSlot, 99)
	}
	if block.ExecutedTransactionCount != 50 {
		t.Errorf("ExecutedTransactionCount = %v, want %v", block.ExecutedTransactionCount, 50)
	}
	if block.BlockTime == nil || *block.BlockTime != blockTime {
		t.Errorf("BlockTime = %v, want %v", block.BlockTime, blockTime)
	}
	if block.BlockHeight == nil || *block.BlockHeight != blockHeight {
		t.Errorf("BlockHeight = %v, want %v", block.BlockHeight, blockHeight)
	}
	if block.ReceivedAt.IsZero() {
		t.Error("ReceivedAt should not be zero")
	}
}

func TestConvertBlock_NilOptionalFields(t *testing.T) {
	client := &Client{}

	pb := &blockUpdate{
		Slot:       100,
		ParentSlot: 99,
		// BlockTime and BlockHeight are nil
	}

	block := client.convertBlock(pb)

	if block.BlockTime != nil {
		t.Error("BlockTime should be nil")
	}
	if block.BlockHeight != nil {
		t.Error("BlockHeight should be nil")
	}
}

func TestConvertBlock_WithRewards(t *testing.T) {
	client := &Client{}

	pb := &blockUpdate{
		Slot:       100,
		ParentSlot: 99,
		Rewards: &rewards{
			Rewards: []*reward{
				{
					Pubkey:      "11111111111111111111111111111111",
					Lamports:    1000000,
					PostBalance: 5000000,
					RewardType:  int32(RewardTypeStaking),
					Commission:  "5",
				},
				{
					Pubkey:      "22222222222222222222222222222222",
					Lamports:    500000,
					PostBalance: 2000000,
					RewardType:  int32(RewardTypeVoting),
				},
			},
		},
	}

	block := client.convertBlock(pb)

	if len(block.Rewards) != 2 {
		t.Fatalf("len(Rewards) = %v, want %v", len(block.Rewards), 2)
	}

	if block.Rewards[0].Lamports != 1000000 {
		t.Errorf("Rewards[0].Lamports = %v, want %v", block.Rewards[0].Lamports, 1000000)
	}
	if block.Rewards[0].PostBalance != 5000000 {
		t.Errorf("Rewards[0].PostBalance = %v, want %v", block.Rewards[0].PostBalance, 5000000)
	}
	if block.Rewards[0].RewardType != RewardTypeStaking {
		t.Errorf("Rewards[0].RewardType = %v, want %v", block.Rewards[0].RewardType, RewardTypeStaking)
	}
	if block.Rewards[0].Commission == nil || *block.Rewards[0].Commission != 5 {
		t.Errorf("Rewards[0].Commission = %v, want %v", block.Rewards[0].Commission, 5)
	}
}

func TestConvertBlock_WithEntries(t *testing.T) {
	client := &Client{}

	hash := make([]byte, types.HashSize)
	hash[0] = 0xAB

	pb := &blockUpdate{
		Slot:       100,
		ParentSlot: 99,
		Entries: []*entryUpdate{
			{
				Slot:                     100,
				Index:                    0,
				NumHashes:                1000,
				Hash:                     hash,
				ExecutedTransactionCount: 5,
				StartingTransactionIndex: 0,
			},
			{
				Slot:                     100,
				Index:                    1,
				NumHashes:                500,
				ExecutedTransactionCount: 10,
				StartingTransactionIndex: 5,
			},
		},
	}

	block := client.convertBlock(pb)

	if len(block.Entries) != 2 {
		t.Fatalf("len(Entries) = %v, want %v", len(block.Entries), 2)
	}

	if block.Entries[0].Index != 0 {
		t.Errorf("Entries[0].Index = %v, want %v", block.Entries[0].Index, 0)
	}
	if block.Entries[0].NumHashes != 1000 {
		t.Errorf("Entries[0].NumHashes = %v, want %v", block.Entries[0].NumHashes, 1000)
	}
	if block.Entries[0].Hash[0] != 0xAB {
		t.Errorf("Entries[0].Hash[0] = %v, want %v", block.Entries[0].Hash[0], 0xAB)
	}
	if block.Entries[1].StartingTransactionIndex != 5 {
		t.Errorf("Entries[1].StartingTransactionIndex = %v, want %v", block.Entries[1].StartingTransactionIndex, 5)
	}
}

func TestConvertTransaction_BasicFields(t *testing.T) {
	client := &Client{}

	sig := make([]byte, types.SignatureSize)
	sig[0] = 0xDE
	sig[1] = 0xAD

	pb := &transactionInfo{
		Signature: sig,
		IsVote:    true,
		Index:     5,
	}

	tx := client.convertTransaction(pb)

	if tx.IsVote != true {
		t.Error("IsVote = false, want true")
	}
	if tx.Index != 5 {
		t.Errorf("Index = %v, want %v", tx.Index, 5)
	}
	if tx.Signature[0] != 0xDE || tx.Signature[1] != 0xAD {
		t.Error("Signature not correctly copied")
	}
}

func TestConvertTransaction_WithMessage(t *testing.T) {
	client := &Client{}

	accountKey := make([]byte, types.PubkeySize)
	accountKey[0] = 0x11

	recentBlockhash := make([]byte, types.HashSize)
	recentBlockhash[0] = 0x22

	pb := &transactionInfo{
		Signature: make([]byte, types.SignatureSize),
		Transaction: &compiledTransaction{
			Signatures: [][]byte{make([]byte, types.SignatureSize)},
			Message: &message{
				Header: &messageHeader{
					NumRequiredSignatures:       2,
					NumReadonlySignedAccounts:   1,
					NumReadonlyUnsignedAccounts: 3,
				},
				AccountKeys:     [][]byte{accountKey},
				RecentBlockhash: recentBlockhash,
				Instructions: []*compiledInstruction{
					{
						ProgramIDIndex: 0,
						Accounts:       []byte{0, 1, 2},
						Data:           []byte{0xAA, 0xBB},
					},
				},
				Versioned: false,
			},
		},
	}

	tx := client.convertTransaction(pb)

	if tx.Message.Header.NumRequiredSignatures != 2 {
		t.Errorf("Header.NumRequiredSignatures = %v, want %v", tx.Message.Header.NumRequiredSignatures, 2)
	}
	if tx.Message.Header.NumReadonlySignedAccounts != 1 {
		t.Errorf("Header.NumReadonlySignedAccounts = %v, want %v", tx.Message.Header.NumReadonlySignedAccounts, 1)
	}
	if tx.Message.Header.NumReadonlyUnsignedAccounts != 3 {
		t.Errorf("Header.NumReadonlyUnsignedAccounts = %v, want %v", tx.Message.Header.NumReadonlyUnsignedAccounts, 3)
	}
	if len(tx.Message.AccountKeys) != 1 {
		t.Fatalf("len(AccountKeys) = %v, want %v", len(tx.Message.AccountKeys), 1)
	}
	if tx.Message.AccountKeys[0][0] != 0x11 {
		t.Error("AccountKey not correctly copied")
	}
	if tx.Message.RecentBlockhash[0] != 0x22 {
		t.Error("RecentBlockhash not correctly copied")
	}
	if !tx.Message.IsLegacy {
		t.Error("IsLegacy should be true for non-versioned message")
	}
	if len(tx.Message.Instructions) != 1 {
		t.Fatalf("len(Instructions) = %v, want %v", len(tx.Message.Instructions), 1)
	}
	if tx.Message.Instructions[0].ProgramIDIndex != 0 {
		t.Errorf("Instructions[0].ProgramIDIndex = %v, want %v", tx.Message.Instructions[0].ProgramIDIndex, 0)
	}
}

func TestConvertTransaction_WithMeta(t *testing.T) {
	client := &Client{}

	computeUnits := uint64(150000)
	pb := &transactionInfo{
		Signature: make([]byte, types.SignatureSize),
		Meta: &transactionMeta{
			Fee:                  5000,
			PreBalances:         []uint64{1000000, 2000000},
			PostBalances:        []uint64{995000, 2000000},
			ComputeUnitsConsumed: &computeUnits,
			LogMessages:         []string{"Program log: Hello", "Program log: World"},
			Err: &transactionError{
				Err: []byte("InstructionError"),
			},
		},
	}

	tx := client.convertTransaction(pb)

	if tx.Meta == nil {
		t.Fatal("Meta is nil")
	}
	if tx.Meta.Fee != 5000 {
		t.Errorf("Meta.Fee = %v, want %v", tx.Meta.Fee, 5000)
	}
	if len(tx.Meta.PreBalances) != 2 || tx.Meta.PreBalances[0] != 1000000 {
		t.Error("PreBalances not correctly copied")
	}
	if len(tx.Meta.PostBalances) != 2 || tx.Meta.PostBalances[0] != 995000 {
		t.Error("PostBalances not correctly copied")
	}
	if tx.Meta.ComputeUnitsConsumed != 150000 {
		t.Errorf("ComputeUnitsConsumed = %v, want %v", tx.Meta.ComputeUnitsConsumed, 150000)
	}
	if len(tx.Meta.LogMessages) != 2 {
		t.Errorf("len(LogMessages) = %v, want %v", len(tx.Meta.LogMessages), 2)
	}
	if tx.Meta.Err == nil {
		t.Error("Meta.Err is nil, want error")
	}
}

func TestConvertSlotUpdate(t *testing.T) {
	client := &Client{}

	parent := uint64(99)
	pb := &slotUpdate{
		Slot:   100,
		Parent: &parent,
		Status: int32(SlotStatusConfirmed),
	}

	slot := client.convertSlotUpdate(pb)

	if slot.Slot != 100 {
		t.Errorf("Slot = %v, want %v", slot.Slot, 100)
	}
	if slot.ParentSlot == nil || *slot.ParentSlot != 99 {
		t.Errorf("ParentSlot = %v, want %v", slot.ParentSlot, 99)
	}
	if slot.Status != SlotStatusConfirmed {
		t.Errorf("Status = %v, want %v", slot.Status, SlotStatusConfirmed)
	}
	if slot.ReceivedAt.IsZero() {
		t.Error("ReceivedAt should not be zero")
	}
}

func TestConvertEntry(t *testing.T) {
	client := &Client{}

	hash := make([]byte, types.HashSize)
	for i := range hash {
		hash[i] = byte(i)
	}

	pb := &entryUpdate{
		Slot:                     100,
		Index:                    5,
		NumHashes:                12345,
		Hash:                     hash,
		ExecutedTransactionCount: 10,
		StartingTransactionIndex: 25,
	}

	entry := client.convertEntry(pb)

	if entry.Slot != 100 {
		t.Errorf("Slot = %v, want %v", entry.Slot, 100)
	}
	if entry.Index != 5 {
		t.Errorf("Index = %v, want %v", entry.Index, 5)
	}
	if entry.NumHashes != 12345 {
		t.Errorf("NumHashes = %v, want %v", entry.NumHashes, 12345)
	}
	if entry.ExecutedTransactionCount != 10 {
		t.Errorf("ExecutedTransactionCount = %v, want %v", entry.ExecutedTransactionCount, 10)
	}
	if entry.StartingTransactionIndex != 25 {
		t.Errorf("StartingTransactionIndex = %v, want %v", entry.StartingTransactionIndex, 25)
	}
	for i := 0; i < types.HashSize; i++ {
		if entry.Hash[i] != byte(i) {
			t.Errorf("Hash[%d] = %v, want %v", i, entry.Hash[i], byte(i))
			break
		}
	}
}

func TestConvertReward(t *testing.T) {
	client := &Client{}

	pb := &reward{
		Pubkey:      "11111111111111111111111111111111",
		Lamports:    1000000,
		PostBalance: 5000000,
		RewardType:  int32(RewardTypeFee),
		Commission:  "10",
	}

	r := client.convertReward(pb)

	if r.Lamports != 1000000 {
		t.Errorf("Lamports = %v, want %v", r.Lamports, 1000000)
	}
	if r.PostBalance != 5000000 {
		t.Errorf("PostBalance = %v, want %v", r.PostBalance, 5000000)
	}
	if r.RewardType != RewardTypeFee {
		t.Errorf("RewardType = %v, want %v", r.RewardType, RewardTypeFee)
	}
	if r.Commission == nil || *r.Commission != 10 {
		t.Errorf("Commission = %v, want %v", r.Commission, 10)
	}
}

// =============================================================================
// TestClientHealth - Test health status reporting
// =============================================================================

func TestClientHealth_NotConnected(t *testing.T) {
	config := Config{
		Endpoint:            "grpc.example.com:443",
		StaleTimeout:        60 * time.Second,
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	health := client.Health()

	if health.Connected {
		t.Error("Connected should be false for new client")
	}
	if health.Provider != "grpc.example.com:443" {
		t.Errorf("Provider = %v, want %v", health.Provider, "grpc.example.com:443")
	}
	if health.ReconnectCount != 0 {
		t.Errorf("ReconnectCount = %v, want %v", health.ReconnectCount, 0)
	}
	if health.LastSlot != 0 {
		t.Errorf("LastSlot = %v, want %v", health.LastSlot, 0)
	}
}

func TestClientHealth_Fields(t *testing.T) {
	config := Config{
		Endpoint:     "test.endpoint:443",
		StaleTimeout: 60 * time.Second,
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// Simulate some state updates
	client.lastSlot.Store(12345)
	client.reconnectCount.Store(3)
	client.lastUpdate.Store(time.Now().UnixNano())

	health := client.Health()

	if health.LastSlot != 12345 {
		t.Errorf("LastSlot = %v, want %v", health.LastSlot, 12345)
	}
	if health.ReconnectCount != 3 {
		t.Errorf("ReconnectCount = %v, want %v", health.ReconnectCount, 3)
	}
	if health.LastUpdate.IsZero() {
		t.Error("LastUpdate should not be zero")
	}
}

func TestClientHealth_LastError(t *testing.T) {
	config := Config{
		Endpoint:     "test.endpoint:443",
		StaleTimeout: 60 * time.Second,
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// Set an error
	client.setLastError(ErrStreamClosed)

	health := client.Health()

	if health.LastError != ErrStreamClosed {
		t.Errorf("LastError = %v, want %v", health.LastError, ErrStreamClosed)
	}
}

// =============================================================================
// TestReconnectionBackoff - Test exponential backoff calculation
// =============================================================================

func TestMinDuration(t *testing.T) {
	tests := []struct {
		a, b, want time.Duration
	}{
		{time.Second, time.Minute, time.Second},
		{time.Minute, time.Second, time.Second},
		{time.Second, time.Second, time.Second},
		{100 * time.Millisecond, 50 * time.Millisecond, 50 * time.Millisecond},
	}

	for _, tt := range tests {
		got := minDuration(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("minDuration(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestExponentialBackoff_Calculation(t *testing.T) {
	// Simulate the backoff calculation from reconnect()
	minDelay := 1 * time.Second
	maxDelay := 60 * time.Second

	backoff := minDelay

	// First backoff
	if backoff != time.Second {
		t.Errorf("Initial backoff = %v, want %v", backoff, time.Second)
	}

	// Double it (as done in reconnect)
	backoff = minDuration(backoff*2, maxDelay)
	if backoff != 2*time.Second {
		t.Errorf("Second backoff = %v, want %v", backoff, 2*time.Second)
	}

	// Keep doubling
	backoff = minDuration(backoff*2, maxDelay)
	if backoff != 4*time.Second {
		t.Errorf("Third backoff = %v, want %v", backoff, 4*time.Second)
	}

	// Continue until we hit max
	for i := 0; i < 10; i++ {
		backoff = minDuration(backoff*2, maxDelay)
	}
	if backoff != maxDelay {
		t.Errorf("Final backoff = %v, want %v", backoff, maxDelay)
	}
}

func TestBackoffCapped_AtMaxDelay(t *testing.T) {
	minDelay := 1 * time.Second
	maxDelay := 8 * time.Second

	backoff := minDelay
	expectedBackoffs := []time.Duration{
		1 * time.Second,  // initial
		2 * time.Second,  // 1 * 2
		4 * time.Second,  // 2 * 2
		8 * time.Second,  // 4 * 2 = maxDelay
		8 * time.Second,  // capped at max
		8 * time.Second,  // still capped
	}

	for i, expected := range expectedBackoffs {
		if backoff != expected {
			t.Errorf("Backoff at step %d = %v, want %v", i, backoff, expected)
		}
		backoff = minDuration(backoff*2, maxDelay)
	}
}

// =============================================================================
// TestDefaultConfig - Test default configuration values
// =============================================================================

func TestDefaultConfig_Values(t *testing.T) {
	config := DefaultConfig()

	if !config.UseTLS {
		t.Error("UseTLS should default to true")
	}
	if config.Commitment != CommitmentConfirmed {
		t.Errorf("Commitment = %v, want %v", config.Commitment, CommitmentConfirmed)
	}
	if !config.IncludeTransactions {
		t.Error("IncludeTransactions should default to true")
	}
	if !config.IncludeEntries {
		t.Error("IncludeEntries should default to true")
	}
	if config.IncludeAccounts {
		t.Error("IncludeAccounts should default to false")
	}
	if !config.IncludeVotes {
		t.Error("IncludeVotes should default to true")
	}
	if !config.IncludeFailed {
		t.Error("IncludeFailed should default to true")
	}
	if config.KeepaliveTime != DefaultKeepaliveTime {
		t.Errorf("KeepaliveTime = %v, want %v", config.KeepaliveTime, DefaultKeepaliveTime)
	}
	if config.KeepaliveTimeout != DefaultKeepaliveTimeout {
		t.Errorf("KeepaliveTimeout = %v, want %v", config.KeepaliveTimeout, DefaultKeepaliveTimeout)
	}
	if config.ReconnectMinDelay != DefaultReconnectMinDelay {
		t.Errorf("ReconnectMinDelay = %v, want %v", config.ReconnectMinDelay, DefaultReconnectMinDelay)
	}
	if config.ReconnectMaxDelay != DefaultReconnectMaxDelay {
		t.Errorf("ReconnectMaxDelay = %v, want %v", config.ReconnectMaxDelay, DefaultReconnectMaxDelay)
	}
	if config.MaxReconnects != 0 {
		t.Errorf("MaxReconnects = %v, want %v", config.MaxReconnects, 0)
	}
	if config.BlockChannelSize != DefaultBlockChannelSize {
		t.Errorf("BlockChannelSize = %v, want %v", config.BlockChannelSize, DefaultBlockChannelSize)
	}
	if config.SlotChannelSize != DefaultSlotChannelSize {
		t.Errorf("SlotChannelSize = %v, want %v", config.SlotChannelSize, DefaultSlotChannelSize)
	}
	if config.MaxMessageSize != DefaultMaxMessageSize {
		t.Errorf("MaxMessageSize = %v, want %v", config.MaxMessageSize, DefaultMaxMessageSize)
	}
	if config.PingInterval != DefaultPingInterval {
		t.Errorf("PingInterval = %v, want %v", config.PingInterval, DefaultPingInterval)
	}
	if config.HealthCheckInterval != DefaultHealthCheckInterval {
		t.Errorf("HealthCheckInterval = %v, want %v", config.HealthCheckInterval, DefaultHealthCheckInterval)
	}
	if config.StaleTimeout != DefaultStaleTimeout {
		t.Errorf("StaleTimeout = %v, want %v", config.StaleTimeout, DefaultStaleTimeout)
	}
	if config.Headers == nil {
		t.Error("Headers should not be nil")
	}
}

func TestWithDefaults_AppliesDefaults(t *testing.T) {
	config := Config{
		Endpoint: "test.endpoint:443",
		// All other fields are zero values
	}

	config = config.WithDefaults()

	if config.KeepaliveTime != DefaultKeepaliveTime {
		t.Errorf("KeepaliveTime = %v, want %v", config.KeepaliveTime, DefaultKeepaliveTime)
	}
	if config.BlockChannelSize != DefaultBlockChannelSize {
		t.Errorf("BlockChannelSize = %v, want %v", config.BlockChannelSize, DefaultBlockChannelSize)
	}
	if config.Headers == nil {
		t.Error("Headers should not be nil after WithDefaults")
	}
}

func TestWithDefaults_PreservesSetValues(t *testing.T) {
	config := Config{
		Endpoint:         "test.endpoint:443",
		KeepaliveTime:    30 * time.Second,
		BlockChannelSize: 500,
	}

	config = config.WithDefaults()

	if config.KeepaliveTime != 30*time.Second {
		t.Errorf("KeepaliveTime = %v, want %v", config.KeepaliveTime, 30*time.Second)
	}
	if config.BlockChannelSize != 500 {
		t.Errorf("BlockChannelSize = %v, want %v", config.BlockChannelSize, 500)
	}
}

// =============================================================================
// TestExpandedToken - Test environment variable expansion
// =============================================================================

func TestExpandedToken_NoEnvVar(t *testing.T) {
	config := Config{
		Token: "plain-token",
	}

	got := config.ExpandedToken()
	if got != "plain-token" {
		t.Errorf("ExpandedToken() = %v, want %v", got, "plain-token")
	}
}

func TestExpandedToken_WithEnvVar(t *testing.T) {
	os.Setenv("TEST_GEYSER_TOKEN", "secret-value")
	defer os.Unsetenv("TEST_GEYSER_TOKEN")

	config := Config{
		Token: "${TEST_GEYSER_TOKEN}",
	}

	got := config.ExpandedToken()
	if got != "secret-value" {
		t.Errorf("ExpandedToken() = %v, want %v", got, "secret-value")
	}
}

func TestExpandedToken_MixedContent(t *testing.T) {
	os.Setenv("TEST_TOKEN_PREFIX", "abc")
	os.Setenv("TEST_TOKEN_SUFFIX", "xyz")
	defer os.Unsetenv("TEST_TOKEN_PREFIX")
	defer os.Unsetenv("TEST_TOKEN_SUFFIX")

	config := Config{
		Token: "Bearer ${TEST_TOKEN_PREFIX}-${TEST_TOKEN_SUFFIX}",
	}

	got := config.ExpandedToken()
	if got != "Bearer abc-xyz" {
		t.Errorf("ExpandedToken() = %v, want %v", got, "Bearer abc-xyz")
	}
}

func TestExpandedToken_UnsetEnvVar(t *testing.T) {
	os.Unsetenv("NONEXISTENT_VAR")

	config := Config{
		Token: "${NONEXISTENT_VAR}",
	}

	got := config.ExpandedToken()
	if got != "" {
		t.Errorf("ExpandedToken() = %v, want empty string", got)
	}
}

// =============================================================================
// TestCommitmentLevel - Test commitment level strings
// =============================================================================

func TestCommitmentLevel_String(t *testing.T) {
	tests := []struct {
		level CommitmentLevel
		want  string
	}{
		{CommitmentProcessed, "processed"},
		{CommitmentConfirmed, "confirmed"},
		{CommitmentFinalized, "finalized"},
		{CommitmentLevel(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.level.String()
		if got != tt.want {
			t.Errorf("CommitmentLevel(%d).String() = %v, want %v", tt.level, got, tt.want)
		}
	}
}

func TestSlotStatus_String(t *testing.T) {
	tests := []struct {
		status SlotStatus
		want   string
	}{
		{SlotStatusProcessed, "processed"},
		{SlotStatusConfirmed, "confirmed"},
		{SlotStatusFinalized, "finalized"},
		{SlotStatus(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.status.String()
		if got != tt.want {
			t.Errorf("SlotStatus(%d).String() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestRewardType_String(t *testing.T) {
	tests := []struct {
		rt   RewardType
		want string
	}{
		{RewardTypeUnspecified, "unspecified"},
		{RewardTypeFee, "fee"},
		{RewardTypeRent, "rent"},
		{RewardTypeStaking, "staking"},
		{RewardTypeVoting, "voting"},
		{RewardType(99), "unspecified"},
	}

	for _, tt := range tests {
		got := tt.rt.String()
		if got != tt.want {
			t.Errorf("RewardType(%d).String() = %v, want %v", tt.rt, got, tt.want)
		}
	}
}

// =============================================================================
// TestClientErrors - Test client error conditions
// =============================================================================

func TestClientErrors_Defined(t *testing.T) {
	// Ensure all expected errors are defined
	errors := []error{
		ErrNotConnected,
		ErrAlreadyConnected,
		ErrClosed,
		ErrSubscribeFailed,
		ErrStreamClosed,
		ErrMaxReconnects,
		ErrNoEndpoint,
		ErrInvalidConfig,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Expected error to be defined, got nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

func TestClientClose_NotConnected(t *testing.T) {
	config := Config{
		Endpoint: "grpc.example.com:443",
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// Close without connecting
	err = client.Close()
	if err != nil {
		t.Errorf("Close() on non-connected client should not error, got %v", err)
	}

	// Second close should return ErrClosed
	err = client.Close()
	if err != ErrClosed {
		t.Errorf("Close() second time = %v, want %v", err, ErrClosed)
	}
}

// =============================================================================
// TestIsRetryableError - Test retryable error detection
// =============================================================================

func TestIsRetryableError_NilError(t *testing.T) {
	if isRetryableError(nil) {
		t.Error("isRetryableError(nil) should return false")
	}
}

func TestIsRetryableError_StreamClosed(t *testing.T) {
	if !isRetryableError(ErrStreamClosed) {
		t.Error("isRetryableError(ErrStreamClosed) should return true")
	}
}

// =============================================================================
// TestBoolPtr - Test helper function
// =============================================================================

func TestBoolPtr(t *testing.T) {
	truePtr := boolPtr(true)
	if truePtr == nil || *truePtr != true {
		t.Error("boolPtr(true) should return pointer to true")
	}

	falsePtr := boolPtr(false)
	if falsePtr == nil || *falsePtr != false {
		t.Error("boolPtr(false) should return pointer to false")
	}
}

// =============================================================================
// blockHeight type alias for test (matching client.go internal type)
// =============================================================================

type blockHeight_ = blockHeight
