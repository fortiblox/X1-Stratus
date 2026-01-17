package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
	"github.com/fortiblox/X1-Stratus/pkg/blockstore"
)

// mockNodeStats implements NodeStats for testing.
type mockNodeStats struct {
	currentSlot     uint64
	latestSlot      uint64
	isSyncing       bool
	isRunning       bool
	uptime          time.Duration
	blocksProcessed uint64
	txsProcessed    uint64
	avgSlotTimeMs   float64
	geyserConnected bool
	geyserEndpoint  string
	lastError       error
}

func (m *mockNodeStats) CurrentSlot() uint64         { return m.currentSlot }
func (m *mockNodeStats) LatestSlot() uint64          { return m.latestSlot }
func (m *mockNodeStats) IsSyncing() bool             { return m.isSyncing }
func (m *mockNodeStats) IsRunning() bool             { return m.isRunning }
func (m *mockNodeStats) Uptime() time.Duration       { return m.uptime }
func (m *mockNodeStats) BlocksProcessed() uint64     { return m.blocksProcessed }
func (m *mockNodeStats) TxsProcessed() uint64        { return m.txsProcessed }
func (m *mockNodeStats) AvgSlotTimeMs() float64      { return m.avgSlotTimeMs }
func (m *mockNodeStats) GeyserConnected() bool       { return m.geyserConnected }
func (m *mockNodeStats) GeyserEndpoint() string      { return m.geyserEndpoint }
func (m *mockNodeStats) LastError() error            { return m.lastError }

// mockBlockstore implements blockstore.Store for testing.
type mockBlockstore struct {
	blocks       map[uint64]*blockstore.Block
	transactions map[types.Signature]*blockstore.Transaction
	stats        *blockstore.Stats
}

func newMockBlockstore() *mockBlockstore {
	return &mockBlockstore{
		blocks:       make(map[uint64]*blockstore.Block),
		transactions: make(map[types.Signature]*blockstore.Transaction),
		stats: &blockstore.Stats{
			LatestSlot:       1000,
			FinalizedSlot:    950,
			ConfirmedSlot:    980,
			OldestSlot:       100,
			BlockCount:       900,
			TransactionCount: 5000,
			DatabaseSize:     1024 * 1024,
		},
	}
}

func (m *mockBlockstore) GetBlock(slot uint64) (*blockstore.Block, error) {
	if b, ok := m.blocks[slot]; ok {
		return b, nil
	}
	return nil, blockstore.ErrBlockNotFound
}

