package dashboard

import (
	"encoding/json"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/blockstore"
)

// API response types

// StatusResponse is the response for GET /api/status.
type StatusResponse struct {
	CurrentSlot      uint64  `json:"currentSlot"`
	LatestSlot       uint64  `json:"latestSlot"`
	SlotsBehind      uint64  `json:"slotsBehind"`
	IsSyncing        bool    `json:"isSyncing"`
	IsRunning        bool    `json:"isRunning"`
	SyncStatus       string  `json:"syncStatus"`
	Uptime           string  `json:"uptime"`
	UptimeSeconds    float64 `json:"uptimeSeconds"`
	BlocksProcessed  uint64  `json:"blocksProcessed"`
	TxsProcessed     uint64  `json:"txsProcessed"`
	BlocksPerSec     float64 `json:"blocksPerSec"`
	AvgSlotTimeMs    float64 `json:"avgSlotTimeMs"`
	AccountsCount    uint64  `json:"accountsCount"`
	GeyserConnected  bool    `json:"geyserConnected"`
	GeyserEndpoint   string  `json:"geyserEndpoint,omitempty"`
	LastError        string  `json:"lastError,omitempty"`
}

// BlockResponse is the response for GET /api/blocks/:slot.
type BlockResponse struct {
	Slot              uint64              `json:"slot"`
	ParentSlot        uint64              `json:"parentSlot"`
	Blockhash         string              `json:"blockhash"`
	PreviousBlockhash string              `json:"previousBlockhash"`
	BlockTime         *int64              `json:"blockTime,omitempty"`
	BlockHeight       *uint64             `json:"blockHeight,omitempty"`
	TransactionCount  int                 `json:"transactionCount"`
	Transactions      []TransactionBrief  `json:"transactions,omitempty"`
	Rewards           []RewardResponse    `json:"rewards,omitempty"`
}

// TransactionBrief is a brief transaction summary.
type TransactionBrief struct {
	Signature   string `json:"signature"`
	Success     bool   `json:"success"`
	Fee         uint64 `json:"fee"`
	Accounts    int    `json:"accounts"`
	Instructions int   `json:"instructions"`
}

// RewardResponse is a reward entry.
type RewardResponse struct {
	Pubkey      string `json:"pubkey"`
	Lamports    int64  `json:"lamports"`
	PostBalance uint64 `json:"postBalance"`
	RewardType  string `json:"rewardType"`
}

// BlocksListResponse is the response for GET /api/blocks.
type BlocksListResponse struct {
	Blocks      []BlockBrief `json:"blocks"`
	CurrentPage int          `json:"currentPage"`
	TotalPages  int          `json:"totalPages"`
	HasPrev     bool         `json:"hasPrev"`
	HasNext     bool         `json:"hasNext"`
}

// BlockBrief is a brief block summary.
type BlockBrief struct {
	Slot             uint64 `json:"slot"`
	Blockhash        string `json:"blockhash"`
	TransactionCount int    `json:"transactionCount"`
	BlockTime        *int64 `json:"blockTime,omitempty"`
}

// AccountResponse is the response for GET /api/accounts/:pubkey.
type AccountResponse struct {
	Pubkey     string `json:"pubkey"`
	Lamports   uint64 `json:"lamports"`
	Owner      string `json:"owner"`
	Executable bool   `json:"executable"`
	RentEpoch  uint64 `json:"rentEpoch"`
	DataLen    int    `json:"dataLen"`
	DataHex    string `json:"dataHex,omitempty"` // First 256 bytes as hex
}

// TransactionResponse is the response for GET /api/transactions/:sig.
type TransactionResponse struct {
	Signature           string                 `json:"signature"`
	Slot                uint64                 `json:"slot"`
	Success             bool                   `json:"success"`
	Error               string                 `json:"error,omitempty"`
	Fee                 uint64                 `json:"fee"`
	ComputeUnitsConsumed uint64                `json:"computeUnitsConsumed"`
	Accounts            []string               `json:"accounts"`
	Instructions        []InstructionResponse  `json:"instructions"`
	LogMessages         []string               `json:"logMessages,omitempty"`
	PreBalances         []uint64               `json:"preBalances,omitempty"`
	PostBalances        []uint64               `json:"postBalances,omitempty"`
}

