# Geyser Integration Design Document

## Overview

This document outlines the integration strategy for consuming Solana Geyser data in X1-Stratus, a minimal verification node for the X1 network. Geyser is the primary mechanism for streaming real-time blockchain data from validators, and will serve as the preferred block source for Stratus.

---

## 1. Geyser Plugin Protocol

### What is Geyser?

Geyser is Solana's plugin system for streaming real-time blockchain data directly from validators. It provides callback-based access to account updates, transactions, slots, blocks, and entries without requiring RPC polling.

The plugin interface is defined in the `solana-geyser-plugin-interface` crate (now `agave-geyser-plugin-interface`) and implemented as a Rust trait called `GeyserPlugin`.

### How It Works

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Solana Validator                             │
│                                                                     │
│  ┌─────────────┐    ┌──────────────────┐    ┌─────────────────┐    │
│  │   Replay    │───▶│  Plugin Manager  │───▶│  Geyser Plugin  │    │
│  │   Stage     │    │                  │    │  (e.g., gRPC)   │    │
│  └─────────────┘    └──────────────────┘    └────────┬────────┘    │
│                                                      │             │
└──────────────────────────────────────────────────────┼─────────────┘
                                                       │
                                                       ▼
                                            ┌─────────────────────┐
                                            │   External Client   │
                                            │   (X1-Stratus)      │
                                            └─────────────────────┘
```

### GeyserPlugin Trait

The validator loads a Rust-based plugin implementing the `GeyserPlugin` trait. Each time an account, slot, transaction, or block updates, Solana invokes a corresponding callback method:

```rust
pub trait GeyserPlugin: Any + Send + Sync + Debug {
    // Required
    fn name(&self) -> &'static str;

    // Lifecycle
    fn on_load(&mut self, config_file: &str, is_reload: bool) -> Result<()>;
    fn on_unload(&mut self);

    // Data Callbacks
    fn update_account(
        &self,
        account: ReplicaAccountInfoVersions<'_>,
        slot: Slot,
        is_startup: bool,
    ) -> Result<()>;

    fn notify_transaction(
        &self,
        transaction: ReplicaTransactionInfoVersions<'_>,
        slot: Slot,
    ) -> Result<()>;

    fn update_slot_status(
        &self,
        slot: Slot,
        parent: Option<u64>,
        status: SlotStatus,
    ) -> Result<()>;

    fn notify_entry(
        &self,
        entry: ReplicaEntryInfoVersions<'_>,
    ) -> Result<()>;

    fn notify_block_metadata(
        &self,
        blockinfo: ReplicaBlockInfoVersions<'_>,
    ) -> Result<()>;

