package rpcfetch

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/geyser"
)

// RPCClient handles JSON-RPC requests to Solana endpoints.
type RPCClient struct {
	httpClient *http.Client
	pool       Pool
}

// NewRPCClient creates a new RPC client with the given pool.
func NewRPCClient(pool Pool, timeout time.Duration) *RPCClient {
	return &RPCClient{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		pool: pool,
	}
}

// rpcRequest represents a JSON-RPC 2.0 request.
type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params,omitempty"`
}

// rpcResponse represents a JSON-RPC 2.0 response.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError represents a JSON-RPC error.
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// call makes a JSON-RPC call to a healthy endpoint.
func (c *RPCClient) call(ctx context.Context, method string, params []interface{}, result interface{}) error {
	endpoint, err := c.pool.GetEndpoint(ctx)
	if err != nil {
		return fmt.Errorf("get endpoint: %w", err)
	}

	start := time.Now()

	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.pool.MarkUnhealthy(endpoint.URL, err)
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.pool.MarkUnhealthy(endpoint.URL, err)
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.pool.MarkUnhealthy(endpoint.URL, fmt.Errorf("status %d", resp.StatusCode))
		return fmt.Errorf("http status %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		c.pool.MarkUnhealthy(endpoint.URL, err)
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		// RPC errors are not endpoint health issues
		return &RPCError{
			Code:    rpcResp.Error.Code,
			Message: rpcResp.Error.Message,
		}
	}

	if result != nil {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}

	c.pool.MarkHealthy(endpoint.URL, time.Since(start))
	return nil
}

// GetSlot fetches the current slot from the cluster.
func (c *RPCClient) GetSlot(ctx context.Context, commitment string) (uint64, error) {
	params := []interface{}{
		map[string]interface{}{
			"commitment": commitment,
		},
	}

	var slot uint64
	if err := c.call(ctx, "getSlot", params, &slot); err != nil {
		return 0, err
	}
	return slot, nil
}

// GetBlock fetches a block by slot with full transaction details.
func (c *RPCClient) GetBlock(ctx context.Context, slot uint64) (*geyser.Block, error) {
	params := []interface{}{
		slot,
		map[string]interface{}{
			"encoding":                       "json",
			"transactionDetails":             "full",
			"maxSupportedTransactionVersion": 0,
			"rewards":                        true,
		},
	}

	var blockResp *blockResponse
	if err := c.call(ctx, "getBlock", params, &blockResp); err != nil {
		return nil, err
	}

	if blockResp == nil {
		return nil, ErrSlotSkipped
	}

	return convertBlockResponse(slot, blockResp)
}

// blockResponse represents the getBlock RPC response.
type blockResponse struct {
	Blockhash         string                `json:"blockhash"`
	PreviousBlockhash string                `json:"previousBlockhash"`
	ParentSlot        uint64                `json:"parentSlot"`
	Transactions      []transactionWithMeta `json:"transactions"`
	Rewards           []rewardInfo          `json:"rewards"`
	BlockTime         *int64                `json:"blockTime"`
	BlockHeight       *uint64               `json:"blockHeight"`
}

// transactionWithMeta represents a transaction with execution metadata.
type transactionWithMeta struct {
	Transaction json.RawMessage `json:"transaction"`
	Meta        *txMeta         `json:"meta"`
	Version     interface{}     `json:"version"`
}

// txMeta represents transaction execution metadata.
type txMeta struct {
	Err                  interface{}              `json:"err"`
	Fee                  uint64                   `json:"fee"`
	PreBalances          []uint64                 `json:"preBalances"`
	PostBalances         []uint64                 `json:"postBalances"`
	PreTokenBalances     []tokenBalanceInfo       `json:"preTokenBalances"`
	PostTokenBalances    []tokenBalanceInfo       `json:"postTokenBalances"`
	InnerInstructions    []innerInstructionGroup  `json:"innerInstructions"`
	LogMessages          []string                 `json:"logMessages"`
	LoadedAddresses      *loadedAddresses         `json:"loadedAddresses"`
	ComputeUnitsConsumed *uint64                  `json:"computeUnitsConsumed"`
	ReturnData           *returnDataInfo          `json:"returnData"`
}

