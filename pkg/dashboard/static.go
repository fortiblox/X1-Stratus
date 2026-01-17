package dashboard

// Static assets for the dashboard.
// These are embedded as strings for simplicity.
// In a larger application, you would use //go:embed with actual files.

// getStaticAsset returns a static asset by name.
// Returns the content, content type, and whether the asset was found.
func getStaticAsset(name string) (content string, contentType string, ok bool) {
	switch name {
	case "style.css":
		return cssStyles, "text/css", true
	case "app.js":
		return jsApp, "application/javascript", true
	case "favicon.ico":
		return "", "image/x-icon", false // No favicon embedded
	default:
		return "", "", false
	}
}

// cssStyles contains additional custom CSS styles.
// Most styling is done via Tailwind CSS CDN, but we include some custom styles here.
const cssStyles = `
/* X1-Stratus Dashboard Custom Styles */

/* Root variables */
:root {
    --color-primary: #3b82f6;
    --color-primary-hover: #2563eb;
    --color-success: #10b981;
    --color-warning: #f59e0b;
    --color-error: #ef4444;
    --color-bg-dark: #111827;
    --color-bg-card: #1f2937;
    --color-border: #374151;
    --color-text: #f9fafb;
    --color-text-muted: #9ca3af;
}

/* Base styles */
* {
    box-sizing: border-box;
}

body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
    background-color: var(--color-bg-dark);
    color: var(--color-text);
    line-height: 1.6;
}

/* Monospace font for hashes and addresses */
.mono {
    font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
}

/* Custom scrollbar */
::-webkit-scrollbar {
    width: 8px;
    height: 8px;
}

::-webkit-scrollbar-track {
    background: var(--color-bg-dark);
}

::-webkit-scrollbar-thumb {
    background: var(--color-border);
    border-radius: 4px;
}

::-webkit-scrollbar-thumb:hover {
    background: #4b5563;
}

/* Card hover effects */
.card-hover {
    transition: transform 0.15s ease, box-shadow 0.15s ease;
}

.card-hover:hover {
    transform: translateY(-2px);
    box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.3);
}

/* Loading spinner */
.spinner {
    display: inline-block;
    width: 20px;
    height: 20px;
    border: 2px solid rgba(255, 255, 255, 0.3);
    border-radius: 50%;
    border-top-color: var(--color-primary);
    animation: spin 1s ease-in-out infinite;
}

@keyframes spin {
    to {
        transform: rotate(360deg);
    }
}

/* Pulse animation for syncing indicator */
.sync-pulse {
    animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

@keyframes pulse {
    0%, 100% {
        opacity: 1;
    }
    50% {
        opacity: 0.5;
    }
}

/* Status indicators */
.status-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    display: inline-block;
}

.status-dot.connected {
    background-color: var(--color-success);
    box-shadow: 0 0 10px var(--color-success);
}

.status-dot.disconnected {
    background-color: var(--color-error);
    box-shadow: 0 0 10px var(--color-error);
}

.status-dot.syncing {
    background-color: var(--color-warning);
    box-shadow: 0 0 10px var(--color-warning);
    animation: pulse 2s ease-in-out infinite;
}

/* Table styles */
.table-hover tbody tr:hover {
    background-color: rgba(55, 65, 81, 0.5);
}

/* Truncated hash display */
.truncate-hash {
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: inline-block;
}

/* Data preview box */
.data-preview {
    max-height: 200px;
    overflow-y: auto;
    word-break: break-all;
}

/* Toast notifications */
.toast {
    position: fixed;
    bottom: 20px;
    right: 20px;
    padding: 12px 20px;
    border-radius: 8px;
    background-color: var(--color-bg-card);
    border: 1px solid var(--color-border);
    box-shadow: 0 10px 25px rgba(0, 0, 0, 0.3);
    z-index: 1000;
    animation: slideIn 0.3s ease;
}

.toast.success {
    border-left: 4px solid var(--color-success);
}

.toast.error {
    border-left: 4px solid var(--color-error);
}

.toast.warning {
    border-left: 4px solid var(--color-warning);
}

@keyframes slideIn {
    from {
        transform: translateX(100%);
        opacity: 0;
    }
    to {
        transform: translateX(0);
        opacity: 1;
    }
}

/* Responsive adjustments */
@media (max-width: 768px) {
    .truncate-hash {
        max-width: 100px;
    }

    .nav-links {
        display: none;
    }

    .mobile-menu {
        display: block;
    }
}

/* Progress bar for sync */
.progress-bar {
    height: 4px;
    background-color: var(--color-border);
    border-radius: 2px;
    overflow: hidden;
}

.progress-bar .progress {
    height: 100%;
    background-color: var(--color-primary);
    transition: width 0.3s ease;
}

/* Copy button styles */
.copy-btn {
    opacity: 0;
    transition: opacity 0.15s ease;
}

.copy-container:hover .copy-btn {
    opacity: 1;
}

/* Tooltip styles */
.tooltip {
    position: relative;
}

.tooltip::after {
    content: attr(data-tooltip);
    position: absolute;
    bottom: 100%;
    left: 50%;
    transform: translateX(-50%);
    padding: 4px 8px;
    background-color: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: 4px;
    font-size: 12px;
    white-space: nowrap;
    opacity: 0;
    visibility: hidden;
    transition: opacity 0.15s ease, visibility 0.15s ease;
    z-index: 100;
}

.tooltip:hover::after {
    opacity: 1;
    visibility: visible;
}

/* Badge styles */
.badge {
    display: inline-flex;
    align-items: center;
    padding: 2px 8px;
    font-size: 12px;
    font-weight: 500;
    border-radius: 9999px;
}

.badge-success {
    background-color: rgba(16, 185, 129, 0.2);
    color: #10b981;
}

.badge-error {
    background-color: rgba(239, 68, 68, 0.2);
    color: #ef4444;
}

.badge-warning {
    background-color: rgba(245, 158, 11, 0.2);
    color: #f59e0b;
}

.badge-info {
    background-color: rgba(59, 130, 246, 0.2);
    color: #3b82f6;
}

/* Keyboard shortcut hints */
.kbd {
    display: inline-block;
    padding: 2px 6px;
    font-family: var(--font-mono);
    font-size: 11px;
    line-height: 1.4;
    color: var(--color-text-muted);
    background-color: var(--color-bg-dark);
    border: 1px solid var(--color-border);
    border-radius: 4px;
}
`