func (m *mockBlockstore) PutBlock(block *blockstore.Block) error {
	m.blocks[block.Slot] = block
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

func (m *mockBlockstore) GetSlotMeta(slot uint64) (*blockstore.SlotMeta, error) {
	return nil, blockstore.ErrSlotNotFound
}

func (m *mockBlockstore) PutSlotMeta(meta *blockstore.SlotMeta) error {
	return nil
}

func (m *mockBlockstore) SetCommitment(slot uint64, commitment blockstore.CommitmentLevel) error {
	return nil
}

func (m *mockBlockstore) GetTransaction(signature types.Signature) (*blockstore.Transaction, error) {
	if tx, ok := m.transactions[signature]; ok {
		return tx, nil
	}
	return nil, blockstore.ErrTransactionNotFound
}

func (m *mockBlockstore) GetTransactionStatus(signature types.Signature) (*blockstore.TransactionStatus, error) {
	return nil, blockstore.ErrTransactionNotFound
}

func (m *mockBlockstore) GetSignaturesForAddress(address types.Pubkey, opts *blockstore.SignatureQueryOptions) ([]blockstore.SignatureInfo, error) {
	return nil, nil
}

func (m *mockBlockstore) GetLatestSlot() uint64    { return m.stats.LatestSlot }
func (m *mockBlockstore) GetFinalizedSlot() uint64 { return m.stats.FinalizedSlot }
func (m *mockBlockstore) GetConfirmedSlot() uint64 { return m.stats.ConfirmedSlot }
func (m *mockBlockstore) GetOldestSlot() uint64    { return m.stats.OldestSlot }

func (m *mockBlockstore) SetLatestSlot(slot uint64) error {
	m.stats.LatestSlot = slot
	return nil
}

func (m *mockBlockstore) SetFinalizedSlot(slot uint64) error {
	m.stats.FinalizedSlot = slot
	return nil
}

func (m *mockBlockstore) SetConfirmedSlot(slot uint64) error {
	m.stats.ConfirmedSlot = slot
	return nil
}

func (m *mockBlockstore) IsRoot(slot uint64) bool         { return false }
func (m *mockBlockstore) SetRoot(slot uint64) error       { return nil }
func (m *mockBlockstore) GetRoots(start, end uint64) ([]uint64, error) { return nil, nil }
func (m *mockBlockstore) Prune(keepSlots uint64) (uint64, error) { return 0, nil }
func (m *mockBlockstore) GetStats() (*blockstore.Stats, error) { return m.stats, nil }
func (m *mockBlockstore) Sync() error                     { return nil }
func (m *mockBlockstore) Close() error                    { return nil }

// Helper to create a test dashboard
func newTestDashboard(t *testing.T) *Dashboard {
	t.Helper()

	blocks := newMockBlockstore()
	accts := accounts.NewMemoryDB()
	stats := &mockNodeStats{
		currentSlot:     1000,
		latestSlot:      1005,
		isSyncing:       false,
		isRunning:       true,
		uptime:          5 * time.Hour,
		blocksProcessed: 10000,
		txsProcessed:    50000,
		avgSlotTimeMs:   50.5,
		geyserConnected: true,
		geyserEndpoint:  "grpc.example.com:443",
	}

	// Add some test blocks
	for slot := uint64(990); slot <= 1000; slot++ {
		blocks.PutBlock(&blockstore.Block{
			Slot:       slot,
			ParentSlot: slot - 1,
			Blockhash:  types.ComputeHash([]byte{byte(slot)}),
			Transactions: []blockstore.Transaction{
				{
					Signature: types.Signature{byte(slot)},
					Slot:      slot,
					Message: blockstore.TransactionMessage{
						AccountKeys: []types.Pubkey{{byte(slot)}},
					},
				},
			},
		})
	}

	// Add a test account
	testPubkey, _ := types.PubkeyFromBase58("11111111111111111111111111111111")
	accts.SetAccount(testPubkey, &accounts.Account{
		Lamports:   1000000000,
		Data:       []byte{1, 2, 3, 4},
		Owner:      types.Pubkey{},
		Executable: false,
		RentEpoch:  0,
	})

	dash, err := New(DefaultConfig(), blocks, accts, stats)
	if err != nil {
		t.Fatalf("Failed to create dashboard: %v", err)
	}

	return dash
}

func TestDashboardNew(t *testing.T) {
	blocks := newMockBlockstore()
	accts := accounts.NewMemoryDB()

	// Test with defaults
	dash, err := New(Config{}, blocks, accts, nil)
	if err != nil {
		t.Fatalf("Failed to create dashboard with defaults: %v", err)
	}

	if dash.config.BindAddress != "127.0.0.1" {
		t.Errorf("Expected default bind address 127.0.0.1, got %s", dash.config.BindAddress)
	}

	if dash.config.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", dash.config.Port)
	}

	// Test with custom config
	customConfig := Config{
		BindAddress: "0.0.0.0",
		Port:        9000,
	}

	dash, err = New(customConfig, blocks, accts, nil)
	if err != nil {
		t.Fatalf("Failed to create dashboard with custom config: %v", err)
	}

	if dash.config.BindAddress != "0.0.0.0" {
		t.Errorf("Expected bind address 0.0.0.0, got %s", dash.config.BindAddress)
	}

	if dash.config.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", dash.config.Port)
	}
}

