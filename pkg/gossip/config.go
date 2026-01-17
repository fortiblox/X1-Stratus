package gossip

import (
	"errors"
	"fmt"
	"time"
)

// Default X1 mainnet entrypoints.
var DefaultEntrypoints = []string{
	"entrypoint0.mainnet.x1.xyz:8001",
	"entrypoint1.mainnet.x1.xyz:8001",
	"entrypoint2.mainnet.x1.xyz:8001",
}

// Default configuration values.
const (
	// DefaultGossipPort is the default UDP port for gossip protocol.
	DefaultGossipPort = 8001

	// DefaultTimeout is the default timeout for gossip operations.
	DefaultTimeout = 30 * time.Second

	// DefaultPullInterval is the default interval between pull requests.
	DefaultPullInterval = 5 * time.Second

	// DefaultMaxRetries is the default number of retries for failed operations.
	DefaultMaxRetries = 3

	// DefaultBufferSize is the default UDP receive buffer size.
	DefaultBufferSize = 65535

	// DefaultMaxNodes is the default maximum number of nodes to discover.
	DefaultMaxNodes = 1000

	// DefaultRPCTimeout is the default timeout for RPC fallback requests.
	DefaultRPCTimeout = 10 * time.Second
)

// Configuration errors.
var (
	ErrNoEntrypoints   = errors.New("at least one entrypoint is required")
	ErrInvalidConfig   = errors.New("invalid gossip configuration")
	ErrInvalidTimeout  = errors.New("timeout must be positive")
)

// Config holds the configuration for the gossip client.
type Config struct {
	// Entrypoints is the list of gossip entrypoint addresses (host:port).
	// At least one entrypoint is required.
	Entrypoints []string

	// Timeout is the overall timeout for discovery operations.
	Timeout time.Duration

	// PullInterval is the interval between pull requests.
	PullInterval time.Duration

	// MaxRetries is the number of retries for failed operations.
	MaxRetries int

	// BufferSize is the UDP receive buffer size.
	BufferSize int

	// MaxNodes is the maximum number of nodes to discover.
	// Set to 0 for unlimited.
	MaxNodes int

	// LocalPort is the local UDP port to bind to.
	// If 0, a random port is chosen.
	LocalPort int

	// RPCEndpoint is an optional RPC endpoint for fallback discovery.
	// If set, the client will use getClusterNodes when gossip fails.
	RPCEndpoint string

	// RPCTimeout is the timeout for RPC fallback requests.
	RPCTimeout time.Duration

	// UseRPCOnly skips gossip protocol and only uses RPC fallback.
	// Useful when UDP is blocked or for simpler deployments.
	UseRPCOnly bool

	// ShredVersion filters nodes by shred version.
	// If non-zero, only nodes with matching shred version are returned.
	ShredVersion uint16

	// OnNodeDiscovered is called when a new node is discovered.
	// Called synchronously - should not block.
	OnNodeDiscovered func(*Node)

	// OnError is called when a non-fatal error occurs.
	OnError func(error)
}