    // Feature Flags
    fn account_data_notifications_enabled(&self) -> bool;  // default: true
    fn transaction_notifications_enabled(&self) -> bool;   // default: false
    fn entry_notifications_enabled(&self) -> bool;         // default: false
}
```

### Key Characteristics

1. **Direct Memory Access**: Plugins receive data directly from validator memory, bypassing RPC overhead
2. **Callback-Based**: Updates are pushed, not polled
3. **Processed Level**: By default, data is streamed at the "processed" commitment level
4. **Performance Optimized**: Since Solana 1.16, methods use `&self` instead of `&mut self`, eliminating lock contention

---

## 2. Data Types Streamed

### Account Updates

Delivered via `update_account()` with `ReplicaAccountInfoVersions`:

| Field | Type | Description |
|-------|------|-------------|
| `pubkey` | `[u8; 32]` | Account address |
| `lamports` | `u64` | SOL balance (in lamports) |
| `owner` | `[u8; 32]` | Program that owns the account |
| `executable` | `bool` | Whether account contains executable code |
| `rent_epoch` | `u64` | Epoch at which rent is due |
| `data` | `&[u8]` | Account data bytes |
| `write_version` | `u64` | Monotonic version for ordering updates |
| `txn_signature` | `Option<Signature>` | Transaction that caused this update |

**Use Case**: Track state changes for specific programs or accounts.

### Slot Notifications

Delivered via `update_slot_status()`:

| Field | Type | Description |
|-------|------|-------------|
| `slot` | `u64` | Slot number |
| `parent` | `Option<u64>` | Parent slot (for fork tracking) |
| `status` | `SlotStatus` | Current status |

**SlotStatus Values**:
- `Processed` - Slot has been processed by this node
- `Confirmed` - Slot has received 2/3+ stake votes
- `Rooted` / `Finalized` - Slot is now permanent

**Intra-Slot Events** (via gRPC):
- `FirstShredReceived` - First shred from leader arrived
- `CreatedBank` - Bank created for this slot
- `Completed` - All expected shreds received
- `Dead` - Slot determined to be dead/skipped

**Use Case**: Track consensus progress, detect finality.

### Transaction Notifications

Delivered via `notify_transaction()` with `ReplicaTransactionInfoVersions`:

| Field | Type | Description |
|-------|------|-------------|
| `signature` | `Signature` | Transaction signature |
| `is_vote` | `bool` | Whether this is a vote transaction |
| `transaction` | `SanitizedTransaction` | Full transaction |
| `transaction_status_meta` | `TransactionStatusMeta` | Execution result |
| `index` | `usize` | Position in block |

**TransactionStatusMeta includes**:
- `status` - Success/Error
- `fee` - Transaction fee
- `pre_balances` / `post_balances` - Account balances before/after
- `pre_token_balances` / `post_token_balances` - SPL token balances
- `inner_instructions` - CPI trace
- `log_messages` - Program logs
- `rewards` - Any rewards distributed
- `compute_units_consumed` - CUs used

**Use Case**: Index transactions, monitor program activity.

### Block Metadata

Delivered via `notify_block_metadata()` with `ReplicaBlockInfoVersions`:

| Field | Type | Description |
|-------|------|-------------|
| `slot` | `u64` | Block slot |
| `blockhash` | `Hash` | Block hash |
| `parent_slot` | `u64` | Parent block slot |
| `parent_blockhash` | `Hash` | Parent block hash |
| `block_time` | `Option<UnixTimestamp>` | Block timestamp |
| `block_height` | `Option<u64>` | Block height |
| `rewards` | `&[Reward]` | Block rewards |
| `executed_transaction_count` | `u64` | Successful transactions |

**Use Case**: Verify block structure, track rewards.

### Entry Notifications

Delivered via `notify_entry()` with `ReplicaEntryInfoVersions`:

| Field | Type | Description |
|-------|------|-------------|
| `slot` | `u64` | Containing slot |
| `index` | `usize` | Entry index within slot |
| `num_hashes` | `u64` | PoH tick count |
| `hash` | `Hash` | Entry hash |
| `executed_transaction_count` | `u64` | Transactions in entry |

**Use Case**: PoH verification, fine-grained block reconstruction.

---

## 3. gRPC Interface (Yellowstone/Dragon's Mouth)

The most common way to consume Geyser data is via gRPC, exposed by plugins like Yellowstone (Triton One) and Jito's geyser-grpc-plugin.

### Service Definition

```protobuf
service Geyser {
    // Bidirectional streaming for real-time updates
    rpc Subscribe(stream SubscribeRequest) returns (stream SubscribeUpdate) {}

    // Replay information (earliest available slot)
    rpc SubscribeReplayInfo(SubscribeReplayInfoRequest)
        returns (SubscribeReplayInfoResponse) {}

    // Connection health
    rpc Ping(PingRequest) returns (PongResponse) {}

    // Unary methods (request-response)
    rpc GetLatestBlockhash(GetLatestBlockhashRequest)
        returns (GetLatestBlockhashResponse) {}
    rpc GetBlockHeight(GetBlockHeightRequest)
        returns (GetBlockHeightResponse) {}
    rpc GetSlot(GetSlotRequest) returns (GetSlotResponse) {}
    rpc IsBlockhashValid(IsBlockhashValidRequest)
        returns (IsBlockhashValidResponse) {}
    rpc GetVersion(GetVersionRequest) returns (GetVersionResponse) {}
}
```

### SubscribeRequest

```protobuf
message SubscribeRequest {
    map<string, SubscribeRequestFilterAccounts> accounts = 1;
    map<string, SubscribeRequestFilterSlots> slots = 2;
    map<string, SubscribeRequestFilterTransactions> transactions = 3;
    map<string, SubscribeRequestFilterTransactions> transactions_status = 4;
    map<string, SubscribeRequestFilterBlocks> blocks = 5;
    map<string, SubscribeRequestFilterBlocksMeta> blocks_meta = 6;
    map<string, SubscribeRequestFilterEntry> entry = 7;
    optional CommitmentLevel commitment = 8;
    repeated SubscribeRequestAccountsDataSlice accounts_data_slice = 9;
    optional SubscribeRequestPing ping = 10;
    optional uint64 from_slot = 11;
}
```

### Filter Types

**Account Filters**:
```protobuf
message SubscribeRequestFilterAccounts {
    repeated string account = 1;      // Specific pubkeys
    repeated string owner = 2;        // Program owners
    repeated SubscribeRequestFilterAccountsFilter filters = 3;
    bool nonempty_txn_signature = 4;
}

