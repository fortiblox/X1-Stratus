package gossip

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// Client errors.
var (
	ErrClosed           = errors.New("gossip client is closed")
	ErrDiscoveryFailed  = errors.New("node discovery failed")
	ErrNoNodesFound     = errors.New("no nodes discovered")
	ErrConnectionFailed = errors.New("failed to connect to entrypoints")
)

// Client is a gossip protocol client for discovering validators.
//
// The client connects to X1 network entrypoints and uses the Solana gossip
// protocol to discover active validators. It supports both native UDP gossip
// and an RPC fallback using getClusterNodes.
type Client struct {
	config Config
	codec  *Codec

	// UDP socket
	conn *net.UDPConn

	// Identity keypair for signing messages
	publicKey  types.Pubkey
	privateKey ed25519.PrivateKey

	// Discovered nodes
	nodesMu sync.RWMutex
	nodes   map[types.Pubkey]*Node

	// State
	closed  atomic.Bool
	running atomic.Bool

	// Cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// NewClient creates a new gossip client with the given configuration.
func NewClient(config Config) (*Client, error) {
	cfg := config.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Generate ephemeral keypair for this client
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate keypair: %w", err)
	}

	var publicKey types.Pubkey
	copy(publicKey[:], pub)

	return &Client{
		config:     cfg,
		codec:      NewCodec(),
		publicKey:  publicKey,
		privateKey: priv,
		nodes:      make(map[types.Pubkey]*Node),
	}, nil
}

// Discover performs node discovery and returns the results.
//
// This method connects to the configured entrypoints, sends gossip pull
// requests, and collects responses containing contact information for
// validators in the cluster.
//
// If gossip discovery fails or UseRPCOnly is set, it falls back to the
// RPC getClusterNodes method if an RPC endpoint is configured.
func (c *Client) Discover(ctx context.Context) (*DiscoveryResult, error) {
	if c.closed.Load() {
		return nil, ErrClosed
	}

	startTime := time.Now()
	result := &DiscoveryResult{
		Nodes: make([]*Node, 0),
	}

	// Use RPC-only mode if configured
	if c.config.UseRPCOnly {
		return c.discoverViaRPC(ctx)
	}

	// Try gossip discovery first
	gossipCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	err := c.discoverViaGossip(gossipCtx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("gossip discovery: %w", err))

		// Try RPC fallback if configured
		if c.config.RPCEndpoint != "" {
			rpcResult, rpcErr := c.discoverViaRPC(ctx)
			if rpcErr != nil {
				result.Errors = append(result.Errors, fmt.Errorf("RPC fallback: %w", rpcErr))
			} else {
				result.Nodes = rpcResult.Nodes
				result.Errors = append(result.Errors, rpcResult.Errors...)
			}
		}
	}

	// Collect discovered nodes
	c.nodesMu.RLock()
	for _, node := range c.nodes {
		if c.config.ShredVersion == 0 || node.ShredVersion == c.config.ShredVersion {
			result.Nodes = append(result.Nodes, node)
		}
		if c.config.MaxNodes > 0 && len(result.Nodes) >= c.config.MaxNodes {
			break
		}
	}
	c.nodesMu.RUnlock()

	result.Duration = time.Since(startTime)

	if len(result.Nodes) == 0 && len(result.Errors) > 0 {
		return result, ErrNoNodesFound
	}

	return result, nil
}

