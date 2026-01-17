// Package rpc provides JSON-RPC 2.0 types for Solana-compatible API.
package rpc

import (
	"encoding/json"

	"github.com/fortiblox/X1-Stratus/internal/types"
)

// JSON-RPC 2.0 constants.
const (
	JSONRPCVersion = "2.0"
)

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Context provides slot context for RPC responses.
type Context struct {
	Slot       uint64 `json:"slot"`
	APIVersion string `json:"apiVersion,omitempty"`
}

// ResponseWithContext wraps a value with context.
type ResponseWithContext struct {
	Context Context     `json:"context"`
	Value   interface{} `json:"value"`
}

// Commitment levels for RPC requests.
type Commitment string

const (
	CommitmentProcessed Commitment = "processed"
	CommitmentConfirmed Commitment = "confirmed"
	CommitmentFinalized Commitment = "finalized"
)

// Encoding types for account data.
type Encoding string

const (
	EncodingBase58     Encoding = "base58"
	EncodingBase64     Encoding = "base64"
	EncodingBase64Zstd Encoding = "base64+zstd"
	EncodingJSONParsed Encoding = "jsonParsed"
)

// DataSlice specifies a portion of account data to return.
type DataSlice struct {
	Offset uint64 `json:"offset"`
	Length uint64 `json:"length"`
}

// AccountInfoConfig configures getAccountInfo requests.
type AccountInfoConfig struct {
	Encoding   Encoding   `json:"encoding,omitempty"`
	Commitment Commitment `json:"commitment,omitempty"`
	DataSlice  *DataSlice `json:"dataSlice,omitempty"`
	MinContextSlot *uint64 `json:"minContextSlot,omitempty"`
}

// BalanceConfig configures getBalance requests.
type BalanceConfig struct {
	Commitment     Commitment `json:"commitment,omitempty"`
	MinContextSlot *uint64    `json:"minContextSlot,omitempty"`
}

// MultipleAccountsConfig configures getMultipleAccounts requests.
type MultipleAccountsConfig struct {
	Encoding   Encoding   `json:"encoding,omitempty"`
	Commitment Commitment `json:"commitment,omitempty"`
	DataSlice  *DataSlice `json:"dataSlice,omitempty"`
	MinContextSlot *uint64 `json:"minContextSlot,omitempty"`
}

// ProgramAccountsConfig configures getProgramAccounts requests.
type ProgramAccountsConfig struct {
	Encoding       Encoding               `json:"encoding,omitempty"`
	Commitment     Commitment             `json:"commitment,omitempty"`
	DataSlice      *DataSlice             `json:"dataSlice,omitempty"`
	Filters        []ProgramAccountFilter `json:"filters,omitempty"`
	WithContext    bool                   `json:"withContext,omitempty"`
	MinContextSlot *uint64                `json:"minContextSlot,omitempty"`
}

// ProgramAccountFilter filters program accounts.
type ProgramAccountFilter struct {
	Memcmp  *MemcmpFilter `json:"memcmp,omitempty"`
	DataSize *uint64      `json:"dataSize,omitempty"`
}

// MemcmpFilter matches account data at an offset.
type MemcmpFilter struct {
	Offset uint64 `json:"offset"`
	Bytes  string `json:"bytes"`
	Encoding Encoding `json:"encoding,omitempty"`
}

// BlockConfig configures getBlock requests.
type BlockConfig struct {
	Encoding                       Encoding   `json:"encoding,omitempty"`
	TransactionDetails             string     `json:"transactionDetails,omitempty"`
	Rewards                        *bool      `json:"rewards,omitempty"`
	Commitment                     Commitment `json:"commitment,omitempty"`
	MaxSupportedTransactionVersion *uint64    `json:"maxSupportedTransactionVersion,omitempty"`
}

// TransactionConfig configures getTransaction requests.
type TransactionConfig struct {
	Encoding                       Encoding   `json:"encoding,omitempty"`
	Commitment                     Commitment `json:"commitment,omitempty"`
	MaxSupportedTransactionVersion *uint64    `json:"maxSupportedTransactionVersion,omitempty"`
}

// SignaturesForAddressConfig configures getSignaturesForAddress requests.
type SignaturesForAddressConfig struct {
	Limit          int        `json:"limit,omitempty"`
	Before         string     `json:"before,omitempty"`
	Until          string     `json:"until,omitempty"`
	Commitment     Commitment `json:"commitment,omitempty"`
	MinContextSlot *uint64    `json:"minContextSlot,omitempty"`
}