message SubscribeRequestFilterAccountsFilter {
    oneof filter {
        SubscribeRequestFilterAccountsFilterMemcmp memcmp = 1;
        uint64 datasize = 2;
        bool token_account_state = 3;
        SubscribeRequestFilterAccountsFilterLamports lamports = 4;
    }
}
```

**Transaction Filters**:
```protobuf
message SubscribeRequestFilterTransactions {
    optional bool vote = 1;                    // Include vote txns
    optional bool failed = 2;                  // Include failed txns
    optional string signature = 3;             // Specific signature
    repeated string account_include = 4;       // Must involve these accounts
    repeated string account_exclude = 5;       // Must NOT involve these
    repeated string account_required = 6;      // Must involve ALL of these
}
```

**Block Filters**:
```protobuf
message SubscribeRequestFilterBlocks {
    repeated string account_include = 1;
    optional bool include_transactions = 2;
    optional bool include_accounts = 3;
    optional bool include_entries = 4;
}
```

### SubscribeUpdate

```protobuf
message SubscribeUpdate {
    repeated string filters = 1;
    google.protobuf.Timestamp created_at = 2;

    oneof update_oneof {
        SubscribeUpdateAccount account = 3;
        SubscribeUpdateSlot slot = 4;
        SubscribeUpdateTransaction transaction = 5;
        SubscribeUpdateTransactionStatus transaction_status = 10;
        SubscribeUpdateBlock block = 6;
        SubscribeUpdateBlockMeta block_meta = 7;
        SubscribeUpdateEntry entry = 8;
        SubscribeUpdatePing ping = 9;
        SubscribeUpdatePong pong = 11;
    }
}
```

### Commitment Levels

```protobuf
enum CommitmentLevel {
    PROCESSED = 0;   // Immediate, may be rolled back
    CONFIRMED = 1;   // 2/3+ stake voted, unlikely to rollback
    FINALIZED = 2;   // Permanent, fully finalized
}
```

### Connection Keep-Alive

Cloud providers may close idle connections. Use ping mechanism:

```go
// Send ping periodically
stream.Send(&pb.SubscribeRequest{
    Ping: &pb.SubscribeRequestPing{Id: pingID},
})

// Server responds with Pong every ~15 seconds
```

---

## 4. Integration Approaches for Go Clients

### Option A: gRPC Client (Recommended)

**Pros**:
- Lowest latency (direct streaming)
- Type-safe with generated code
- Efficient binary protocol
- Supports filtering at server

**Cons**:
- Requires gRPC-enabled provider
- Not browser-compatible

**Implementation**:

```go
package geyser

