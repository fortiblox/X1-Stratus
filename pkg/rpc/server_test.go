package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
	bstore "github.com/fortiblox/X1-Stratus/pkg/blockstore"
)

// mockBlockstore implements bstore.Store for testing.
type mockBlockstore struct {
	blocks       map[uint64]*bstore.Block
	transactions map[types.Signature]*bstore.Transaction
	latestSlot   uint64
	finalizedSlot uint64
	confirmedSlot uint64
	oldestSlot   uint64
}

func newMockBlockstore() *mockBlockstore {
	return &mockBlockstore{
		blocks:       make(map[uint64]*bstore.Block),
		transactions: make(map[types.Signature]*bstore.Transaction),
	}
}

func (m *mockBlockstore) GetBlock(slot uint64) (*bstore.Block, error) {
	block, ok := m.blocks[slot]
	if !ok {
		return nil, bstore.ErrBlockNotFound
	}
	return block, nil
}

func (m *mockBlockstore) PutBlock(block *bstore.Block) error {
	m.blocks[block.Slot] = block
	if block.Slot > m.latestSlot {
		m.latestSlot = block.Slot
	}
	return nil
}

func (m *mockBlockstore) HasBlock(slot uint64) bool {
	_, ok := m.blocks[slot]
	return ok
}

func (m *mockBlockstore) DeleteBlock(slot uint64) error {
	delete(m.blocks, slot)
	return nil
}

func (m *mockBlockstore) GetSlotMeta(slot uint64) (*bstore.SlotMeta, error) {
	return nil, bstore.ErrSlotNotFound
}

func (m *mockBlockstore) PutSlotMeta(meta *bstore.SlotMeta) error {
	return nil
}

func (m *mockBlockstore) SetCommitment(slot uint64, commitment bstore.CommitmentLevel) error {
	return nil
}

func (m *mockBlockstore) GetTransaction(signature types.Signature) (*bstore.Transaction, error) {
	tx, ok := m.transactions[signature]
	if !ok {
		return nil, bstore.ErrTransactionNotFound
	}
	return tx, nil
}

func (m *mockBlockstore) GetTransactionStatus(signature types.Signature) (*bstore.TransactionStatus, error) {
	tx, ok := m.transactions[signature]
	if !ok {
		return nil, bstore.ErrTransactionNotFound
	}
	return &bstore.TransactionStatus{
		Slot:               tx.Slot,
		Signature:          tx.Signature,
		ConfirmationStatus: bstore.CommitmentFinalized,
	}, nil
}

func (m *mockBlockstore) GetSignaturesForAddress(address types.Pubkey, opts *bstore.SignatureQueryOptions) ([]bstore.SignatureInfo, error) {
	return []bstore.SignatureInfo{}, nil
}

func (m *mockBlockstore) GetLatestSlot() uint64        { return m.latestSlot }
func (m *mockBlockstore) GetFinalizedSlot() uint64     { return m.finalizedSlot }
func (m *mockBlockstore) GetConfirmedSlot() uint64     { return m.confirmedSlot }
func (m *mockBlockstore) GetOldestSlot() uint64        { return m.oldestSlot }
func (m *mockBlockstore) SetLatestSlot(slot uint64) error    { m.latestSlot = slot; return nil }
func (m *mockBlockstore) SetFinalizedSlot(slot uint64) error { m.finalizedSlot = slot; return nil }
func (m *mockBlockstore) SetConfirmedSlot(slot uint64) error { m.confirmedSlot = slot; return nil }
func (m *mockBlockstore) IsRoot(slot uint64) bool            { return false }
func (m *mockBlockstore) SetRoot(slot uint64) error          { return nil }
func (m *mockBlockstore) GetRoots(start, end uint64) ([]uint64, error) { return nil, nil }
func (m *mockBlockstore) Prune(keepSlots uint64) (uint64, error)       { return 0, nil }
func (m *mockBlockstore) GetStats() (*bstore.Stats, error)         { return &bstore.Stats{}, nil }
func (m *mockBlockstore) Sync() error                                  { return nil }
func (m *mockBlockstore) Close() error                                 { return nil }

// Helper function to create a test server with mock dependencies.
func newTestServer() (*Server, *accounts.MemoryDB, *mockBlockstore) {
	accountsDB := accounts.NewMemoryDB()
	blockstore := newMockBlockstore()

	config := DefaultConfig()
	config.Addr = ":0" // Random port for testing

	server := New(config, accountsDB, blockstore)
	return server, accountsDB, blockstore
}