// SlotConfig configures getSlot requests.
type SlotConfig struct {
	Commitment     Commitment `json:"commitment,omitempty"`
	MinContextSlot *uint64    `json:"minContextSlot,omitempty"`
}

// AccountInfo represents account information returned by RPC.
type AccountInfo struct {
	Data       interface{} `json:"data"` // [base64string, encoding] or parsed JSON
	Executable bool        `json:"executable"`
	Lamports   uint64      `json:"lamports"`
	Owner      string      `json:"owner"`
	RentEpoch  uint64      `json:"rentEpoch"`
	Space      uint64      `json:"space"`
}

// KeyedAccountInfo wraps AccountInfo with its pubkey.
type KeyedAccountInfo struct {
	Pubkey  string       `json:"pubkey"`
	Account *AccountInfo `json:"account"`
}

// BlockResponse represents a block returned by RPC.
type BlockResponse struct {
	Blockhash         string                    `json:"blockhash"`
	PreviousBlockhash string                    `json:"previousBlockhash"`
	ParentSlot        uint64                    `json:"parentSlot"`
	Transactions      []TransactionWithMeta     `json:"transactions,omitempty"`
	Signatures        []string                  `json:"signatures,omitempty"`
	Rewards           []RewardInfo              `json:"rewards,omitempty"`
	BlockTime         *int64                    `json:"blockTime"`
	BlockHeight       *uint64                   `json:"blockHeight"`
}

// TransactionWithMeta represents a transaction with metadata.
type TransactionWithMeta struct {
	Transaction interface{}         `json:"transaction"` // Encoded or parsed transaction
	Meta        *TransactionMeta    `json:"meta"`
	Version     interface{}         `json:"version,omitempty"` // "legacy" or version number
}

// TransactionMeta contains transaction execution metadata.
type TransactionMeta struct {
	Err               interface{}            `json:"err"`
	Fee               uint64                 `json:"fee"`
	PreBalances       []uint64               `json:"preBalances"`
	PostBalances      []uint64               `json:"postBalances"`
	PreTokenBalances  []TokenBalance         `json:"preTokenBalances,omitempty"`
	PostTokenBalances []TokenBalance         `json:"postTokenBalances,omitempty"`
	InnerInstructions []InnerInstructionSet  `json:"innerInstructions,omitempty"`
	LogMessages       []string               `json:"logMessages,omitempty"`
	LoadedAddresses   *LoadedAddresses       `json:"loadedAddresses,omitempty"`
	ComputeUnitsConsumed *uint64             `json:"computeUnitsConsumed,omitempty"`
}

// InnerInstructionSet groups inner instructions by invoking instruction index.
type InnerInstructionSet struct {
	Index        uint8              `json:"index"`
	Instructions []InnerInstruction `json:"instructions"`
}

// InnerInstruction represents a CPI instruction.
type InnerInstruction struct {
	ProgramIDIndex uint8  `json:"programIdIndex"`
	Accounts       []uint8 `json:"accounts"`
	Data           string `json:"data"`
	StackHeight    *uint8 `json:"stackHeight,omitempty"`
}

// TokenBalance represents a token account balance.
type TokenBalance struct {
	AccountIndex  uint8           `json:"accountIndex"`
	Mint          string          `json:"mint"`
	Owner         string          `json:"owner,omitempty"`
	ProgramID     string          `json:"programId,omitempty"`
	UITokenAmount UITokenAmount   `json:"uiTokenAmount"`
}

// UITokenAmount represents a token amount with UI formatting.
type UITokenAmount struct {
	Amount         string   `json:"amount"`
	Decimals       uint8    `json:"decimals"`
	UIAmount       *float64 `json:"uiAmount"`
	UIAmountString string   `json:"uiAmountString"`
}

// LoadedAddresses contains addresses loaded from lookup tables.
type LoadedAddresses struct {
	Writable []string `json:"writable"`
	Readonly []string `json:"readonly"`
}

// RewardInfo represents block reward information.
type RewardInfo struct {
	Pubkey      string  `json:"pubkey"`
	Lamports    int64   `json:"lamports"`
	PostBalance uint64  `json:"postBalance"`
	RewardType  string  `json:"rewardType,omitempty"`
	Commission  *uint8  `json:"commission,omitempty"`
}