// jsApp contains the main JavaScript for the dashboard.
const jsApp = `
// X1-Stratus Dashboard JavaScript

(function() {
    'use strict';

    // Configuration
    const CONFIG = {
        refreshInterval: 5000,  // 5 seconds
        toastDuration: 3000,    // 3 seconds
    };

    // State
    const state = {
        connected: true,
        syncing: false,
        currentSlot: 0,
        latestSlot: 0,
    };

    // DOM Ready
    document.addEventListener('DOMContentLoaded', function() {
        initializeDashboard();
    });

    // Initialize dashboard
    function initializeDashboard() {
        // Update time display
        updateTime();
        setInterval(updateTime, 1000);

        // Start auto-refresh on home page
        if (window.location.pathname === '/') {
            startAutoRefresh();
        }

        // Add copy functionality to addresses/hashes
        initializeCopyButtons();

        // Initialize keyboard shortcuts
        initializeKeyboardShortcuts();
    }

    // Update current time display
    function updateTime() {
        const timeEl = document.getElementById('current-time');
        if (timeEl) {
            timeEl.textContent = new Date().toUTCString();
        }
    }

    // Start auto-refresh for status
    function startAutoRefresh() {
        refreshStatus();
        setInterval(refreshStatus, CONFIG.refreshInterval);
    }

    // Refresh status from API
    async function refreshStatus() {
        try {
            const response = await fetch('/api/status');
            if (!response.ok) throw new Error('Failed to fetch status');

            const data = await response.json();
            updateStatusDisplay(data);
            state.connected = true;
        } catch (error) {
            console.error('Failed to refresh status:', error);
            state.connected = false;
            updateConnectionStatus(false);
        }
    }

    // Update status display
    function updateStatusDisplay(data) {
        // Update slot
        updateElement('current-slot', formatNumber(data.currentSlot));

        // Update slots behind
        updateElement('slots-behind', formatNumber(data.slotsBehind));

        // Update blocks processed
        updateElement('blocks-processed', formatNumber(data.blocksProcessed));

        // Update transactions processed
        updateElement('txs-processed', formatNumber(data.txsProcessed));

        // Update sync status
        const statusEl = document.getElementById('sync-status');
        if (statusEl) {
            statusEl.textContent = data.syncStatus || 'Unknown';
            if (data.isSyncing) {
                statusEl.className = 'text-3xl font-bold mt-1 text-yellow-500 animate-pulse';
            } else {
                statusEl.className = 'text-3xl font-bold mt-1 text-green-500';
            }
        }

        // Update uptime
        updateElement('uptime', data.uptime);

        // Update connection status
        updateConnectionStatus(data.geyserConnected);
    }

    // Update single element
    function updateElement(id, value) {
        const el = document.getElementById(id);
        if (el) {
            el.textContent = value;
        }
    }

    // Update connection status indicator
    function updateConnectionStatus(connected) {
        const connEl = document.getElementById('connection-status');
        if (!connEl) return;

        const dot = connEl.querySelector('span:first-child');
        const text = connEl.querySelector('span:last-child');

        if (connected) {
            dot.className = 'w-2 h-2 rounded-full bg-green-500';
            text.textContent = 'Connected';
        } else {
            dot.className = 'w-2 h-2 rounded-full bg-red-500';
            text.textContent = 'Disconnected';
        }
    }

    // Format large numbers
    function formatNumber(num) {
        if (num === undefined || num === null) return '0';
        return num.toLocaleString();
    }

    // Initialize copy buttons
    function initializeCopyButtons() {
        document.querySelectorAll('[data-copy]').forEach(function(el) {
            el.addEventListener('click', function() {
                const text = el.getAttribute('data-copy');
                copyToClipboard(text);
            });
        });
    }

    // Copy text to clipboard
    function copyToClipboard(text) {
        navigator.clipboard.writeText(text).then(function() {
            showToast('Copied to clipboard', 'success');
        }).catch(function(err) {
            console.error('Failed to copy:', err);
            showToast('Failed to copy', 'error');
        });
    }

    // Show toast notification
    function showToast(message, type) {
        const toast = document.createElement('div');
        toast.className = 'toast ' + (type || 'info');
        toast.textContent = message;
        document.body.appendChild(toast);

        setTimeout(function() {
            toast.style.opacity = '0';
            setTimeout(function() {
                document.body.removeChild(toast);
            }, 300);
        }, CONFIG.toastDuration);
    }

    // Initialize keyboard shortcuts
    function initializeKeyboardShortcuts() {
        document.addEventListener('keydown', function(e) {
            // Only trigger if not in an input
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') {
                return;
            }

            // g + h = go home
            // g + b = go blocks
            // g + a = go accounts
            // g + s = go settings
            // / = focus search

            if (e.key === '/') {
                e.preventDefault();
                const searchInput = document.querySelector('input[name="q"]');
                if (searchInput) {
                    searchInput.focus();
                }
            }
        });
    }

    // Expose to global scope for inline handlers
    window.Dashboard = {
        copyToClipboard: copyToClipboard,
        showToast: showToast,
        refreshStatus: refreshStatus,
    };
})();
`
