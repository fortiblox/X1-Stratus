// Package dashboard provides an embedded web dashboard for monitoring the X1-Stratus node.
//
// The dashboard provides:
// - Real-time sync status and node health
// - Recent blocks browser with pagination
// - Account lookup by public key
// - Transaction search by signature
// - System metrics (memory, CPU, uptime)
//
// All static assets are embedded using Go 1.16+ embed, making the binary self-contained.
package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
	"github.com/fortiblox/X1-Stratus/pkg/blockstore"
)

// Config holds dashboard configuration options.
type Config struct {
	// BindAddress is the address to bind the HTTP server to.
	// Default: "127.0.0.1"
	BindAddress string

	// Port is the port to listen on.
	// Default: 8080
	Port int

	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration

	// IdleTimeout is the maximum time to wait for the next request.
	IdleTimeout time.Duration
}

// DefaultConfig returns the default dashboard configuration.
func DefaultConfig() Config {
	return Config{
		BindAddress:  "127.0.0.1",
		Port:         8080,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// NodeStats provides node statistics to the dashboard.
// This interface abstracts the node's internal stats for dashboard consumption.
type NodeStats interface {
	// CurrentSlot returns the most recently processed slot.
	CurrentSlot() uint64

	// LatestSlot returns the latest slot seen from the network.
	LatestSlot() uint64

	// IsSyncing returns true if the node is currently syncing.
	IsSyncing() bool

	// IsRunning returns true if the node is running.
	IsRunning() bool

	// Uptime returns how long the node has been running.
	Uptime() time.Duration

	// BlocksProcessed returns the total number of blocks processed.
	BlocksProcessed() uint64

	// TxsProcessed returns the total number of transactions processed.
	TxsProcessed() uint64

	// AvgSlotTimeMs returns the average slot processing time in milliseconds.
	AvgSlotTimeMs() float64

	// GeyserConnected returns true if connected to Geyser.
	GeyserConnected() bool

	// GeyserEndpoint returns the Geyser endpoint.
	GeyserEndpoint() string

	// LastError returns the last error encountered, if any.
	LastError() error
}

// Dashboard is the web dashboard server.
type Dashboard struct {
	config    Config
	server    *http.Server
	blocks    blockstore.Store
	accounts  accounts.DB
	nodeStats NodeStats

	// Cached templates
	templates *template.Template

	// State
	mu        sync.RWMutex
	running   bool
	startTime time.Time
}

// New creates a new dashboard server.
func New(config Config, blocks blockstore.Store, accts accounts.DB, stats NodeStats) (*Dashboard, error) {
	// Apply defaults
	if config.BindAddress == "" {
		config.BindAddress = DefaultConfig().BindAddress
	}
	if config.Port == 0 {
		config.Port = DefaultConfig().Port
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = DefaultConfig().ReadTimeout
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = DefaultConfig().WriteTimeout
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = DefaultConfig().IdleTimeout
	}

	d := &Dashboard{
		config:    config,
		blocks:    blocks,
		accounts:  accts,
		nodeStats: stats,
	}

	// Parse templates
	tmpl, err := d.parseTemplates()
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	d.templates = tmpl

	return d, nil
}

// parseTemplates parses all embedded templates.
func (d *Dashboard) parseTemplates() (*template.Template, error) {
	funcMap := template.FuncMap{
		"formatDuration": formatDuration,
		"formatNumber":   formatNumber,
		"formatBytes":    formatBytes,
		"formatTime":     formatTime,
		"truncateHash":   truncateHash,
		"add":            func(a, b int) int { return a + b },
		"sub":            func(a, b int) int { return a - b },
		"mul":            func(a, b int) int { return a * b },
		"div":            func(a, b int) int { return a / b },
		"divf":           func(a uint64, b float64) float64 { return float64(a) / b },
		"int":            func(v uint64) int { return int(v) },
		"seq":            seq,
	}

	tmpl := template.New("").Funcs(funcMap)

	// Parse layout with explicit name
	_, err := tmpl.New("layout").Parse(layoutTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}

	// Parse page templates
	templates := map[string]string{
		"home":        homeTemplate,
		"blocks":      blocksTemplate,
		"block":       blockDetailTemplate,
		"accounts":    accountsTemplate,
		"account":     accountDetailTemplate,
		"transaction": transactionTemplate,
		"settings":    settingsTemplate,
	}

	for name, content := range templates {
		_, err := tmpl.New(name).Parse(content)
		if err != nil {
			return nil, fmt.Errorf("parse %s template: %w", name, err)
		}
	}

	return tmpl, nil
}

// Start starts the dashboard HTTP server.
func (d *Dashboard) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("dashboard already running")
	}
	d.running = true
	d.startTime = time.Now()
	d.mu.Unlock()

	// Create the HTTP mux
	mux := http.NewServeMux()

	// Static assets
	mux.HandleFunc("/static/", d.handleStatic)

	// Page routes
	mux.HandleFunc("/", d.handleHome)
	mux.HandleFunc("/blocks", d.handleBlocks)
	mux.HandleFunc("/blocks/", d.handleBlockDetail)
	mux.HandleFunc("/accounts", d.handleAccounts)
	mux.HandleFunc("/accounts/", d.handleAccountDetail)
	mux.HandleFunc("/transactions/", d.handleTransaction)
	mux.HandleFunc("/settings", d.handleSettings)

	// API routes
	mux.HandleFunc("/api/status", d.handleAPIStatus)
	mux.HandleFunc("/api/blocks", d.handleAPIBlocks)
	mux.HandleFunc("/api/blocks/", d.handleAPIBlock)
	mux.HandleFunc("/api/accounts/", d.handleAPIAccount)
	mux.HandleFunc("/api/transactions/", d.handleAPITransaction)
	mux.HandleFunc("/api/metrics", d.handleAPIMetrics)

	// Create the server
	addr := fmt.Sprintf("%s:%d", d.config.BindAddress, d.config.Port)
	d.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  d.config.ReadTimeout,
		WriteTimeout: d.config.WriteTimeout,
		IdleTimeout:  d.config.IdleTimeout,
		BaseContext:  func(net.Listener) context.Context { return ctx },
	}

	// Start serving
	go func() {
		<-ctx.Done()
		d.Stop()
	}()

	return d.server.ListenAndServe()
}