import (
    "context"
    "crypto/tls"
    "time"

    pb "github.com/rpcpool/yellowstone-grpc/examples/golang/proto"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
    "google.golang.org/grpc/keepalive"
)

type Client struct {
    conn   *grpc.ClientConn
    client pb.GeyserClient
}

func NewClient(endpoint, token string) (*Client, error) {
    // Configure TLS and keepalive
    kacp := keepalive.ClientParameters{
        Time:                10 * time.Second,
        Timeout:             5 * time.Second,
        PermitWithoutStream: true,
    }

    opts := []grpc.DialOption{
        grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
        grpc.WithKeepaliveParams(kacp),
        grpc.WithDefaultCallOptions(
            grpc.MaxCallRecvMsgSize(1024 * 1024 * 1024),
        ),
        grpc.WithPerRPCCredentials(&tokenAuth{token: token}),
    }

    conn, err := grpc.Dial(endpoint, opts...)
    if err != nil {
        return nil, err
    }

    return &Client{
        conn:   conn,
        client: pb.NewGeyserClient(conn),
    }, nil
}

func (c *Client) SubscribeBlocks(ctx context.Context, commitment pb.CommitmentLevel) (
    <-chan *pb.SubscribeUpdateBlock, error) {

    stream, err := c.client.Subscribe(ctx)
    if err != nil {
        return nil, err
    }

    // Subscribe to blocks with specified commitment
    req := &pb.SubscribeRequest{
        Commitment: &commitment,
        Blocks: map[string]*pb.SubscribeRequestFilterBlocks{
            "all_blocks": {
                IncludeTransactions: boolPtr(true),
                IncludeAccounts:     boolPtr(false),
                IncludeEntries:      boolPtr(true),
            },
        },
    }

    if err := stream.Send(req); err != nil {
        return nil, err
    }

    blocks := make(chan *pb.SubscribeUpdateBlock, 100)

    go func() {
        defer close(blocks)
        for {
            update, err := stream.Recv()
            if err != nil {
                return
            }

            if block := update.GetBlock(); block != nil {
                blocks <- block
            }
        }
    }()

    return blocks, nil
}

// Token authentication
type tokenAuth struct {
    token string
}

func (t *tokenAuth) GetRequestMetadata(ctx context.Context, uri ...string) (
    map[string]string, error) {
    return map[string]string{"authorization": t.token}, nil
}

func (t *tokenAuth) RequireTransportSecurity() bool {
    return true
}

func boolPtr(b bool) *bool {
    return &b
}
```

### Option B: WebSocket (Alternative)

**Pros**:
- Widely available
- Works with standard Solana RPC providers
- Browser-compatible

**Cons**:
- Higher latency than gRPC
- Less efficient (JSON encoding)
- Limited filtering options

**Supported Methods**:
- `accountSubscribe` - Account changes
- `slotSubscribe` - Slot updates
- `signatureSubscribe` - Transaction status
- `blockSubscribe` - New blocks (some providers)
- `logsSubscribe` - Program logs

**Implementation**:

```go
package rpc

import (
    "context"
    "encoding/json"

    "github.com/gorilla/websocket"
)

type WSClient struct {
    conn *websocket.Conn
}

func NewWSClient(endpoint string) (*WSClient, error) {
    conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
    if err != nil {
        return nil, err
    }
    return &WSClient{conn: conn}, nil
}

func (c *WSClient) SubscribeSlots(ctx context.Context) (<-chan SlotUpdate, error) {
    // Send subscription request
    req := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      1,
        "method":  "slotSubscribe",
    }

    if err := c.conn.WriteJSON(req); err != nil {
        return nil, err
    }

    slots := make(chan SlotUpdate, 100)

    go func() {
        defer close(slots)
        for {
            _, msg, err := c.conn.ReadMessage()
            if err != nil {
                return
            }

            var resp struct {
                Params struct {
                    Result SlotUpdate `json:"result"`
                } `json:"params"`
            }

            if err := json.Unmarshal(msg, &resp); err == nil {
                slots <- resp.Params.Result
            }
        }
    }()

    return slots, nil
}

