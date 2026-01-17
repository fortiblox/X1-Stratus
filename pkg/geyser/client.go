package geyser

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Client errors.
var (
	ErrNotConnected     = errors.New("geyser client not connected")
	ErrAlreadyConnected = errors.New("geyser client already connected")
	ErrClosed           = errors.New("geyser client closed")
	ErrSubscribeFailed  = errors.New("subscribe request failed")
	ErrStreamClosed     = errors.New("geyser stream closed")
	ErrMaxReconnects    = errors.New("max reconnection attempts reached")
)

// Client is a Geyser gRPC client for receiving block updates.
//
// It manages the gRPC connection, handles subscriptions, and provides
// channel-based APIs for receiving blocks and slot updates. The client
// automatically reconnects on connection loss with exponential backoff.
type Client struct {
	config Config

	// gRPC connection and stream
	conn   *grpc.ClientConn
	stream *geyserStream

	// Output channels
	blocks chan *Block
	slots  chan *SlotUpdate

	// State management
	mu              sync.RWMutex
	connected       atomic.Bool
	closed          atomic.Bool
	lastSlot        atomic.Uint64
	lastUpdate      atomic.Int64 // Unix nano timestamp
	reconnectCount  atomic.Int32
	pingID          atomic.Int32
	cancelFunc      context.CancelFunc
	wg              sync.WaitGroup
	lastError       error
	lastErrorMu     sync.RWMutex

	// Context for the current connection
	ctx context.Context
}

// geyserStream wraps a gRPC bidirectional stream for Geyser subscriptions.
type geyserStream struct {
	stream grpc.ClientStream
}

// Send sends a subscription request to the server.
func (s *geyserStream) Send(req *subscribeRequest) error {
	return s.stream.SendMsg(req)
}

// Recv receives a subscription update from the server.
func (s *geyserStream) Recv() (*subscribeUpdate, error) {
	update := &subscribeUpdate{}
	if err := s.stream.RecvMsg(update); err != nil {
		return nil, err
	}
	return update, nil
}

// CloseSend closes the send side of the stream.
func (s *geyserStream) CloseSend() error {
	return s.stream.CloseSend()
}

// subscribeRequest is a placeholder for the gRPC SubscribeRequest message.
// In production, this would be generated from the Yellowstone proto files.
// We define it here to avoid dependency on proto generation for the initial implementation.
type subscribeRequest struct {
	Accounts           map[string]*accountsFilter      `protobuf:"bytes,1,rep,name=accounts"`
	Slots              map[string]*slotsFilter         `protobuf:"bytes,2,rep,name=slots"`
	Transactions       map[string]*transactionsFilter  `protobuf:"bytes,3,rep,name=transactions"`
	TransactionsStatus map[string]*transactionsFilter  `protobuf:"bytes,4,rep,name=transactions_status"`
	Blocks             map[string]*blocksFilter        `protobuf:"bytes,5,rep,name=blocks"`
	BlocksMeta         map[string]*blocksMetaFilter    `protobuf:"bytes,6,rep,name=blocks_meta"`
	Entry              map[string]*entryFilter         `protobuf:"bytes,7,rep,name=entry"`
	Commitment         *int32                          `protobuf:"varint,8,opt,name=commitment"`
	AccountsDataSlice  []*accountsDataSlice            `protobuf:"bytes,9,rep,name=accounts_data_slice"`
	Ping               *pingRequest                    `protobuf:"bytes,10,opt,name=ping"`
	FromSlot           *uint64                         `protobuf:"varint,11,opt,name=from_slot"`
}

func (x *subscribeRequest) Reset()         { *x = subscribeRequest{} }
func (x *subscribeRequest) String() string { return fmt.Sprintf("%+v", *x) }
func (x *subscribeRequest) ProtoMessage()  {}

type accountsFilter struct {
	Account              []string `protobuf:"bytes,1,rep,name=account"`
	Owner                []string `protobuf:"bytes,2,rep,name=owner"`
	NonemptyTxnSignature bool     `protobuf:"varint,4,opt,name=nonempty_txn_signature"`
}

type slotsFilter struct {
	FilterByCommitment *int32 `protobuf:"varint,1,opt,name=filter_by_commitment"`
}

type transactionsFilter struct {
	Vote            *bool    `protobuf:"varint,1,opt,name=vote"`
	Failed          *bool    `protobuf:"varint,2,opt,name=failed"`
	Signature       *string  `protobuf:"bytes,3,opt,name=signature"`
	AccountInclude  []string `protobuf:"bytes,4,rep,name=account_include"`
	AccountExclude  []string `protobuf:"bytes,5,rep,name=account_exclude"`
	AccountRequired []string `protobuf:"bytes,6,rep,name=account_required"`
}

type blocksFilter struct {
	AccountInclude      []string `protobuf:"bytes,1,rep,name=account_include"`
	IncludeTransactions *bool    `protobuf:"varint,2,opt,name=include_transactions"`
	IncludeAccounts     *bool    `protobuf:"varint,3,opt,name=include_accounts"`
	IncludeEntries      *bool    `protobuf:"varint,4,opt,name=include_entries"`
}

