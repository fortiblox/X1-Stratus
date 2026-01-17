package dashboard

// HTML templates for the dashboard pages.
// These are embedded as strings and parsed at runtime.

const layoutTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>X1-Stratus Dashboard</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <style>
        /* Custom scrollbar */
        ::-webkit-scrollbar { width: 8px; height: 8px; }
        ::-webkit-scrollbar-track { background: #1f2937; }
        ::-webkit-scrollbar-thumb { background: #4b5563; border-radius: 4px; }
        ::-webkit-scrollbar-thumb:hover { background: #6b7280; }

        /* Custom styles */
        .mono { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
        .truncate-hash { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

        /* Animation for syncing indicator */
        @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
        .animate-pulse { animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite; }

        /* Data preview styling */
        .data-preview { max-height: 200px; overflow-y: auto; }
    </style>
</head>
<body class="bg-gray-900 text-gray-100 min-h-screen">
    <!-- Navigation -->
    <nav class="bg-gray-800 border-b border-gray-700 sticky top-0 z-50">
        <div class="container mx-auto px-4">
            <div class="flex items-center justify-between h-16">
                <div class="flex items-center space-x-8">
                    <a href="/" class="flex items-center space-x-2">
                        <svg class="w-8 h-8 text-blue-500" fill="currentColor" viewBox="0 0 24 24">
                            <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
                        </svg>
                        <span class="text-xl font-bold text-white">X1-Stratus</span>
                    </a>
                    <div class="hidden md:flex items-center space-x-4">
                        <a href="/" class="px-3 py-2 rounded-md text-sm font-medium {{if eq .PageName "home"}}bg-gray-900 text-white{{else}}text-gray-300 hover:bg-gray-700 hover:text-white{{end}}">Overview</a>
                        <a href="/blocks" class="px-3 py-2 rounded-md text-sm font-medium {{if eq .PageName "blocks"}}bg-gray-900 text-white{{else}}text-gray-300 hover:bg-gray-700 hover:text-white{{end}}">Blocks</a>
                        <a href="/accounts" class="px-3 py-2 rounded-md text-sm font-medium {{if eq .PageName "accounts"}}bg-gray-900 text-white{{else}}text-gray-300 hover:bg-gray-700 hover:text-white{{end}}">Accounts</a>
                        <a href="/settings" class="px-3 py-2 rounded-md text-sm font-medium {{if eq .PageName "settings"}}bg-gray-900 text-white{{else}}text-gray-300 hover:bg-gray-700 hover:text-white{{end}}">Settings</a>
                    </div>
                </div>
                <div class="flex items-center space-x-4">
                    <div id="connection-status" class="flex items-center space-x-2">
                        <span class="w-2 h-2 rounded-full bg-green-500"></span>
                        <span class="text-sm text-gray-300">Connected</span>
                    </div>
                </div>
            </div>
        </div>
    </nav>

    <!-- Main Content -->
    <main class="container mx-auto px-4 py-6">
        {{.Content}}
    </main>

    <!-- Footer -->
    <footer class="bg-gray-800 border-t border-gray-700 mt-8 py-4">
        <div class="container mx-auto px-4 text-center text-gray-400 text-sm">
            X1-Stratus Verification Node | <span id="current-time"></span>
        </div>
    </footer>

    <!-- Auto-refresh script -->
    <script>
        // Update current time
        function updateTime() {
            const now = new Date();
            document.getElementById('current-time').textContent = now.toUTCString();
        }
        updateTime();
        setInterval(updateTime, 1000);

        // Auto-refresh status for home page
        if (window.location.pathname === '/') {
            setInterval(async () => {
                try {
                    const resp = await fetch('/api/status');
                    const data = await resp.json();

                    // Update slot
                    const slotEl = document.getElementById('current-slot');
                    if (slotEl) slotEl.textContent = data.currentSlot?.toLocaleString() || '0';

                    // Update slots behind
                    const behindEl = document.getElementById('slots-behind');
                    if (behindEl) behindEl.textContent = data.slotsBehind?.toLocaleString() || '0';

                    // Update blocks processed
                    const blocksEl = document.getElementById('blocks-processed');
                    if (blocksEl) blocksEl.textContent = data.blocksProcessed?.toLocaleString() || '0';

                    // Update txs processed
                    const txsEl = document.getElementById('txs-processed');
                    if (txsEl) txsEl.textContent = data.txsProcessed?.toLocaleString() || '0';

                    // Update sync status
                    const statusEl = document.getElementById('sync-status');
                    if (statusEl) {
                        statusEl.textContent = data.syncStatus || 'Unknown';
                        statusEl.className = data.isSyncing ?
                            'text-yellow-500 animate-pulse' :
                            'text-green-500';
                    }

                    // Update uptime
                    const uptimeEl = document.getElementById('uptime');
                    if (uptimeEl) uptimeEl.textContent = data.uptime || '0s';

                    // Update connection indicator
                    const connEl = document.getElementById('connection-status');
                    if (connEl) {
                        const dot = connEl.querySelector('span:first-child');
                        const text = connEl.querySelector('span:last-child');
                        if (data.geyserConnected) {
                            dot.className = 'w-2 h-2 rounded-full bg-green-500';
                            text.textContent = 'Connected';
                        } else {
                            dot.className = 'w-2 h-2 rounded-full bg-red-500';
                            text.textContent = 'Disconnected';
                        }
                    }
                } catch (e) {
                    console.error('Failed to fetch status:', e);
                }
            }, 5000);
        }
    </script>
</body>
</html>`

const homeTemplate = `
<div class="space-y-6">
    <!-- Status Cards -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <!-- Current Slot -->
        <div class="bg-gray-800 rounded-lg p-6 border border-gray-700">
            <div class="flex items-center justify-between">
                <div>
                    <p class="text-gray-400 text-sm font-medium">Current Slot</p>
                    <p class="text-3xl font-bold text-white mt-1" id="current-slot">{{formatNumber .CurrentSlot}}</p>
                </div>
                <div class="p-3 bg-blue-500/10 rounded-full">
                    <svg class="w-6 h-6 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/>
                    </svg>
                </div>
            </div>
        </div>

        <!-- Sync Status -->
        <div class="bg-gray-800 rounded-lg p-6 border border-gray-700">
            <div class="flex items-center justify-between">
                <div>
                    <p class="text-gray-400 text-sm font-medium">Sync Status</p>
                    <p class="text-3xl font-bold mt-1 {{if .IsSyncing}}text-yellow-500 animate-pulse{{else}}text-green-500{{end}}" id="sync-status">{{.SyncStatus}}</p>
                    <p class="text-sm text-gray-500 mt-1"><span id="slots-behind">{{formatNumber .SlotsBehind}}</span> slots behind</p>
                </div>
                <div class="p-3 {{if .IsSyncing}}bg-yellow-500/10{{else}}bg-green-500/10{{end}} rounded-full">
                    <svg class="w-6 h-6 {{if .IsSyncing}}text-yellow-500{{else}}text-green-500{{end}}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
                    </svg>
                </div>
            </div>
        </div>

        <!-- Blocks Processed -->
        <div class="bg-gray-800 rounded-lg p-6 border border-gray-700">
            <div class="flex items-center justify-between">
                <div>
                    <p class="text-gray-400 text-sm font-medium">Blocks Processed</p>
                    <p class="text-3xl font-bold text-white mt-1" id="blocks-processed">{{formatNumber .BlocksProcessed}}</p>
                    {{if .BlocksPerSec}}<p class="text-sm text-gray-500 mt-1">{{printf "%.1f" .BlocksPerSec}} blocks/sec</p>{{end}}
                </div>
                <div class="p-3 bg-purple-500/10 rounded-full">
                    <svg class="w-6 h-6 text-purple-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"/>
                    </svg>
                </div>
            </div>
        </div>

        <!-- Uptime -->
        <div class="bg-gray-800 rounded-lg p-6 border border-gray-700">
            <div class="flex items-center justify-between">
                <div>
                    <p class="text-gray-400 text-sm font-medium">Uptime</p>
                    <p class="text-3xl font-bold text-white mt-1" id="uptime">{{formatDuration .Uptime}}</p>
                </div>
                <div class="p-3 bg-green-500/10 rounded-full">
                    <svg class="w-6 h-6 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/>
                    </svg>
                </div>
            </div>
        </div>
    </div>

    <!-- Second Row -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <!-- Transactions -->
        <div class="bg-gray-800 rounded-lg p-6 border border-gray-700">
            <p class="text-gray-400 text-sm font-medium">Transactions Processed</p>
            <p class="text-2xl font-bold text-white mt-1" id="txs-processed">{{formatNumber .TxsProcessed}}</p>
        </div>

        <!-- Accounts -->
        <div class="bg-gray-800 rounded-lg p-6 border border-gray-700">
            <p class="text-gray-400 text-sm font-medium">Total Accounts</p>
            <p class="text-2xl font-bold text-white mt-1">{{formatNumber .AccountsCount}}</p>
        </div>

        <!-- Avg Slot Time -->
        <div class="bg-gray-800 rounded-lg p-6 border border-gray-700">
            <p class="text-gray-400 text-sm font-medium">Avg Slot Time</p>
            <p class="text-2xl font-bold text-white mt-1">{{printf "%.2f" .AvgSlotTimeMs}} ms</p>
        </div>
    </div>

    {{if .LastError}}
    <!-- Error Alert -->
    <div class="bg-red-900/50 border border-red-500 rounded-lg p-4">
        <div class="flex items-center">
            <svg class="w-5 h-5 text-red-500 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
            </svg>
            <span class="text-red-200 text-sm">{{.LastError}}</span>
        </div>
    </div>
    {{end}}

    <!-- Geyser Connection -->
    <div class="bg-gray-800 rounded-lg p-6 border border-gray-700">
        <h2 class="text-lg font-semibold text-white mb-4">Geyser Connection</h2>
        <div class="flex items-center space-x-4">
            <div class="flex items-center space-x-2">
                <span class="w-3 h-3 rounded-full {{if .GeyserConnected}}bg-green-500{{else}}bg-red-500{{end}}"></span>
                <span class="text-gray-300">{{if .GeyserConnected}}Connected{{else}}Disconnected{{end}}</span>
            </div>
            {{if .GeyserEndpoint}}
            <span class="text-gray-500">|</span>
            <span class="text-gray-400 mono text-sm">{{.GeyserEndpoint}}</span>
            {{end}}
        </div>
    </div>
</div>
`

const blocksTemplate = `
<div class="space-y-6">
    <div class="flex items-center justify-between">
        <h1 class="text-2xl font-bold text-white">Recent Blocks</h1>
    </div>

    <!-- Blocks Table -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 overflow-hidden">
        <table class="w-full">
            <thead class="bg-gray-700/50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Slot</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Blockhash</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Transactions</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Time</th>
                </tr>
            </thead>
            <tbody class="divide-y divide-gray-700">
                {{range .Blocks}}
                <tr class="hover:bg-gray-700/50 cursor-pointer" onclick="window.location='/blocks/{{.Slot}}'">
                    <td class="px-6 py-4 whitespace-nowrap">
                        <a href="/blocks/{{.Slot}}" class="text-blue-400 hover:text-blue-300 font-medium">{{.Slot}}</a>
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap">
                        <span class="mono text-sm text-gray-300 truncate-hash">{{truncateHash .Blockhash.String 8}}</span>
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-gray-300">
                        {{len .Transactions}}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-gray-400 text-sm">
                        {{formatTime .BlockTime}}
                    </td>
                </tr>
                {{else}}
                <tr>
                    <td colspan="4" class="px-6 py-8 text-center text-gray-500">No blocks found</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>

    <!-- Pagination -->
    {{if gt .TotalPages 1}}
    <div class="flex items-center justify-between">
        <div class="text-sm text-gray-400">
            Page {{.CurrentPage}} of {{.TotalPages}}
        </div>
        <div class="flex items-center space-x-2">
            {{if .HasPrev}}
            <a href="/blocks?page={{sub .CurrentPage 1}}" class="px-4 py-2 bg-gray-700 text-white rounded hover:bg-gray-600">Previous</a>
            {{end}}
            {{if .HasNext}}
            <a href="/blocks?page={{add .CurrentPage 1}}" class="px-4 py-2 bg-gray-700 text-white rounded hover:bg-gray-600">Next</a>
            {{end}}
        </div>
    </div>
    {{end}}
</div>
`

const blockDetailTemplate = `
<div class="space-y-6">
    {{if .Error}}
    <div class="bg-red-900/50 border border-red-500 rounded-lg p-4">
        <p class="text-red-200">{{.Error}}</p>
    </div>
    <a href="/blocks" class="inline-block text-blue-400 hover:text-blue-300">&larr; Back to blocks</a>
    {{else}}
    <div class="flex items-center justify-between">
        <div class="flex items-center space-x-4">
            <a href="/blocks" class="text-gray-400 hover:text-white">&larr;</a>
            <h1 class="text-2xl font-bold text-white">Block {{.Block.Slot}}</h1>
        </div>
    </div>

    <!-- Block Info -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 p-6 space-y-4">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
                <p class="text-gray-400 text-sm">Slot</p>
                <p class="text-white font-medium">{{.Block.Slot}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Parent Slot</p>
                <p class="text-white font-medium">
                    <a href="/blocks/{{.Block.ParentSlot}}" class="text-blue-400 hover:text-blue-300">{{.Block.ParentSlot}}</a>
                </p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Blockhash</p>
                <p class="text-white mono text-sm break-all">{{.Block.Blockhash.String}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Previous Blockhash</p>
                <p class="text-white mono text-sm break-all">{{.Block.PreviousBlockhash.String}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Block Time</p>
                <p class="text-white">{{formatTime .Block.BlockTime}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Transactions</p>
                <p class="text-white font-medium">{{len .Block.Transactions}}</p>
            </div>
        </div>
    </div>

    <!-- Transactions -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 overflow-hidden">
        <div class="px-6 py-4 border-b border-gray-700">
            <h2 class="text-lg font-semibold text-white">Transactions ({{len .Block.Transactions}})</h2>
        </div>
        <table class="w-full">
            <thead class="bg-gray-700/50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">#</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Signature</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Status</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Instructions</th>
                </tr>
            </thead>
            <tbody class="divide-y divide-gray-700">
                {{range $i, $tx := .Block.Transactions}}
                <tr class="hover:bg-gray-700/50">
                    <td class="px-6 py-4 whitespace-nowrap text-gray-400">{{add $i 1}}</td>
                    <td class="px-6 py-4 whitespace-nowrap">
                        <a href="/transactions/{{$tx.Signature.String}}" class="text-blue-400 hover:text-blue-300 mono text-sm">
                            {{truncateHash $tx.Signature.String 12}}
                        </a>
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap">
                        {{if and $tx.Meta $tx.Meta.Err}}
                        <span class="px-2 py-1 text-xs font-medium rounded bg-red-500/20 text-red-400">Failed</span>
                        {{else}}
                        <span class="px-2 py-1 text-xs font-medium rounded bg-green-500/20 text-green-400">Success</span>
                        {{end}}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-gray-300">
                        {{len $tx.Message.Instructions}}
                    </td>
                </tr>
                {{else}}
                <tr>
                    <td colspan="4" class="px-6 py-8 text-center text-gray-500">No transactions</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>

    {{if .Block.Rewards}}
    <!-- Rewards -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 overflow-hidden">
        <div class="px-6 py-4 border-b border-gray-700">
            <h2 class="text-lg font-semibold text-white">Rewards ({{len .Block.Rewards}})</h2>
        </div>
        <table class="w-full">
            <thead class="bg-gray-700/50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Account</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Type</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Amount</th>
                </tr>
            </thead>
            <tbody class="divide-y divide-gray-700">
                {{range .Block.Rewards}}
                <tr class="hover:bg-gray-700/50">
                    <td class="px-6 py-4 whitespace-nowrap">
                        <a href="/accounts/{{.Pubkey.String}}" class="text-blue-400 hover:text-blue-300 mono text-sm">
                            {{truncateHash .Pubkey.String 12}}
                        </a>
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-gray-300">{{.RewardType}}</td>
                    <td class="px-6 py-4 whitespace-nowrap text-gray-300">{{.Lamports}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{end}}
    {{end}}
</div>
`

const accountsTemplate = `
<div class="space-y-6">
    <h1 class="text-2xl font-bold text-white">Account Lookup</h1>

    <!-- Search Form -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 p-6">
        <form action="/accounts" method="get" class="flex space-x-4">
            <input type="text" name="q" value="{{.Query}}" placeholder="Enter public key (base58)..."
                   class="flex-1 px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white placeholder-gray-400 focus:outline-none focus:border-blue-500 mono">
            <button type="submit" class="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-500 font-medium">
                Search
            </button>
        </form>
    </div>

    {{if .SearchErr}}
    <div class="bg-red-900/50 border border-red-500 rounded-lg p-4">
        <p class="text-red-200">{{.SearchErr}}</p>
    </div>
    {{end}}

    {{if .Account}}
    <!-- Account Details -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 p-6 space-y-4">
        <h2 class="text-lg font-semibold text-white mb-4">Account Details</h2>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
                <p class="text-gray-400 text-sm">Public Key</p>
                <p class="text-white mono text-sm break-all">{{.Pubkey}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Balance</p>
                <p class="text-white font-medium">{{printf "%.9f" (divf .Account.Lamports 1000000000.0)}} SOL</p>
                <p class="text-gray-500 text-sm">{{.Account.Lamports}} lamports</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Owner</p>
                <a href="/accounts/{{.Account.Owner.String}}" class="text-blue-400 hover:text-blue-300 mono text-sm break-all">
                    {{.Account.Owner.String}}
                </a>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Executable</p>
                <p class="text-white">{{if .Account.Executable}}Yes{{else}}No{{end}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Rent Epoch</p>
                <p class="text-white">{{.Account.RentEpoch}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Data Size</p>
                <p class="text-white">{{len .Account.Data}} bytes</p>
            </div>
        </div>

        {{if .Account.Data}}
        <div class="mt-4">
            <p class="text-gray-400 text-sm mb-2">Data Preview (hex)</p>
            <div class="bg-gray-900 rounded p-4 mono text-xs text-gray-300 data-preview overflow-x-auto">
                {{range $i, $b := .Account.Data}}{{if lt $i 256}}{{printf "%02x " $b}}{{end}}{{end}}{{if gt (len .Account.Data) 256}}...{{end}}
            </div>
        </div>
        {{end}}
    </div>
    {{end}}
</div>
`

const accountDetailTemplate = `
<div class="space-y-6">
    {{if .Error}}
    <div class="bg-red-900/50 border border-red-500 rounded-lg p-4">
        <p class="text-red-200">{{.Error}}</p>
    </div>
    <a href="/accounts" class="inline-block text-blue-400 hover:text-blue-300">&larr; Back to accounts</a>
    {{else}}
    <div class="flex items-center justify-between">
        <div class="flex items-center space-x-4">
            <a href="/accounts" class="text-gray-400 hover:text-white">&larr;</a>
            <h1 class="text-2xl font-bold text-white">Account Details</h1>
        </div>
    </div>

    <div class="bg-gray-800 rounded-lg border border-gray-700 p-6 space-y-4">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
                <p class="text-gray-400 text-sm">Public Key</p>
                <p class="text-white mono text-sm break-all">{{.Pubkey}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Balance</p>
                <p class="text-white font-medium">{{printf "%.9f" (divf .Account.Lamports 1000000000.0)}} SOL</p>
                <p class="text-gray-500 text-sm">{{.Account.Lamports}} lamports</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Owner</p>
                <a href="/accounts/{{.Account.Owner.String}}" class="text-blue-400 hover:text-blue-300 mono text-sm break-all">
                    {{.Account.Owner.String}}
                </a>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Executable</p>
                <p class="text-white">{{if .Account.Executable}}Yes{{else}}No{{end}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Rent Epoch</p>
                <p class="text-white">{{.Account.RentEpoch}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Data Size</p>
                <p class="text-white">{{len .Account.Data}} bytes</p>
            </div>
        </div>

        {{if .Account.Data}}
        <div class="mt-4">
            <p class="text-gray-400 text-sm mb-2">Data Preview (hex)</p>
            <div class="bg-gray-900 rounded p-4 mono text-xs text-gray-300 data-preview overflow-x-auto">
                {{range $i, $b := .Account.Data}}{{if lt $i 256}}{{printf "%02x " $b}}{{end}}{{end}}{{if gt (len .Account.Data) 256}}...{{end}}
            </div>
        </div>
        {{end}}
    </div>
    {{end}}
</div>
`

const transactionTemplate = `
<div class="space-y-6">
    {{if .Error}}
    <div class="bg-red-900/50 border border-red-500 rounded-lg p-4">
        <p class="text-red-200">{{.Error}}</p>
    </div>
    <a href="/" class="inline-block text-blue-400 hover:text-blue-300">&larr; Back to home</a>
    {{else}}
    <div class="flex items-center space-x-4">
        <a href="javascript:history.back()" class="text-gray-400 hover:text-white">&larr;</a>
        <h1 class="text-2xl font-bold text-white">Transaction Details</h1>
    </div>

    <!-- Basic Info -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 p-6 space-y-4">
        <div class="grid grid-cols-1 gap-4">
            <div>
                <p class="text-gray-400 text-sm">Signature</p>
                <p class="text-white mono text-sm break-all">{{.Signature}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Slot</p>
                <p class="text-white">
                    <a href="/blocks/{{.Transaction.Slot}}" class="text-blue-400 hover:text-blue-300">{{.Transaction.Slot}}</a>
                </p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Status</p>
                {{if and .Transaction.Meta .Transaction.Meta.Err}}
                <span class="px-2 py-1 text-sm font-medium rounded bg-red-500/20 text-red-400">Failed</span>
                <p class="text-red-400 text-sm mt-1">{{.Transaction.Meta.Err.Message}}</p>
                {{else}}
                <span class="px-2 py-1 text-sm font-medium rounded bg-green-500/20 text-green-400">Success</span>
                {{end}}
            </div>
            {{if .Transaction.Meta}}
            <div>
                <p class="text-gray-400 text-sm">Fee</p>
                <p class="text-white">{{.Transaction.Meta.Fee}} lamports</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Compute Units</p>
                <p class="text-white">{{.Transaction.Meta.ComputeUnitsConsumed}}</p>
            </div>
            {{end}}
        </div>
    </div>

    <!-- Account Keys -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 overflow-hidden">
        <div class="px-6 py-4 border-b border-gray-700">
            <h2 class="text-lg font-semibold text-white">Account Keys ({{len .Transaction.Message.AccountKeys}})</h2>
        </div>
        <table class="w-full">
            <thead class="bg-gray-700/50">
                <tr>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">#</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Address</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Pre Balance</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Post Balance</th>
                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">Change</th>
                </tr>
            </thead>
            <tbody class="divide-y divide-gray-700">
                {{$meta := .Transaction.Meta}}
                {{range $i, $key := .Transaction.Message.AccountKeys}}
                <tr class="hover:bg-gray-700/50">
                    <td class="px-6 py-4 whitespace-nowrap text-gray-400">{{$i}}</td>
                    <td class="px-6 py-4 whitespace-nowrap">
                        <a href="/accounts/{{$key.String}}" class="text-blue-400 hover:text-blue-300 mono text-sm">
                            {{truncateHash $key.String 12}}
                        </a>
                    </td>
                    {{if $meta}}
                    <td class="px-6 py-4 whitespace-nowrap text-gray-300 mono text-sm">
                        {{if lt $i (len $meta.PreBalances)}}{{index $meta.PreBalances $i}}{{else}}-{{end}}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap text-gray-300 mono text-sm">
                        {{if lt $i (len $meta.PostBalances)}}{{index $meta.PostBalances $i}}{{else}}-{{end}}
                    </td>
                    <td class="px-6 py-4 whitespace-nowrap mono text-sm">
                        {{if and (lt $i (len $meta.PreBalances)) (lt $i (len $meta.PostBalances))}}
                            {{$pre := index $meta.PreBalances $i}}
                            {{$post := index $meta.PostBalances $i}}
                            {{if gt $post $pre}}
                            <span class="text-green-400">+{{sub (int $post) (int $pre)}}</span>
                            {{else if lt $post $pre}}
                            <span class="text-red-400">-{{sub (int $pre) (int $post)}}</span>
                            {{else}}
                            <span class="text-gray-500">0</span>
                            {{end}}
                        {{else}}-{{end}}
                    </td>
                    {{else}}
                    <td class="px-6 py-4 whitespace-nowrap text-gray-500">-</td>
                    <td class="px-6 py-4 whitespace-nowrap text-gray-500">-</td>
                    <td class="px-6 py-4 whitespace-nowrap text-gray-500">-</td>
                    {{end}}
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>

    <!-- Instructions -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 overflow-hidden">
        <div class="px-6 py-4 border-b border-gray-700">
            <h2 class="text-lg font-semibold text-white">Instructions ({{len .Transaction.Message.Instructions}})</h2>
        </div>
        <div class="divide-y divide-gray-700">
            {{range $i, $ix := .Transaction.Message.Instructions}}
            <div class="p-6">
                <div class="flex items-center justify-between mb-2">
                    <span class="text-white font-medium">Instruction #{{add $i 1}}</span>
                    <span class="px-2 py-1 text-xs bg-gray-700 rounded text-gray-300">Program: {{$ix.ProgramIDIndex}}</span>
                </div>
                <div class="mt-2">
                    <p class="text-gray-400 text-sm">Accounts: {{range $j, $acc := $ix.AccountIndexes}}{{if $j}}, {{end}}{{$acc}}{{end}}</p>
                </div>
                {{if $ix.Data}}
                <div class="mt-2">
                    <p class="text-gray-400 text-sm mb-1">Data ({{len $ix.Data}} bytes):</p>
                    <div class="bg-gray-900 rounded p-2 mono text-xs text-gray-300 overflow-x-auto">
                        {{range $j, $b := $ix.Data}}{{if lt $j 64}}{{printf "%02x " $b}}{{end}}{{end}}{{if gt (len $ix.Data) 64}}...{{end}}
                    </div>
                </div>
                {{end}}
            </div>
            {{end}}
        </div>
    </div>

    {{if and .Transaction.Meta .Transaction.Meta.LogMessages}}
    <!-- Log Messages -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 overflow-hidden">
        <div class="px-6 py-4 border-b border-gray-700">
            <h2 class="text-lg font-semibold text-white">Log Messages</h2>
        </div>
        <div class="p-4 bg-gray-900 mono text-sm overflow-x-auto max-h-96 overflow-y-auto">
            {{range .Transaction.Meta.LogMessages}}
            <div class="py-1 text-gray-300">{{.}}</div>
            {{end}}
        </div>
    </div>
    {{end}}
    {{end}}
</div>
`

const settingsTemplate = `
<div class="space-y-6">
    <h1 class="text-2xl font-bold text-white">Settings & Configuration</h1>

    <!-- Geyser Connection -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 p-6">
        <h2 class="text-lg font-semibold text-white mb-4">Geyser Connection</h2>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
                <p class="text-gray-400 text-sm">Status</p>
                <div class="flex items-center space-x-2 mt-1">
                    <span class="w-3 h-3 rounded-full {{if .GeyserConnected}}bg-green-500{{else}}bg-red-500{{end}}"></span>
                    <span class="text-white">{{if .GeyserConnected}}Connected{{else}}Disconnected{{end}}</span>
                </div>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Endpoint</p>
                <p class="text-white mono text-sm">{{if .GeyserEndpoint}}{{.GeyserEndpoint}}{{else}}Not configured{{end}}</p>
            </div>
        </div>
    </div>

    <!-- Database Stats -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 p-6">
        <h2 class="text-lg font-semibold text-white mb-4">Database Statistics</h2>
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
                <p class="text-gray-400 text-sm">Accounts Count</p>
                <p class="text-white font-medium text-xl">{{formatNumber .AccountsCount}}</p>
            </div>
            {{if .BlockstoreStats}}
            <div>
                <p class="text-gray-400 text-sm">Block Count</p>
                <p class="text-white font-medium text-xl">{{formatNumber .BlockstoreStats.BlockCount}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Transaction Count</p>
                <p class="text-white font-medium text-xl">{{formatNumber .BlockstoreStats.TransactionCount}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Latest Slot</p>
                <p class="text-white font-medium">{{.BlockstoreStats.LatestSlot}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Oldest Slot</p>
                <p class="text-white font-medium">{{.BlockstoreStats.OldestSlot}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Database Size</p>
                <p class="text-white font-medium">{{formatBytes .BlockstoreStats.DatabaseSize}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Finalized Slot</p>
                <p class="text-white font-medium">{{.BlockstoreStats.FinalizedSlot}}</p>
            </div>
            <div>
                <p class="text-gray-400 text-sm">Confirmed Slot</p>
                <p class="text-white font-medium">{{.BlockstoreStats.ConfirmedSlot}}</p>
            </div>
            {{end}}
        </div>
    </div>

    <!-- Dashboard Info -->
    <div class="bg-gray-800 rounded-lg border border-gray-700 p-6">
        <h2 class="text-lg font-semibold text-white mb-4">Dashboard</h2>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
                <p class="text-gray-400 text-sm">Listening Address</p>
                <p class="text-white mono">{{.DashboardAddress}}</p>
            </div>
        </div>
    </div>
</div>
`