// Helper function to make an RPC request.
func makeRPCRequest(t *testing.T, server *Server, method string, params interface{}) *Response {
	t.Helper()

	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			t.Fatalf("Failed to marshal params: %v", err)
		}
	}

	req := Request{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  method,
		Params:  paramsRaw,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	httpReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.handleRPC(rr, httpReq)

	var resp Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	return &resp
}

// Test getHealth
func TestGetHealth(t *testing.T) {
	server, _, _ := newTestServer()

	resp := makeRPCRequest(t, server, "getHealth", nil)
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	result, ok := resp.Result.(string)
	if !ok {
		t.Fatalf("Expected string result, got: %T", resp.Result)
	}

	if result != "ok" {
		t.Errorf("Expected 'ok', got: %s", result)
	}
}

// Test getVersion
func TestGetVersion(t *testing.T) {
	server, _, _ := newTestServer()

	resp := makeRPCRequest(t, server, "getVersion", nil)
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got: %T", resp.Result)
	}

	if _, ok := result["solana-core"]; !ok {
		t.Error("Expected 'solana-core' in version response")
	}
}

// Test getSlot
func TestGetSlot(t *testing.T) {
	server, accountsDB, blockstore := newTestServer()

	// Set a slot
	accountsDB.SetSlot(12345)
	blockstore.latestSlot = 12345

	resp := makeRPCRequest(t, server, "getSlot", nil)
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	slot, ok := resp.Result.(float64) // JSON numbers are float64
	if !ok {
		t.Fatalf("Expected float64 result, got: %T", resp.Result)
	}

	if uint64(slot) != 12345 {
		t.Errorf("Expected slot 12345, got: %v", slot)
	}
}

// Test getBalance
func TestGetBalance(t *testing.T) {
	server, accountsDB, _ := newTestServer()

	// Create an account
	pubkey, _ := types.PubkeyFromBase58("11111111111111111111111111111111")
	account := &accounts.Account{
		Lamports:   1000000000, // 1 SOL
		Data:       []byte{},
		Owner:      types.SystemProgramAddr,
		Executable: false,
		RentEpoch:  0,
	}
	accountsDB.SetAccount(pubkey, account)

	resp := makeRPCRequest(t, server, "getBalance", []interface{}{pubkey.String()})
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got: %T", resp.Result)
	}

	value, ok := result["value"].(float64)
	if !ok {
		t.Fatalf("Expected value in result")
	}

	if uint64(value) != 1000000000 {
		t.Errorf("Expected balance 1000000000, got: %v", value)
	}
}

// Test getBalance for non-existent account
func TestGetBalanceNotFound(t *testing.T) {
	server, _, _ := newTestServer()

	pubkey, _ := types.PubkeyFromBase58("NonExistent11111111111111111111111")

	resp := makeRPCRequest(t, server, "getBalance", []interface{}{pubkey.String()})
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got: %T", resp.Result)
	}

	value, ok := result["value"].(float64)
	if !ok {
		t.Fatalf("Expected value in result")
	}

	if uint64(value) != 0 {
		t.Errorf("Expected balance 0, got: %v", value)
	}
}

// Test getAccountInfo
func TestGetAccountInfo(t *testing.T) {
	server, accountsDB, _ := newTestServer()

	// Create an account with data
	pubkey, _ := types.PubkeyFromBase58("11111111111111111111111111111111")
	accountData := []byte{1, 2, 3, 4, 5}
	account := &accounts.Account{
		Lamports:   1000000000,
		Data:       accountData,
		Owner:      types.SystemProgramAddr,
		Executable: false,
		RentEpoch:  100,
	}
	accountsDB.SetAccount(pubkey, account)

	resp := makeRPCRequest(t, server, "getAccountInfo", []interface{}{
		pubkey.String(),
		map[string]string{"encoding": "base64"},
	})
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got: %T", resp.Result)
	}

	value, ok := result["value"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected value in result")
	}

	// Check lamports
	lamports, ok := value["lamports"].(float64)
	if !ok || uint64(lamports) != 1000000000 {
		t.Errorf("Expected lamports 1000000000, got: %v", lamports)
	}

	// Check owner
	owner, ok := value["owner"].(string)
	if !ok || owner != types.SystemProgramAddr.String() {
		t.Errorf("Expected owner %s, got: %v", types.SystemProgramAddr.String(), owner)
	}

	// Check executable
	executable, ok := value["executable"].(bool)
	if !ok || executable != false {
		t.Errorf("Expected executable false, got: %v", executable)
	}
}

