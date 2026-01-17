package gossip

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// Protocol constants.
const (
	// MaxGossipPacketSize is the maximum size of a gossip UDP packet.
	MaxGossipPacketSize = 1232

	// ContactInfoSize is the estimated size of a serialized ContactInfo.
	ContactInfoSize = 512
)

// Protocol errors.
var (
	ErrPacketTooSmall    = errors.New("packet too small")
	ErrPacketTooLarge    = errors.New("packet too large")
	ErrInvalidSignature  = errors.New("invalid signature")
	ErrUnknownMessage    = errors.New("unknown gossip message type")
	ErrMalformedPacket   = errors.New("malformed gossip packet")
	ErrInvalidIPAddress  = errors.New("invalid IP address in contact info")
)

// Codec handles encoding and decoding of gossip protocol messages.
type Codec struct{}

// NewCodec creates a new protocol codec.
func NewCodec() *Codec {
	return &Codec{}
}

// EncodePullRequest encodes a pull request message.
func (c *Codec) EncodePullRequest(req *PullRequest) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Message type
	buf.WriteByte(byte(MessageTypePullRequest))

	// Filter
	if err := c.encodeFilter(buf, &req.Filter); err != nil {
		return nil, fmt.Errorf("encode filter: %w", err)
	}

	// Self contact info value
	if err := c.encodeCRDSValue(buf, &req.Value); err != nil {
		return nil, fmt.Errorf("encode self value: %w", err)
	}

	return buf.Bytes(), nil
}

// EncodePingMessage encodes a ping message.
func (c *Codec) EncodePingMessage(ping *PingMessage) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Message type
	buf.WriteByte(byte(MessageTypePingMessage))

	// From pubkey
	buf.Write(ping.From[:])

	// Token
	buf.Write(ping.Token[:])

	// Signature
	buf.Write(ping.Signature[:])

	return buf.Bytes(), nil
}

// EncodePongMessage encodes a pong message.
func (c *Codec) EncodePongMessage(pong *PongMessage) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Message type
	buf.WriteByte(byte(MessageTypePongMessage))

	// From pubkey
	buf.Write(pong.From[:])

	// Hash
	buf.Write(pong.Hash[:])

	// Signature
	buf.Write(pong.Signature[:])

	return buf.Bytes(), nil
}

// DecodeMessage decodes a gossip message from raw bytes.
func (c *Codec) DecodeMessage(data []byte) (GossipMessageType, interface{}, error) {
	if len(data) < 1 {
		return 0, nil, ErrPacketTooSmall
	}

	msgType := GossipMessageType(data[0])
	payload := data[1:]

	switch msgType {
	case MessageTypePullResponse:
		resp, err := c.decodePullResponse(payload)
		return msgType, resp, err

	case MessageTypePushMessage:
		push, err := c.decodePushMessage(payload)
		return msgType, push, err

	case MessageTypePingMessage:
		ping, err := c.decodePingMessage(payload)
		return msgType, ping, err

	case MessageTypePongMessage:
		pong, err := c.decodePongMessage(payload)
		return msgType, pong, err

	default:
		// Unknown or unimplemented message type
		return msgType, nil, nil
	}
}

// decodePullResponse decodes a pull response message.
func (c *Codec) decodePullResponse(data []byte) (*PullResponse, error) {
	if len(data) < types.PubkeySize {
		return nil, ErrPacketTooSmall
	}

	resp := &PullResponse{}

	// From pubkey
	copy(resp.From[:], data[:types.PubkeySize])
	data = data[types.PubkeySize:]

	// Number of values (as varint)
	numValues, n := binary.Uvarint(data)
	if n <= 0 {
		return nil, ErrMalformedPacket
	}
	data = data[n:]

	// Decode each value
	for i := uint64(0); i < numValues && len(data) > 0; i++ {
		value, bytesRead, err := c.decodeCRDSValue(data)
		if err != nil {
			// Skip malformed values
			break
		}
		resp.Values = append(resp.Values, *value)
		data = data[bytesRead:]
	}

	return resp, nil
}