type blocksMetaFilter struct{}

type entryFilter struct{}

type accountsDataSlice struct {
	Offset uint64 `protobuf:"varint,1,opt,name=offset"`
	Length uint64 `protobuf:"varint,2,opt,name=length"`
}

type pingRequest struct {
	ID int32 `protobuf:"varint,1,opt,name=id"`
}

// subscribeUpdate is a placeholder for the gRPC SubscribeUpdate message.
type subscribeUpdate struct {
	Filters           []string                 `protobuf:"bytes,1,rep,name=filters"`
	CreatedAt         *timestamp               `protobuf:"bytes,2,opt,name=created_at"`
	Account           *accountUpdate           `protobuf:"bytes,3,opt,name=account"`
	Slot              *slotUpdate              `protobuf:"bytes,4,opt,name=slot"`
	Transaction       *transactionUpdate       `protobuf:"bytes,5,opt,name=transaction"`
	TransactionStatus *transactionStatusUpdate `protobuf:"bytes,10,opt,name=transaction_status"`
	Block             *blockUpdate             `protobuf:"bytes,6,opt,name=block"`
	BlockMeta         *blockMetaUpdate         `protobuf:"bytes,7,opt,name=block_meta"`
	Entry             *entryUpdate             `protobuf:"bytes,8,opt,name=entry"`
	Ping              *pingUpdate              `protobuf:"bytes,9,opt,name=ping"`
	Pong              *pongUpdate              `protobuf:"bytes,11,opt,name=pong"`
}

func (x *subscribeUpdate) Reset()         { *x = subscribeUpdate{} }
func (x *subscribeUpdate) String() string { return fmt.Sprintf("%+v", *x) }
func (x *subscribeUpdate) ProtoMessage()  {}

type timestamp struct {
	Seconds int64 `protobuf:"varint,1,opt,name=seconds"`
	Nanos   int32 `protobuf:"varint,2,opt,name=nanos"`
}

type accountUpdate struct {
	Pubkey       []byte  `protobuf:"bytes,1,opt,name=pubkey"`
	Lamports     uint64  `protobuf:"varint,2,opt,name=lamports"`
	Owner        []byte  `protobuf:"bytes,3,opt,name=owner"`
	Executable   bool    `protobuf:"varint,4,opt,name=executable"`
	RentEpoch    uint64  `protobuf:"varint,5,opt,name=rent_epoch"`
	Data         []byte  `protobuf:"bytes,6,opt,name=data"`
	WriteVersion uint64  `protobuf:"varint,7,opt,name=write_version"`
	Slot         uint64  `protobuf:"varint,8,opt,name=slot"`
	TxnSignature []byte  `protobuf:"bytes,9,opt,name=txn_signature"`
}

type slotUpdate struct {
	Slot   uint64  `protobuf:"varint,1,opt,name=slot"`
	Parent *uint64 `protobuf:"varint,2,opt,name=parent"`
	Status int32   `protobuf:"varint,3,opt,name=status"`
}

type transactionUpdate struct {
	Transaction *transactionInfo `protobuf:"bytes,1,opt,name=transaction"`
	Slot        uint64           `protobuf:"varint,2,opt,name=slot"`
}

type transactionInfo struct {
	Signature   []byte               `protobuf:"bytes,1,opt,name=signature"`
	IsVote      bool                 `protobuf:"varint,2,opt,name=is_vote"`
	Transaction *compiledTransaction `protobuf:"bytes,3,opt,name=transaction"`
	Meta        *transactionMeta     `protobuf:"bytes,4,opt,name=meta"`
	Index       uint64               `protobuf:"varint,5,opt,name=index"`
}

type compiledTransaction struct {
	Signatures [][]byte `protobuf:"bytes,1,rep,name=signatures"`
	Message    *message `protobuf:"bytes,2,opt,name=message"`
}

type message struct {
	Header              *messageHeader         `protobuf:"bytes,1,opt,name=header"`
	AccountKeys         [][]byte               `protobuf:"bytes,2,rep,name=account_keys"`
	RecentBlockhash     []byte                 `protobuf:"bytes,3,opt,name=recent_blockhash"`
	Instructions        []*compiledInstruction `protobuf:"bytes,4,rep,name=instructions"`
	Versioned           bool                   `protobuf:"varint,5,opt,name=versioned"`
	AddressTableLookups []*addressTableLookup  `protobuf:"bytes,6,rep,name=address_table_lookups"`
}

type messageHeader struct {
	NumRequiredSignatures       uint32 `protobuf:"varint,1,opt,name=num_required_signatures"`
	NumReadonlySignedAccounts   uint32 `protobuf:"varint,2,opt,name=num_readonly_signed_accounts"`
	NumReadonlyUnsignedAccounts uint32 `protobuf:"varint,3,opt,name=num_readonly_unsigned_accounts"`
}

