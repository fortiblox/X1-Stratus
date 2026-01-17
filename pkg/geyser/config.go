package geyser

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Default configuration values.
const (
	// DefaultKeepaliveTime is the default interval for keepalive pings.
	DefaultKeepaliveTime = 10 * time.Second

	// DefaultKeepaliveTimeout is the default timeout for keepalive responses.
	DefaultKeepaliveTimeout = 5 * time.Second

	// DefaultReconnectMinDelay is the minimum delay before reconnecting.
	DefaultReconnectMinDelay = 1 * time.Second

	// DefaultReconnectMaxDelay is the maximum delay before reconnecting.
	DefaultReconnectMaxDelay = 60 * time.Second

	// DefaultBlockChannelSize is the default buffer size for the block channel.
	DefaultBlockChannelSize = 100

	// DefaultSlotChannelSize is the default buffer size for the slot channel.
	DefaultSlotChannelSize = 500

	// DefaultMaxMessageSize is the default maximum gRPC message size (1GB).
	// Solana blocks can be large due to many transactions.
	DefaultMaxMessageSize = 1024 * 1024 * 1024

	// DefaultPingInterval is the interval between ping messages.
	DefaultPingInterval = 15 * time.Second

	// DefaultHealthCheckInterval is the interval between health checks.
	DefaultHealthCheckInterval = 30 * time.Second

	// DefaultStaleTimeout is how long without updates before connection is considered stale.
	DefaultStaleTimeout = 60 * time.Second
)

// Configuration errors.
var (
	ErrNoEndpoint    = errors.New("geyser endpoint is required")
	ErrInvalidConfig = errors.New("invalid geyser configuration")
)

// Config holds the configuration for the Geyser client.
type Config struct {
	// Endpoint is the gRPC endpoint URL (e.g., "grpc.example.com:443").
	// Required.
	Endpoint string

	// Token is the authentication token for the gRPC service.
	// Usually provided via x-token header or bearer token.
	// Can use environment variable expansion with ${VAR_NAME}.
	Token string

	// UseTLS enables TLS for the gRPC connection.
	// Should be true for production endpoints.
	UseTLS bool

	// Commitment is the commitment level for block subscriptions.
	// Defaults to CommitmentConfirmed.
	Commitment CommitmentLevel

	// IncludeTransactions includes full transaction data in blocks.
	// Required for replay - defaults to true.
	IncludeTransactions bool

	// IncludeEntries includes PoH entries in blocks.
	// Required for PoH verification - defaults to true.
	IncludeEntries bool

	// IncludeAccounts includes account updates in blocks.
	// Not needed for replay since we compute state ourselves.
	// Defaults to false.
	IncludeAccounts bool

	// IncludeVotes includes vote transactions.
	// Defaults to true since we need them for consensus tracking.
	IncludeVotes bool

	// IncludeFailed includes failed transactions.
	// Defaults to true since they still affect state (fees are charged).
	IncludeFailed bool

	// FromSlot is the starting slot for historical replay.
	// If nil, starts from the current slot.
	FromSlot *uint64

	// Keepalive configuration.
	KeepaliveTime    time.Duration
	KeepaliveTimeout time.Duration

	// Reconnection configuration.
	ReconnectMinDelay time.Duration
	ReconnectMaxDelay time.Duration
	MaxReconnects     int // 0 = unlimited

	// Channel buffer sizes.
	BlockChannelSize int
	SlotChannelSize  int

	// MaxMessageSize is the maximum gRPC message size in bytes.
	MaxMessageSize int

	// PingInterval is the interval between ping messages for keepalive.
	PingInterval time.Duration

	// HealthCheckInterval is how often to check connection health.
	HealthCheckInterval time.Duration

	// StaleTimeout is how long without updates before reconnecting.
	StaleTimeout time.Duration

	// Headers are additional headers to send with gRPC requests.
	// Useful for custom authentication schemes.
	Headers map[string]string

	// OnBlock is called for each received block (optional).
	// Called synchronously - should not block.
	OnBlock func(*Block)

	// OnSlot is called for each slot update (optional).
	// Called synchronously - should not block.
	OnSlot func(*SlotUpdate)

	// OnConnect is called when connection is established (optional).
	OnConnect func()

	// OnDisconnect is called when connection is lost (optional).
	OnDisconnect func(error)

	// OnReconnect is called when reconnection succeeds (optional).
	OnReconnect func(attempt int)
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		UseTLS:              true,
		Commitment:          CommitmentConfirmed,
		IncludeTransactions: true,
		IncludeEntries:      true,
		IncludeAccounts:     false,
		IncludeVotes:        true,
		IncludeFailed:       true,

		KeepaliveTime:    DefaultKeepaliveTime,
		KeepaliveTimeout: DefaultKeepaliveTimeout,

		ReconnectMinDelay: DefaultReconnectMinDelay,
		ReconnectMaxDelay: DefaultReconnectMaxDelay,
		MaxReconnects:     0, // unlimited

		BlockChannelSize: DefaultBlockChannelSize,
		SlotChannelSize:  DefaultSlotChannelSize,
		MaxMessageSize:   DefaultMaxMessageSize,
		PingInterval:     DefaultPingInterval,

		HealthCheckInterval: DefaultHealthCheckInterval,
		StaleTimeout:        DefaultStaleTimeout,

		Headers: make(map[string]string),
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return ErrNoEndpoint
	}

	if c.BlockChannelSize <= 0 {
		return fmt.Errorf("%w: block channel size must be positive", ErrInvalidConfig)
	}

	if c.SlotChannelSize <= 0 {
		return fmt.Errorf("%w: slot channel size must be positive", ErrInvalidConfig)
	}

	if c.MaxMessageSize <= 0 {
		return fmt.Errorf("%w: max message size must be positive", ErrInvalidConfig)
	}

	if c.KeepaliveTime <= 0 {
		return fmt.Errorf("%w: keepalive time must be positive", ErrInvalidConfig)
	}

	if c.KeepaliveTimeout <= 0 {
		return fmt.Errorf("%w: keepalive timeout must be positive", ErrInvalidConfig)
	}

	if c.ReconnectMinDelay <= 0 {
		return fmt.Errorf("%w: reconnect min delay must be positive", ErrInvalidConfig)
	}

	if c.ReconnectMaxDelay < c.ReconnectMinDelay {
		return fmt.Errorf("%w: reconnect max delay must be >= min delay", ErrInvalidConfig)
	}

	if c.Commitment < CommitmentProcessed || c.Commitment > CommitmentFinalized {
		return fmt.Errorf("%w: invalid commitment level", ErrInvalidConfig)
	}

	return nil
}