// decodePushMessage decodes a push message.
func (c *Codec) decodePushMessage(data []byte) (*PushMessage, error) {
	if len(data) < types.PubkeySize {
		return nil, ErrPacketTooSmall
	}

	push := &PushMessage{}

	// From pubkey
	copy(push.From[:], data[:types.PubkeySize])
	data = data[types.PubkeySize:]

	// Number of values
	numValues, n := binary.Uvarint(data)
	if n <= 0 {
		return nil, ErrMalformedPacket
	}
	data = data[n:]

	// Decode each value
	for i := uint64(0); i < numValues && len(data) > 0; i++ {
		value, bytesRead, err := c.decodeCRDSValue(data)
		if err != nil {
			break
		}
		push.Values = append(push.Values, *value)
		data = data[bytesRead:]
	}

	return push, nil
}

// decodePingMessage decodes a ping message.
func (c *Codec) decodePingMessage(data []byte) (*PingMessage, error) {
	minSize := types.PubkeySize + types.HashSize + types.SignatureSize
	if len(data) < minSize {
		return nil, ErrPacketTooSmall
	}

	ping := &PingMessage{}
	offset := 0

	// From pubkey
	copy(ping.From[:], data[offset:offset+types.PubkeySize])
	offset += types.PubkeySize

	// Token
	copy(ping.Token[:], data[offset:offset+types.HashSize])
	offset += types.HashSize

	// Signature
	copy(ping.Signature[:], data[offset:offset+types.SignatureSize])

	return ping, nil
}

// decodePongMessage decodes a pong message.
func (c *Codec) decodePongMessage(data []byte) (*PongMessage, error) {
	minSize := types.PubkeySize + types.HashSize + types.SignatureSize
	if len(data) < minSize {
		return nil, ErrPacketTooSmall
	}

	pong := &PongMessage{}
	offset := 0

	// From pubkey
	copy(pong.From[:], data[offset:offset+types.PubkeySize])
	offset += types.PubkeySize

	// Hash
	copy(pong.Hash[:], data[offset:offset+types.HashSize])
	offset += types.HashSize

	// Signature
	copy(pong.Signature[:], data[offset:offset+types.SignatureSize])

	return pong, nil
}

// encodeFilter encodes a CRDS bloom filter.
func (c *Codec) encodeFilter(buf *bytes.Buffer, filter *CRDSFilter) error {
	// Filter bits length
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(filter.Filter))); err != nil {
		return err
	}

	// Filter bits
	buf.Write(filter.Filter)

	// Hash index
	if err := binary.Write(buf, binary.LittleEndian, filter.HashIndex); err != nil {
		return err
	}

	// Mask bits
	if err := binary.Write(buf, binary.LittleEndian, filter.MaskBits); err != nil {
		return err
	}

	return nil
}

// encodeCRDSValue encodes a CRDS value.
func (c *Codec) encodeCRDSValue(buf *bytes.Buffer, value *CRDSValue) error {
	// Signature
	buf.Write(value.Signature[:])

	// Data type and content
	switch data := value.Data.(type) {
	case *CRDSContactInfo:
		if err := c.encodeContactInfo(buf, &data.ContactInfo); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported CRDS data type: %T", value.Data)
	}

	return nil
}

// encodeContactInfo encodes a ContactInfo structure.
func (c *Codec) encodeContactInfo(buf *bytes.Buffer, info *ContactInfo) error {
	// CRDS data type (0 = ContactInfo)
	buf.WriteByte(0)

	// Pubkey
	buf.Write(info.Pubkey[:])

	// Wall clock (milliseconds)
	binary.Write(buf, binary.LittleEndian, info.WallClock)

	// Shred version
	binary.Write(buf, binary.LittleEndian, info.ShredVersion)

	// Version
	c.encodeVersion(buf, &info.Version)

	// Socket addresses
	c.encodeSocketAddr(buf, &info.Gossip)
	c.encodeSocketAddr(buf, &info.TVU)
	c.encodeSocketAddr(buf, &info.TVUForwards)
	c.encodeSocketAddr(buf, &info.Repair)
	c.encodeSocketAddr(buf, &info.TPU)
	c.encodeSocketAddr(buf, &info.TPUForwards)
	c.encodeSocketAddr(buf, &info.TPUVote)
	c.encodeSocketAddr(buf, &info.RPC)
	c.encodeSocketAddr(buf, &info.RPCPubsub)
	c.encodeSocketAddr(buf, &info.ServeRepair)

	return nil
}