type compiledInstruction struct {
	ProgramIDIndex uint32 `protobuf:"varint,1,opt,name=program_id_index"`
	Accounts       []byte `protobuf:"bytes,2,opt,name=accounts"`
	Data           []byte `protobuf:"bytes,3,opt,name=data"`
}

type addressTableLookup struct {
	AccountKey      []byte `protobuf:"bytes,1,opt,name=account_key"`
	WritableIndexes []byte `protobuf:"bytes,2,opt,name=writable_indexes"`
	ReadonlyIndexes []byte `protobuf:"bytes,3,opt,name=readonly_indexes"`
}

type transactionMeta struct {
	Err                     *transactionError    `protobuf:"bytes,1,opt,name=err"`
	Fee                     uint64               `protobuf:"varint,2,opt,name=fee"`
	PreBalances             []uint64             `protobuf:"varint,3,rep,name=pre_balances"`
	PostBalances            []uint64             `protobuf:"varint,4,rep,name=post_balances"`
	InnerInstructions       []*innerInstructions `protobuf:"bytes,5,rep,name=inner_instructions"`
	InnerInstructionsNone   bool                 `protobuf:"varint,10,opt,name=inner_instructions_none"`
	LogMessages             []string             `protobuf:"bytes,6,rep,name=log_messages"`
	LogMessagesNone         bool                 `protobuf:"varint,11,opt,name=log_messages_none"`
	PreTokenBalances        []*tokenBalance      `protobuf:"bytes,7,rep,name=pre_token_balances"`
	PostTokenBalances       []*tokenBalance      `protobuf:"bytes,8,rep,name=post_token_balances"`
	Rewards                 []*reward            `protobuf:"bytes,9,rep,name=rewards"`
	LoadedWritableAddresses [][]byte             `protobuf:"bytes,12,rep,name=loaded_writable_addresses"`
	LoadedReadonlyAddresses [][]byte             `protobuf:"bytes,13,rep,name=loaded_readonly_addresses"`
	ReturnData              *returnData          `protobuf:"bytes,14,opt,name=return_data"`
	ReturnDataNone          bool                 `protobuf:"varint,15,opt,name=return_data_none"`
	ComputeUnitsConsumed    *uint64              `protobuf:"varint,16,opt,name=compute_units_consumed"`
}

type transactionError struct {
	Err []byte `protobuf:"bytes,1,opt,name=err"`
}

type innerInstructions struct {
	Index        uint32                 `protobuf:"varint,1,opt,name=index"`
	Instructions []*compiledInstruction `protobuf:"bytes,2,rep,name=instructions"`
}

type tokenBalance struct {
	AccountIndex  uint32         `protobuf:"varint,1,opt,name=account_index"`
	Mint          string         `protobuf:"bytes,2,opt,name=mint"`
	UiTokenAmount *uiTokenAmount `protobuf:"bytes,3,opt,name=ui_token_amount"`
	Owner         string         `protobuf:"bytes,4,opt,name=owner"`
	ProgramID     string         `protobuf:"bytes,5,opt,name=program_id"`
}

type uiTokenAmount struct {
	UiAmount       float64 `protobuf:"fixed64,1,opt,name=ui_amount"`
	Decimals       uint32  `protobuf:"varint,2,opt,name=decimals"`
	Amount         string  `protobuf:"bytes,3,opt,name=amount"`
	UiAmountString string  `protobuf:"bytes,4,opt,name=ui_amount_string"`
}

type reward struct {
	Pubkey      string `protobuf:"bytes,1,opt,name=pubkey"`
	Lamports    int64  `protobuf:"varint,2,opt,name=lamports"`
	PostBalance uint64 `protobuf:"varint,3,opt,name=post_balance"`
	RewardType  int32  `protobuf:"varint,4,opt,name=reward_type"`
	Commission  string `protobuf:"bytes,5,opt,name=commission"`
}

type returnData struct {
	ProgramID []byte `protobuf:"bytes,1,opt,name=program_id"`
	Data      []byte `protobuf:"bytes,2,opt,name=data"`
}

type transactionStatusUpdate struct {
	Slot      uint64            `protobuf:"varint,1,opt,name=slot"`
	Signature string            `protobuf:"bytes,2,opt,name=signature"`
	IsVote    bool              `protobuf:"varint,3,opt,name=is_vote"`
	Index     uint64            `protobuf:"varint,4,opt,name=index"`
	Err       *transactionError `protobuf:"bytes,5,opt,name=err"`
}

type blockUpdate struct {
	Slot                     uint64             `protobuf:"varint,1,opt,name=slot"`
	Blockhash                string             `protobuf:"bytes,2,opt,name=blockhash"`
	Rewards                  *rewards           `protobuf:"bytes,3,opt,name=rewards"`
	BlockTime                *unixTimestamp     `protobuf:"bytes,4,opt,name=block_time"`
	BlockHeight              *blockHeight       `protobuf:"bytes,5,opt,name=block_height"`
	ParentSlot               uint64             `protobuf:"varint,7,opt,name=parent_slot"`
	ParentBlockhash          string             `protobuf:"bytes,8,opt,name=parent_blockhash"`
	ExecutedTransactionCount uint64             `protobuf:"varint,9,opt,name=executed_transaction_count"`
	Transactions             []*transactionInfo `protobuf:"bytes,6,rep,name=transactions"`
	UpdatedAccountCount      uint64             `protobuf:"varint,10,opt,name=updated_account_count"`
	Accounts                 []*accountUpdate   `protobuf:"bytes,11,rep,name=accounts"`
	EntriesCount             uint64             `protobuf:"varint,12,opt,name=entries_count"`
	Entries                  []*entryUpdate     `protobuf:"bytes,13,rep,name=entries"`
}