// discoverViaGossip performs discovery using the native gossip protocol.
func (c *Client) discoverViaGossip(ctx context.Context) error {
	// Create UDP socket
	localAddr := &net.UDPAddr{Port: c.config.LocalPort}
	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}
	defer conn.Close()

	c.conn = conn

	// Set read buffer size
	if err := conn.SetReadBuffer(c.config.BufferSize); err != nil {
		if c.config.OnError != nil {
			c.config.OnError(fmt.Errorf("set read buffer: %w", err))
		}
	}

	// Start receiver goroutine
	var wg sync.WaitGroup
	receiverCtx, cancelReceiver := context.WithCancel(ctx)
	defer cancelReceiver()

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.receiveLoop(receiverCtx, conn)
	}()

	// Create self contact info
	selfInfo := c.createSelfContactInfo()

	// Connect to entrypoints
	connectedEntrypoints := 0
	for _, entrypoint := range c.config.Entrypoints {
		addr, err := net.ResolveUDPAddr("udp", entrypoint)
		if err != nil {
			if c.config.OnError != nil {
				c.config.OnError(fmt.Errorf("resolve %s: %w", entrypoint, err))
			}
			continue
		}

		// Send pull request
		if err := c.sendPullRequest(conn, addr, selfInfo); err != nil {
			if c.config.OnError != nil {
				c.config.OnError(fmt.Errorf("send to %s: %w", entrypoint, err))
			}
			continue
		}

		connectedEntrypoints++
	}

	if connectedEntrypoints == 0 {
		return ErrConnectionFailed
	}

	// Send periodic pull requests
	ticker := time.NewTicker(c.config.PullInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			cancelReceiver()
			wg.Wait()
			return ctx.Err()

		case <-ticker.C:
			// Resend pull requests to entrypoints
			for _, entrypoint := range c.config.Entrypoints {
				addr, err := net.ResolveUDPAddr("udp", entrypoint)
				if err != nil {
					continue
				}
				c.sendPullRequest(conn, addr, selfInfo)
			}

			// Also send to discovered nodes (gossip propagation)
			c.nodesMu.RLock()
			nodeAddrs := make([]*net.UDPAddr, 0, len(c.nodes))
			for _, node := range c.nodes {
				if node.GossipAddr != nil {
					nodeAddrs = append(nodeAddrs, node.GossipAddr)
				}
			}
			c.nodesMu.RUnlock()

			for i, addr := range nodeAddrs {
				if i >= 10 { // Limit to 10 peers per round
					break
				}
				c.sendPullRequest(conn, addr, selfInfo)
			}
		}
	}
}

// receiveLoop receives and processes incoming gossip messages.
func (c *Client) receiveLoop(ctx context.Context, conn *net.UDPConn) {
	buf := make([]byte, c.config.BufferSize)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read deadline for cancellation check
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if c.config.OnError != nil {
				c.config.OnError(fmt.Errorf("read UDP: %w", err))
			}
			continue
		}

		if n == 0 {
			continue
		}

		// Decode and process message
		msgType, msg, err := c.codec.DecodeMessage(buf[:n])
		if err != nil {
			continue
		}

		c.processMessage(msgType, msg)
	}
}

// processMessage processes a decoded gossip message.
func (c *Client) processMessage(msgType GossipMessageType, msg interface{}) {
	switch msgType {
	case MessageTypePullResponse:
		if resp, ok := msg.(*PullResponse); ok {
			c.processPullResponse(resp)
		}

	case MessageTypePushMessage:
		if push, ok := msg.(*PushMessage); ok {
			c.processPushMessage(push)
		}

	case MessageTypePingMessage:
		// We could respond with pong, but for discovery we can ignore pings

	case MessageTypePongMessage:
		// Pong confirms liveness, we can ignore for simple discovery
	}
}

// processPullResponse processes a pull response containing CRDS values.
func (c *Client) processPullResponse(resp *PullResponse) {
	for _, value := range resp.Values {
		c.processCRDSValue(&value)
	}
}

// processPushMessage processes a push message containing CRDS values.
func (c *Client) processPushMessage(push *PushMessage) {
	for _, value := range push.Values {
		c.processCRDSValue(&value)
	}
}