func TestAPIStatusEndpoint(t *testing.T) {
	dash := newTestDashboard(t)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()

	dash.handleAPIStatus(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", resp.Header.Get("Content-Type"))
	}

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if status.CurrentSlot != 1000 {
		t.Errorf("Expected current slot 1000, got %d", status.CurrentSlot)
	}

	if status.LatestSlot != 1005 {
		t.Errorf("Expected latest slot 1005, got %d", status.LatestSlot)
	}

	if status.SlotsBehind != 5 {
		t.Errorf("Expected slots behind 5, got %d", status.SlotsBehind)
	}

	if !status.GeyserConnected {
		t.Error("Expected geyser connected to be true")
	}
}

func TestAPIBlocksEndpoint(t *testing.T) {
	dash := newTestDashboard(t)

	req := httptest.NewRequest(http.MethodGet, "/api/blocks?page=1&limit=5", nil)
	w := httptest.NewRecorder()

	dash.handleAPIBlocks(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %d", resp.StatusCode)
	}

	var result BlocksListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(result.Blocks) == 0 {
		t.Error("Expected at least one block in response")
	}
}

func TestAPIBlockEndpoint(t *testing.T) {
	dash := newTestDashboard(t)

	// Test existing block
	req := httptest.NewRequest(http.MethodGet, "/api/blocks/1000", nil)
	w := httptest.NewRecorder()

	dash.handleAPIBlock(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK for existing block, got %d", resp.StatusCode)
	}

	var block BlockResponse
	if err := json.NewDecoder(resp.Body).Decode(&block); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if block.Slot != 1000 {
		t.Errorf("Expected slot 1000, got %d", block.Slot)
	}

	// Test non-existing block
	req = httptest.NewRequest(http.MethodGet, "/api/blocks/99999", nil)
	w = httptest.NewRecorder()

	dash.handleAPIBlock(w, req)

	resp = w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status NotFound for non-existing block, got %d", resp.StatusCode)
	}
}

func TestAPIAccountEndpoint(t *testing.T) {
	dash := newTestDashboard(t)

	// Test existing account
	req := httptest.NewRequest(http.MethodGet, "/api/accounts/11111111111111111111111111111111", nil)
	w := httptest.NewRecorder()

	dash.handleAPIAccount(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK for existing account, got %d", resp.StatusCode)
	}

	var account AccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if account.Lamports != 1000000000 {
		t.Errorf("Expected lamports 1000000000, got %d", account.Lamports)
	}

	// Test non-existing account - use valid format pubkey that isn't in the database
	// The system program ID is a valid pubkey format but with a different address
	req = httptest.NewRequest(http.MethodGet, "/api/accounts/TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA", nil)
	w = httptest.NewRecorder()

	dash.handleAPIAccount(w, req)

	resp = w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status NotFound for non-existing account, got %d", resp.StatusCode)
	}

	// Test invalid pubkey
	req = httptest.NewRequest(http.MethodGet, "/api/accounts/invalid", nil)
	w = httptest.NewRecorder()

	dash.handleAPIAccount(w, req)

	resp = w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status BadRequest for invalid pubkey, got %d", resp.StatusCode)
	}
}

func TestAPIMetricsEndpoint(t *testing.T) {
	dash := newTestDashboard(t)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()

	dash.handleAPIMetrics(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %d", resp.StatusCode)
	}

	var metrics MetricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if metrics.NumCPU == 0 {
		t.Error("Expected non-zero CPU count")
	}

	if metrics.GoVersion == "" {
		t.Error("Expected non-empty Go version")
	}
}