// Test getAccountInfo for non-existent account
func TestGetAccountInfoNotFound(t *testing.T) {
	server, _, _ := newTestServer()

	pubkey, _ := types.PubkeyFromBase58("NonExistent11111111111111111111111")

	resp := makeRPCRequest(t, server, "getAccountInfo", []interface{}{pubkey.String()})
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got: %T", resp.Result)
	}

	// Value should be null for non-existent account
	if result["value"] != nil {
		t.Errorf("Expected null value for non-existent account, got: %v", result["value"])
	}
}

// Test getBlock
func TestGetBlock(t *testing.T) {
	server, _, blockstore := newTestServer()

	// Create a block
	blockTime := int64(1234567890)
	blockHeight := uint64(100)
	block := &bstore.Block{
		Slot:              12345,
		ParentSlot:        12344,
		Blockhash:         types.Hash{1, 2, 3},
		PreviousBlockhash: types.Hash{0, 1, 2},
		BlockTime:         &blockTime,
		BlockHeight:       &blockHeight,
		Transactions:      []bstore.Transaction{},
	}
	blockstore.PutBlock(block)

	resp := makeRPCRequest(t, server, "getBlock", []interface{}{12345})
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got: %T", resp.Result)
	}

	// Check parent slot
	parentSlot, ok := result["parentSlot"].(float64)
	if !ok || uint64(parentSlot) != 12344 {
		t.Errorf("Expected parentSlot 12344, got: %v", parentSlot)
	}

	// Check block time
	blockTimeResult, ok := result["blockTime"].(float64)
	if !ok || int64(blockTimeResult) != 1234567890 {
		t.Errorf("Expected blockTime 1234567890, got: %v", blockTimeResult)
	}
}

// Test getBlock for non-existent block
func TestGetBlockNotFound(t *testing.T) {
	server, _, _ := newTestServer()

	resp := makeRPCRequest(t, server, "getBlock", []interface{}{99999})
	// For future slots, should return null
	if resp.Result != nil {
		// For past slots that don't exist, should return error
		if resp.Error == nil {
			t.Error("Expected null result or error for non-existent block")
		}
	}
}

// Test getBlockHeight
func TestGetBlockHeight(t *testing.T) {
	server, _, blockstore := newTestServer()

	// Create a block with block height
	blockHeight := uint64(500)
	block := &bstore.Block{
		Slot:        1000,
		BlockHeight: &blockHeight,
	}
	blockstore.PutBlock(block)

	resp := makeRPCRequest(t, server, "getBlockHeight", nil)
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	height, ok := resp.Result.(float64)
	if !ok {
		t.Fatalf("Expected float64 result, got: %T", resp.Result)
	}

	if uint64(height) != 500 {
		t.Errorf("Expected block height 500, got: %v", height)
	}
}

// Test getGenesisHash
func TestGetGenesisHash(t *testing.T) {
	server, _, _ := newTestServer()

	resp := makeRPCRequest(t, server, "getGenesisHash", nil)
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	_, ok := resp.Result.(string)
	if !ok {
		t.Fatalf("Expected string result, got: %T", resp.Result)
	}
}

// Test getEpochInfo
func TestGetEpochInfo(t *testing.T) {
	server, _, blockstore := newTestServer()

	// Set a slot
	blockstore.latestSlot = 500000

	resp := makeRPCRequest(t, server, "getEpochInfo", nil)
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got: %T", resp.Result)
	}

	// Check absolute slot
	absoluteSlot, ok := result["absoluteSlot"].(float64)
	if !ok || uint64(absoluteSlot) != 500000 {
		t.Errorf("Expected absoluteSlot 500000, got: %v", absoluteSlot)
	}

	// Check epoch
	epoch, ok := result["epoch"].(float64)
	if !ok || uint64(epoch) != 1 {
		t.Errorf("Expected epoch 1, got: %v", epoch)
	}
}

// Test getMultipleAccounts
func TestGetMultipleAccounts(t *testing.T) {
	server, accountsDB, _ := newTestServer()

	// Create two accounts
	pubkey1, _ := types.PubkeyFromBase58("11111111111111111111111111111111")
	pubkey2, _ := types.PubkeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")

	account1 := &accounts.Account{Lamports: 1000, Owner: types.SystemProgramAddr}
	account2 := &accounts.Account{Lamports: 2000, Owner: types.SystemProgramAddr}

	accountsDB.SetAccount(pubkey1, account1)
	accountsDB.SetAccount(pubkey2, account2)

	resp := makeRPCRequest(t, server, "getMultipleAccounts", []interface{}{
		[]string{pubkey1.String(), pubkey2.String()},
		map[string]string{"encoding": "base64"},
	})
	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got: %T", resp.Result)
	}

	value, ok := result["value"].([]interface{})
	if !ok {
		t.Fatalf("Expected array value in result")
	}

	if len(value) != 2 {
		t.Errorf("Expected 2 accounts, got: %d", len(value))
	}
}

