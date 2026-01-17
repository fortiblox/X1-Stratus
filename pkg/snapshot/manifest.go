package snapshot

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// BincodeReader provides helpers for reading bincode-serialized data.
// Bincode is the serialization format used by Solana for snapshot data.
type BincodeReader struct {
	reader io.Reader
	buf    []byte
}

// NewBincodeReader creates a new bincode reader.
func NewBincodeReader(r io.Reader) *BincodeReader {
	return &BincodeReader{
		reader: r,
		buf:    make([]byte, 8),
	}
}

// ReadU8 reads a uint8.
func (r *BincodeReader) ReadU8() (uint8, error) {
	if _, err := io.ReadFull(r.reader, r.buf[:1]); err != nil {
		return 0, err
	}
	return r.buf[0], nil
}

// ReadU16 reads a little-endian uint16.
func (r *BincodeReader) ReadU16() (uint16, error) {
	if _, err := io.ReadFull(r.reader, r.buf[:2]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(r.buf[:2]), nil
}

// ReadU32 reads a little-endian uint32.
func (r *BincodeReader) ReadU32() (uint32, error) {
	if _, err := io.ReadFull(r.reader, r.buf[:4]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(r.buf[:4]), nil
}

// ReadU64 reads a little-endian uint64.
func (r *BincodeReader) ReadU64() (uint64, error) {
	if _, err := io.ReadFull(r.reader, r.buf[:8]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(r.buf[:8]), nil
}

// ReadI64 reads a little-endian int64.
func (r *BincodeReader) ReadI64() (int64, error) {
	v, err := r.ReadU64()
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

// ReadU128 reads a little-endian uint128.
func (r *BincodeReader) ReadU128() (uint128, error) {
	lo, err := r.ReadU64()
	if err != nil {
		return uint128{}, err
	}
	hi, err := r.ReadU64()
	if err != nil {
		return uint128{}, err
	}
	return uint128{Lo: lo, Hi: hi}, nil
}

// ReadF64 reads a little-endian float64.
func (r *BincodeReader) ReadF64() (float64, error) {
	v, err := r.ReadU64()
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(v), nil
}

// ReadBool reads a boolean (1 byte).
func (r *BincodeReader) ReadBool() (bool, error) {
	v, err := r.ReadU8()
	if err != nil {
		return false, err
	}
	return v != 0, nil
}

// ReadBytes reads exactly n bytes.
func (r *BincodeReader) ReadBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(r.reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// ReadPubkey reads a 32-byte pubkey.
func (r *BincodeReader) ReadPubkey() (types.Pubkey, error) {
	var pubkey types.Pubkey
	if _, err := io.ReadFull(r.reader, pubkey[:]); err != nil {
		return pubkey, err
	}
	return pubkey, nil
}

// ReadHash reads a 32-byte hash.
func (r *BincodeReader) ReadHash() (types.Hash, error) {
	var hash types.Hash
	if _, err := io.ReadFull(r.reader, hash[:]); err != nil {
		return hash, err
	}
	return hash, nil
}

// ReadVecLen reads a vector length prefix (u64).
func (r *BincodeReader) ReadVecLen() (uint64, error) {
	return r.ReadU64()
}

// ReadOption reads an Option<T>, returning (value, isSome, error).
// The caller must provide a function to read the inner value.
func (r *BincodeReader) ReadOption(readInner func() (interface{}, error)) (interface{}, bool, error) {
	isSome, err := r.ReadBool()
	if err != nil {
		return nil, false, err
	}
	if !isSome {
		return nil, false, nil
	}
	v, err := readInner()
	if err != nil {
		return nil, false, err
	}
	return v, true, nil
}

// Skip skips n bytes.
func (r *BincodeReader) Skip(n int) error {
	if n <= 0 {
		return nil
	}
	_, err := io.CopyN(io.Discard, r.reader, int64(n))
	return err
}

// ParseBankFields parses bank fields from bincode-serialized data.
func ParseBankFields(r io.Reader) (*BankFields, error) {
	br := NewBincodeReader(r)
	fields := &BankFields{}

	var err error

	// blockhash_queue - we skip this complex structure
	// Format: Vec<(Hash, FeeCalculator)> with additional fields
	// For now, we'll read the parts we need and skip the rest

	// First, let's read the slot
	fields.Slot, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read slot: %w", err)
	}

	// parent_slot
	fields.ParentSlot, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read parent_slot: %w", err)
	}

	// Read the blockhash_queue
	// ages: HashMap<Hash, HashAge>
	numAges, err := br.ReadVecLen()
	if err != nil {
		return nil, fmt.Errorf("read blockhash_queue ages len: %w", err)
	}
	for i := uint64(0); i < numAges; i++ {
		// Hash
		if err := br.Skip(32); err != nil {
			return nil, fmt.Errorf("skip hash in ages: %w", err)
		}
		// HashAge { hash_index: u64, timestamp: u64, fee_calculator: FeeCalculator }
		// fee_calculator is { lamports_per_signature: u64 }
		if err := br.Skip(8 + 8 + 8); err != nil {
			return nil, fmt.Errorf("skip hash_age: %w", err)
		}
	}

	// last_hash_index: u64
	if err := br.Skip(8); err != nil {
		return nil, fmt.Errorf("skip last_hash_index: %w", err)
	}

	// max_age: usize (u64 on 64-bit)
	if err := br.Skip(8); err != nil {
		return nil, fmt.Errorf("skip max_age: %w", err)
	}

	// ancestors: AncestorsForSerialization
	// This is Vec<(Slot, usize)>
	numAncestors, err := br.ReadVecLen()
	if err != nil {
		return nil, fmt.Errorf("read ancestors len: %w", err)
	}
	for i := uint64(0); i < numAncestors; i++ {
		if err := br.Skip(8 + 8); err != nil {
			return nil, fmt.Errorf("skip ancestor: %w", err)
		}
	}

	// hash: Hash (blockhash)
	fields.Blockhash, err = br.ReadHash()
	if err != nil {
		return nil, fmt.Errorf("read blockhash: %w", err)
	}

	// parent_hash: Hash
	fields.ParentBlockhash, err = br.ReadHash()
	if err != nil {
		return nil, fmt.Errorf("read parent_blockhash: %w", err)
	}

	// transaction_count: u64
	fields.TransactionCount, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read transaction_count: %w", err)
	}

	// tick_height: u64 (skip)
	if err := br.Skip(8); err != nil {
		return nil, fmt.Errorf("skip tick_height: %w", err)
	}

	// signature_count: u64
	fields.SignatureCount, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read signature_count: %w", err)
	}

	// capitalization: u64
	fields.Capitalization, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read capitalization: %w", err)
	}

	// max_tick_height: u64
	fields.MaxTickHeight, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read max_tick_height: %w", err)
	}

	// hashes_per_tick: Option<u64>
	hasHashesPerTick, err := br.ReadBool()
	if err != nil {
		return nil, fmt.Errorf("read hashes_per_tick option: %w", err)
	}
	if hasHashesPerTick {
		v, err := br.ReadU64()
		if err != nil {
			return nil, fmt.Errorf("read hashes_per_tick: %w", err)
		}
		fields.HashesPerTick = &v
	}

	// ticks_per_slot: u64
	fields.TicksPerSlot, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read ticks_per_slot: %w", err)
	}

	// ns_per_slot: u128
	fields.NsPerSlot, err = br.ReadU128()
	if err != nil {
		return nil, fmt.Errorf("read ns_per_slot: %w", err)
	}

	// genesis_creation_time: UnixTimestamp (i64)
	fields.GenesisCreationTime, err = br.ReadI64()
	if err != nil {
		return nil, fmt.Errorf("read genesis_creation_time: %w", err)
	}

	// slots_per_year: f64
	fields.SlotsPerYear, err = br.ReadF64()
	if err != nil {
		return nil, fmt.Errorf("read slots_per_year: %w", err)
	}

	// accounts_data_len: u64
	fields.AccountsDataLen, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read accounts_data_len: %w", err)
	}

	// slot (again, may be duplicate or different field)
	// The actual Solana struct has some version-dependent fields here
	// We'll read what we can and be lenient with errors

	// epoch: Epoch (u64)
	fields.Epoch, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read epoch: %w", err)
	}

	// block_height: u64
	fields.BlockHeight, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read block_height: %w", err)
	}

	// collector_id: Pubkey
	fields.CollectorID, err = br.ReadPubkey()
	if err != nil {
		return nil, fmt.Errorf("read collector_id: %w", err)
	}

	// collector_fees: u64
	fields.CollectorFees, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read collector_fees: %w", err)
	}

	// fee_calculator: FeeCalculator
	fields.FeeCalculator.LamportsPerSignature, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read fee_calculator: %w", err)
	}

	// fee_rate_governor: FeeRateGovernor
	fields.FeeRateGovernor, err = readFeeRateGovernor(br)
	if err != nil {
		return nil, fmt.Errorf("read fee_rate_governor: %w", err)
	}

	// collected_rent: u64 (skip)
	if err := br.Skip(8); err != nil {
		return nil, fmt.Errorf("skip collected_rent: %w", err)
	}

	// rent_collector: RentCollector
	fields.RentCollector, err = readRentCollector(br)
	if err != nil {
		return nil, fmt.Errorf("read rent_collector: %w", err)
	}

	// epoch_schedule: EpochSchedule
	fields.EpochSchedule, err = readEpochSchedule(br)
	if err != nil {
		return nil, fmt.Errorf("read epoch_schedule: %w", err)
	}

	// inflation: Inflation
	fields.Inflation, err = readInflation(br)
	if err != nil {
		return nil, fmt.Errorf("read inflation: %w", err)
	}

	// Skip stakes, epoch_stakes, and other complex structures
	// We primarily need the above fields for verification

	return fields, nil
}