// tokenBalanceInfo represents a token balance.
type tokenBalanceInfo struct {
	AccountIndex  uint8          `json:"accountIndex"`
	Mint          string         `json:"mint"`
	Owner         string         `json:"owner"`
	ProgramID     string         `json:"programId"`
	UITokenAmount uiTokenAmount  `json:"uiTokenAmount"`
}

// uiTokenAmount represents token amount with UI formatting.
type uiTokenAmount struct {
	Amount         string   `json:"amount"`
	Decimals       uint8    `json:"decimals"`
	UIAmount       *float64 `json:"uiAmount"`
	UIAmountString string   `json:"uiAmountString"`
}

// innerInstructionGroup groups inner instructions by invoking instruction.
type innerInstructionGroup struct {
	Index        uint8             `json:"index"`
	Instructions []innerInstruction `json:"instructions"`
}

// innerInstruction represents a CPI instruction.
type innerInstruction struct {
	ProgramIDIndex uint8   `json:"programIdIndex"`
	Accounts       []uint8 `json:"accounts"`
	Data           string  `json:"data"`
}

// loadedAddresses contains addresses loaded from lookup tables.
type loadedAddresses struct {
	Writable []string `json:"writable"`
	Readonly []string `json:"readonly"`
}

// returnDataInfo contains program return data.
type returnDataInfo struct {
	ProgramID string   `json:"programId"`
	Data      []string `json:"data"`
}

// rewardInfo represents a block reward.
type rewardInfo struct {
	Pubkey      string `json:"pubkey"`
	Lamports    int64  `json:"lamports"`
	PostBalance uint64 `json:"postBalance"`
	RewardType  string `json:"rewardType"`
	Commission  *uint8 `json:"commission"`
}

// parsedTransaction represents a parsed transaction from JSON encoding.
type parsedTransaction struct {
	Signatures []string       `json:"signatures"`
	Message    parsedMessage  `json:"message"`
}

// parsedMessage represents a parsed transaction message.
type parsedMessage struct {
	AccountKeys         []interface{}         `json:"accountKeys"`
	RecentBlockhash     string                `json:"recentBlockhash"`
	Instructions        []parsedInstruction   `json:"instructions"`
	AddressTableLookups []addressTableLookup  `json:"addressTableLookups"`
}

// parsedInstruction represents a parsed or compiled instruction.
type parsedInstruction struct {
	ProgramIDIndex *uint8      `json:"programIdIndex,omitempty"`
	Accounts       []uint8     `json:"accounts,omitempty"`
	Data           string      `json:"data,omitempty"`
	ProgramID      string      `json:"programId,omitempty"`
	Parsed         interface{} `json:"parsed,omitempty"`
}

// addressTableLookup represents an address lookup table reference.
type addressTableLookup struct {
	AccountKey      string  `json:"accountKey"`
	WritableIndexes []uint8 `json:"writableIndexes"`
	ReadonlyIndexes []uint8 `json:"readonlyIndexes"`
}

// convertBlockResponse converts an RPC block response to geyser.Block.
func convertBlockResponse(slot uint64, resp *blockResponse) (*geyser.Block, error) {
	block := &geyser.Block{
		Slot:       slot,
		ParentSlot: resp.ParentSlot,
		BlockTime:  resp.BlockTime,
		BlockHeight: resp.BlockHeight,
		ReceivedAt: time.Now(),
		ExecutedTransactionCount: uint64(len(resp.Transactions)),
	}

	// Convert blockhash
	if resp.Blockhash != "" {
		hash, err := types.HashFromBase58(resp.Blockhash)
		if err != nil {
			return nil, fmt.Errorf("parse blockhash: %w", err)
		}
		block.Blockhash = hash
	}

	// Convert parent blockhash
	if resp.PreviousBlockhash != "" {
		hash, err := types.HashFromBase58(resp.PreviousBlockhash)
		if err != nil {
			return nil, fmt.Errorf("parse parent blockhash: %w", err)
		}
		block.ParentBlockhash = hash
	}

	// Convert transactions
	block.Transactions = make([]geyser.Transaction, len(resp.Transactions))
	for i, txWithMeta := range resp.Transactions {
		tx, err := convertTransaction(uint64(i), txWithMeta)
		if err != nil {
			return nil, fmt.Errorf("convert transaction %d: %w", i, err)
		}
		block.Transactions[i] = tx
	}

	// Convert rewards
	block.Rewards = make([]geyser.Reward, len(resp.Rewards))
	for i, r := range resp.Rewards {
		reward, err := convertReward(r)
		if err != nil {
			return nil, fmt.Errorf("convert reward %d: %w", i, err)
		}
		block.Rewards[i] = reward
	}

	return block, nil
}