type SlotUpdate struct {
    Slot   uint64 `json:"slot"`
    Parent uint64 `json:"parent"`
    Root   uint64 `json:"root"`
}
```

### Option C: RPC Polling (Fallback)

**Pros**:
- Works with any RPC provider
- Simple implementation
- Good for historical data

**Cons**:
- Higher latency (polling interval)
- Higher bandwidth usage
- Rate limit concerns
- Not real-time

**Implementation**:

```go
package rpc

import (
    "context"
    "time"
)

type Poller struct {
    client   *RPCClient
    interval time.Duration
}

func (p *Poller) PollBlocks(ctx context.Context, startSlot uint64) <-chan *Block {
    blocks := make(chan *Block, 100)

    go func() {
        defer close(blocks)
        currentSlot := startSlot

        for {
            select {
            case <-ctx.Done():
                return
            case <-time.After(p.interval):
                // Get current finalized slot
                latestSlot, err := p.client.GetSlot(ctx, "finalized")
                if err != nil {
                    continue
                }

                // Fetch missing blocks
                for slot := currentSlot; slot <= latestSlot; slot++ {
                    block, err := p.client.GetBlock(ctx, slot)
                    if err != nil {
                        continue // Skip failed slots
                    }
                    blocks <- block
                    currentSlot = slot + 1
                }
            }
        }
    }()

    return blocks
}
```

### Comparison Matrix

| Feature | gRPC | WebSocket | RPC Polling |
|---------|------|-----------|-------------|
| Latency | ~50ms | ~200ms | 400ms-2s |
| Data Format | Protobuf | JSON | JSON |
| Filtering | Rich | Limited | None |
| Provider Support | Specialized | Common | Universal |
| Bandwidth | Low | Medium | High |
| Complexity | Medium | Low | Low |

---

## 5. Existing Implementations

### Yellowstone gRPC (Triton One)

**Repository**: https://github.com/rpcpool/yellowstone-grpc

**Features**:
- Full gRPC service for Solana Geyser
- Bidirectional streaming
- Rich filtering (accounts, transactions, blocks)
- Supports commitment levels
- Ping/pong for keepalive

**Proto Files**: `yellowstone-grpc-proto/proto/geyser.proto`

**Go Client**: `examples/golang/` directory contains reference implementation

**Providers**:
- Triton One (Dragon's Mouth)
- Helius
- QuickNode
- Chainstack

### Jito Geyser gRPC

**Repository**: https://github.com/jito-foundation/geyser-grpc-plugin

**Features**:
- Lightweight gRPC plugin
- Account and slot streaming
- Designed for MEV use cases

**Differences from Yellowstone**:
- Simpler proto definition
- Focused on accounts/slots (less full-block support)
- Optimized for Jito's MEV infrastructure

### Comparison

| Feature | Yellowstone | Jito |
|---------|-------------|------|
| Account streaming | Yes | Yes |
| Slot streaming | Yes | Yes |
| Transaction streaming | Yes | Limited |
| Block streaming | Yes | No |
| Entry streaming | Yes | No |
| Filter complexity | High | Low |
| Production providers | Many | Jito infra |

**Recommendation**: Use Yellowstone gRPC for X1-Stratus due to full block/entry support needed for verification.

---

## 6. Recommended Integration for X1-Stratus

### Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         X1-Stratus                                   │
│                                                                     │
│  ┌────────────────────────────────────────────────────────────┐     │
│  │                    Input Manager                            │     │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │     │
│  │  │   Geyser     │  │   WebSocket  │  │   RPC Poller     │  │     │
│  │  │   Client     │  │   Client     │  │   (Fallback)     │  │     │
│  │  │  (Primary)   │  │ (Secondary)  │  │                  │  │     │
│  │  └──────┬───────┘  └──────┬───────┘  └────────┬─────────┘  │     │
│  │         │                 │                   │            │     │
│  │         └─────────────────┼───────────────────┘            │     │
│  │                           ▼                                │     │
│  │  ┌────────────────────────────────────────────────────┐    │     │
│  │  │              Block Normalizer                       │    │     │
│  │  │    (Convert provider-specific format to internal)   │    │     │
│  │  └──────────────────────────┬─────────────────────────┘    │     │
│  │                             │                              │     │
│  └─────────────────────────────┼──────────────────────────────┘     │
│                                ▼                                    │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                    Block Queue                               │    │
│  │   (Confirmed blocks, ordered by slot, with gap detection)    │    │
│  └──────────────────────────────┬──────────────────────────────┘    │
│                                 ▼                                   │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                   Replay Engine                              │    │
│  │   (Execute transactions, verify bank hash)                   │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
1. Connect to Geyser gRPC endpoint
                │
                ▼
2. Subscribe to CONFIRMED blocks with:
   - IncludeTransactions: true
   - IncludeEntries: true
   - IncludeAccounts: false (we'll compute state)
                │
                ▼
3. For each SubscribeUpdateBlock:
   │
   ├─▶ Extract block metadata (slot, blockhash, parent)
   │
   ├─▶ Extract entries with PoH hashes
   │
   ├─▶ Extract transactions with:
   │   - Signatures
   │   - Instructions
   │   - Message
   │   - Status metadata (for verification only)
   │
   └─▶ Convert to internal Block type
                │
                ▼
4. Queue block for replay
                │
                ▼
5. Replay engine executes transactions
                │
                ▼
6. Compute bank hash, verify against received blockhash
```