// WithDefaults returns a new config with default values applied for any
// zero values in the original config.
func (c Config) WithDefaults() Config {
	defaults := DefaultConfig()

	if c.KeepaliveTime == 0 {
		c.KeepaliveTime = defaults.KeepaliveTime
	}
	if c.KeepaliveTimeout == 0 {
		c.KeepaliveTimeout = defaults.KeepaliveTimeout
	}
	if c.ReconnectMinDelay == 0 {
		c.ReconnectMinDelay = defaults.ReconnectMinDelay
	}
	if c.ReconnectMaxDelay == 0 {
		c.ReconnectMaxDelay = defaults.ReconnectMaxDelay
	}
	if c.BlockChannelSize == 0 {
		c.BlockChannelSize = defaults.BlockChannelSize
	}
	if c.SlotChannelSize == 0 {
		c.SlotChannelSize = defaults.SlotChannelSize
	}
	if c.MaxMessageSize == 0 {
		c.MaxMessageSize = defaults.MaxMessageSize
	}
	if c.PingInterval == 0 {
		c.PingInterval = defaults.PingInterval
	}
	if c.HealthCheckInterval == 0 {
		c.HealthCheckInterval = defaults.HealthCheckInterval
	}
	if c.StaleTimeout == 0 {
		c.StaleTimeout = defaults.StaleTimeout
	}
	if c.Headers == nil {
		c.Headers = defaults.Headers
	}

	return c
}

// ExpandedToken returns the token with environment variable expansion.
// Supports ${VAR_NAME} syntax.
func (c *Config) ExpandedToken() string {
	return expandEnvVars(c.Token)
}

// expandEnvVars expands ${VAR} and $VAR references in a string.
func expandEnvVars(s string) string {
	// Handle ${VAR} syntax
	result := s
	for {
		start := strings.Index(result, "${")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		varName := result[start+2 : end]
		varValue := os.Getenv(varName)
		result = result[:start] + varValue + result[end+1:]
	}
	return result
}

// ConfigBuilder provides a fluent interface for building Config.
type ConfigBuilder struct {
	config Config
}

// NewConfigBuilder creates a new ConfigBuilder with default values.
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: DefaultConfig(),
	}
}

// Endpoint sets the gRPC endpoint.
func (b *ConfigBuilder) Endpoint(endpoint string) *ConfigBuilder {
	b.config.Endpoint = endpoint
	return b
}

// Token sets the authentication token.
func (b *ConfigBuilder) Token(token string) *ConfigBuilder {
	b.config.Token = token
	return b
}

// UseTLS enables or disables TLS.
func (b *ConfigBuilder) UseTLS(useTLS bool) *ConfigBuilder {
	b.config.UseTLS = useTLS
	return b
}

// Commitment sets the commitment level.
func (b *ConfigBuilder) Commitment(commitment CommitmentLevel) *ConfigBuilder {
	b.config.Commitment = commitment
	return b
}