// Test method not found
func TestMethodNotFound(t *testing.T) {
	server, _, _ := newTestServer()

	resp := makeRPCRequest(t, server, "nonExistentMethod", nil)
	if resp.Error == nil {
		t.Fatal("Expected error for non-existent method")
	}

	if resp.Error.Code != MethodNotFound {
		t.Errorf("Expected error code %d, got: %d", MethodNotFound, resp.Error.Code)
	}
}

// Test invalid params
func TestInvalidParams(t *testing.T) {
	server, _, _ := newTestServer()

	// getBalance requires a pubkey
	resp := makeRPCRequest(t, server, "getBalance", []interface{}{})
	if resp.Error == nil {
		t.Fatal("Expected error for missing params")
	}

	if resp.Error.Code != InvalidParams {
		t.Errorf("Expected error code %d, got: %d", InvalidParams, resp.Error.Code)
	}
}

// Test batch request
func TestBatchRequest(t *testing.T) {
	server, _, _ := newTestServer()

	requests := []Request{
		{JSONRPC: JSONRPCVersion, ID: 1, Method: "getHealth"},
		{JSONRPC: JSONRPCVersion, ID: 2, Method: "getVersion"},
	}

	body, _ := json.Marshal(requests)
	httpReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.handleRPC(rr, httpReq)

	var responses []Response
	if err := json.Unmarshal(rr.Body.Bytes(), &responses); err != nil {
		t.Fatalf("Failed to unmarshal batch response: %v", err)
	}

	if len(responses) != 2 {
		t.Errorf("Expected 2 responses, got: %d", len(responses))
	}

	for _, resp := range responses {
		if resp.Error != nil {
			t.Errorf("Unexpected error in batch response: %v", resp.Error)
		}
	}
}

// Test CORS headers
func TestCORSHeaders(t *testing.T) {
	server, _, _ := newTestServer()

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://example.com")

	rr := httptest.NewRecorder()
	handler := server.corsMiddleware(http.HandlerFunc(server.handleRPC))
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("Expected status %d for OPTIONS, got: %d", http.StatusNoContent, rr.Code)
	}

	if rr.Header().Get("Access-Control-Allow-Origin") != "http://example.com" {
		t.Error("Expected CORS Allow-Origin header")
	}
}

// Test server lifecycle
func TestServerLifecycle(t *testing.T) {
	server, _, _ := newTestServer()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Wait for context to timeout
	<-ctx.Done()

	// Wait for server to stop
	select {
	case err := <-errCh:
		if err != nil && err != context.DeadlineExceeded {
			t.Errorf("Unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Server did not stop in time")
	}
}

// Test encoding helpers
func TestEncoding(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5}

	// Test base64 encoding
	encoded, err := EncodeAccountData(data, EncodingBase64)
	if err != nil {
		t.Fatalf("Failed to encode base64: %v", err)
	}

	encArray, ok := encoded.([]string)
	if !ok || len(encArray) != 2 {
		t.Fatal("Expected [data, encoding] array")
	}

	if encArray[1] != string(EncodingBase64) {
		t.Errorf("Expected encoding 'base64', got: %s", encArray[1])
	}

	// Decode and verify
	decoded, err := DecodeAccountData(encArray[0], EncodingBase64)
	if err != nil {
		t.Fatalf("Failed to decode base64: %v", err)
	}

	if !bytes.Equal(decoded, data) {
		t.Errorf("Decoded data doesn't match original")
	}
}

// Test data slice
func TestDataSlice(t *testing.T) {
	data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	slice := &DataSlice{Offset: 2, Length: 4}
	result := ApplyDataSlice(data, slice)

	expected := []byte{2, 3, 4, 5}
	if !bytes.Equal(result, expected) {
		t.Errorf("Expected %v, got: %v", expected, result)
	}

	// Test nil slice returns original
	result = ApplyDataSlice(data, nil)
	if !bytes.Equal(result, data) {
		t.Error("Expected original data when slice is nil")
	}

	// Test offset beyond data length
	slice = &DataSlice{Offset: 100, Length: 4}
	result = ApplyDataSlice(data, slice)
	if len(result) != 0 {
		t.Errorf("Expected empty slice when offset beyond data, got: %v", result)
	}
}