type rewards struct {
	Rewards []*reward `protobuf:"bytes,1,rep,name=rewards"`
}

type unixTimestamp struct {
	Timestamp int64 `protobuf:"varint,1,opt,name=timestamp"`
}

type blockHeight struct {
	BlockHeight uint64 `protobuf:"varint,1,opt,name=block_height"`
}

type blockMetaUpdate struct {
	Slot                     uint64         `protobuf:"varint,1,opt,name=slot"`
	Blockhash                string         `protobuf:"bytes,2,opt,name=blockhash"`
	Rewards                  *rewards       `protobuf:"bytes,3,opt,name=rewards"`
	BlockTime                *unixTimestamp `protobuf:"bytes,4,opt,name=block_time"`
	BlockHeight              *blockHeight   `protobuf:"bytes,5,opt,name=block_height"`
	ParentSlot               uint64         `protobuf:"varint,6,opt,name=parent_slot"`
	ParentBlockhash          string         `protobuf:"bytes,7,opt,name=parent_blockhash"`
	ExecutedTransactionCount uint64         `protobuf:"varint,8,opt,name=executed_transaction_count"`
}

type entryUpdate struct {
	Slot                     uint64 `protobuf:"varint,1,opt,name=slot"`
	Index                    uint64 `protobuf:"varint,2,opt,name=index"`
	NumHashes                uint64 `protobuf:"varint,3,opt,name=num_hashes"`
	Hash                     []byte `protobuf:"bytes,4,opt,name=hash"`
	ExecutedTransactionCount uint64 `protobuf:"varint,5,opt,name=executed_transaction_count"`
	StartingTransactionIndex uint64 `protobuf:"varint,6,opt,name=starting_transaction_index"`
}

type pingUpdate struct{}

type pongUpdate struct {
	ID int32 `protobuf:"varint,1,opt,name=id"`
}

// NewClient creates a new Geyser client with the given configuration.
// The client is not connected until Connect() is called.
func NewClient(config Config) (*Client, error) {
	config = config.WithDefaults()
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Client{
		config: config,
		blocks: make(chan *Block, config.BlockChannelSize),
		slots:  make(chan *SlotUpdate, config.SlotChannelSize),
	}, nil
}

// Connect establishes the gRPC connection and starts the subscription.
// This method blocks until the connection is established or an error occurs.
func (c *Client) Connect(ctx context.Context) error {
	if c.closed.Load() {
		return ErrClosed
	}
	if c.connected.Load() {
		return ErrAlreadyConnected
	}

	// Create a cancellable context for this connection
	ctx, cancel := context.WithCancel(ctx)
	c.cancelFunc = cancel
	c.ctx = ctx

	// Establish connection
	if err := c.connect(ctx); err != nil {
		cancel()
		return err
	}

	// Start the receive loop
	c.wg.Add(1)
	go c.receiveLoop()

	// Start the ping loop for keepalive
	c.wg.Add(1)
	go c.pingLoop()

	// Start the health check loop
	c.wg.Add(1)
	go c.healthCheckLoop()

	c.connected.Store(true)
	c.lastUpdate.Store(time.Now().UnixNano())

	if c.config.OnConnect != nil {
		c.config.OnConnect()
	}

	return nil
}

// connect establishes the gRPC connection.
func (c *Client) connect(ctx context.Context) error {
	// Configure keepalive
	kacp := keepalive.ClientParameters{
		Time:                c.config.KeepaliveTime,
		Timeout:             c.config.KeepaliveTimeout,
		PermitWithoutStream: true,
	}

	// Configure dial options
	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(kacp),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(c.config.MaxMessageSize),
			grpc.MaxCallSendMsgSize(c.config.MaxMessageSize),
		),
	}

	// TLS configuration
	if c.config.UseTLS {
		opts = append(opts, grpc.WithTransportCredentials(
			credentials.NewTLS(&tls.Config{
				MinVersion: tls.VersionTLS12,
			}),
		))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Add authentication if configured
	if c.config.Token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(&tokenAuth{
			token:      c.config.ExpandedToken(),
			requireTLS: c.config.UseTLS,
		}))
	}

	// Dial the server using the legacy Dial API for compatibility
	//nolint:staticcheck // Using Dial for compatibility with older gRPC versions
	conn, err := grpc.Dial(c.config.Endpoint, opts...)
	if err != nil {
		return fmt.Errorf("failed to dial gRPC: %w", err)
	}
	c.conn = conn

	// Create metadata context with custom headers
	md := metadata.New(c.config.Headers)
	streamCtx := metadata.NewOutgoingContext(ctx, md)

	// Create the bidirectional stream using raw gRPC
	// In production, this would use the generated Geyser client
	streamDesc := &grpc.StreamDesc{
		StreamName:    "Subscribe",
		ServerStreams: true,
		ClientStreams: true,
	}

	stream, err := conn.NewStream(streamCtx, streamDesc, "/geyser.Geyser/Subscribe")
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create stream: %w", err)
	}

	// Wrap in our typed stream interface
	c.stream = &geyserStream{stream: stream}

	// Send the subscription request
	if err := c.sendSubscribeRequest(); err != nil {
		stream.CloseSend()
		conn.Close()
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	return nil
}

