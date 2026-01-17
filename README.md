# X1-Stratus

**A Lightweight Verification Node for the X1 Network**

Stratus is a minimal, resource-efficient full node client for the X1 blockchain, built from the ground up in Go. Inspired by [Mithril](https://github.com/Overclock-Validator/mithril) for Solana, Stratus aims to make personal verification of the X1 chain accessible to anyone with consumer hardware.

> *"Stratus clouds form at the lowest altitudes, close to the earth—lightweight, pervasive, and always present."*

## Vision

The X1 network currently requires significant resources to run a full verification node (~64GB RAM for validators, ~8GB for non-voting nodes using Tachyon). Stratus aims to lower these barriers dramatically by:

1. **Only executing confirmed blocks** — No optimistic execution, no maintaining multiple fork states
2. **Bypassing Gossip overhead** — Fetch confirmed blocks via Geyser/RPC instead of real-time shreds
3. **Batched replay** — Process blocks in efficient batches rather than real-time streaming
4. **Minimal footprint** — Target: 2-4GB RAM on consumer hardware

## How Stratus Differs from X1-Aether

| | X1-Aether | X1-Stratus |
|--|-----------|------------|
| **Architecture** | Tachyon with `--no-voting` | Purpose-built minimal verifier |
| **Block Source** | Gossip/Turbine (real-time) | Geyser/RPC (confirmed only) |
| **Execution** | Optimistic (before confirmation) | Post-confirmation only |
| **Fork States** | Multiple (RAM overhead) | Single confirmed state |
| **RAM Target** | ~6.5 GB | ~2-4 GB |
| **Bandwidth** | ~260 Mbps | ~100 Mbps |
| **Latency** | Real-time | Few seconds behind |
| **Language** | Rust (Tachyon binary) | Go (from scratch) |

## Project Status

**Phase: Research & Development**

This project is in early development. See [ROADMAP.md](docs/ROADMAP.md) for detailed milestones.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      X1-Stratus                             │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Block Fetch │  │  Snapshot   │  │    RPC Interface    │  │
│  │   (Geyser)  │  │   Loader    │  │   (JSON-RPC 2.0)    │  │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘  │
│         │                │                    │             │
│         ▼                ▼                    │             │
│  ┌─────────────────────────────────────┐      │             │
│  │         Block Replay Engine         │◄─────┘             │
│  │   (Batched, Post-Confirmation)      │                    │
│  └──────────────────┬──────────────────┘                    │
│                     │                                       │
│         ┌───────────┼───────────┐                           │
│         ▼           ▼           ▼                           │
│  ┌───────────┐ ┌─────────┐ ┌─────────────┐                  │
│  │    SVM    │ │   STF   │ │  Consensus  │                  │
│  │ (sBPF VM) │ │  Rules  │ │  Verifier   │                  │
│  └─────┬─────┘ └────┬────┘ └──────┬──────┘                  │
│        │            │             │                         │
│        ▼            ▼             ▼                         │
│  ┌─────────────────────────────────────┐                    │
│  │            AccountsDB               │                    │
│  │     (Verified Chain State)          │                    │
│  └─────────────────────────────────────┘                    │
└─────────────────────────────────────────────────────────────┘
```

## Components

| Component | Description | Status |
|-----------|-------------|--------|
| `pkg/svm` | Solana Virtual Machine (sBPF execution) | Planned |
| `pkg/accounts` | AccountsDB for state storage | Planned |
| `pkg/blockstore` | Block storage and retrieval | Planned |
| `pkg/snapshot` | Snapshot decoding and loading | Planned |
| `pkg/rpc` | JSON-RPC interface | Planned |
| `pkg/gossip` | Optional Gossip participation | Planned |
| `pkg/consensus` | Fork choice and confirmation tracking | Planned |

## Target Hardware

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 4 cores | 8 cores |
| RAM | 2 GB | 4 GB |
| Storage | 100 GB SSD | 500 GB NVMe |
| Network | 50 Mbps | 100 Mbps |

## Building

```bash
# Clone the repository
git clone https://github.com/fortiblox/X1-Stratus.git
cd X1-Stratus

# Build
go build -o stratus ./cmd/stratus

# Run
./stratus --config config.yaml
```

## Development Roadmap

### Milestone 1: SVM Implementation
- sBPF virtual machine
- Native programs (System, Vote, Stake, BPF Loader)
- Syscalls and CPI
- Compute metering
- Conformance testing

### Milestone 2: Block Replay
- Snapshot syncing and decoding
- AccountsDB implementation
- Block replay engine
- STF rules (block/epoch level)
- Basic RPC interface

### Milestone 3: Optimization
- Batched replay (Erigon-style staged sync)
- BlockSTM parallel execution
- Alternative block sources (Geyser, custom relay)
- Cross-platform builds

See [docs/ROADMAP.md](docs/ROADMAP.md) for detailed timelines.

## Team

We're looking for contributors! See [docs/TEAM.md](docs/TEAM.md) for open positions.

## Prior Art

Stratus builds on the shoulders of giants:

- [Mithril](https://github.com/Overclock-Validator/mithril) — Solana verification node in Go (Overclock)
- [Radiance](https://github.com/gagliardetto/radiance) — Early Solana Go client (Richard Patel)
- [Firedancer](https://github.com/firedancer-io/firedancer) — High-performance Solana validator (Jump)
- [Sig](https://github.com/Syndica/sig) — Solana validator in Zig (Syndica)

## License

Apache 2.0

## Links

- [X1-Aether](https://github.com/fortiblox/X1-Aether) — Non-voting verification (Tachyon-based, works today)
- [X1-Forge](https://github.com/fortiblox/X1-Forge) — Voting validator (Tachyon-based)
- [Tachyon](https://github.com/x1-labs/tachyon) — X1's validator implementation