### Subscription Strategy

For X1-Stratus verification, we need:

1. **Blocks at CONFIRMED level**: We don't need FINALIZED (adds latency) and PROCESSED is too risky (may rollback)

2. **Full transactions**: Required for execution

3. **Entries**: Required for PoH verification

4. **Slot updates**: Track consensus progress

```go
req := &pb.SubscribeRequest{
    Commitment: pb.CommitmentLevel_CONFIRMED.Enum(),

    // Full blocks for replay
    Blocks: map[string]*pb.SubscribeRequestFilterBlocks{
        "verified_blocks": {
            IncludeTransactions: boolPtr(true),
            IncludeEntries:      boolPtr(true),
            IncludeAccounts:     boolPtr(false),
        },
    },

    // Slot progress tracking
    Slots: map[string]*pb.SubscribeRequestFilterSlots{
        "all_slots": {},
    },
}
```

### Failover Strategy

```
Primary: Geyser gRPC
    │
    ├─ Connection lost?
    │   └─▶ Retry with exponential backoff
    │
    ├─ Provider unavailable?
    │   └─▶ Switch to secondary Geyser provider
    │
    └─ All Geyser providers down?
        └─▶ Fallback to RPC polling
```

### Gap Detection and Recovery

```go
type BlockQueue struct {
    expected uint64           // Next expected slot
    buffer   map[uint64]*Block // Out-of-order buffer

    geyserClient *GeyserClient
    rpcClient    *RPCClient
}

func (q *BlockQueue) HandleBlock(block *Block) {
    if block.Slot == q.expected {
        // In order, process immediately
        q.process(block)
        q.expected++
        q.drainBuffer()
    } else if block.Slot > q.expected {
        // Gap detected, buffer and request missing
        q.buffer[block.Slot] = block
        go q.requestMissingBlocks(q.expected, block.Slot)
    }
    // block.Slot < q.expected: duplicate, ignore
}

func (q *BlockQueue) requestMissingBlocks(from, to uint64) {
    for slot := from; slot < to; slot++ {
        // Try RPC fallback for missing blocks
        block, err := q.rpcClient.GetBlock(ctx, slot)
        if err == nil {
            q.incoming <- block
        }
    }
}
```

---