// DefaultConfig returns a configuration with sensible defaults for X1 mainnet.
func DefaultConfig() Config {
	return Config{
		Entrypoints:  DefaultEntrypoints,
		Timeout:      DefaultTimeout,
		PullInterval: DefaultPullInterval,
		MaxRetries:   DefaultMaxRetries,
		BufferSize:   DefaultBufferSize,
		MaxNodes:     DefaultMaxNodes,
		RPCTimeout:   DefaultRPCTimeout,
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if !c.UseRPCOnly && len(c.Entrypoints) == 0 {
		return ErrNoEntrypoints
	}

	if c.UseRPCOnly && c.RPCEndpoint == "" {
		return fmt.Errorf("%w: RPC endpoint required when UseRPCOnly is set", ErrInvalidConfig)
	}

	if c.Timeout <= 0 {
		return fmt.Errorf("%w: %v", ErrInvalidTimeout, c.Timeout)
	}

	if c.PullInterval <= 0 {
		return fmt.Errorf("%w: pull interval must be positive", ErrInvalidConfig)
	}

	if c.BufferSize <= 0 {
		return fmt.Errorf("%w: buffer size must be positive", ErrInvalidConfig)
	}

	if c.RPCTimeout <= 0 && c.RPCEndpoint != "" {
		return fmt.Errorf("%w: RPC timeout must be positive", ErrInvalidConfig)
	}

	return nil
}

// WithDefaults returns a new config with default values applied for any
// zero values in the original config.
func (c Config) WithDefaults() Config {
	defaults := DefaultConfig()

	if len(c.Entrypoints) == 0 {
		c.Entrypoints = defaults.Entrypoints
	}
	if c.Timeout == 0 {
		c.Timeout = defaults.Timeout
	}
	if c.PullInterval == 0 {
		c.PullInterval = defaults.PullInterval
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaults.MaxRetries
	}
	if c.BufferSize == 0 {
		c.BufferSize = defaults.BufferSize
	}
	if c.MaxNodes == 0 {
		c.MaxNodes = defaults.MaxNodes
	}
	if c.RPCTimeout == 0 {
		c.RPCTimeout = defaults.RPCTimeout
	}

	return c
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

// Entrypoints sets the gossip entrypoint addresses.
func (b *ConfigBuilder) Entrypoints(entrypoints ...string) *ConfigBuilder {
	b.config.Entrypoints = entrypoints
	return b
}

// AddEntrypoint adds a single entrypoint.
func (b *ConfigBuilder) AddEntrypoint(entrypoint string) *ConfigBuilder {
	b.config.Entrypoints = append(b.config.Entrypoints, entrypoint)
	return b
}

// Timeout sets the discovery timeout.
func (b *ConfigBuilder) Timeout(timeout time.Duration) *ConfigBuilder {
	b.config.Timeout = timeout
	return b
}

// PullInterval sets the pull request interval.
func (b *ConfigBuilder) PullInterval(interval time.Duration) *ConfigBuilder {
	b.config.PullInterval = interval
	return b
}

// MaxRetries sets the maximum number of retries.
func (b *ConfigBuilder) MaxRetries(retries int) *ConfigBuilder {
	b.config.MaxRetries = retries
	return b
}

// MaxNodes sets the maximum number of nodes to discover.
func (b *ConfigBuilder) MaxNodes(max int) *ConfigBuilder {
	b.config.MaxNodes = max
	return b
}

// LocalPort sets the local UDP port to bind to.
func (b *ConfigBuilder) LocalPort(port int) *ConfigBuilder {
	b.config.LocalPort = port
	return b
}

// RPCEndpoint sets the RPC endpoint for fallback discovery.
func (b *ConfigBuilder) RPCEndpoint(endpoint string) *ConfigBuilder {
	b.config.RPCEndpoint = endpoint
	return b
}

// RPCTimeout sets the RPC fallback timeout.
func (b *ConfigBuilder) RPCTimeout(timeout time.Duration) *ConfigBuilder {
	b.config.RPCTimeout = timeout
	return b
}

// UseRPCOnly enables RPC-only mode (skips gossip protocol).
func (b *ConfigBuilder) UseRPCOnly(rpcOnly bool) *ConfigBuilder {
	b.config.UseRPCOnly = rpcOnly
	return b
}

// ShredVersion filters nodes by shred version.
func (b *ConfigBuilder) ShredVersion(version uint16) *ConfigBuilder {
	b.config.ShredVersion = version
	return b
}

// OnNodeDiscovered sets the node discovery callback.
func (b *ConfigBuilder) OnNodeDiscovered(fn func(*Node)) *ConfigBuilder {
	b.config.OnNodeDiscovered = fn
	return b
}

// OnError sets the error callback.
func (b *ConfigBuilder) OnError(fn func(error)) *ConfigBuilder {
	b.config.OnError = fn
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
		panic(fmt.Sprintf("invalid gossip config: %v", err))
	}
	return cfg
}

// Presets for common network configurations.

// X1MainnetConfig returns a configuration for X1 mainnet.
func X1MainnetConfig() Config {
	return DefaultConfig()
}

// X1TestnetConfig returns a configuration for X1 testnet.
func X1TestnetConfig() Config {
	cfg := DefaultConfig()
	cfg.Entrypoints = []string{
		"entrypoint0.testnet.x1.xyz:8001",
		"entrypoint1.testnet.x1.xyz:8001",
		"entrypoint2.testnet.x1.xyz:8001",
	}
	return cfg
}

// X1DevnetConfig returns a configuration for X1 devnet.
func X1DevnetConfig() Config {
	cfg := DefaultConfig()
	cfg.Entrypoints = []string{
		"entrypoint0.devnet.x1.xyz:8001",
		"entrypoint1.devnet.x1.xyz:8001",
	}
	return cfg
}

// SolanaMainnetConfig returns a configuration for Solana mainnet.
func SolanaMainnetConfig() Config {
	cfg := DefaultConfig()
	cfg.Entrypoints = []string{
		"entrypoint.mainnet-beta.solana.com:8001",
		"entrypoint2.mainnet-beta.solana.com:8001",
		"entrypoint3.mainnet-beta.solana.com:8001",
		"entrypoint4.mainnet-beta.solana.com:8001",
		"entrypoint5.mainnet-beta.solana.com:8001",
	}
	return cfg
}
