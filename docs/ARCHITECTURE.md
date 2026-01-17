# X1-Stratus Architecture

## Design Philosophy

Stratus is designed around one core principle: **Do only what's necessary to verify confirmed state.**

Unlike full validators that must execute optimistically and maintain multiple fork states, Stratus waits for consensus confirmation before execution. This dramatically simplifies the architecture and reduces resource requirements.

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                            X1-Stratus                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                        INPUT LAYER                                │   │
│  ├────────────────┬─────────────────┬───────────────────────────────┤   │
│  │  Geyser Client │  RPC Poller     │  Gossip (Optional)            │   │
│  │  (Primary)     │  (Fallback)     │  (Network Contribution)       │   │
│  └───────┬────────┴────────┬────────┴──────────────┬────────────────┘   │
│          │                 │                       │                    │
│          ▼                 ▼                       ▼                    │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                     BLOCK QUEUE                                   │   │
│  │              (Confirmed blocks only, ordered by slot)             │   │
│  └──────────────────────────────┬───────────────────────────────────┘   │
│                                 │                                       │
│                                 ▼                                       │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                     REPLAY ENGINE                                 │   │
│  │                                                                   │   │
│  │   ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐   │   │
│  │   │   Block     │  │   Entry     │  │     Transaction         │   │   │
│  │   │ Validator   │─▶│  Processor  │─▶│      Executor           │   │   │
│  │   └─────────────┘  └─────────────┘  └───────────┬─────────────┘   │   │
│  │                                                 │                 │   │
│  └─────────────────────────────────────────────────┼─────────────────┘   │
│                                                    │                     │
│                                                    ▼                     │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                     EXECUTION LAYER                               │   │
│  │                                                                   │   │
│  │   ┌─────────────────────────────────────────────────────────┐     │   │
│  │   │                         SVM                              │     │   │
│  │   │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │     │   │
│  │   │  │  sBPF    │  │ Native   │  │ Syscalls │  │   CPI    │ │     │   │
│  │   │  │   VM     │  │ Programs │  │          │  │          │ │     │   │
│  │   │  └──────────┘  └──────────┘  └──────────┘  └──────────┘ │     │   │
│  │   └─────────────────────────────────────────────────────────┘     │   │
│  │                                                                   │   │
│  │   ┌─────────────────────────────────────────────────────────┐     │   │
│  │   │                    STF Rules                             │     │   │
│  │   │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │     │   │
│  │   │  │  Block   │  │  Entry   │  │  Epoch   │  │ BankHash │ │     │   │
│  │   │  │  Rules   │  │  Rules   │  │  Rules   │  │  Calc    │ │     │   │
│  │   │  └──────────┘  └──────────┘  └──────────┘  └──────────┘ │     │   │
│  │   └─────────────────────────────────────────────────────────┘     │   │
│  │                                                                   │   │
│  └──────────────────────────────────┬───────────────────────────────┘   │
│                                     │                                   │
│                                     ▼                                   │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                     STORAGE LAYER                                 │   │
│  │                                                                   │   │
│  │   ┌────────────────────────┐  ┌────────────────────────┐          │   │
│  │   │      AccountsDB        │  │       Blockstore       │          │   │
│  │   │  (Verified State)      │  │  (Confirmed Blocks)    │          │   │
│  │   └────────────────────────┘  └────────────────────────┘          │   │
│  │                                                                   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                     OUTPUT LAYER                                  │   │
│  │                                                                   │   │
│  │   ┌────────────────────────┐  ┌────────────────────────┐          │   │
│  │   │     RPC Server         │  │    Metrics / Health    │          │   │
│  │   │   (JSON-RPC 2.0)       │  │   (Prometheus)         │          │   │
│  │   └────────────────────────┘  └────────────────────────┘          │   │
│  │                                                                   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Component Details

### 1. Input Layer