## 7. Go Implementation Strategy

### Package Structure

```
pkg/
  geyser/
    client.go          # gRPC client wrapper
    subscription.go    # Subscription management
    types.go           # Internal type definitions
    convert.go         # Proto to internal conversion

  rpc/
    client.go          # JSON-RPC client
    websocket.go       # WebSocket subscriptions
    poller.go          # Polling fallback

  input/
    manager.go         # Input source orchestration
    queue.go           # Block queue with gap detection
    failover.go        # Provider failover logic
```

### Dependencies

```go
// go.mod
require (
    google.golang.org/grpc v1.60.0
    google.golang.org/protobuf v1.32.0
    github.com/gorilla/websocket v1.5.1
)
```

### Proto Generation

```bash
# Generate Go code from Yellowstone proto files
protoc --go_out=. --go-grpc_out=. \
    -I yellowstone-grpc-proto/proto \
    yellowstone-grpc-proto/proto/geyser.proto \
    yellowstone-grpc-proto/proto/solana-storage.proto
```

### Interface Design

```go
// pkg/input/interface.go

// BlockSource provides confirmed blocks for replay
type BlockSource interface {
    // Start begins streaming blocks from the given slot
    Start(ctx context.Context, fromSlot uint64) error

    // Blocks returns channel of confirmed blocks
    Blocks() <-chan *types.Block

    // Slots returns channel of slot status updates
    Slots() <-chan *types.SlotUpdate

    // Close shuts down the source
    Close() error

    // Health returns current connection status
    Health() SourceHealth
}

type SourceHealth struct {
    Connected   bool
    LastSlot    uint64
    LastUpdate  time.Time
    Provider    string
    Latency     time.Duration
}
```

### Configuration

```yaml
# config.yaml
input:
  # Primary source
  primary:
    type: geyser
    endpoint: "https://grpc.example.com"
    token: "${GEYSER_TOKEN}"
    commitment: confirmed

  # Secondary source (automatic failover)
  secondary:
    type: geyser
    endpoint: "https://grpc-backup.example.com"
    token: "${GEYSER_TOKEN_BACKUP}"

  # Fallback (when all gRPC sources fail)
  fallback:
    type: rpc
    endpoint: "https://rpc.example.com"
    poll_interval: 1s

  # Queue settings
  queue:
    buffer_size: 1000
    gap_timeout: 30s
```

### Error Handling

```go
// Retry with exponential backoff
func (c *Client) connectWithRetry(ctx context.Context) error {
    backoff := 1 * time.Second
    maxBackoff := 60 * time.Second

    for {
        err := c.connect(ctx)
        if err == nil {
            return nil
        }

        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(backoff):
            backoff = min(backoff*2, maxBackoff)
        }
    }
}
```

---

## 8. Performance Considerations

### Latency Targets

| Metric | Target | Notes |
|--------|--------|-------|
| Block receive latency | <100ms | From confirmation to receipt |
| Processing queue depth | <10 blocks | Avoid falling behind |
| Reconnection time | <5s | After disconnect |

### Bandwidth Estimates

| Data Type | Size/Block | Blocks/s | Bandwidth |
|-----------|------------|----------|-----------|
| Full blocks | ~1-5 MB | 2.5 | 2.5-12.5 MB/s |
| Block metadata only | ~1 KB | 2.5 | ~2.5 KB/s |
| Transactions only | ~500 KB | 2.5 | ~1.25 MB/s |

### Optimization Strategies

1. **Request only needed data**: Use `IncludeAccounts: false` since we compute state ourselves

2. **Use data slicing for large accounts**: If monitoring specific accounts, request only needed byte ranges

3. **Buffer management**: Pre-allocate buffers to reduce GC pressure

4. **Connection pooling**: Maintain warm connections to multiple providers

---

## 9. Security Considerations

### Authentication

- Store tokens in environment variables, not config files
- Rotate tokens periodically
- Use TLS for all connections

### Data Validation