// InstructionResponse is an instruction in a transaction.
type InstructionResponse struct {
	ProgramIDIndex int    `json:"programIdIndex"`
	Accounts       []int  `json:"accounts"`
	DataHex        string `json:"dataHex"`
}

// MetricsResponse is the response for GET /api/metrics.
type MetricsResponse struct {
	// Memory stats
	MemAlloc      uint64  `json:"memAlloc"`       // Currently allocated heap memory
	MemTotalAlloc uint64  `json:"memTotalAlloc"`  // Total allocated (cumulative)
	MemSys        uint64  `json:"memSys"`         // Memory obtained from OS
	MemHeapInuse  uint64  `json:"memHeapInuse"`   // Heap memory in use
	MemHeapIdle   uint64  `json:"memHeapIdle"`    // Heap memory not in use
	NumGC         uint32  `json:"numGC"`          // Number of GC cycles

	// Runtime stats
	NumGoroutine  int     `json:"numGoroutine"`   // Number of goroutines
	NumCPU        int     `json:"numCPU"`         // Number of CPUs
	GoVersion     string  `json:"goVersion"`      // Go version

	// Database stats
	AccountsCount    uint64 `json:"accountsCount"`
	BlockCount       uint64 `json:"blockCount"`
	TransactionCount uint64 `json:"transactionCount"`
	DatabaseSize     int64  `json:"databaseSize"`

	// Node stats (if available)
	CurrentSlot     uint64  `json:"currentSlot"`
	LatestSlot      uint64  `json:"latestSlot"`
	BlocksProcessed uint64  `json:"blocksProcessed"`
	TxsProcessed    uint64  `json:"txsProcessed"`
	Uptime          float64 `json:"uptimeSeconds"`
}

// handleAPIStatus handles GET /api/status.
func (d *Dashboard) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := d.getStatusData()

	resp := StatusResponse{
		CurrentSlot:     getUint64(data, "CurrentSlot"),
		LatestSlot:      getUint64(data, "LatestSlot"),
		SlotsBehind:     getUint64(data, "SlotsBehind"),
		IsSyncing:       getBool(data, "IsSyncing"),
		IsRunning:       getBool(data, "IsRunning"),
		SyncStatus:      getString(data, "SyncStatus"),
		Uptime:          formatDuration(getDuration(data, "Uptime")),
		UptimeSeconds:   getDuration(data, "Uptime").Seconds(),
		BlocksProcessed: getUint64(data, "BlocksProcessed"),
		TxsProcessed:    getUint64(data, "TxsProcessed"),
		BlocksPerSec:    getFloat64(data, "BlocksPerSec"),
		AvgSlotTimeMs:   getFloat64(data, "AvgSlotTimeMs"),
		AccountsCount:   getUint64(data, "AccountsCount"),
		GeyserConnected: getBool(data, "GeyserConnected"),
		GeyserEndpoint:  getString(data, "GeyserEndpoint"),
		LastError:       getString(data, "LastError"),
	}

	writeJSON(w, resp)
}

// handleAPIBlocks handles GET /api/blocks.
func (d *Dashboard) handleAPIBlocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	perPage := 25
	if pp := r.URL.Query().Get("limit"); pp != "" {
		if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 100 {
			perPage = parsed
		}
	}

	blocks, totalPages := d.getRecentBlocks(page, perPage)

	var blockBriefs []BlockBrief
	for _, b := range blocks {
		blockBriefs = append(blockBriefs, BlockBrief{
			Slot:             b.Slot,
			Blockhash:        b.Blockhash.String(),
			TransactionCount: len(b.Transactions),
			BlockTime:        b.BlockTime,
		})
	}

	resp := BlocksListResponse{
		Blocks:      blockBriefs,
		CurrentPage: page,
		TotalPages:  totalPages,
		HasPrev:     page > 1,
		HasNext:     page < totalPages,
	}

	writeJSON(w, resp)
}