// convertTransaction converts an RPC transaction to geyser.Transaction.
func convertTransaction(index uint64, txm transactionWithMeta) (geyser.Transaction, error) {
	tx := geyser.Transaction{
		Index: index,
	}

	// Parse the transaction JSON
	var parsed parsedTransaction
	if err := json.Unmarshal(txm.Transaction, &parsed); err != nil {
		return tx, fmt.Errorf("parse transaction: %w", err)
	}

	// Convert signatures
	tx.Signatures = make([]types.Signature, len(parsed.Signatures))
	for i, sigStr := range parsed.Signatures {
		sig, err := types.SignatureFromBase58(sigStr)
		if err != nil {
			return tx, fmt.Errorf("parse signature %d: %w", i, err)
		}
		tx.Signatures[i] = sig
	}
	if len(tx.Signatures) > 0 {
		tx.Signature = tx.Signatures[0]
	}

	// Determine if versioned
	isLegacy := txm.Version == nil || txm.Version == "legacy"

	// Convert message
	msg := geyser.TransactionMessage{
		IsLegacy: isLegacy,
	}

	// Parse account keys - can be strings or objects
	msg.AccountKeys = make([]types.Pubkey, len(parsed.Message.AccountKeys))
	numSigners := uint8(0)
	numReadonlySigned := uint8(0)
	numReadonlyUnsigned := uint8(0)

	for i, keyData := range parsed.Message.AccountKeys {
		var keyStr string
		switch k := keyData.(type) {
		case string:
			keyStr = k
		case map[string]interface{}:
			if pubkey, ok := k["pubkey"].(string); ok {
				keyStr = pubkey
			}
			// Extract header info from extended account keys
			if signer, ok := k["signer"].(bool); ok && signer {
				numSigners++
				if writable, ok := k["writable"].(bool); ok && !writable {
					numReadonlySigned++
				}
			} else {
				if writable, ok := k["writable"].(bool); ok && !writable {
					numReadonlyUnsigned++
				}
			}
		}

		if keyStr != "" {
			pubkey, err := types.PubkeyFromBase58(keyStr)
			if err != nil {
				return tx, fmt.Errorf("parse account key %d: %w", i, err)
			}
			msg.AccountKeys[i] = pubkey
		}
	}

	// Set header based on signatures count if not determined from account keys
	if numSigners == 0 {
		numSigners = uint8(len(parsed.Signatures))
	}
	msg.Header = geyser.MessageHeader{
		NumRequiredSignatures:       numSigners,
		NumReadonlySignedAccounts:   numReadonlySigned,
		NumReadonlyUnsignedAccounts: numReadonlyUnsigned,
	}

	// Convert recent blockhash
	if parsed.Message.RecentBlockhash != "" {
		hash, err := types.HashFromBase58(parsed.Message.RecentBlockhash)
		if err != nil {
			return tx, fmt.Errorf("parse recent blockhash: %w", err)
		}
		msg.RecentBlockhash = hash
	}

	// Convert instructions
	msg.Instructions = make([]geyser.CompiledInstruction, len(parsed.Message.Instructions))
	for i, ix := range parsed.Message.Instructions {
		ci := geyser.CompiledInstruction{
			AccountIndexes: ix.Accounts,
		}

		// Handle programIdIndex
		if ix.ProgramIDIndex != nil {
			ci.ProgramIDIndex = *ix.ProgramIDIndex
		}

		// Decode instruction data from base58
		if ix.Data != "" {
			data, err := decodeBase58(ix.Data)
			if err != nil {
				// Try base64 as fallback
				data, err = base64.StdEncoding.DecodeString(ix.Data)
				if err != nil {
					return tx, fmt.Errorf("decode instruction data %d: %w", i, err)
				}
			}
			ci.Data = data
		}

		msg.Instructions[i] = ci
	}

	// Convert address table lookups
	if len(parsed.Message.AddressTableLookups) > 0 {
		msg.AddressTableLookups = make([]geyser.AddressTableLookup, len(parsed.Message.AddressTableLookups))
		for i, atl := range parsed.Message.AddressTableLookups {
			lookup := geyser.AddressTableLookup{
				WritableIndexes: atl.WritableIndexes,
				ReadonlyIndexes: atl.ReadonlyIndexes,
			}
			if atl.AccountKey != "" {
				pubkey, err := types.PubkeyFromBase58(atl.AccountKey)
				if err != nil {
					return tx, fmt.Errorf("parse lookup table key %d: %w", i, err)
				}
				lookup.AccountKey = pubkey
			}
			msg.AddressTableLookups[i] = lookup
		}
	}

	tx.Message = msg

	// Convert meta
	if txm.Meta != nil {
		tx.Meta = convertMeta(txm.Meta)
	}

	// Detect vote transactions
	tx.IsVote = isVoteTransaction(&tx)

	return tx, nil
}