// IncludeTransactions enables or disables transaction data.
func (b *ConfigBuilder) IncludeTransactions(include bool) *ConfigBuilder {
	b.config.IncludeTransactions = include
	return b
}

// IncludeEntries enables or disables PoH entry data.
func (b *ConfigBuilder) IncludeEntries(include bool) *ConfigBuilder {
	b.config.IncludeEntries = include
	return b
}

// IncludeAccounts enables or disables account data.
func (b *ConfigBuilder) IncludeAccounts(include bool) *ConfigBuilder {
	b.config.IncludeAccounts = include
	return b
}

// FromSlot sets the starting slot.
func (b *ConfigBuilder) FromSlot(slot uint64) *ConfigBuilder {
	b.config.FromSlot = &slot
	return b
}

// BlockChannelSize sets the block channel buffer size.
func (b *ConfigBuilder) BlockChannelSize(size int) *ConfigBuilder {
	b.config.BlockChannelSize = size
	return b
}

// SlotChannelSize sets the slot channel buffer size.
func (b *ConfigBuilder) SlotChannelSize(size int) *ConfigBuilder {
	b.config.SlotChannelSize = size
	return b
}

// ReconnectPolicy sets the reconnection parameters.
func (b *ConfigBuilder) ReconnectPolicy(minDelay, maxDelay time.Duration, maxAttempts int) *ConfigBuilder {
	b.config.ReconnectMinDelay = minDelay
	b.config.ReconnectMaxDelay = maxDelay
	b.config.MaxReconnects = maxAttempts
	return b
}

// Header adds a custom header.
func (b *ConfigBuilder) Header(key, value string) *ConfigBuilder {
	if b.config.Headers == nil {
		b.config.Headers = make(map[string]string)
	}
	b.config.Headers[key] = value
	return b
}

// OnBlock sets the block callback.
func (b *ConfigBuilder) OnBlock(fn func(*Block)) *ConfigBuilder {
	b.config.OnBlock = fn
	return b
}

// OnSlot sets the slot callback.
func (b *ConfigBuilder) OnSlot(fn func(*SlotUpdate)) *ConfigBuilder {
	b.config.OnSlot = fn
	return b
}

// OnConnect sets the connect callback.
func (b *ConfigBuilder) OnConnect(fn func()) *ConfigBuilder {
	b.config.OnConnect = fn
	return b
}

// OnDisconnect sets the disconnect callback.
func (b *ConfigBuilder) OnDisconnect(fn func(error)) *ConfigBuilder {
	b.config.OnDisconnect = fn
	return b
}

// OnReconnect sets the reconnect callback.
func (b *ConfigBuilder) OnReconnect(fn func(int)) *ConfigBuilder {
	b.config.OnReconnect = fn
	return b
}

// Build validates and returns the Config.
func (b *ConfigBuilder) Build() (Config, error) {
	cfg := b.config.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// MustBuild validates and returns the Config, panicking on error.
func (b *ConfigBuilder) MustBuild() Config {
	cfg, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("invalid geyser config: %v", err))
	}
	return cfg
}

// ProviderConfig contains provider-specific configuration presets.
type ProviderConfig struct {
	// Name is the provider name for logging.
	Name string

	// Endpoint is the provider's gRPC endpoint.
	Endpoint string

	// AuthHeader is the header name for authentication (e.g., "x-token").
	AuthHeader string

	// UseTLS indicates if the provider requires TLS.
	UseTLS bool
}

// Common provider presets.
var (
	// TritonProvider is the configuration preset for Triton One (Dragon's Mouth).
	TritonProvider = ProviderConfig{
		Name:       "triton",
		AuthHeader: "x-token",
		UseTLS:     true,
	}

	// HeliusProvider is the configuration preset for Helius.
	HeliusProvider = ProviderConfig{
		Name:       "helius",
		AuthHeader: "x-token",
		UseTLS:     true,
	}

	// QuickNodeProvider is the configuration preset for QuickNode.
	QuickNodeProvider = ProviderConfig{
		Name:       "quicknode",
		AuthHeader: "x-token",
		UseTLS:     true,
	}

	// ChainstackProvider is the configuration preset for Chainstack.
	ChainstackProvider = ProviderConfig{
		Name:       "chainstack",
		AuthHeader: "authorization",
		UseTLS:     true,
	}
)

// ApplyProvider applies a provider preset to a config builder.
func (b *ConfigBuilder) ApplyProvider(provider ProviderConfig, endpoint, token string) *ConfigBuilder {
	b.config.Endpoint = endpoint
	b.config.Token = token
	b.config.UseTLS = provider.UseTLS
	if provider.AuthHeader != "" && token != "" {
		b.Header(provider.AuthHeader, token)
	}
	return b
}