// processCRDSValue processes a single CRDS value.
func (c *Client) processCRDSValue(value *CRDSValue) {
	switch data := value.Data.(type) {
	case *CRDSContactInfo:
		node := ContactInfoToNode(&data.ContactInfo)

		c.nodesMu.Lock()
		existing, exists := c.nodes[node.Identity]
		if !exists || node.WallClock.After(existing.WallClock) {
			c.nodes[node.Identity] = node

			if c.config.OnNodeDiscovered != nil && !exists {
				c.config.OnNodeDiscovered(node)
			}
		}
		c.nodesMu.Unlock()
	}
}

// sendPullRequest sends a pull request to the given address.
func (c *Client) sendPullRequest(conn *net.UDPConn, addr *net.UDPAddr, selfInfo *ContactInfo) error {
	req, err := CreatePullRequest(selfInfo, c.privateKey)
	if err != nil {
		return fmt.Errorf("create pull request: %w", err)
	}

	data, err := c.codec.EncodePullRequest(req)
	if err != nil {
		return fmt.Errorf("encode pull request: %w", err)
	}

	_, err = conn.WriteToUDP(data, addr)
	return err
}

// createSelfContactInfo creates a ContactInfo for this client.
func (c *Client) createSelfContactInfo() *ContactInfo {
	localIP := getLocalIP()

	return &ContactInfo{
		Pubkey:       c.publicKey,
		WallClock:    uint64(time.Now().UnixMilli()),
		ShredVersion: c.config.ShredVersion,
		Version: Version{
			Major:      1,
			Minor:      0,
			Patch:      0,
			FeatureSet: 0,
		},
		Gossip: SocketAddr{
			IP:   localIP,
			Port: uint16(c.config.LocalPort),
		},
	}
}

// GetNodes returns a copy of all discovered nodes.
func (c *Client) GetNodes() []*Node {
	c.nodesMu.RLock()
	defer c.nodesMu.RUnlock()

	nodes := make([]*Node, 0, len(c.nodes))
	for _, node := range c.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetRPCNodes returns only nodes that have RPC endpoints.
func (c *Client) GetRPCNodes() []*Node {
	c.nodesMu.RLock()
	defer c.nodesMu.RUnlock()

	nodes := make([]*Node, 0)
	for _, node := range c.nodes {
		if node.HasRPC() {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// GetNode returns a specific node by identity pubkey.
func (c *Client) GetNode(identity types.Pubkey) *Node {
	c.nodesMu.RLock()
	defer c.nodesMu.RUnlock()
	return c.nodes[identity]
}

// NodeCount returns the number of discovered nodes.
func (c *Client) NodeCount() int {
	c.nodesMu.RLock()
	defer c.nodesMu.RUnlock()
	return len(c.nodes)
}

// Close closes the client and releases resources.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil
	}

	if c.cancel != nil {
		c.cancel()
	}

	if c.conn != nil {
		c.conn.Close()
	}

	return nil
}

// getLocalIP returns the local IP address for gossiping.
func getLocalIP() net.IP {
	// Try to get an actual local IP
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ip4 := ipnet.IP.To4(); ip4 != nil {
					return ip4
				}
			}
		}
	}
	return net.IPv4(127, 0, 0, 1)
}

// DiscoverRPCEndpoints is a convenience function that performs discovery
// and returns only the RPC endpoint URLs.
func DiscoverRPCEndpoints(ctx context.Context, config Config) ([]string, error) {
	client, err := NewClient(config)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	result, err := client.Discover(ctx)
	if err != nil {
		return nil, err
	}

	endpoints := make([]string, 0)
	for _, node := range result.RPCNodes() {
		endpoints = append(endpoints, node.RPCURL())
	}

	return endpoints, nil
}

// DiscoverValidators is a convenience function that performs discovery
// and returns all discovered validators.
func DiscoverValidators(ctx context.Context, config Config) ([]*Node, error) {
	client, err := NewClient(config)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	result, err := client.Discover(ctx)
	if err != nil && len(result.Nodes) == 0 {
		return nil, err
	}

	return result.Nodes, nil
}