// handleAPIBlock handles GET /api/blocks/:slot.
func (d *Dashboard) handleAPIBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract slot from path
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/blocks/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, "Slot number required", http.StatusBadRequest)
		return
	}

	slot, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		writeError(w, "Invalid slot number", http.StatusBadRequest)
		return
	}

	block, err := d.blocks.GetBlock(slot)
	if err != nil {
		writeError(w, "Block not found", http.StatusNotFound)
		return
	}

	// Build response
	resp := BlockResponse{
		Slot:              block.Slot,
		ParentSlot:        block.ParentSlot,
		Blockhash:         block.Blockhash.String(),
		PreviousBlockhash: block.PreviousBlockhash.String(),
		BlockTime:         block.BlockTime,
		BlockHeight:       block.BlockHeight,
		TransactionCount:  len(block.Transactions),
	}

	// Add transactions
	includeTxs := r.URL.Query().Get("transactions") != "false"
	if includeTxs {
		for _, tx := range block.Transactions {
			brief := TransactionBrief{
				Signature:    tx.Signature.String(),
				Success:      tx.Meta == nil || tx.Meta.Err == nil,
				Accounts:     len(tx.Message.AccountKeys),
				Instructions: len(tx.Message.Instructions),
			}
			if tx.Meta != nil {
				brief.Fee = tx.Meta.Fee
			}
			resp.Transactions = append(resp.Transactions, brief)
		}
	}

	// Add rewards
	for _, r := range block.Rewards {
		resp.Rewards = append(resp.Rewards, RewardResponse{
			Pubkey:      r.Pubkey.String(),
			Lamports:    r.Lamports,
			PostBalance: r.PostBalance,
			RewardType:  rewardTypeToString(r.RewardType),
		})
	}

	writeJSON(w, resp)
}

// handleAPIAccount handles GET /api/accounts/:pubkey.
func (d *Dashboard) handleAPIAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract pubkey from path
	pubkeyStr := strings.TrimPrefix(r.URL.Path, "/api/accounts/")
	if pubkeyStr == "" {
		writeError(w, "Public key required", http.StatusBadRequest)
		return
	}

	pubkey, err := types.PubkeyFromBase58(pubkeyStr)
	if err != nil {
		writeError(w, "Invalid public key", http.StatusBadRequest)
		return
	}

	account, err := d.accounts.GetAccount(pubkey)
	if err != nil {
		writeError(w, "Account not found", http.StatusNotFound)
		return
	}

	resp := AccountResponse{
		Pubkey:     pubkey.String(),
		Lamports:   account.Lamports,
		Owner:      account.Owner.String(),
		Executable: account.Executable,
		RentEpoch:  account.RentEpoch,
		DataLen:    len(account.Data),
	}

	// Include first 256 bytes of data as hex
	if len(account.Data) > 0 {
		maxLen := 256
		if len(account.Data) < maxLen {
			maxLen = len(account.Data)
		}
		resp.DataHex = bytesToHex(account.Data[:maxLen])
	}

	writeJSON(w, resp)
}

// handleAPITransaction handles GET /api/transactions/:sig.
func (d *Dashboard) handleAPITransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract signature from path
	sigStr := strings.TrimPrefix(r.URL.Path, "/api/transactions/")
	if sigStr == "" {
		writeError(w, "Signature required", http.StatusBadRequest)
		return
	}

	sig, err := types.SignatureFromBase58(sigStr)
	if err != nil {
		writeError(w, "Invalid signature", http.StatusBadRequest)
		return
	}

	tx, err := d.blocks.GetTransaction(sig)
	if err != nil {
		writeError(w, "Transaction not found", http.StatusNotFound)
		return
	}

	resp := TransactionResponse{
		Signature: tx.Signature.String(),
		Slot:      tx.Slot,
		Success:   tx.Meta == nil || tx.Meta.Err == nil,
	}

	// Add error message if failed
	if tx.Meta != nil && tx.Meta.Err != nil {
		resp.Error = tx.Meta.Err.Message
	}

	// Add fee and compute units
	if tx.Meta != nil {
		resp.Fee = tx.Meta.Fee
		resp.ComputeUnitsConsumed = tx.Meta.ComputeUnitsConsumed
		resp.LogMessages = tx.Meta.LogMessages
		resp.PreBalances = tx.Meta.PreBalances
		resp.PostBalances = tx.Meta.PostBalances
	}

	// Add account keys
	for _, key := range tx.Message.AccountKeys {
		resp.Accounts = append(resp.Accounts, key.String())
	}

	// Add instructions
	for _, ix := range tx.Message.Instructions {
		var accounts []int
		for _, acc := range ix.AccountIndexes {
			accounts = append(accounts, int(acc))
		}
		resp.Instructions = append(resp.Instructions, InstructionResponse{
			ProgramIDIndex: int(ix.ProgramIDIndex),
			Accounts:       accounts,
			DataHex:        bytesToHex(ix.Data),
		})
	}

	writeJSON(w, resp)
}