// convertMeta converts transaction metadata.
func convertMeta(m *txMeta) *geyser.TransactionMeta {
	meta := &geyser.TransactionMeta{
		Fee:          m.Fee,
		PreBalances:  m.PreBalances,
		PostBalances: m.PostBalances,
		LogMessages:  m.LogMessages,
	}

	// Convert error
	if m.Err != nil {
		meta.Err = &geyser.TransactionError{
			Message: fmt.Sprintf("%v", m.Err),
		}
	}

	// Convert compute units
	if m.ComputeUnitsConsumed != nil {
		meta.ComputeUnitsConsumed = *m.ComputeUnitsConsumed
	}

	// Convert inner instructions
	if len(m.InnerInstructions) > 0 {
		meta.InnerInstructions = make([]geyser.InnerInstructions, len(m.InnerInstructions))
		for i, inner := range m.InnerInstructions {
			meta.InnerInstructions[i] = geyser.InnerInstructions{
				Index: inner.Index,
			}
			if len(inner.Instructions) > 0 {
				meta.InnerInstructions[i].Instructions = make([]geyser.CompiledInstruction, len(inner.Instructions))
				for j, ix := range inner.Instructions {
					ci := geyser.CompiledInstruction{
						ProgramIDIndex: ix.ProgramIDIndex,
						AccountIndexes: ix.Accounts,
					}
					if ix.Data != "" {
						data, err := decodeBase58(ix.Data)
						if err != nil {
							data, _ = base64.StdEncoding.DecodeString(ix.Data)
						}
						ci.Data = data
					}
					meta.InnerInstructions[i].Instructions[j] = ci
				}
			}
		}
	}

	// Convert pre token balances
	if len(m.PreTokenBalances) > 0 {
		meta.PreTokenBalances = make([]geyser.TokenBalance, len(m.PreTokenBalances))
		for i, tb := range m.PreTokenBalances {
			meta.PreTokenBalances[i] = convertTokenBalance(tb)
		}
	}

	// Convert post token balances
	if len(m.PostTokenBalances) > 0 {
		meta.PostTokenBalances = make([]geyser.TokenBalance, len(m.PostTokenBalances))
		for i, tb := range m.PostTokenBalances {
			meta.PostTokenBalances[i] = convertTokenBalance(tb)
		}
	}

	// Convert loaded addresses
	if m.LoadedAddresses != nil {
		if len(m.LoadedAddresses.Writable) > 0 {
			meta.LoadedWritableAddresses = make([]types.Pubkey, len(m.LoadedAddresses.Writable))
			for i, addr := range m.LoadedAddresses.Writable {
				if pubkey, err := types.PubkeyFromBase58(addr); err == nil {
					meta.LoadedWritableAddresses[i] = pubkey
				}
			}
		}
		if len(m.LoadedAddresses.Readonly) > 0 {
			meta.LoadedReadonlyAddresses = make([]types.Pubkey, len(m.LoadedAddresses.Readonly))
			for i, addr := range m.LoadedAddresses.Readonly {
				if pubkey, err := types.PubkeyFromBase58(addr); err == nil {
					meta.LoadedReadonlyAddresses[i] = pubkey
				}
			}
		}
	}

	// Convert return data
	if m.ReturnData != nil && len(m.ReturnData.Data) >= 1 {
		meta.ReturnData = &geyser.ReturnData{}
		if m.ReturnData.ProgramID != "" {
			if pubkey, err := types.PubkeyFromBase58(m.ReturnData.ProgramID); err == nil {
				meta.ReturnData.ProgramID = pubkey
			}
		}
		// Return data is [base64_data, encoding]
		if len(m.ReturnData.Data) > 0 {
			data, _ := base64.StdEncoding.DecodeString(m.ReturnData.Data[0])
			meta.ReturnData.Data = data
		}
	}

	return meta
}

