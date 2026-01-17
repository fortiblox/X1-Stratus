package rpc

import (
	"encoding/base64"
	"fmt"

	"github.com/klauspost/compress/zstd"
	"github.com/mr-tron/base58"
)

// EncodeAccountData encodes account data according to the specified encoding.
func EncodeAccountData(data []byte, encoding Encoding) (interface{}, error) {
	switch encoding {
	case EncodingBase58:
		return []string{base58.Encode(data), string(EncodingBase58)}, nil

	case EncodingBase64:
		return []string{base64.StdEncoding.EncodeToString(data), string(EncodingBase64)}, nil

	case EncodingBase64Zstd:
		compressed, err := compressZstd(data)
		if err != nil {
			return nil, fmt.Errorf("zstd compression failed: %w", err)
		}
		return []string{base64.StdEncoding.EncodeToString(compressed), string(EncodingBase64Zstd)}, nil

	case EncodingJSONParsed:
		// JSON parsing is program-specific and handled separately
		// Fall back to base64 for unparsed data
		return []string{base64.StdEncoding.EncodeToString(data), string(EncodingBase64)}, nil

	default:
		// Default to base64
		return []string{base64.StdEncoding.EncodeToString(data), string(EncodingBase64)}, nil
	}
}

// DecodeAccountData decodes account data from the specified encoding.
func DecodeAccountData(encoded string, encoding Encoding) ([]byte, error) {
	switch encoding {
	case EncodingBase58:
		return base58.Decode(encoded)

	case EncodingBase64:
		return base64.StdEncoding.DecodeString(encoded)

	case EncodingBase64Zstd:
		compressed, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("base64 decode failed: %w", err)
		}
		return decompressZstd(compressed)

	default:
		return base64.StdEncoding.DecodeString(encoded)
	}
}

// EncodeBase58 encodes bytes to base58.
func EncodeBase58(data []byte) string {
	return base58.Encode(data)
}

// DecodeBase58 decodes a base58 string to bytes.
func DecodeBase58(s string) ([]byte, error) {
	return base58.Decode(s)
}

// EncodeBase64 encodes bytes to base64.
func EncodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeBase64 decodes a base64 string to bytes.
func DecodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// compressZstd compresses data using zstd.
func compressZstd(data []byte) ([]byte, error) {
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, err
	}
	defer encoder.Close()
	return encoder.EncodeAll(data, nil), nil
}

// decompressZstd decompresses zstd-compressed data.
func decompressZstd(data []byte) ([]byte, error) {
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	return decoder.DecodeAll(data, nil)
}

// EncodeTransaction encodes transaction data according to the specified encoding.
func EncodeTransaction(data []byte, encoding Encoding) (interface{}, error) {
	switch encoding {
	case EncodingBase58:
		return base58.Encode(data), nil

	case EncodingBase64:
		return base64.StdEncoding.EncodeToString(data), nil

	case EncodingJSONParsed:
		// JSON parsing handled separately
		return base64.StdEncoding.EncodeToString(data), nil

	default:
		return base64.StdEncoding.EncodeToString(data), nil
	}
}

// EncodeInstructionData encodes instruction data to base58.
func EncodeInstructionData(data []byte) string {
	return base58.Encode(data)
}

// DecodeInstructionData decodes instruction data from base58.
func DecodeInstructionData(s string) ([]byte, error) {
	return base58.Decode(s)
}

// ApplyDataSlice applies a data slice to account data.
func ApplyDataSlice(data []byte, slice *DataSlice) []byte {
	if slice == nil {
		return data
	}

	start := slice.Offset
	if start >= uint64(len(data)) {
		return []byte{}
	}

	end := start + slice.Length
	if end > uint64(len(data)) {
		end = uint64(len(data))
	}

	return data[start:end]
}

// ParseEncoding parses an encoding string to Encoding type.
func ParseEncoding(s string) Encoding {
	switch s {
	case "base58":
		return EncodingBase58
	case "base64":
		return EncodingBase64
	case "base64+zstd":
		return EncodingBase64Zstd
	case "jsonParsed":
		return EncodingJSONParsed
	default:
		return EncodingBase64
	}
}

// ParseCommitment parses a commitment string to Commitment type.
func ParseCommitment(s string) Commitment {
	switch s {
	case "processed":
		return CommitmentProcessed
	case "confirmed":
		return CommitmentConfirmed
	case "finalized":
		return CommitmentFinalized
	default:
		return CommitmentConfirmed
	}
}

// GetDefaultEncoding returns the default encoding for account data.
func GetDefaultEncoding() Encoding {
	return EncodingBase64
}

// GetDefaultCommitment returns the default commitment level.
func GetDefaultCommitment() Commitment {
	return CommitmentFinalized
}
