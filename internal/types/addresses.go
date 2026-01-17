// Package types provides well-known program addresses for the Solana/X1 network.
package types

// Native program addresses.
// These are the same across Solana mainnet and X1.
var (
	// SystemProgramAddr is the System Program address.
	SystemProgramAddr = MustPubkeyFromBase58("11111111111111111111111111111111")

	// VoteProgramAddr is the Vote Program address.
	VoteProgramAddr = MustPubkeyFromBase58("Vote111111111111111111111111111111111111111")

	// StakeProgramAddr is the Stake Program address.
	StakeProgramAddr = MustPubkeyFromBase58("Stake11111111111111111111111111111111111111")

	// ConfigProgramAddr is the Config Program address.
	ConfigProgramAddr = MustPubkeyFromBase58("Config1111111111111111111111111111111111111")

	// ComputeBudgetProgramAddr is the Compute Budget Program address.
	ComputeBudgetProgramAddr = MustPubkeyFromBase58("ComputeBudget111111111111111111111111111111")

	// AddressLookupTableProgramAddr is the Address Lookup Table Program address.
	AddressLookupTableProgramAddr = MustPubkeyFromBase58("AddressLookupTab1e1111111111111111111111111")

	// BPFLoaderAddr is the BPF Loader address.
	BPFLoaderAddr = MustPubkeyFromBase58("BPFLoader1111111111111111111111111111111111")

	// BPFLoader2Addr is the BPF Loader 2 address.
	BPFLoader2Addr = MustPubkeyFromBase58("BPFLoader2111111111111111111111111111111111")

	// BPFLoaderUpgradeableAddr is the BPF Loader Upgradeable address.
	BPFLoaderUpgradeableAddr = MustPubkeyFromBase58("BPFLoaderUpgradeab1e11111111111111111111111")

	// LoaderV4Addr is the Loader V4 address.
	LoaderV4Addr = MustPubkeyFromBase58("LoaderV411111111111111111111111111111111111")

	// NativeLoaderAddr is the Native Loader address.
	NativeLoaderAddr = MustPubkeyFromBase58("NativeLoader1111111111111111111111111111111")

	// Ed25519PrecompileAddr is the Ed25519 signature verification precompile.
	Ed25519PrecompileAddr = MustPubkeyFromBase58("Ed25519SigVerify111111111111111111111111111")

	// Secp256k1PrecompileAddr is the Secp256k1 recovery precompile.
	Secp256k1PrecompileAddr = MustPubkeyFromBase58("KeccakSecp256k11111111111111111111111111111")

	// Secp256r1PrecompileAddr is the Secp256r1 precompile.
	Secp256r1PrecompileAddr = MustPubkeyFromBase58("Secp256r1SigVerify1111111111111111111111111")

	// FeatureGateProgramAddr is the Feature Gate Program address.
	FeatureGateProgramAddr = MustPubkeyFromBase58("Feature111111111111111111111111111111111111")

	// ZkTokenProofProgramAddr is the ZK Token Proof Program address.
	ZkTokenProofProgramAddr = MustPubkeyFromBase58("ZkTokenProof1111111111111111111111111111111")

	// ZkElGamalProofProgramAddr is the ZK ElGamal Proof Program address.
	ZkElGamalProofProgramAddr = MustPubkeyFromBase58("ZkE1Gama1Proof11111111111111111111111111111")
)

// Sysvar addresses.
var (
	// SysvarClockAddr is the Clock sysvar address.
	SysvarClockAddr = MustPubkeyFromBase58("SysvarC1ock11111111111111111111111111111111")

	// SysvarRentAddr is the Rent sysvar address.
	SysvarRentAddr = MustPubkeyFromBase58("SysvarRent111111111111111111111111111111111")

	// SysvarEpochScheduleAddr is the Epoch Schedule sysvar address.
	SysvarEpochScheduleAddr = MustPubkeyFromBase58("SysvarEpochSchedu1e111111111111111111111111")

	// SysvarFeesAddr is the Fees sysvar address (deprecated).
	SysvarFeesAddr = MustPubkeyFromBase58("SysvarFees111111111111111111111111111111111")

	// SysvarRecentBlockhashesAddr is the Recent Blockhashes sysvar address (deprecated).
	SysvarRecentBlockhashesAddr = MustPubkeyFromBase58("SysvarRecentB1teleases111111111111111111111")

	// SysvarSlotHashesAddr is the Slot Hashes sysvar address.
	SysvarSlotHashesAddr = MustPubkeyFromBase58("SysvarS1otHashes111111111111111111111111111")

	// SysvarSlotHistoryAddr is the Slot History sysvar address.
	SysvarSlotHistoryAddr = MustPubkeyFromBase58("SysvarS1otHistory11111111111111111111111111")

	// SysvarStakeHistoryAddr is the Stake History sysvar address.
	SysvarStakeHistoryAddr = MustPubkeyFromBase58("SysvarStakeHistory1111111111111111111111111")

	// SysvarInstructionsAddr is the Instructions sysvar address.
	SysvarInstructionsAddr = MustPubkeyFromBase58("Sysvar1nstructions1111111111111111111111111")

	// SysvarEpochRewardsAddr is the Epoch Rewards sysvar address.
	SysvarEpochRewardsAddr = MustPubkeyFromBase58("SysvarEpochRewards1111111111111111111111111")

	// SysvarLastRestartSlotAddr is the Last Restart Slot sysvar address.
	SysvarLastRestartSlotAddr = MustPubkeyFromBase58("SysvarLastRestartS1ot1111111111111111111111")
)

// MustPubkeyFromBase58 parses a base58 pubkey or panics.
// Only use for compile-time constants.
func MustPubkeyFromBase58(s string) Pubkey {
	p, err := PubkeyFromBase58(s)
	if err != nil {
		panic(fmt.Sprintf("invalid pubkey constant %q: %v", s, err))
	}
	return p
}

// IsNativeProgram returns true if the pubkey is a native program.
func IsNativeProgram(p Pubkey) bool {
	switch p {
	case SystemProgramAddr,
		VoteProgramAddr,
		StakeProgramAddr,
		ConfigProgramAddr,
		ComputeBudgetProgramAddr,
		AddressLookupTableProgramAddr,
		BPFLoaderAddr,
		BPFLoader2Addr,
		BPFLoaderUpgradeableAddr,
		LoaderV4Addr,
		NativeLoaderAddr,
		Ed25519PrecompileAddr,
		Secp256k1PrecompileAddr,
		Secp256r1PrecompileAddr:
		return true
	default:
		return false
	}
}

// IsSysvar returns true if the pubkey is a sysvar.
func IsSysvar(p Pubkey) bool {
	switch p {
	case SysvarClockAddr,
		SysvarRentAddr,
		SysvarEpochScheduleAddr,
		SysvarFeesAddr,
		SysvarRecentBlockhashesAddr,
		SysvarSlotHashesAddr,
		SysvarSlotHistoryAddr,
		SysvarStakeHistoryAddr,
		SysvarInstructionsAddr,
		SysvarEpochRewardsAddr,
		SysvarLastRestartSlotAddr:
		return true
	default:
		return false
	}
}

// IsPrecompile returns true if the pubkey is a precompile program.
func IsPrecompile(p Pubkey) bool {
	switch p {
	case Ed25519PrecompileAddr,
		Secp256k1PrecompileAddr,
		Secp256r1PrecompileAddr:
		return true
	default:
		return false
	}
}