// sendSubscribeRequest sends the subscription request to the server.
func (c *Client) sendSubscribeRequest() error {
	commitment := int32(c.config.Commitment)

	// Build the subscription request
	req := &subscribeRequest{
		Commitment: &commitment,
		Blocks: map[string]*blocksFilter{
			"blocks": {
				IncludeTransactions: boolPtr(c.config.IncludeTransactions),
				IncludeAccounts:     boolPtr(c.config.IncludeAccounts),
				IncludeEntries:      boolPtr(c.config.IncludeEntries),
			},
		},
		Slots: map[string]*slotsFilter{
			"slots": {},
		},
	}

	// Set starting slot if configured
	if c.config.FromSlot != nil {
		req.FromSlot = c.config.FromSlot
	}

	return c.stream.Send(req)
}

// receiveLoop continuously receives updates from the gRPC stream.
func (c *Client) receiveLoop() {
	defer c.wg.Done()
	defer c.handleDisconnect(nil)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		update, err := c.stream.Recv()
		if err != nil {
			if err == io.EOF {
				c.setLastError(ErrStreamClosed)
				c.handleDisconnect(ErrStreamClosed)
				return
			}
			if c.ctx.Err() != nil {
				// Context cancelled, normal shutdown
				return
			}
			c.setLastError(err)
			c.handleDisconnect(err)
			return
		}

		c.lastUpdate.Store(time.Now().UnixNano())
		c.processUpdate(update)
	}
}

// processUpdate processes a single update from the stream.
func (c *Client) processUpdate(update *subscribeUpdate) {
	if update == nil {
		return
	}

	// Handle block updates
	if update.Block != nil {
		block := c.convertBlock(update.Block)
		c.lastSlot.Store(block.Slot)

		// Call callback if configured
		if c.config.OnBlock != nil {
			c.config.OnBlock(block)
		}

		// Send to channel (non-blocking with potential drop)
		select {
		case c.blocks <- block:
		default:
			// Channel full, drop oldest block
			select {
			case <-c.blocks:
			default:
			}
			c.blocks <- block
		}
	}

	// Handle slot updates
	if update.Slot != nil {
		slot := c.convertSlotUpdate(update.Slot)
		c.lastSlot.Store(slot.Slot)

		// Call callback if configured
		if c.config.OnSlot != nil {
			c.config.OnSlot(slot)
		}

		// Send to channel (non-blocking)
		select {
		case c.slots <- slot:
		default:
			// Channel full, drop oldest
			select {
			case <-c.slots:
			default:
			}
			c.slots <- slot
		}
	}

	// Handle pong (keepalive response)
	if update.Pong != nil {
		// Pong received, connection is alive
	}
}

// convertBlock converts a protobuf block to our Block type.
func (c *Client) convertBlock(pb *blockUpdate) *Block {
	block := &Block{
		Slot:                     pb.Slot,
		ParentSlot:               pb.ParentSlot,
		ExecutedTransactionCount: pb.ExecutedTransactionCount,
		ReceivedAt:               time.Now(),
	}

	// Convert blockhash
	if pb.Blockhash != "" {
		if hash, err := types.HashFromBase58(pb.Blockhash); err == nil {
			block.Blockhash = hash
		}
	}

	// Convert parent blockhash
	if pb.ParentBlockhash != "" {
		if hash, err := types.HashFromBase58(pb.ParentBlockhash); err == nil {
			block.ParentBlockhash = hash
		}
	}

	// Convert block time
	if pb.BlockTime != nil {
		ts := pb.BlockTime.Timestamp
		block.BlockTime = &ts
	}

	// Convert block height
	if pb.BlockHeight != nil {
		height := pb.BlockHeight.BlockHeight
		block.BlockHeight = &height
	}

	// Convert transactions
	if len(pb.Transactions) > 0 {
		block.Transactions = make([]Transaction, len(pb.Transactions))
		for i, txInfo := range pb.Transactions {
			block.Transactions[i] = c.convertTransaction(txInfo)
		}
	}

	// Convert entries
	if len(pb.Entries) > 0 {
		block.Entries = make([]Entry, len(pb.Entries))
		for i, entry := range pb.Entries {
			block.Entries[i] = c.convertEntry(entry)
		}
	}

	// Convert rewards
	if pb.Rewards != nil && len(pb.Rewards.Rewards) > 0 {
		block.Rewards = make([]Reward, len(pb.Rewards.Rewards))
		for i, r := range pb.Rewards.Rewards {
			block.Rewards[i] = c.convertReward(r)
		}
	}

	return block
}