// TransactionResponse represents a transaction returned by RPC.
type TransactionResponse struct {
	Slot        uint64              `json:"slot"`
	Transaction interface{}         `json:"transaction"` // Encoded or parsed
	Meta        *TransactionMeta    `json:"meta"`
	BlockTime   *int64              `json:"blockTime,omitempty"`
	Version     interface{}         `json:"version,omitempty"`
}

// SignatureInfo represents signature information for getSignaturesForAddress.
type SignatureInfo struct {
	Signature          string      `json:"signature"`
	Slot               uint64      `json:"slot"`
	Err                interface{} `json:"err"`
	Memo               *string     `json:"memo"`
	BlockTime          *int64      `json:"blockTime"`
	ConfirmationStatus string      `json:"confirmationStatus,omitempty"`
}

// SignatureStatus represents the status of a transaction signature.
type SignatureStatus struct {
	Slot               uint64      `json:"slot"`
	Confirmations      *uint64     `json:"confirmations"`
	Err                interface{} `json:"err"`
	ConfirmationStatus string      `json:"confirmationStatus,omitempty"`
}

// VersionInfo represents node version information.
type VersionInfo struct {
	SolanaCore string `json:"solana-core"`
	FeatureSet uint64 `json:"feature-set,omitempty"`
}

// Identity represents node identity.
type Identity struct {
	Identity string `json:"identity"`
}

// EncodedTransaction represents an encoded transaction.
type EncodedTransaction struct {
	Content  []string `json:"-"` // [encoded_data, encoding]
	Encoding Encoding `json:"-"`
}

// MarshalJSON implements json.Marshaler for EncodedTransaction.
func (e EncodedTransaction) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Content)
}

// ParsedTransaction represents a parsed transaction.
type ParsedTransaction struct {
	Signatures []string              `json:"signatures"`
	Message    ParsedMessage         `json:"message"`
}

// ParsedMessage represents a parsed transaction message.
type ParsedMessage struct {
	AccountKeys         []ParsedAccountKey      `json:"accountKeys"`
	RecentBlockhash     string                  `json:"recentBlockhash"`
	Instructions        []ParsedInstruction     `json:"instructions"`
	AddressTableLookups []AddressTableLookup    `json:"addressTableLookups,omitempty"`
}

// ParsedAccountKey represents an account key with metadata.
type ParsedAccountKey struct {
	Pubkey   string `json:"pubkey"`
	Signer   bool   `json:"signer"`
	Source   string `json:"source,omitempty"` // "transaction" or "lookupTable"
	Writable bool   `json:"writable"`
}

// ParsedInstruction represents a parsed instruction.
type ParsedInstruction struct {
	Program       string      `json:"program,omitempty"`
	ProgramID     string      `json:"programId"`
	Parsed        interface{} `json:"parsed,omitempty"`
	Accounts      []string    `json:"accounts,omitempty"`
	Data          string      `json:"data,omitempty"`
	StackHeight   *uint8      `json:"stackHeight,omitempty"`
}

// AddressTableLookup represents an address lookup table reference.
type AddressTableLookup struct {
	AccountKey      string  `json:"accountKey"`
	WritableIndexes []uint8 `json:"writableIndexes"`
	ReadonlyIndexes []uint8 `json:"readonlyIndexes"`
}

// LeaderScheduleEntry represents a leader schedule entry.
type LeaderScheduleEntry map[string][]uint64

// ClusterNode represents a cluster node.
type ClusterNode struct {
	Pubkey       string  `json:"pubkey"`
	Gossip       *string `json:"gossip"`
	TPU          *string `json:"tpu"`
	RPC          *string `json:"rpc"`
	Version      *string `json:"version"`
	FeatureSet   *uint32 `json:"featureSet"`
	ShredVersion *uint16 `json:"shredVersion"`
}

// EpochInfo represents epoch information.
type EpochInfo struct {
	AbsoluteSlot     uint64 `json:"absoluteSlot"`
	BlockHeight      uint64 `json:"blockHeight"`
	Epoch            uint64 `json:"epoch"`
	SlotIndex        uint64 `json:"slotIndex"`
	SlotsInEpoch     uint64 `json:"slotsInEpoch"`
	TransactionCount *uint64 `json:"transactionCount,omitempty"`
}