// convertTokenBalance converts a token balance.
func convertTokenBalance(tb tokenBalanceInfo) geyser.TokenBalance {
	balance := geyser.TokenBalance{
		AccountIndex: tb.AccountIndex,
		UITokenAmount: geyser.UITokenAmount{
			Amount:         tb.UITokenAmount.Amount,
			Decimals:       tb.UITokenAmount.Decimals,
			UIAmount:       tb.UITokenAmount.UIAmount,
			UIAmountString: tb.UITokenAmount.UIAmountString,
		},
	}

	if tb.Mint != "" {
		if pubkey, err := types.PubkeyFromBase58(tb.Mint); err == nil {
			balance.Mint = pubkey
		}
	}
	if tb.Owner != "" {
		if pubkey, err := types.PubkeyFromBase58(tb.Owner); err == nil {
			balance.Owner = pubkey
		}
	}
	if tb.ProgramID != "" {
		if pubkey, err := types.PubkeyFromBase58(tb.ProgramID); err == nil {
			balance.ProgramID = pubkey
		}
	}

	return balance
}

// convertReward converts a reward.
func convertReward(r rewardInfo) (geyser.Reward, error) {
	reward := geyser.Reward{
		Lamports:    r.Lamports,
		PostBalance: r.PostBalance,
		Commission:  r.Commission,
	}

	if r.Pubkey != "" {
		pubkey, err := types.PubkeyFromBase58(r.Pubkey)
		if err != nil {
			return reward, fmt.Errorf("parse reward pubkey: %w", err)
		}
		reward.Pubkey = pubkey
	}

	// Convert reward type
	switch r.RewardType {
	case "fee", "Fee":
		reward.RewardType = geyser.RewardTypeFee
	case "rent", "Rent":
		reward.RewardType = geyser.RewardTypeRent
	case "staking", "Staking":
		reward.RewardType = geyser.RewardTypeStaking
	case "voting", "Voting":
		reward.RewardType = geyser.RewardTypeVoting
	default:
		reward.RewardType = geyser.RewardTypeUnspecified
	}

	return reward, nil
}

// isVoteTransaction checks if a transaction is a vote transaction.
func isVoteTransaction(tx *geyser.Transaction) bool {
	// Vote program ID
	voteProgram := "Vote111111111111111111111111111111111111111"

	for _, ix := range tx.Message.Instructions {
		if int(ix.ProgramIDIndex) < len(tx.Message.AccountKeys) {
			programID := tx.Message.AccountKeys[ix.ProgramIDIndex]
			if programID.String() == voteProgram {
				return true
			}
		}
	}
	return false
}

// decodeBase58 decodes a base58-encoded string.
func decodeBase58(s string) ([]byte, error) {
	// Try to parse as base58
	alphabet := "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

	// Quick validation
	for _, c := range s {
		found := false
		for _, a := range alphabet {
			if c == a {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("invalid base58 character: %c", c)
		}
	}

	// Decode
	var result []byte
	for _, c := range s {
		carry := int64(0)
		for i := 0; i < len(alphabet); i++ {
			if rune(alphabet[i]) == c {
				carry = int64(i)
				break
			}
		}
		for j := len(result) - 1; j >= 0; j-- {
			carry += int64(result[j]) * 58
			result[j] = byte(carry & 0xff)
			carry >>= 8
		}
		for carry > 0 {
			result = append([]byte{byte(carry & 0xff)}, result...)
			carry >>= 8
		}
	}

	// Add leading zeros
	for _, c := range s {
		if c != '1' {
			break
		}
		result = append([]byte{0}, result...)
	}

	return result, nil
}

// parseInt parses a string or number to uint64.
func parseInt(v interface{}) (uint64, error) {
	switch val := v.(type) {
	case float64:
		return uint64(val), nil
	case string:
		return strconv.ParseUint(val, 10, 64)
	case int:
		return uint64(val), nil
	case int64:
		return uint64(val), nil
	case uint64:
		return val, nil
	default:
		return 0, fmt.Errorf("cannot parse %T as uint64", v)
	}
}