func TestStaticAssets(t *testing.T) {
	// Test CSS
	content, contentType, ok := getStaticAsset("style.css")
	if !ok {
		t.Error("Expected style.css to exist")
	}
	if contentType != "text/css" {
		t.Errorf("Expected text/css, got %s", contentType)
	}
	if content == "" {
		t.Error("Expected non-empty CSS content")
	}

	// Test JS
	content, contentType, ok = getStaticAsset("app.js")
	if !ok {
		t.Error("Expected app.js to exist")
	}
	if contentType != "application/javascript" {
		t.Errorf("Expected application/javascript, got %s", contentType)
	}
	if content == "" {
		t.Error("Expected non-empty JS content")
	}

	// Test non-existing asset
	_, _, ok = getStaticAsset("nonexistent.txt")
	if ok {
		t.Error("Expected nonexistent.txt to not exist")
	}
}

func TestTemplateHelpers(t *testing.T) {
	// Test formatDuration
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m 30s"},
		{2 * time.Hour, "2h 0m"},
		{25 * time.Hour, "1d 1h"},
	}

	for _, tc := range tests {
		result := formatDuration(tc.duration)
		if result != tc.expected {
			t.Errorf("formatDuration(%v) = %s, expected %s", tc.duration, result, tc.expected)
		}
	}

	// Test formatNumber
	if formatNumber(1000) != "1.0K" {
		t.Errorf("Expected 1.0K, got %s", formatNumber(1000))
	}
	if formatNumber(1500000) != "1.5M" {
		t.Errorf("Expected 1.5M, got %s", formatNumber(1500000))
	}

	// Test formatBytes
	if formatBytes(1024) != "1.0 KB" {
		t.Errorf("Expected 1.0 KB, got %s", formatBytes(1024))
	}
	if formatBytes(1048576) != "1.0 MB" {
		t.Errorf("Expected 1.0 MB, got %s", formatBytes(1048576))
	}

	// Test truncateHash
	hash := "abcdefghijklmnopqrstuvwxyz"
	truncated := truncateHash(hash, 4)
	if truncated != "abcd...wxyz" {
		t.Errorf("Expected abcd...wxyz, got %s", truncated)
	}
}

func TestHomePageHandler(t *testing.T) {
	dash := newTestDashboard(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	dash.handleHome(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %d. Body: %s", resp.StatusCode, string(body))
	}

	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("Expected Content-Type text/html, got %s", resp.Header.Get("Content-Type"))
	}
}

func TestBlocksPageHandler(t *testing.T) {
	dash := newTestDashboard(t)

	req := httptest.NewRequest(http.MethodGet, "/blocks", nil)
	w := httptest.NewRecorder()

	dash.handleBlocks(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %d", resp.StatusCode)
	}
}

func TestAccountsPageHandler(t *testing.T) {
	dash := newTestDashboard(t)

	// Test without query
	req := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	w := httptest.NewRecorder()

	dash.handleAccounts(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %d", resp.StatusCode)
	}

	// Test with query
	req = httptest.NewRequest(http.MethodGet, "/accounts?q=11111111111111111111111111111111", nil)
	w = httptest.NewRecorder()

	dash.handleAccounts(w, req)

	resp = w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %d", resp.StatusCode)
	}
}

func TestSettingsPageHandler(t *testing.T) {
	dash := newTestDashboard(t)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()

	dash.handleSettings(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %d", resp.StatusCode)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	dash := newTestDashboard(t)

	// Test POST to status endpoint (should fail)
	req := httptest.NewRequest(http.MethodPost, "/api/status", nil)
	w := httptest.NewRecorder()

	dash.handleAPIStatus(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status MethodNotAllowed, got %d", resp.StatusCode)
	}
}

func TestBytesToHex(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte{}, ""},
		{[]byte{0x00}, "00"},
		{[]byte{0xff}, "ff"},
		{[]byte{0x12, 0x34}, "1234"},
		{[]byte{0xab, 0xcd, 0xef}, "abcdef"},
	}

	for _, tc := range tests {
		result := bytesToHex(tc.input)
		if result != tc.expected {
			t.Errorf("bytesToHex(%v) = %s, expected %s", tc.input, result, tc.expected)
		}
	}
}