// Stop gracefully stops the dashboard server.
func (d *Dashboard) Stop() error {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return nil
	}
	d.running = false
	d.mu.Unlock()

	if d.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return d.server.Shutdown(ctx)
	}

	return nil
}

// Address returns the address the dashboard is listening on.
func (d *Dashboard) Address() string {
	return fmt.Sprintf("%s:%d", d.config.BindAddress, d.config.Port)
}

// handleHome renders the home/overview page.
func (d *Dashboard) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data := d.getStatusData()
	d.renderPage(w, "home", data)
}

// handleBlocks renders the blocks list page.
func (d *Dashboard) handleBlocks(w http.ResponseWriter, r *http.Request) {
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	perPage := 25
	blocks, totalPages := d.getRecentBlocks(page, perPage)

	data := map[string]interface{}{
		"Blocks":      blocks,
		"CurrentPage": page,
		"TotalPages":  totalPages,
		"HasPrev":     page > 1,
		"HasNext":     page < totalPages,
	}

	d.renderPage(w, "blocks", data)
}

// handleBlockDetail renders a single block's details.
func (d *Dashboard) handleBlockDetail(w http.ResponseWriter, r *http.Request) {
	// Extract slot from path: /blocks/{slot}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/blocks/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Redirect(w, r, "/blocks", http.StatusFound)
		return
	}

	slot, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid slot number", http.StatusBadRequest)
		return
	}

	block, err := d.blocks.GetBlock(slot)
	if err != nil {
		d.renderPage(w, "block", map[string]interface{}{
			"Error": fmt.Sprintf("Block not found: %v", err),
			"Slot":  slot,
		})
		return
	}

	d.renderPage(w, "block", map[string]interface{}{
		"Block": block,
	})
}

// handleAccounts renders the accounts search page.
func (d *Dashboard) handleAccounts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	var account *accounts.Account
	var pubkey types.Pubkey
	var searchErr string

	if query != "" {
		var err error
		pubkey, err = types.PubkeyFromBase58(query)
		if err != nil {
			searchErr = fmt.Sprintf("Invalid public key: %v", err)
		} else {
			account, err = d.accounts.GetAccount(pubkey)
			if err != nil {
				searchErr = fmt.Sprintf("Account not found: %v", err)
			}
		}
	}

	d.renderPage(w, "accounts", map[string]interface{}{
		"Query":     query,
		"Account":   account,
		"Pubkey":    pubkey.String(),
		"SearchErr": searchErr,
	})
}