// convertTransaction converts a protobuf transaction to our Transaction type.
func (c *Client) convertTransaction(pb *transactionInfo) Transaction {
	tx := Transaction{
		IsVote: pb.IsVote,
		Index:  pb.Index,
	}

	// Convert signature
	if len(pb.Signature) == types.SignatureSize {
		copy(tx.Signature[:], pb.Signature)
	}

	// Convert compiled transaction
	if pb.Transaction != nil {
		// Convert signatures
		tx.Signatures = make([]types.Signature, len(pb.Transaction.Signatures))
		for i, sig := range pb.Transaction.Signatures {
			if len(sig) == types.SignatureSize {
				copy(tx.Signatures[i][:], sig)
			}
		}

		// Convert message
		if pb.Transaction.Message != nil {
			tx.Message = c.convertMessage(pb.Transaction.Message)
		}
	}

	// Convert meta
	if pb.Meta != nil {
		tx.Meta = c.convertMeta(pb.Meta)
	}

	return tx
}

// convertMessage converts a protobuf message to our TransactionMessage type.
func (c *Client) convertMessage(pb *message) TransactionMessage {
	msg := TransactionMessage{
		IsLegacy: !pb.Versioned,
	}

	// Convert header
	if pb.Header != nil {
		msg.Header = MessageHeader{
			NumRequiredSignatures:       uint8(pb.Header.NumRequiredSignatures),
			NumReadonlySignedAccounts:   uint8(pb.Header.NumReadonlySignedAccounts),
			NumReadonlyUnsignedAccounts: uint8(pb.Header.NumReadonlyUnsignedAccounts),
		}
	}

	// Convert account keys
	msg.AccountKeys = make([]types.Pubkey, len(pb.AccountKeys))
	for i, key := range pb.AccountKeys {
		if len(key) == types.PubkeySize {
			copy(msg.AccountKeys[i][:], key)
		}
	}

	// Convert recent blockhash
	if len(pb.RecentBlockhash) == types.HashSize {
		copy(msg.RecentBlockhash[:], pb.RecentBlockhash)
	}

	// Convert instructions
	msg.Instructions = make([]CompiledInstruction, len(pb.Instructions))
	for i, ix := range pb.Instructions {
		msg.Instructions[i] = CompiledInstruction{
			ProgramIDIndex: uint8(ix.ProgramIDIndex),
			AccountIndexes: ix.Accounts,
			Data:           ix.Data,
		}
	}

	// Convert address table lookups
	if len(pb.AddressTableLookups) > 0 {
		msg.AddressTableLookups = make([]AddressTableLookup, len(pb.AddressTableLookups))
		for i, lookup := range pb.AddressTableLookups {
			atl := AddressTableLookup{
				WritableIndexes: lookup.WritableIndexes,
				ReadonlyIndexes: lookup.ReadonlyIndexes,
			}
			if len(lookup.AccountKey) == types.PubkeySize {
				copy(atl.AccountKey[:], lookup.AccountKey)
			}
			msg.AddressTableLookups[i] = atl
		}
	}

	return msg
}

// convertMeta converts a protobuf transaction meta to our TransactionMeta type.
func (c *Client) convertMeta(pb *transactionMeta) *TransactionMeta {
	meta := &TransactionMeta{
		Fee:          pb.Fee,
		PreBalances:  pb.PreBalances,
		PostBalances: pb.PostBalances,
	}

	// Convert error
	if pb.Err != nil && len(pb.Err.Err) > 0 {
		meta.Err = &TransactionError{
			Message: string(pb.Err.Err),
		}
	}

	// Convert compute units
	if pb.ComputeUnitsConsumed != nil {
		meta.ComputeUnitsConsumed = *pb.ComputeUnitsConsumed
	}

	// Convert log messages
	if !pb.LogMessagesNone {
		meta.LogMessages = pb.LogMessages
	}

	// Convert inner instructions
	if !pb.InnerInstructionsNone && len(pb.InnerInstructions) > 0 {
		meta.InnerInstructions = make([]InnerInstructions, len(pb.InnerInstructions))
		for i, inner := range pb.InnerInstructions {
			meta.InnerInstructions[i] = InnerInstructions{
				Index: uint8(inner.Index),
			}
			if len(inner.Instructions) > 0 {
				meta.InnerInstructions[i].Instructions = make([]CompiledInstruction, len(inner.Instructions))
				for j, ix := range inner.Instructions {
					meta.InnerInstructions[i].Instructions[j] = CompiledInstruction{
						ProgramIDIndex: uint8(ix.ProgramIDIndex),
						AccountIndexes: ix.Accounts,
						Data:           ix.Data,
					}
				}
			}
		}
	}

	// Convert token balances
	if len(pb.PreTokenBalances) > 0 {
		meta.PreTokenBalances = c.convertTokenBalances(pb.PreTokenBalances)
	}
	if len(pb.PostTokenBalances) > 0 {
		meta.PostTokenBalances = c.convertTokenBalances(pb.PostTokenBalances)
	}

	// Convert loaded addresses
	if len(pb.LoadedWritableAddresses) > 0 {
		meta.LoadedWritableAddresses = make([]types.Pubkey, len(pb.LoadedWritableAddresses))
		for i, addr := range pb.LoadedWritableAddresses {
			if len(addr) == types.PubkeySize {
				copy(meta.LoadedWritableAddresses[i][:], addr)
			}
		}
	}
	if len(pb.LoadedReadonlyAddresses) > 0 {
		meta.LoadedReadonlyAddresses = make([]types.Pubkey, len(pb.LoadedReadonlyAddresses))
		for i, addr := range pb.LoadedReadonlyAddresses {
			if len(addr) == types.PubkeySize {
				copy(meta.LoadedReadonlyAddresses[i][:], addr)
			}
		}
	}

	// Convert return data
	if !pb.ReturnDataNone && pb.ReturnData != nil {
		meta.ReturnData = &ReturnData{
			Data: pb.ReturnData.Data,
		}
		if len(pb.ReturnData.ProgramID) == types.PubkeySize {
			copy(meta.ReturnData.ProgramID[:], pb.ReturnData.ProgramID)
		}
	}

	return meta
}