// readFeeRateGovernor reads a FeeRateGovernor from bincode.
func readFeeRateGovernor(br *BincodeReader) (FeeRateGovernor, error) {
	var gov FeeRateGovernor
	var err error

	gov.LamportsPerSignature, err = br.ReadU64()
	if err != nil {
		return gov, err
	}

	gov.TargetLamportsPerSignature, err = br.ReadU64()
	if err != nil {
		return gov, err
	}

	gov.TargetSignaturesPerSlot, err = br.ReadU64()
	if err != nil {
		return gov, err
	}

	gov.MinLamportsPerSignature, err = br.ReadU64()
	if err != nil {
		return gov, err
	}

	gov.MaxLamportsPerSignature, err = br.ReadU64()
	if err != nil {
		return gov, err
	}

	gov.BurnPercent, err = br.ReadU8()
	if err != nil {
		return gov, err
	}

	return gov, nil
}

// readRentCollector reads a RentCollector from bincode.
func readRentCollector(br *BincodeReader) (RentCollector, error) {
	var rc RentCollector
	var err error

	rc.Epoch, err = br.ReadU64()
	if err != nil {
		return rc, err
	}

	rc.EpochSchedule, err = readEpochSchedule(br)
	if err != nil {
		return rc, err
	}

	rc.SlotsPerYear, err = br.ReadF64()
	if err != nil {
		return rc, err
	}

	rc.Rent, err = readRent(br)
	if err != nil {
		return rc, err
	}

	return rc, nil
}