// handleAccountDetail renders account details.
func (d *Dashboard) handleAccountDetail(w http.ResponseWriter, r *http.Request) {
	// Extract pubkey from path: /accounts/{pubkey}
	pubkeyStr := strings.TrimPrefix(r.URL.Path, "/accounts/")
	if pubkeyStr == "" {
		http.Redirect(w, r, "/accounts", http.StatusFound)
		return
	}

	pubkey, err := types.PubkeyFromBase58(pubkeyStr)
	if err != nil {
		d.renderPage(w, "account", map[string]interface{}{
			"Error":  fmt.Sprintf("Invalid public key: %v", err),
			"Pubkey": pubkeyStr,
		})
		return
	}

	account, err := d.accounts.GetAccount(pubkey)
	if err != nil {
		d.renderPage(w, "account", map[string]interface{}{
			"Error":  fmt.Sprintf("Account not found: %v", err),
			"Pubkey": pubkeyStr,
		})
		return
	}

	d.renderPage(w, "account", map[string]interface{}{
		"Account": account,
		"Pubkey":  pubkey.String(),
	})
}

// handleTransaction renders transaction details.
func (d *Dashboard) handleTransaction(w http.ResponseWriter, r *http.Request) {
	// Extract signature from path: /transactions/{signature}
	sigStr := strings.TrimPrefix(r.URL.Path, "/transactions/")
	if sigStr == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	sig, err := types.SignatureFromBase58(sigStr)
	if err != nil {
		d.renderPage(w, "transaction", map[string]interface{}{
			"Error":     fmt.Sprintf("Invalid signature: %v", err),
			"Signature": sigStr,
		})
		return
	}

	tx, err := d.blocks.GetTransaction(sig)
	if err != nil {
		d.renderPage(w, "transaction", map[string]interface{}{
			"Error":     fmt.Sprintf("Transaction not found: %v", err),
			"Signature": sigStr,
		})
		return
	}

	d.renderPage(w, "transaction", map[string]interface{}{
		"Transaction": tx,
		"Signature":   sigStr,
	})
}

// handleSettings renders the settings/config page.
func (d *Dashboard) handleSettings(w http.ResponseWriter, r *http.Request) {
	stats, _ := d.blocks.GetStats()
	accountsCount, _ := d.accounts.AccountsCount()

	data := map[string]interface{}{
		"BlockstoreStats":  stats,
		"AccountsCount":    accountsCount,
		"GeyserConnected":  false,
		"GeyserEndpoint":   "",
		"DashboardAddress": d.Address(),
	}

	if d.nodeStats != nil {
		data["GeyserConnected"] = d.nodeStats.GeyserConnected()
		data["GeyserEndpoint"] = d.nodeStats.GeyserEndpoint()
	}

	d.renderPage(w, "settings", data)
}

// handleStatic serves embedded static assets.
func (d *Dashboard) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/static/")

	content, contentType, ok := getStaticAsset(name)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write([]byte(content))
}

