// Package gossip implements a Solana gossip protocol client for validator discovery.
//
// The gossip protocol is used by Solana/X1 validators to discover each other and
// exchange cluster information. This package provides functionality to:
//   - Connect to X1 network entrypoints
//   - Discover active validators in the cluster
//   - Extract validator information including RPC endpoints
//
// The implementation supports both native UDP gossip protocol and a fallback
// RPC-based discovery using the getClusterNodes method.
package gossip

import (
	"net"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// Node represents a discovered validator node in the cluster.
type Node struct {
	// Identity is the node's Ed25519 public key.
	Identity types.Pubkey

	// GossipAddr is the address for gossip protocol (UDP).
	GossipAddr *net.UDPAddr

	// TPUAddr is the address for Transaction Processing Unit.
	TPUAddr *net.UDPAddr

	// TPUForwardsAddr is the address for TPU forwarding.
	TPUForwardsAddr *net.UDPAddr

	// RPCAddr is the HTTP RPC endpoint address (if exposed).
	// This is what clients typically connect to.
	RPCAddr string

	// PubsubAddr is the WebSocket pubsub endpoint address (if exposed).
	PubsubAddr string

	// Version is the node's software version.
	Version string

	// ShredVersion is the cluster shred version for network compatibility.
	ShredVersion uint16

	// FeatureSet is the feature set hash for compatibility checking.
	FeatureSet uint32

	// WallClock is the timestamp of the last contact info update.
	WallClock time.Time
}

// HasRPC returns true if the node exposes an RPC endpoint.
func (n *Node) HasRPC() bool {
	return n.RPCAddr != ""
}

// RPCURL returns the full HTTP URL for the RPC endpoint.
func (n *Node) RPCURL() string {
	if n.RPCAddr == "" {
		return ""
	}
	return "http://" + n.RPCAddr
}

// ContactInfo represents the CRDS ContactInfo structure used in gossip.
// This is the primary data structure validators advertise about themselves.
type ContactInfo struct {
	// Pubkey is the node's identity public key.
	Pubkey types.Pubkey

	// WallClock is the timestamp in milliseconds since Unix epoch.
	WallClock uint64

	// Shred version for network compatibility.
	ShredVersion uint16

	// Version information.
	Version Version

	// Socket addresses for various services.
	Gossip       SocketAddr
	TPU          SocketAddr
	TPUForwards  SocketAddr
	TPUVote      SocketAddr
	Repair       SocketAddr
	ServeRepair  SocketAddr
	RPC          SocketAddr
	RPCPubsub    SocketAddr
	TVU          SocketAddr
	TVUForwards  SocketAddr
}

// SocketAddr represents an IP:port combination.
type SocketAddr struct {
	IP   net.IP
	Port uint16
}

// IsValid returns true if the socket address is set.
func (s SocketAddr) IsValid() bool {
	return len(s.IP) > 0 && s.Port > 0
}

// String returns the address as "IP:port".
func (s SocketAddr) String() string {
	if !s.IsValid() {
		return ""
	}
	return net.JoinHostPort(s.IP.String(), itoa(int(s.Port)))
}

// ToUDPAddr converts to *net.UDPAddr.
func (s SocketAddr) ToUDPAddr() *net.UDPAddr {
	if !s.IsValid() {
		return nil
	}
	return &net.UDPAddr{
		IP:   s.IP,
		Port: int(s.Port),
	}
}

// Version represents a Solana node version.
type Version struct {
	Major      uint16
	Minor      uint16
	Patch      uint16
	Commit     uint32
	FeatureSet uint32
}

// String returns the version as "major.minor.patch".
func (v Version) String() string {
	return itoa(int(v.Major)) + "." + itoa(int(v.Minor)) + "." + itoa(int(v.Patch))
}

// CRDSValue represents a value in the Cluster Replicated Data Store.
type CRDSValue struct {
	// Signature of the data by the origin node.
	Signature types.Signature

	// Data is the actual CRDS data (ContactInfo, Vote, etc.)
	Data CRDSData
}

// CRDSData is the interface for different CRDS data types.
type CRDSData interface {
	crdsData()
}

// CRDSContactInfo is a ContactInfo stored in CRDS.
type CRDSContactInfo struct {
	ContactInfo
}

func (CRDSContactInfo) crdsData() {}

// CRDSVote is a vote stored in CRDS.
type CRDSVote struct {
	Index uint8
	From  types.Pubkey
	Slot  uint64
	Hash  types.Hash
}

func (CRDSVote) crdsData() {}

// CRDSEpochSlots tracks slots a node has for an epoch.
type CRDSEpochSlots struct {
	From       types.Pubkey
	Slots      []uint64
	WallClock  uint64
}

func (CRDSEpochSlots) crdsData() {}

// GossipMessage types used in the protocol.
type GossipMessageType uint8

const (
	// PullRequest requests CRDS data from a peer.
	MessageTypePullRequest GossipMessageType = 0

	// PullResponse is the response to a pull request.
	MessageTypePullResponse GossipMessageType = 1

	// PushMessage pushes new CRDS values to peers.
	MessageTypePushMessage GossipMessageType = 2

	// PruneMessage requests pruning of push messages.
	MessageTypePruneMessage GossipMessageType = 3

	// PingMessage is used for liveness checks.
	MessageTypePingMessage GossipMessageType = 4

	// PongMessage is the response to a ping.
	MessageTypePongMessage GossipMessageType = 5
)

// String returns the message type name.
func (t GossipMessageType) String() string {
	switch t {
	case MessageTypePullRequest:
		return "PullRequest"
	case MessageTypePullResponse:
		return "PullResponse"
	case MessageTypePushMessage:
		return "PushMessage"
	case MessageTypePruneMessage:
		return "PruneMessage"
	case MessageTypePingMessage:
		return "PingMessage"
	case MessageTypePongMessage:
		return "PongMessage"
	default:
		return "Unknown"
	}
}

// PullRequest is sent to request CRDS data matching a filter.
type PullRequest struct {
	// Filter is a bloom filter for CRDS keys the sender already has.
	Filter CRDSFilter

	// Value is the sender's ContactInfo.
	Value CRDSValue
}

// PullResponse contains CRDS values matching the pull request.
type PullResponse struct {
	// From is the sender's public key.
	From types.Pubkey

	// Values are the CRDS values matching the request.
	Values []CRDSValue
}

// PushMessage pushes new CRDS values to peers.
type PushMessage struct {
	// From is the sender's public key.
	From types.Pubkey

	// Values are the CRDS values being pushed.
	Values []CRDSValue
}

// PingMessage is used for liveness checking.
type PingMessage struct {
	// From is the sender's public key.
	From types.Pubkey

	// Token is a random token that must be returned in the pong.
	Token types.Hash

	// Signature of the ping by the sender.
	Signature types.Signature
}

// PongMessage is the response to a ping.
type PongMessage struct {
	// From is the sender's public key.
	From types.Pubkey

	// Hash is the hash of the ping token.
	Hash types.Hash

	// Signature of the pong by the sender.
	Signature types.Signature
}

// CRDSFilter is a bloom filter for CRDS keys.
type CRDSFilter struct {
	// Filter is the bloom filter bits.
	Filter []byte

	// HashIndex selects which hash function to use.
	HashIndex uint64

	// Mask for the filter.
	MaskBits uint32
}

// DiscoveryResult contains the results of a node discovery operation.
type DiscoveryResult struct {
	// Nodes is the list of discovered nodes.
	Nodes []*Node

	// Duration is how long the discovery took.
	Duration time.Duration

	// Errors encountered during discovery (non-fatal).
	Errors []error
}

// RPCNodes returns only nodes that have RPC endpoints.
func (r *DiscoveryResult) RPCNodes() []*Node {
	var result []*Node
	for _, n := range r.Nodes {
		if n.HasRPC() {
			result = append(result, n)
		}
	}
	return result
}

// NodeCount returns the total number of discovered nodes.
func (r *DiscoveryResult) NodeCount() int {
	return len(r.Nodes)
}

// RPCNodeCount returns the number of nodes with RPC endpoints.
func (r *DiscoveryResult) RPCNodeCount() int {
	count := 0
	for _, n := range r.Nodes {
		if n.HasRPC() {
			count++
		}
	}
	return count
}

// Helper function to convert int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := i < 0
	if negative {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