// encodeVersion encodes a Version structure.
func (c *Codec) encodeVersion(buf *bytes.Buffer, v *Version) {
	binary.Write(buf, binary.LittleEndian, v.Major)
	binary.Write(buf, binary.LittleEndian, v.Minor)
	binary.Write(buf, binary.LittleEndian, v.Patch)
	binary.Write(buf, binary.LittleEndian, v.Commit)
	binary.Write(buf, binary.LittleEndian, v.FeatureSet)
}

// encodeSocketAddr encodes a socket address.
func (c *Codec) encodeSocketAddr(buf *bytes.Buffer, addr *SocketAddr) {
	if !addr.IsValid() {
		// Write empty indicator
		buf.WriteByte(0)
		return
	}

	// Write presence indicator
	buf.WriteByte(1)

	// IPv4 or IPv6
	ip4 := addr.IP.To4()
	if ip4 != nil {
		buf.WriteByte(4) // IPv4
		buf.Write(ip4)
	} else {
		buf.WriteByte(6) // IPv6
		buf.Write(addr.IP.To16())
	}

	// Port
	binary.Write(buf, binary.LittleEndian, addr.Port)
}

// decodeCRDSValue decodes a CRDS value and returns bytes read.
func (c *Codec) decodeCRDSValue(data []byte) (*CRDSValue, int, error) {
	if len(data) < types.SignatureSize+1 {
		return nil, 0, ErrPacketTooSmall
	}

	value := &CRDSValue{}
	offset := 0

	// Signature
	copy(value.Signature[:], data[offset:offset+types.SignatureSize])
	offset += types.SignatureSize

	// Data type
	dataType := data[offset]
	offset++

	switch dataType {
	case 0: // ContactInfo
		info, bytesRead, err := c.decodeContactInfo(data[offset:])
		if err != nil {
			return nil, 0, err
		}
		value.Data = &CRDSContactInfo{ContactInfo: *info}
		offset += bytesRead

	default:
		// Skip unknown data types
		return nil, 0, fmt.Errorf("unknown CRDS data type: %d", dataType)
	}

	return value, offset, nil
}

// decodeContactInfo decodes a ContactInfo structure.
func (c *Codec) decodeContactInfo(data []byte) (*ContactInfo, int, error) {
	minSize := types.PubkeySize + 8 + 2 // pubkey + wallclock + shred version
	if len(data) < minSize {
		return nil, 0, ErrPacketTooSmall
	}

	info := &ContactInfo{}
	offset := 0

	// Pubkey
	copy(info.Pubkey[:], data[offset:offset+types.PubkeySize])
	offset += types.PubkeySize

	// Wall clock
	info.WallClock = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// Shred version
	info.ShredVersion = binary.LittleEndian.Uint16(data[offset:])
	offset += 2

	// Version (if present)
	if len(data) > offset+10 {
		info.Version, offset = c.decodeVersion(data, offset)
	}

	// Socket addresses
	var err error
	info.Gossip, offset, err = c.decodeSocketAddr(data, offset)
	if err != nil {
		return info, offset, nil // Partial decode is OK
	}

	info.TVU, offset, _ = c.decodeSocketAddr(data, offset)
	info.TVUForwards, offset, _ = c.decodeSocketAddr(data, offset)
	info.Repair, offset, _ = c.decodeSocketAddr(data, offset)
	info.TPU, offset, _ = c.decodeSocketAddr(data, offset)
	info.TPUForwards, offset, _ = c.decodeSocketAddr(data, offset)
	info.TPUVote, offset, _ = c.decodeSocketAddr(data, offset)
	info.RPC, offset, _ = c.decodeSocketAddr(data, offset)
	info.RPCPubsub, offset, _ = c.decodeSocketAddr(data, offset)
	info.ServeRepair, offset, _ = c.decodeSocketAddr(data, offset)

	return info, offset, nil
}

// decodeVersion decodes a Version structure.
func (c *Codec) decodeVersion(data []byte, offset int) (Version, int) {
	var v Version
	if len(data) < offset+14 { // 2+2+2+4+4 bytes
		return v, offset
	}

	v.Major = binary.LittleEndian.Uint16(data[offset:])
	offset += 2
	v.Minor = binary.LittleEndian.Uint16(data[offset:])
	offset += 2
	v.Patch = binary.LittleEndian.Uint16(data[offset:])
	offset += 2
	v.Commit = binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	v.FeatureSet = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	return v, offset
}