#### Geyser Client (Primary)
Subscribes to confirmed blocks from a Geyser-enabled RPC node.

```go
type GeyserClient interface {
    SubscribeBlocks(ctx context.Context, startSlot uint64) (<-chan *Block, error)
    GetSnapshot(ctx context.Context, slot uint64) (*Snapshot, error)
}
```

**Advantages:**
- Low latency (pushed, not polled)
- Efficient (only confirmed blocks)
- Lower bandwidth than Gossip

#### RPC Poller (Fallback)
Polls `getBlock` for confirmed blocks when Geyser unavailable.

```go
type RPCPoller interface {
    GetBlock(ctx context.Context, slot uint64) (*Block, error)
    GetSlot(ctx context.Context, commitment string) (uint64, error)
}
```

#### Gossip (Optional)
Full Gossip participation for network mesh contribution.

```go
type GossipNode interface {
    Start(ctx context.Context) error
    ReceiveShreds() <-chan *Shred
    BroadcastRepairRequest(slot uint64) error
}
```

---

### 2. Replay Engine

The replay engine processes confirmed blocks sequentially.

```go
type ReplayEngine struct {
    svm         *SVM
    accountsDB  *AccountsDB
    blockstore  *Blockstore
    stfRules    *STFRules
}

func (r *ReplayEngine) ReplayBlock(block *Block) (*BankHash, error) {
    // 1. Validate block structure
    if err := r.stfRules.ValidateBlock(block); err != nil {
        return nil, err
    }

    // 2. Process each entry
    for _, entry := range block.Entries {
        if err := r.processEntry(entry); err != nil {
            return nil, err
        }
    }

    // 3. Apply epoch boundary rules if needed
    if isEpochBoundary(block.Slot) {
        if err := r.applyEpochTransition(); err != nil {
            return nil, err
        }
    }

    // 4. Calculate and return bank hash
    return r.calculateBankHash(), nil
}
```

---

### 3. Execution Layer

#### SVM (Solana Virtual Machine)

The SVM executes individual transactions.

```go
type SVM struct {
    vm             *sBPFVM
    nativePrograms map[Pubkey]NativeProgram
    syscalls       *SyscallRegistry
}

type ExecutionResult struct {
    Success       bool
    Logs          []string
    ComputeUnits  uint64
    ReturnData    []byte
    AccountDeltas map[Pubkey]*AccountDelta
}

func (s *SVM) ExecuteTransaction(tx *Transaction, accounts []*Account) (*ExecutionResult, error) {
    // 1. Validate transaction structure
    // 2. Load program
    // 3. Execute instructions
    // 4. Apply account changes
    // 5. Return result
}
```

#### Native Programs

| Program | Address | Purpose |
|---------|---------|---------|
| System | `11111111111111111111111111111111` | Account creation, transfers |
| Vote | `Vote111111111111111111111111111111111111111` | Validator voting |
| Stake | `Stake11111111111111111111111111111111111111` | Stake delegation |
| BPF Loader | `BPFLoaderUpgradeab1e11111111111111111111111` | Program deployment |
| Address Lookup | `AddressLookupTab1e1111111111111111111111111` | Transaction compression |
| Config | `Config1111111111111111111111111111111111111` | On-chain configuration |

#### STF Rules

State Transition Function rules beyond SVM execution:

```go
type STFRules struct {
    // Block-level
    maxBlockCU        uint64
    maxBlockSize      uint64
    maxAccountLocks   uint64

    // Epoch-level
    slotsPerEpoch     uint64
    warmupCooldown    uint64
}

func (s *STFRules) ValidateBlock(block *Block) error {
    // Check compute units
    // Check block size
    // Verify entry hashes
    // Verify PoH sequence
    return nil
}
```

---

### 4. Storage Layer

#### AccountsDB

Stores account state with efficient access patterns.