// EpochSchedule represents epoch schedule configuration.
type EpochSchedule struct {
	SlotsPerEpoch             uint64 `json:"slotsPerEpoch"`
	LeaderScheduleSlotOffset  uint64 `json:"leaderScheduleSlotOffset"`
	Warmup                    bool   `json:"warmup"`
	FirstNormalEpoch          uint64 `json:"firstNormalEpoch"`
	FirstNormalSlot           uint64 `json:"firstNormalSlot"`
}

// Supply represents token supply information.
type Supply struct {
	Total                  uint64   `json:"total"`
	Circulating            uint64   `json:"circulating"`
	NonCirculating         uint64   `json:"nonCirculating"`
	NonCirculatingAccounts []string `json:"nonCirculatingAccounts"`
}

// InflationGovernor represents inflation configuration.
type InflationGovernor struct {
	Initial        float64 `json:"initial"`
	Terminal       float64 `json:"terminal"`
	Taper          float64 `json:"taper"`
	Foundation     float64 `json:"foundation"`
	FoundationTerm float64 `json:"foundationTerm"`
}

// InflationRate represents current inflation rate.
type InflationRate struct {
	Total      float64 `json:"total"`
	Validator  float64 `json:"validator"`
	Foundation float64 `json:"foundation"`
	Epoch      uint64  `json:"epoch"`
}

// StakeActivation represents stake activation state.
type StakeActivation struct {
	State    string `json:"state"`
	Active   uint64 `json:"active"`
	Inactive uint64 `json:"inactive"`
}

// VoteAccount represents a vote account.
type VoteAccount struct {
	VotePubkey       string   `json:"votePubkey"`
	NodePubkey       string   `json:"nodePubkey"`
	ActivatedStake   uint64   `json:"activatedStake"`
	EpochVoteAccount bool     `json:"epochVoteAccount"`
	Commission       uint8    `json:"commission"`
	LastVote         uint64   `json:"lastVote"`
	EpochCredits     [][]uint64 `json:"epochCredits"`
	RootSlot         uint64   `json:"rootSlot"`
}

// VoteAccounts represents vote accounts by status.
type VoteAccounts struct {
	Current    []VoteAccount `json:"current"`
	Delinquent []VoteAccount `json:"delinquent"`
}

// SimulateTransactionConfig configures transaction simulation.
type SimulateTransactionConfig struct {
	SigVerify              bool       `json:"sigVerify,omitempty"`
	Commitment             Commitment `json:"commitment,omitempty"`
	Encoding               Encoding   `json:"encoding,omitempty"`
	ReplaceRecentBlockhash bool       `json:"replaceRecentBlockhash,omitempty"`
	Accounts               *SimulateAccountsConfig `json:"accounts,omitempty"`
	MinContextSlot         *uint64    `json:"minContextSlot,omitempty"`
	InnerInstructions      bool       `json:"innerInstructions,omitempty"`
}

// SimulateAccountsConfig configures accounts for simulation.
type SimulateAccountsConfig struct {
	Encoding  Encoding `json:"encoding,omitempty"`
	Addresses []string `json:"addresses"`
}

// SimulationResult represents transaction simulation results.
type SimulationResult struct {
	Err               interface{}           `json:"err"`
	Logs              []string              `json:"logs"`
	Accounts          []*AccountInfo        `json:"accounts,omitempty"`
	UnitsConsumed     *uint64               `json:"unitsConsumed,omitempty"`
	ReturnData        *ReturnData           `json:"returnData,omitempty"`
	InnerInstructions []InnerInstructionSet `json:"innerInstructions,omitempty"`
}

// ReturnData represents program return data.
type ReturnData struct {
	ProgramID string   `json:"programId"`
	Data      []string `json:"data"` // [base64_data, encoding]
}

// FeeForMessage represents the fee for a message.
type FeeForMessage struct {
	Value *uint64 `json:"value"`
}

// BlockCommitment represents block commitment information.
type BlockCommitment struct {
	Commitment []uint64 `json:"commitment"`
	TotalStake uint64   `json:"totalStake"`
}

// LatestBlockhash represents the latest blockhash.
type LatestBlockhash struct {
	Blockhash            string `json:"blockhash"`
	LastValidBlockHeight uint64 `json:"lastValidBlockHeight"`
}

// Helper function to convert internal pubkey to string.
func pubkeyToString(p types.Pubkey) string {
	return p.String()
}

// Helper function to convert internal hash to string.
func hashToString(h types.Hash) string {
	return h.String()
}

// Helper function to convert internal signature to string.
func signatureToString(s types.Signature) string {
	return s.String()
}