// decodeSocketAddr decodes a socket address.
func (c *Codec) decodeSocketAddr(data []byte, offset int) (SocketAddr, int, error) {
	var addr SocketAddr

	if len(data) <= offset {
		return addr, offset, ErrPacketTooSmall
	}

	// Presence indicator
	if data[offset] == 0 {
		return addr, offset + 1, nil
	}
	offset++

	if len(data) <= offset {
		return addr, offset, ErrPacketTooSmall
	}

	// IP version
	ipVersion := data[offset]
	offset++

	switch ipVersion {
	case 4:
		if len(data) < offset+4+2 {
			return addr, offset, ErrPacketTooSmall
		}
		addr.IP = net.IPv4(data[offset], data[offset+1], data[offset+2], data[offset+3])
		offset += 4

	case 6:
		if len(data) < offset+16+2 {
			return addr, offset, ErrPacketTooSmall
		}
		addr.IP = make(net.IP, 16)
		copy(addr.IP, data[offset:offset+16])
		offset += 16

	default:
		return addr, offset, ErrInvalidIPAddress
	}

	// Port
	addr.Port = binary.LittleEndian.Uint16(data[offset:])
	offset += 2

	return addr, offset, nil
}

// CreatePullRequest creates a pull request with the given self contact info.
func CreatePullRequest(selfInfo *ContactInfo, privateKey ed25519.PrivateKey) (*PullRequest, error) {
	// Create an empty bloom filter (we want all data)
	filter := CRDSFilter{
		Filter:    make([]byte, 128), // Empty filter accepts everything
		HashIndex: 0,
		MaskBits:  0,
	}

	// Sign the contact info
	infoBytes := contactInfoToBytes(selfInfo)
	sig := ed25519.Sign(privateKey, infoBytes)

	var signature types.Signature
	copy(signature[:], sig)

	value := CRDSValue{
		Signature: signature,
		Data:      &CRDSContactInfo{ContactInfo: *selfInfo},
	}

	return &PullRequest{
		Filter: filter,
		Value:  value,
	}, nil
}

// CreatePingMessage creates a signed ping message.
func CreatePingMessage(from types.Pubkey, privateKey ed25519.PrivateKey) (*PingMessage, error) {
	// Generate random token
	token := types.ComputeHash([]byte(time.Now().String()))

	// Sign the token
	sig := ed25519.Sign(privateKey, token[:])

	var signature types.Signature
	copy(signature[:], sig)

	return &PingMessage{
		From:      from,
		Token:     token,
		Signature: signature,
	}, nil
}

// CreatePongMessage creates a signed pong message in response to a ping.
func CreatePongMessage(from types.Pubkey, pingToken types.Hash, privateKey ed25519.PrivateKey) (*PongMessage, error) {
	// Hash the ping token
	hash := types.ComputeHash(pingToken[:])

	// Sign the hash
	sig := ed25519.Sign(privateKey, hash[:])

	var signature types.Signature
	copy(signature[:], sig)

	return &PongMessage{
		From:      from,
		Hash:      hash,
		Signature: signature,
	}, nil
}

// contactInfoToBytes serializes ContactInfo for signing.
func contactInfoToBytes(info *ContactInfo) []byte {
	buf := new(bytes.Buffer)
	buf.Write(info.Pubkey[:])
	binary.Write(buf, binary.LittleEndian, info.WallClock)
	binary.Write(buf, binary.LittleEndian, info.ShredVersion)
	return buf.Bytes()
}

// ContactInfoToNode converts a ContactInfo to a Node.
func ContactInfoToNode(info *ContactInfo) *Node {
	node := &Node{
		Identity:     info.Pubkey,
		ShredVersion: info.ShredVersion,
		FeatureSet:   info.Version.FeatureSet,
		Version:      info.Version.String(),
		WallClock:    time.UnixMilli(int64(info.WallClock)),
	}

	if info.Gossip.IsValid() {
		node.GossipAddr = info.Gossip.ToUDPAddr()
	}

	if info.TPU.IsValid() {
		node.TPUAddr = info.TPU.ToUDPAddr()
	}

	if info.TPUForwards.IsValid() {
		node.TPUForwardsAddr = info.TPUForwards.ToUDPAddr()
	}

	if info.RPC.IsValid() {
		node.RPCAddr = info.RPC.String()
	}

	if info.RPCPubsub.IsValid() {
		node.PubsubAddr = info.RPCPubsub.String()
	}

	return node
}
