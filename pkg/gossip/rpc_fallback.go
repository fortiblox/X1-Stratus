package gossip

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// RPC request/response types for getClusterNodes.
type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// clusterNode represents a node in the getClusterNodes response.
type clusterNode struct {
	Pubkey       string  `json:"pubkey"`
	Gossip       *string `json:"gossip"`
	TPU          *string `json:"tpu"`
	RPC          *string `json:"rpc"`
	Version      *string `json:"version"`
	FeatureSet   *uint32 `json:"featureSet"`
	ShredVersion *uint16 `json:"shredVersion"`
}

// discoverViaRPC performs node discovery using the RPC getClusterNodes method.
func (c *Client) discoverViaRPC(ctx context.Context) (*DiscoveryResult, error) {
	if c.config.RPCEndpoint == "" {
		return nil, fmt.Errorf("RPC endpoint not configured")
	}

	startTime := time.Now()
	result := &DiscoveryResult{
		Nodes: make([]*Node, 0),
	}

	nodes, err := GetClusterNodes(ctx, c.config.RPCEndpoint, c.config.RPCTimeout)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result, err
	}

	// Add nodes to the client's cache
	c.nodesMu.Lock()
	for _, node := range nodes {
		// Apply shred version filter
		if c.config.ShredVersion != 0 && node.ShredVersion != c.config.ShredVersion {
			continue
		}

		c.nodes[node.Identity] = node
		result.Nodes = append(result.Nodes, node)

		if c.config.OnNodeDiscovered != nil {
			c.config.OnNodeDiscovered(node)
		}

		if c.config.MaxNodes > 0 && len(result.Nodes) >= c.config.MaxNodes {
			break
		}
	}
	c.nodesMu.Unlock()

	result.Duration = time.Since(startTime)
	return result, nil
}

// GetClusterNodes queries an RPC endpoint for cluster nodes.
//
// This is the RPC fallback method that uses the Solana JSON-RPC API
// getClusterNodes method to discover validators. This is simpler than
// the gossip protocol and works when UDP is blocked.
func GetClusterNodes(ctx context.Context, rpcEndpoint string, timeout time.Duration) ([]*Node, error) {
	// Ensure endpoint has scheme
	if !strings.HasPrefix(rpcEndpoint, "http://") && !strings.HasPrefix(rpcEndpoint, "https://") {
		rpcEndpoint = "http://" + rpcEndpoint
	}

	// Create request
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "getClusterNodes",
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", rpcEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var rpcResp rpcResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	// Parse cluster nodes
	var clusterNodes []clusterNode
	if err := json.Unmarshal(rpcResp.Result, &clusterNodes); err != nil {
		return nil, fmt.Errorf("unmarshal cluster nodes: %w", err)
	}

	// Convert to Node structs
	nodes := make([]*Node, 0, len(clusterNodes))
	for _, cn := range clusterNodes {
		node, err := clusterNodeToNode(&cn)
		if err != nil {
			continue // Skip invalid nodes
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// clusterNodeToNode converts an RPC clusterNode to a Node.
func clusterNodeToNode(cn *clusterNode) (*Node, error) {
	pubkey, err := types.PubkeyFromBase58(cn.Pubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %w", err)
	}

	node := &Node{
		Identity: pubkey,
	}

	// Parse gossip address
	if cn.Gossip != nil && *cn.Gossip != "" {
		if addr, err := parseUDPAddr(*cn.Gossip); err == nil {
			node.GossipAddr = addr
		}
	}

	// Parse TPU address
	if cn.TPU != nil && *cn.TPU != "" {
		if addr, err := parseUDPAddr(*cn.TPU); err == nil {
			node.TPUAddr = addr
		}
	}

	// Set RPC address
	if cn.RPC != nil && *cn.RPC != "" {
		node.RPCAddr = *cn.RPC
	}

	// Set version
	if cn.Version != nil {
		node.Version = *cn.Version
	}

	// Set feature set
	if cn.FeatureSet != nil {
		node.FeatureSet = *cn.FeatureSet
	}

	// Set shred version
	if cn.ShredVersion != nil {
		node.ShredVersion = *cn.ShredVersion
	}

	return node, nil
}

// parseUDPAddr parses an address string into a UDP address.
func parseUDPAddr(addr string) (*net.UDPAddr, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// Try to resolve hostname
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("cannot resolve host: %s", host)
		}
		ip = ips[0]
	}

	port := 0
	for _, c := range portStr {
		if c < '0' || c > '9' {
			return nil, fmt.Errorf("invalid port: %s", portStr)
		}
		port = port*10 + int(c-'0')
	}

	return &net.UDPAddr{IP: ip, Port: port}, nil
}

// RPCDiscoverer provides a simple interface for RPC-only discovery.
type RPCDiscoverer struct {
	endpoint string
	timeout  time.Duration
}

// NewRPCDiscoverer creates a new RPC-based discoverer.
func NewRPCDiscoverer(endpoint string) *RPCDiscoverer {
	return &RPCDiscoverer{
		endpoint: endpoint,
		timeout:  DefaultRPCTimeout,
	}
}

// WithTimeout sets the RPC timeout.
func (d *RPCDiscoverer) WithTimeout(timeout time.Duration) *RPCDiscoverer {
	d.timeout = timeout
	return d
}

// Discover performs RPC-based node discovery.
func (d *RPCDiscoverer) Discover(ctx context.Context) ([]*Node, error) {
	return GetClusterNodes(ctx, d.endpoint, d.timeout)
}

// DiscoverRPC discovers RPC nodes only.
func (d *RPCDiscoverer) DiscoverRPC(ctx context.Context) ([]*Node, error) {
	nodes, err := d.Discover(ctx)
	if err != nil {
		return nil, err
	}

	rpcNodes := make([]*Node, 0)
	for _, node := range nodes {
		if node.HasRPC() {
			rpcNodes = append(rpcNodes, node)
		}
	}

	return rpcNodes, nil
}

// GetRPCEndpoints returns a list of RPC endpoint URLs.
func (d *RPCDiscoverer) GetRPCEndpoints(ctx context.Context) ([]string, error) {
	nodes, err := d.DiscoverRPC(ctx)
	if err != nil {
		return nil, err
	}

	endpoints := make([]string, 0, len(nodes))
	for _, node := range nodes {
		endpoints = append(endpoints, node.RPCURL())
	}

	return endpoints, nil
}

// QuickDiscover is a convenience function that performs quick RPC discovery
// from a single endpoint and returns the discovered nodes.
func QuickDiscover(ctx context.Context, rpcEndpoint string) ([]*Node, error) {
	return NewRPCDiscoverer(rpcEndpoint).Discover(ctx)
}

// QuickDiscoverRPCEndpoints is a convenience function that discovers
// and returns only RPC endpoint URLs.
func QuickDiscoverRPCEndpoints(ctx context.Context, rpcEndpoint string) ([]string, error) {
	return NewRPCDiscoverer(rpcEndpoint).GetRPCEndpoints(ctx)
}