// readRent reads Rent from bincode.
func readRent(br *BincodeReader) (Rent, error) {
	var rent Rent
	var err error

	rent.LamportsPerByteYear, err = br.ReadU64()
	if err != nil {
		return rent, err
	}

	rent.ExemptionThreshold, err = br.ReadF64()
	if err != nil {
		return rent, err
	}

	rent.BurnPercent, err = br.ReadU8()
	if err != nil {
		return rent, err
	}

	return rent, nil
}

// readEpochSchedule reads an EpochSchedule from bincode.
func readEpochSchedule(br *BincodeReader) (EpochSchedule, error) {
	var es EpochSchedule
	var err error

	es.SlotsPerEpoch, err = br.ReadU64()
	if err != nil {
		return es, err
	}

	es.LeaderScheduleSlotOffset, err = br.ReadU64()
	if err != nil {
		return es, err
	}

	es.Warmup, err = br.ReadBool()
	if err != nil {
		return es, err
	}

	es.FirstNormalEpoch, err = br.ReadU64()
	if err != nil {
		return es, err
	}

	es.FirstNormalSlot, err = br.ReadU64()
	if err != nil {
		return es, err
	}

	return es, nil
}

// readInflation reads Inflation from bincode.
func readInflation(br *BincodeReader) (Inflation, error) {
	var inf Inflation
	var err error

	inf.Initial, err = br.ReadF64()
	if err != nil {
		return inf, err
	}

	inf.Terminal, err = br.ReadF64()
	if err != nil {
		return inf, err
	}

	inf.Taper, err = br.ReadF64()
	if err != nil {
		return inf, err
	}

	inf.Foundation, err = br.ReadF64()
	if err != nil {
		return inf, err
	}

	inf.FoundationTerm, err = br.ReadF64()
	if err != nil {
		return inf, err
	}

	return inf, nil
}

// ParseBankFieldsPartial parses only the essential bank fields, being lenient with unknown data.
// This is useful when the exact snapshot version format varies.
func ParseBankFieldsPartial(r io.Reader, fileSize int64) (*BankFields, error) {
	// For partial parsing, we'll try to read the key fields we need
	// and handle errors gracefully
	br := NewBincodeReader(r)
	fields := &BankFields{}

	var err error

	// Read slot - this is typically the first field
	fields.Slot, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read slot: %w", err)
	}

	// parent_slot
	fields.ParentSlot, err = br.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("read parent_slot: %w", err)
	}

	// For very basic loading, we can just use the slot information
	// The full bank fields parsing is complex due to version differences

	return fields, nil
}

// AccountStorageMap represents the mapping of account storage entries.
type AccountStorageMap map[uint64][]AccountStorageEntry

// ParseAccountPaths parses the account_paths.txt file.
func ParseAccountPaths(r io.Reader) ([]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Account paths are newline-separated
	var paths []string
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' || data[i] == '\r' {
			if i > start {
				paths = append(paths, string(data[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(data) {
		paths = append(paths, string(data[start:]))
	}

	return paths, nil
}