```go
type AccountsDB interface {
    GetAccount(pubkey Pubkey) (*Account, error)
    SetAccount(pubkey Pubkey, account *Account) error
    GetSlot() uint64
    Commit() error
    Snapshot() (*Snapshot, error)
}
```

**Implementation options:**
- BadgerDB (default, pure Go)
- RocksDB (performance, CGO)
- Memory-mapped files (low RAM)

#### Blockstore

Stores confirmed blocks for historical queries.

```go
type Blockstore interface {
    PutBlock(block *Block) error
    GetBlock(slot uint64) (*Block, error)
    GetBlockRange(start, end uint64) ([]*Block, error)
    Prune(beforeSlot uint64) error
}
```

---

### 5. Output Layer

#### RPC Server

JSON-RPC 2.0 compatible interface.

```go
type RPCServer struct {
    accountsDB *AccountsDB
    blockstore *Blockstore
    forwarder  *TransactionForwarder
}

// Supported methods
func (r *RPCServer) GetBalance(pubkey string) (uint64, error)
func (r *RPCServer) GetAccountInfo(pubkey string) (*AccountInfo, error)
func (r *RPCServer) GetSlot() (uint64, error)
func (r *RPCServer) GetBlock(slot uint64) (*Block, error)
func (r *RPCServer) SendTransaction(tx string) (string, error)
func (r *RPCServer) SimulateTransaction(tx string) (*SimulateResult, error)
```

---

## Data Flow

### Normal Operation

```
1. Geyser pushes confirmed block
          │
          ▼
2. Block added to queue
          │
          ▼
3. Replay engine validates block structure
          │
          ▼
4. For each entry, for each transaction:
   │
   ├─▶ Load accounts from AccountsDB
   │
   ├─▶ Execute via SVM
   │
   └─▶ Apply account deltas to AccountsDB
          │
          ▼
5. Calculate bank hash, verify against network
          │
          ▼
6. Commit state, update slot
          │
          ▼
7. RPC can now serve queries for this slot
```

### Bootstrap (from snapshot)

```
1. Download snapshot from trusted source
          │
          ▼
2. Verify snapshot integrity
          │
          ▼
3. Load accounts into AccountsDB
          │
          ▼
4. Set initial slot from snapshot
          │
          ▼
5. Begin fetching confirmed blocks from snapshot slot + 1
          │
          ▼
6. Replay blocks until caught up
```

---

## Resource Optimization Strategies

### 1. Confirmed-Only Execution
- No optimistic execution
- No maintaining multiple fork states
- Single AccountsDB state

### 2. Batched Processing
- Buffer N blocks before processing
- Amortize disk I/O across batch
- Parallelize signature verification

### 3. Lazy Account Loading
- Load accounts on-demand during execution
- LRU cache for hot accounts
- Memory-map cold accounts

### 4. Pruning
- Prune old blocks from Blockstore
- Prune historical account versions
- Configurable retention period

---

## Configuration

```yaml
# config.yaml
node:
  data_dir: /var/lib/stratus

input:
  mode: geyser  # geyser, rpc, gossip
  geyser_url: ws://geyser.example.com
  fallback_rpc: https://rpc.example.com

execution:
  batch_size: 10  # blocks per batch
  parallel_signatures: true

storage:
  accounts_backend: badger  # badger, rocksdb
  accounts_cache_mb: 512
  blockstore_retention_slots: 100000

rpc:
  enabled: true
  bind: 0.0.0.0:8899
  max_connections: 100

metrics:
  enabled: true
  bind: 0.0.0.0:9090
```

---

## Security Considerations

### Trust Model
- **Snapshot source:** Trusted initially, verified via replay
- **Block source:** Untrusted, fully verified before acceptance
- **RPC queries:** Served from locally verified state

### Verification
- All transactions executed and verified
- Bank hashes compared against network consensus
- Invalid blocks rejected entirely

### Isolation
- No private keys stored
- Read-only verification
- Transactions forwarded, not signed