// handleAPIMetrics handles GET /api/metrics.
func (d *Dashboard) handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	resp := MetricsResponse{
		// Memory stats
		MemAlloc:      memStats.Alloc,
		MemTotalAlloc: memStats.TotalAlloc,
		MemSys:        memStats.Sys,
		MemHeapInuse:  memStats.HeapInuse,
		MemHeapIdle:   memStats.HeapIdle,
		NumGC:         memStats.NumGC,

		// Runtime stats
		NumGoroutine: runtime.NumGoroutine(),
		NumCPU:       runtime.NumCPU(),
		GoVersion:    runtime.Version(),
	}

	// Add accounts count
	if count, err := d.accounts.AccountsCount(); err == nil {
		resp.AccountsCount = count
	}

	// Add blockstore stats
	if stats, err := d.blocks.GetStats(); err == nil {
		resp.BlockCount = stats.BlockCount
		resp.TransactionCount = stats.TransactionCount
		resp.DatabaseSize = stats.DatabaseSize
	}

	// Add node stats
	if d.nodeStats != nil {
		resp.CurrentSlot = d.nodeStats.CurrentSlot()
		resp.LatestSlot = d.nodeStats.LatestSlot()
		resp.BlocksProcessed = d.nodeStats.BlocksProcessed()
		resp.TxsProcessed = d.nodeStats.TxsProcessed()
		resp.Uptime = d.nodeStats.Uptime().Seconds()
	}

	writeJSON(w, resp)
}

// Helper functions

func getUint64(data map[string]interface{}, key string) uint64 {
	if v, ok := data[key]; ok {
		switch val := v.(type) {
		case uint64:
			return val
		case int64:
			return uint64(val)
		case int:
			return uint64(val)
		}
	}
	return 0
}

func getBool(data map[string]interface{}, key string) bool {
	if v, ok := data[key]; ok {
		if val, ok := v.(bool); ok {
			return val
		}
	}
	return false
}

func getString(data map[string]interface{}, key string) string {
	if v, ok := data[key]; ok {
		if val, ok := v.(string); ok {
			return val
		}
	}
	return ""
}

func getFloat64(data map[string]interface{}, key string) float64 {
	if v, ok := data[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case float32:
			return float64(val)
		case int:
			return float64(val)
		case int64:
			return float64(val)
		}
	}
	return 0
}

func getDuration(data map[string]interface{}, key string) time.Duration {
	if v, ok := data[key]; ok {
		if val, ok := v.(time.Duration); ok {
			return val
		}
	}
	return 0
}

func bytesToHex(data []byte) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, len(data)*2)
	for i, b := range data {
		result[i*2] = hexChars[b>>4]
		result[i*2+1] = hexChars[b&0x0f]
	}
	return string(result)
}

// writeJSONPretty writes a pretty-printed JSON response.
func writeJSONPretty(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.Encode(data)
}

// rewardTypeToString converts a RewardType to its string representation.
func rewardTypeToString(rt blockstore.RewardType) string {
	switch rt {
	case blockstore.RewardTypeFee:
		return "Fee"
	case blockstore.RewardTypeRent:
		return "Rent"
	case blockstore.RewardTypeStaking:
		return "Staking"
	case blockstore.RewardTypeVoting:
		return "Voting"
	default:
		return "Unknown"
	}
}
