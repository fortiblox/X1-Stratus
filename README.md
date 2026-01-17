# X1-Stratus

**The Lightweight Verifier of the X1 Network**

In meteorology, *stratus* clouds form a continuous, uniform layer—covering the sky without demanding attention. X1-Stratus embodies this concept: a minimal, efficient presence that quietly verifies the X1 blockchain without the overhead of a full validator node.

While [X1-Aether](https://github.com/fortiblox/X1-Aether) runs a complete non-voting validator (8GB RAM) and [X1-Forge](https://github.com/fortiblox/X1-Forge) runs a full voting validator (64GB RAM), X1-Stratus takes a different approach. It receives blocks via Geyser streaming and replays transactions locally—achieving verification with just **2GB RAM**.

*Stratus verifies without the weight. Light as a cloud, yet nothing escapes its view.*

## What Does X1-Stratus Do?

- **Receives blocks** — Streams confirmed blocks via Geyser gRPC protocol
- **Replays transactions** — Executes transactions to verify state changes
- **Verifies state** — Confirms account states match the network consensus
- **Minimal footprint** — Runs on modest hardware (2GB RAM, 50GB disk)

*Perfect for developers, researchers, and anyone who wants to verify without running a full node.*

## Stratus vs Aether vs Forge

| | X1-Stratus | X1-Aether | X1-Forge |
|---------|------------|-----------|----------|
| **Role** | Lightweight Verifier | Full Non-Voting Node | Voting Validator |
| **RAM Required** | 2 GB | 8 GB | 64 GB |
| **Disk Required** | 50 GB | 500 GB | 500 GB |
| **Verification Method** | Geyser + Replay | Full Sync | Full Sync |
| **Runs Validator Binary** | No | Yes | Yes |
| **Earns Rewards** | No | No | Yes |
| **Participates in Gossip** | No | Yes | Yes |

*Stratus is light. Aether is silent. Forge is strong. All verify the network.*

## Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 2 cores | 4 cores |
| RAM | 2 GB | 4 GB |
| Storage | 50 GB SSD | 100 GB NVMe |
| Network | 10 Mbps | 50 Mbps |
| OS | Ubuntu 22.04+ | Ubuntu 24.04 |

**Additional requirements:**
- Linux only (Ubuntu/Debian or RHEL/CentOS/Fedora)
- `sudo` access for installation
- Geyser gRPC endpoint (from Triton One, Helius, QuickNode, etc.)

## Installation

### Quick Install

```bash
curl -sSfL https://raw.githubusercontent.com/fortiblox/X1-Stratus/main/install.sh | bash
```

The installer will:
1. Check your system meets requirements
2. Install Go and build dependencies
3. Build X1-Stratus from source
4. Create configuration file
5. Prompt for Geyser endpoint configuration
6. Set up systemd service
7. Optionally start the node

### Manual Installation

If you prefer to build manually:

```bash
# Install Go 1.22+
wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Clone and build
git clone https://github.com/fortiblox/X1-Stratus.git
cd X1-Stratus
go build -o stratus ./cmd/stratus

# Run
./stratus run --geyser-endpoint <YOUR_ENDPOINT> --data-dir ./data
```

## Configuration

### Geyser Endpoint

X1-Stratus requires a Geyser gRPC endpoint to receive blocks. Providers:

| Provider | Website | Notes |
|----------|---------|-------|
| Triton One | https://triton.one | Popular choice, good reliability |
| Helius | https://helius.dev | Developer-focused |
| QuickNode | https://quicknode.com | Enterprise-grade |
| Chainstack | https://chainstack.com | Multi-chain provider |

After obtaining access, configure your endpoint:

```bash
x1-stratus config
```

Or set environment variables:

```bash
export GEYSER_ENDPOINT="grpc.your-provider.com:443"
export GEYSER_TOKEN="your-auth-token"
```

### Configuration File

Location: `~/.config/x1-stratus/config.yaml`

```yaml
# Geyser gRPC connection
geyser_endpoint: "grpc.triton.one:443"
geyser_token: "your-token-here"
geyser_tls: true

# Data storage
data_dir: "/mnt/x1-stratus"

# Commitment level: processed, confirmed, finalized
commitment: "confirmed"

# Logging: debug, info, warn, error
log_level: "info"
```

## Commands

| Command | Description |
|---------|-------------|
| `x1-stratus start` | Start the verification node |
| `x1-stratus stop` | Stop the node |
| `x1-stratus restart` | Restart the node |
| `x1-stratus status` | Show current sync status |
| `x1-stratus logs` | Follow live logs |
| `x1-stratus config` | Edit configuration file |
| `x1-stratus version` | Show version information |

## File Locations

| Path | Description |
|------|-------------|
| `/opt/x1-stratus/bin/stratus` | Main binary |
| `~/.config/x1-stratus/config.yaml` | Configuration file |
| `/mnt/x1-stratus/accounts/` | Account state database (BadgerDB) |
| `/mnt/x1-stratus/blockstore/` | Block storage (BoltDB) |

## How It Works

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│                 │     │                 │     │                 │
│  Geyser gRPC    │────▶│   X1-Stratus    │────▶│  Verified State │
│  (Block Stream) │     │   (Replayer)    │     │  (AccountsDB)   │
│                 │     │                 │     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

1. **Geyser Client** connects to a Geyser gRPC endpoint and subscribes to confirmed blocks
2. **Blockstore** stores received blocks in BoltDB for persistence and indexing
3. **Replayer** executes each transaction to compute state changes
4. **AccountsDB** maintains the verified account state in BadgerDB
5. **Bank Hash** is computed after each slot to verify consensus

## Architecture

```
X1-Stratus/
├── cmd/stratus/          # CLI entry point
│   └── main.go
├── pkg/
│   ├── geyser/           # Geyser gRPC client
│   ├── blockstore/       # Block storage (BoltDB)
│   ├── accounts/         # Account database (BadgerDB)
│   ├── replayer/         # Transaction replay engine
│   ├── node/             # Node orchestrator
│   └── svm/              # Solana Virtual Machine
│       ├── sbpf/         # sBPF interpreter
│       ├── syscall/      # Syscall implementations
│       └── programs/     # Native programs
└── internal/types/       # Core types (Pubkey, Hash, etc.)
```

## Troubleshooting

### "Connection refused" from Geyser

1. Verify your endpoint URL is correct
2. Check your authentication token
3. Ensure TLS setting matches endpoint requirements
4. Try a different provider

### Node stops syncing

```bash
# Check logs for errors
x1-stratus logs

# Restart the node
x1-stratus restart
```

### High disk usage

X1-Stratus stores ~50-100GB of data. To reduce:

```bash
# Enable pruning (keeps only recent blocks)
# Edit config and add:
prune_enabled: true
prune_retain_slots: 100000
```

### Out of memory

X1-Stratus should run with 2GB RAM. If OOM occurs:
- Check for memory leaks in logs
- Ensure no other heavy processes running
- Add swap space if needed

## Development

### Building from Source

```bash
git clone https://github.com/fortiblox/X1-Stratus.git
cd X1-Stratus
go mod tidy
go build -o stratus ./cmd/stratus
```

### Running Tests

```bash
go test ./... -v
```

### Project Structure

| Package | Description |
|---------|-------------|
| `pkg/geyser` | Geyser gRPC client with automatic reconnection |
| `pkg/blockstore` | BoltDB-backed block storage with transaction indexing |
| `pkg/accounts` | BadgerDB-backed account state with snapshots |
| `pkg/replayer` | Transaction execution and bank hash computation |
| `pkg/node` | Orchestrates all components together |
| `pkg/svm` | sBPF VM, syscalls, and native programs |

## Limitations

X1-Stratus is a lightweight verifier with some limitations compared to full nodes:

1. **Limited BPF execution** — Currently verifies native program transactions (System, Vote, Stake). BPF program transactions are recorded but not fully re-executed.

2. **Simplified bank hash** — The bank hash computation is simplified. For exact hash matching, a full validator is needed.

3. **No gossip participation** — Stratus doesn't participate in the gossip network; it relies entirely on Geyser.

4. **Geyser dependency** — Requires a third-party Geyser provider for block data.

## Roadmap

- [x] Core types (Pubkey, Signature, Hash)
- [x] sBPF virtual machine interpreter
- [x] Syscall implementations
- [x] System Program (transfers, account creation)
- [x] Blockstore (BoltDB)
- [x] AccountsDB (BadgerDB)
- [x] Transaction replay engine
- [x] Geyser gRPC client
- [x] Node orchestrator
- [x] CLI entry point
- [ ] Full BPF program execution
- [ ] Exact bank hash computation
- [ ] Snapshot loading
- [ ] RPC server
- [ ] Web dashboard

## Prior Art

Stratus builds on the shoulders of giants:

- [Mithril](https://github.com/Overclock-Validator/mithril) — Solana verification node in Go (Overclock)
- [Radiance](https://github.com/gagliardetto/radiance) — Early Solana Go client (Richard Patel)
- [Firedancer](https://github.com/firedancer-io/firedancer) — High-performance Solana validator (Jump)
- [Sig](https://github.com/Syndica/sig) — Solana validator in Zig (Syndica)

## Contributing

Contributions are welcome! Please open an issue or pull request.

## License

Apache 2.0

## Links

- [X1-Aether](https://github.com/fortiblox/X1-Aether) — Non-voting verification (8GB RAM)
- [X1-Forge](https://github.com/fortiblox/X1-Forge) — Voting validator (64GB RAM)
- [Tachyon](https://github.com/x1-labs/tachyon) — X1's validator implementation

---

*X1-Stratus: Verification without the weight.*