- Verify block hashes match parent chain
- Validate transaction signatures before execution
- Check slot numbers are sequential

### Trust Model

| Data Source | Trust Level | Verification |
|-------------|-------------|--------------|
| Geyser stream | Untrusted | Full execution + hash verification |
| RPC fallback | Untrusted | Full execution + hash verification |
| Snapshots | Semi-trusted | Hash verification + incremental replay |

---

## 10. Testing Strategy

### Unit Tests

- Proto conversion functions
- Queue gap detection
- Failover logic

### Integration Tests

```go
func TestGeyserSubscription(t *testing.T) {
    // Connect to testnet Geyser
    client, err := geyser.NewClient(testnetEndpoint, testToken)
    require.NoError(t, err)
    defer client.Close()

    // Subscribe to blocks
    blocks, err := client.SubscribeBlocks(ctx, pb.CommitmentLevel_CONFIRMED)
    require.NoError(t, err)

    // Verify we receive blocks
    select {
    case block := <-blocks:
        assert.NotZero(t, block.Slot)
        assert.NotEmpty(t, block.Blockhash)
    case <-time.After(30 * time.Second):
        t.Fatal("timeout waiting for block")
    }
}
```

### Conformance Tests

- Compare received block data against RPC `getBlock` results
- Verify computed bank hashes match network

---

## References

### Official Documentation

- [Solana Geyser Plugins](https://docs.solanalabs.com/validator/geyser)
- [Agave Geyser Plugin Interface](https://docs.solana.com/developing/plugins/geyser-plugins)
- [GeyserPlugin Trait Documentation](https://docs.rs/solana-geyser-plugin-interface/latest/solana_geyser_plugin_interface/geyser_plugin_interface/trait.GeyserPlugin.html)

### Yellowstone/Dragon's Mouth

- [Yellowstone gRPC Repository](https://github.com/rpcpool/yellowstone-grpc)
- [Dragon's Mouth Documentation](https://docs.triton.one/project-yellowstone/dragons-mouth-grpc-subscriptions)
- [Yellowstone Go Package](https://pkg.go.dev/github.com/rpcpool/yellowstone-grpc/examples/golang/proto)

### Provider Documentation

- [Helius gRPC Documentation](https://www.helius.dev/docs/grpc)
- [QuickNode Yellowstone Guide](https://www.quicknode.com/guides/solana-development/tooling/geyser/yellowstone-go)
- [Chainstack Yellowstone Guide](https://chainstack.com/how-to-use-the-solana-geyser-plugin-to-stream-data-with-yellowstone-grpc/)

### Other Implementations

- [Jito Geyser gRPC Plugin](https://github.com/jito-foundation/geyser-grpc-plugin)
- [Solana Geyser Park (Plugin List)](https://github.com/rpcpool/solana-geyser-park)

---

## Appendix A: Complete Proto Reference

See: https://github.com/rpcpool/yellowstone-grpc/blob/master/yellowstone-grpc-proto/proto/geyser.proto

## Appendix B: Provider Comparison

| Provider | Endpoint Format | Auth Method | Rate Limits |
|----------|-----------------|-------------|-------------|
| Triton One | `grpc.triton.one:443` | Bearer token | Plan-based |
| Helius | `mainnet.helius-rpc.com:443` | API key header | Plan-based |
| QuickNode | `{endpoint}:10000` | API key in URL | Plan-based |
| Chainstack | `{node}.p2pify.com:443` | API key header | Plan-based |

## Appendix C: Glossary

- **Geyser**: Solana's plugin system for streaming blockchain data
- **Yellowstone**: Triton One's gRPC interface for Geyser
- **Dragon's Mouth**: Triton's commercial Geyser gRPC service
- **Commitment Level**: How confirmed a piece of data is (processed/confirmed/finalized)
- **PoH**: Proof of History, Solana's timing mechanism
- **Bank Hash**: Hash of account state at a given slot
- **Entry**: A batch of transactions with PoH verification
