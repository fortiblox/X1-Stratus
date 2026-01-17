// Package types defines core cryptographic and blockchain types for X1-Stratus.
//
// These types follow Solana conventions and are compatible with the X1 network.
// All types implement proper encoding/decoding for bincode and borsh serialization.
package types

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base58"
	"encoding/hex"
	"errors"
	"fmt"
)

// Size constants for core types.
const (
	PubkeySize    = 32
	SignatureSize = 64
	HashSize      = 32
)

var (
	// ErrInvalidPubkey is returned when a pubkey has invalid length.
	ErrInvalidPubkey = errors.New("invalid pubkey: must be 32 bytes")

	// ErrInvalidSignature is returned when a signature has invalid length.
	ErrInvalidSignature = errors.New("invalid signature: must be 64 bytes")

	// ErrInvalidHash is returned when a hash has invalid length.
	ErrInvalidHash = errors.New("invalid hash: must be 32 bytes")

	// ErrSignatureVerificationFailed is returned when signature verification fails.
	ErrSignatureVerificationFailed = errors.New("signature verification failed")
)

// Pubkey represents a 32-byte Ed25519 public key.
type Pubkey [PubkeySize]byte

// PubkeyFromBase58 parses a base58-encoded public key.
func PubkeyFromBase58(s string) (Pubkey, error) {
	var p Pubkey
	data, err := base58.Decode(s)
	if err != nil {
		return p, fmt.Errorf("base58 decode: %w", err)
	}
	if len(data) != PubkeySize {
		return p, ErrInvalidPubkey
	}
	copy(p[:], data)
	return p, nil
}

// PubkeyFromBytes creates a Pubkey from a byte slice.
func PubkeyFromBytes(b []byte) (Pubkey, error) {
	var p Pubkey
	if len(b) != PubkeySize {
		return p, ErrInvalidPubkey
	}
	copy(p[:], b)
	return p, nil
}

// String returns the base58-encoded representation.
func (p Pubkey) String() string {
	return base58.Encode(p[:])
}

// IsZero returns true if the pubkey is all zeros.
func (p Pubkey) IsZero() bool {
	for _, b := range p {
		if b != 0 {
			return false
		}
	}
	return true
}

// Equals returns true if two pubkeys are equal.
func (p Pubkey) Equals(other Pubkey) bool {
	return p == other
}

// Bytes returns the pubkey as a byte slice.
func (p Pubkey) Bytes() []byte {
	return p[:]
}

// MarshalText implements encoding.TextMarshaler.
func (p Pubkey) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (p *Pubkey) UnmarshalText(text []byte) error {
	parsed, err := PubkeyFromBase58(string(text))
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

// Signature represents a 64-byte Ed25519 signature.
type Signature [SignatureSize]byte

// SignatureFromBase58 parses a base58-encoded signature.
func SignatureFromBase58(s string) (Signature, error) {
	var sig Signature
	data, err := base58.Decode(s)
	if err != nil {
		return sig, fmt.Errorf("base58 decode: %w", err)
	}
	if len(data) != SignatureSize {
		return sig, ErrInvalidSignature
	}
	copy(sig[:], data)
	return sig, nil
}

// SignatureFromBytes creates a Signature from a byte slice.
func SignatureFromBytes(b []byte) (Signature, error) {
	var sig Signature
	if len(b) != SignatureSize {
		return sig, ErrInvalidSignature
	}
	copy(sig[:], b)
	return sig, nil
}

// String returns the base58-encoded representation.
func (s Signature) String() string {
	return base58.Encode(s[:])
}

// IsZero returns true if the signature is all zeros.
func (s Signature) IsZero() bool {
	for _, b := range s {
		if b != 0 {
			return false
		}
	}
	return true
}

// Verify verifies this signature against a message and public key.
func (s Signature) Verify(pubkey Pubkey, message []byte) bool {
	return ed25519.Verify(pubkey[:], message, s[:])
}

// Bytes returns the signature as a byte slice.
func (s Signature) Bytes() []byte {
	return s[:]
}

// MarshalText implements encoding.TextMarshaler.
func (s Signature) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// Hash represents a 32-byte SHA256 hash.
type Hash [HashSize]byte

// HashFromBase58 parses a base58-encoded hash.
func HashFromBase58(s string) (Hash, error) {
	var h Hash
	data, err := base58.Decode(s)
	if err != nil {
		return h, fmt.Errorf("base58 decode: %w", err)
	}
	if len(data) != HashSize {
		return h, ErrInvalidHash
	}
	copy(h[:], data)
	return h, nil
}

// HashFromHex parses a hex-encoded hash.
func HashFromHex(s string) (Hash, error) {
	var h Hash
	data, err := hex.DecodeString(s)
	if err != nil {
		return h, fmt.Errorf("hex decode: %w", err)
	}
	if len(data) != HashSize {
		return h, ErrInvalidHash
	}
	copy(h[:], data)
	return h, nil
}

// HashFromBytes creates a Hash from a byte slice.
func HashFromBytes(b []byte) (Hash, error) {
	var h Hash
	if len(b) != HashSize {
		return h, ErrInvalidHash
	}
	copy(h[:], b)
	return h, nil
}

// ComputeHash computes the SHA256 hash of data.
func ComputeHash(data []byte) Hash {
	return sha256.Sum256(data)
}

// String returns the base58-encoded representation.
func (h Hash) String() string {
	return base58.Encode(h[:])
}

// Hex returns the hex-encoded representation.
func (h Hash) Hex() string {
	return hex.EncodeToString(h[:])
}

// IsZero returns true if the hash is all zeros.
func (h Hash) IsZero() bool {
	for _, b := range h {
		if b != 0 {
			return false
		}
	}
	return true
}

// Equals returns true if two hashes are equal.
func (h Hash) Equals(other Hash) bool {
	return h == other
}

// Bytes returns the hash as a byte slice.
func (h Hash) Bytes() []byte {
	return h[:]
}

// MarshalText implements encoding.TextMarshaler.
func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (h *Hash) UnmarshalText(text []byte) error {
	parsed, err := HashFromBase58(string(text))
	if err != nil {
		return err
	}
	*h = parsed
	return nil
}