// convertTokenBalances converts protobuf token balances to our type.
func (c *Client) convertTokenBalances(pb []*tokenBalance) []TokenBalance {
	balances := make([]TokenBalance, len(pb))
	for i, tb := range pb {
		balances[i] = TokenBalance{
			AccountIndex: uint8(tb.AccountIndex),
		}

		// Convert mint
		if mint, err := types.PubkeyFromBase58(tb.Mint); err == nil {
			balances[i].Mint = mint
		}

		// Convert owner
		if owner, err := types.PubkeyFromBase58(tb.Owner); err == nil {
			balances[i].Owner = owner
		}

		// Convert program ID
		if programID, err := types.PubkeyFromBase58(tb.ProgramID); err == nil {
			balances[i].ProgramID = programID
		}

		// Convert UI token amount
		if tb.UiTokenAmount != nil {
			balances[i].UITokenAmount = UITokenAmount{
				Amount:         tb.UiTokenAmount.Amount,
				Decimals:       uint8(tb.UiTokenAmount.Decimals),
				UIAmountString: tb.UiTokenAmount.UiAmountString,
			}
			if tb.UiTokenAmount.UiAmount != 0 {
				uiAmount := tb.UiTokenAmount.UiAmount
				balances[i].UITokenAmount.UIAmount = &uiAmount
			}
		}
	}
	return balances
}

// convertEntry converts a protobuf entry to our Entry type.
func (c *Client) convertEntry(pb *entryUpdate) Entry {
	entry := Entry{
		Slot:                     pb.Slot,
		Index:                    pb.Index,
		NumHashes:                pb.NumHashes,
		ExecutedTransactionCount: pb.ExecutedTransactionCount,
		StartingTransactionIndex: pb.StartingTransactionIndex,
	}

	// Convert hash
	if len(pb.Hash) == types.HashSize {
		copy(entry.Hash[:], pb.Hash)
	}

	return entry
}

// convertReward converts a protobuf reward to our Reward type.
func (c *Client) convertReward(pb *reward) Reward {
	r := Reward{
		Lamports:    pb.Lamports,
		PostBalance: pb.PostBalance,
		RewardType:  RewardType(pb.RewardType),
	}

	// Convert pubkey
	if pubkey, err := types.PubkeyFromBase58(pb.Pubkey); err == nil {
		r.Pubkey = pubkey
	}

	// Convert commission
	if pb.Commission != "" {
		// Parse commission from string
		var commission uint8
		if _, err := fmt.Sscanf(pb.Commission, "%d", &commission); err == nil {
			r.Commission = &commission
		}
	}

	return r
}

// convertSlotUpdate converts a protobuf slot update to our SlotUpdate type.
func (c *Client) convertSlotUpdate(pb *slotUpdate) *SlotUpdate {
	update := &SlotUpdate{
		Slot:       pb.Slot,
		Status:     SlotStatus(pb.Status),
		ReceivedAt: time.Now(),
	}

	if pb.Parent != nil {
		update.ParentSlot = pb.Parent
	}

	return update
}

// pingLoop sends periodic ping messages to keep the connection alive.
func (c *Client) pingLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if !c.connected.Load() {
				return
			}

			pingID := c.pingID.Add(1)
			req := &subscribeRequest{
				Ping: &pingRequest{ID: pingID},
			}

			if err := c.stream.Send(req); err != nil {
				// Ping failed, but don't disconnect yet - let health check handle it
				c.setLastError(err)
			}
		}
	}
}