// getStatusData returns the current node status data.
func (d *Dashboard) getStatusData() map[string]interface{} {
	data := make(map[string]interface{})

	var currentSlot, latestSlot, blocksProcessed, txsProcessed uint64
	var isSyncing, isRunning, geyserConnected bool
	var uptime time.Duration
	var avgSlotTimeMs float64
	var lastErr error
	var geyserEndpoint string

	if d.nodeStats != nil {
		currentSlot = d.nodeStats.CurrentSlot()
		latestSlot = d.nodeStats.LatestSlot()
		isSyncing = d.nodeStats.IsSyncing()
		isRunning = d.nodeStats.IsRunning()
		uptime = d.nodeStats.Uptime()
		blocksProcessed = d.nodeStats.BlocksProcessed()
		txsProcessed = d.nodeStats.TxsProcessed()
		avgSlotTimeMs = d.nodeStats.AvgSlotTimeMs()
		geyserConnected = d.nodeStats.GeyserConnected()
		geyserEndpoint = d.nodeStats.GeyserEndpoint()
		lastErr = d.nodeStats.LastError()
	} else {
		// Fallback to blockstore stats
		if stats, err := d.blocks.GetStats(); err == nil {
			currentSlot = stats.LatestSlot
			blocksProcessed = stats.BlockCount
			txsProcessed = stats.TransactionCount
		}
		uptime = time.Since(d.startTime)
	}

	slotsBehind := uint64(0)
	if latestSlot > currentSlot {
		slotsBehind = latestSlot - currentSlot
	}

	accountsCount, _ := d.accounts.AccountsCount()

	data["CurrentSlot"] = currentSlot
	data["LatestSlot"] = latestSlot
	data["SlotsBehind"] = slotsBehind
	data["IsSyncing"] = isSyncing
	data["IsRunning"] = isRunning
	data["Uptime"] = uptime
	data["BlocksProcessed"] = blocksProcessed
	data["TxsProcessed"] = txsProcessed
	data["AvgSlotTimeMs"] = avgSlotTimeMs
	data["AccountsCount"] = accountsCount
	data["GeyserConnected"] = geyserConnected
	data["GeyserEndpoint"] = geyserEndpoint

	if lastErr != nil {
		data["LastError"] = lastErr.Error()
	}

	// Calculate blocks per second
	if uptime.Seconds() > 0 {
		data["BlocksPerSec"] = float64(blocksProcessed) / uptime.Seconds()
	}

	// Get sync status string
	if !isRunning {
		data["SyncStatus"] = "Stopped"
	} else if isSyncing {
		data["SyncStatus"] = "Syncing"
	} else {
		data["SyncStatus"] = "Synced"
	}

	return data
}

// getRecentBlocks returns recent blocks for pagination.
func (d *Dashboard) getRecentBlocks(page, perPage int) ([]blockstore.Block, int) {
	stats, err := d.blocks.GetStats()
	if err != nil || stats.BlockCount == 0 {
		return nil, 0
	}

	latestSlot := stats.LatestSlot
	oldestSlot := stats.OldestSlot
	totalSlots := latestSlot - oldestSlot + 1
	totalPages := int((totalSlots + uint64(perPage) - 1) / uint64(perPage))

	if page > totalPages {
		page = totalPages
	}
	if page < 1 {
		page = 1
	}

	// Calculate slot range for this page
	startSlot := latestSlot - uint64((page-1)*perPage)
	endSlot := startSlot - uint64(perPage) + 1
	if endSlot < oldestSlot {
		endSlot = oldestSlot
	}

	var blocks []blockstore.Block
	for slot := startSlot; slot >= endSlot && slot <= startSlot; slot-- {
		block, err := d.blocks.GetBlock(slot)
		if err == nil {
			blocks = append(blocks, *block)
		}
	}

	return blocks, totalPages
}

// renderPage renders a page template with the given data.
func (d *Dashboard) renderPage(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// First render the content template into a buffer
	var contentBuf strings.Builder
	if err := d.templates.ExecuteTemplate(&contentBuf, name, data); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
		return
	}

	// Then render the layout with the content
	pageData := map[string]interface{}{
		"PageName": name,
		"Content":  template.HTML(contentBuf.String()),
	}

	if err := d.templates.ExecuteTemplate(w, "layout", pageData); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Template helper functions

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}

func formatNumber(n interface{}) string {
	switch v := n.(type) {
	case int:
		return formatInt(int64(v))
	case int64:
		return formatInt(v)
	case uint64:
		return formatInt(int64(v))
	case float64:
		return fmt.Sprintf("%.2f", v)
	default:
		return fmt.Sprintf("%v", n)
	}
}

func formatInt(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	if n < 1000000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	return fmt.Sprintf("%.1fB", float64(n)/1000000000)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatTime(t *int64) string {
	if t == nil {
		return "N/A"
	}
	return time.Unix(*t, 0).UTC().Format("2006-01-02 15:04:05 UTC")
}

func truncateHash(s string, n int) string {
	if len(s) <= n*2+3 {
		return s
	}
	return s[:n] + "..." + s[len(s)-n:]
}

func seq(start, end int) []int {
	if start > end {
		return nil
	}
	result := make([]int, end-start+1)
	for i := range result {
		result[i] = start + i
	}
	return result
}

// getMemStats returns current memory statistics.
func getMemStats() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}