// healthCheckLoop monitors connection health and triggers reconnection if needed.
func (c *Client) healthCheckLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if !c.connected.Load() {
				return
			}

			// Check if we've received updates recently
			lastUpdate := time.Unix(0, c.lastUpdate.Load())
			if time.Since(lastUpdate) > c.config.StaleTimeout {
				c.setLastError(fmt.Errorf("connection stale: no updates for %v", time.Since(lastUpdate)))
				c.handleDisconnect(fmt.Errorf("connection stale"))
				return
			}
		}
	}
}

// handleDisconnect handles disconnection and optionally reconnects.
func (c *Client) handleDisconnect(err error) {
	if !c.connected.CompareAndSwap(true, false) {
		return // Already disconnected
	}

	// Call disconnect callback
	if c.config.OnDisconnect != nil {
		c.config.OnDisconnect(err)
	}

	// Clean up current connection
	if c.stream != nil {
		c.stream.CloseSend()
	}
	if c.conn != nil {
		c.conn.Close()
	}

	// Attempt reconnection if not closed
	if !c.closed.Load() {
		go c.reconnect()
	}
}

// reconnect attempts to reconnect with exponential backoff.
func (c *Client) reconnect() {
	backoff := c.config.ReconnectMinDelay
	attempt := 0

	for !c.closed.Load() {
		attempt++
		c.reconnectCount.Add(1)

		// Check max reconnects
		if c.config.MaxReconnects > 0 && attempt > c.config.MaxReconnects {
			c.setLastError(ErrMaxReconnects)
			return
		}

		// Wait before reconnecting
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Create new context for reconnection
		ctx, cancel := context.WithCancel(context.Background())
		c.mu.Lock()
		c.cancelFunc = cancel
		c.ctx = ctx
		c.mu.Unlock()

		// Attempt to connect
		if err := c.connect(ctx); err != nil {
			c.setLastError(err)
			// Exponential backoff
			backoff = minDuration(backoff*2, c.config.ReconnectMaxDelay)
			continue
		}

		// Reconnection successful
		c.connected.Store(true)
		c.lastUpdate.Store(time.Now().UnixNano())

		// Restart loops
		c.wg.Add(3)
		go c.receiveLoop()
		go c.pingLoop()
		go c.healthCheckLoop()

		// Call reconnect callback
		if c.config.OnReconnect != nil {
			c.config.OnReconnect(attempt)
		}

		return
	}
}

// Blocks returns the channel for receiving block updates.
func (c *Client) Blocks() <-chan *Block {
	return c.blocks
}

// Slots returns the channel for receiving slot updates.
func (c *Client) Slots() <-chan *SlotUpdate {
	return c.slots
}

// Health returns the current health status of the client.
func (c *Client) Health() ClientHealth {
	lastUpdate := time.Unix(0, c.lastUpdate.Load())
	latency := time.Since(lastUpdate)
	if c.connected.Load() && latency > c.config.StaleTimeout {
		latency = c.config.StaleTimeout
	}

	return ClientHealth{
		Connected:      c.connected.Load(),
		LastSlot:       c.lastSlot.Load(),
		LastUpdate:     lastUpdate,
		Provider:       c.config.Endpoint,
		Latency:        latency,
		ReconnectCount: int(c.reconnectCount.Load()),
		LastError:      c.getLastError(),
	}
}

// Close closes the client and releases all resources.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return ErrClosed
	}

	// Cancel the context to stop all goroutines
	if c.cancelFunc != nil {
		c.cancelFunc()
	}

	// Wait for goroutines to finish
	c.wg.Wait()

	// Close the stream and connection
	if c.stream != nil {
		c.stream.CloseSend()
	}
	if c.conn != nil {
		c.conn.Close()
	}

	// Close channels
	close(c.blocks)
	close(c.slots)

	return nil
}

// setLastError safely sets the last error.
func (c *Client) setLastError(err error) {
	c.lastErrorMu.Lock()
	c.lastError = err
	c.lastErrorMu.Unlock()
}

// getLastError safely gets the last error.
func (c *Client) getLastError() error {
	c.lastErrorMu.RLock()
	defer c.lastErrorMu.RUnlock()
	return c.lastError
}

// tokenAuth implements grpc.PerRPCCredentials for token authentication.
type tokenAuth struct {
	token      string
	requireTLS bool
}

// GetRequestMetadata returns the authentication metadata.
func (t *tokenAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"x-token": t.token,
	}, nil
}

// RequireTransportSecurity returns whether TLS is required.
func (t *tokenAuth) RequireTransportSecurity() bool {
	return t.requireTLS
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

// minDuration returns the minimum of two durations.
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// encodeVarint encodes a uint64 as a protobuf varint.
func encodeVarint(v uint64) []byte {
	var buf [10]byte
	n := binary.PutUvarint(buf[:], v)
	return buf[:n]
}

// isRetryableError returns true if the error should trigger a retry.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for gRPC status codes
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Aborted, codes.Internal:
			return true
		}
	}

	// Check for specific errors
	return errors.Is(err, io.EOF) || errors.Is(err, ErrStreamClosed)
}

// Ensure types are used to prevent unused import errors
var (
	_ = encodeVarint
	_ = isRetryableError
)
