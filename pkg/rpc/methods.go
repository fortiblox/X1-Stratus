package rpc

import (
	"encoding/json"
	"errors"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
	"github.com/fortiblox/X1-Stratus/pkg/blockstore"
)

// Version information.
const (
	SolanaCore = "stratus-1.0.0"
	FeatureSet = 0
)

// Account Methods

// getAccountInfo retrieves account information.
func (s *Server) getAccountInfo(params json.RawMessage) (interface{}, *RPCError) {
	// Parse params: [pubkey, config?]
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing pubkey parameter")
	}

	var pubkeyStr string
	if err := json.Unmarshal(args[0], &pubkeyStr); err != nil {
		return nil, InvalidParamsError("invalid pubkey")
	}

	pubkey, err := types.PubkeyFromBase58(pubkeyStr)
	if err != nil {
		return nil, InvalidParamsError("invalid pubkey format")
	}

	// Parse optional config
	var config AccountInfoConfig
	if len(args) > 1 {
		if err := json.Unmarshal(args[1], &config); err != nil {
			return nil, InvalidParamsError("invalid config")
		}
	}

	// Set defaults
	if config.Encoding == "" {
		config.Encoding = EncodingBase64
	}

	// Check min context slot
	currentSlot := s.getAccountsSlot()
	if config.MinContextSlot != nil && *config.MinContextSlot > currentSlot {
		return nil, MinContextSlotError(*config.MinContextSlot, currentSlot)
	}

	// Get account
	account, err := s.accountsDB.GetAccount(pubkey)
	if err != nil {
		if errors.Is(err, accounts.ErrAccountNotFound) {
			return ResponseWithContext{
				Context: Context{Slot: currentSlot},
				Value:   nil,
			}, nil
		}
		return nil, InternalServerErrorf("failed to get account: %v", err)
	}

	// Prepare account info
	accountInfo, rpcErr := s.accountToAccountInfo(account, config.Encoding, config.DataSlice)
	if rpcErr != nil {
		return nil, rpcErr
	}

	return ResponseWithContext{
		Context: Context{Slot: currentSlot},
		Value:   accountInfo,
	}, nil
}

// getBalance retrieves account balance.
func (s *Server) getBalance(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing pubkey parameter")
	}

	var pubkeyStr string
	if err := json.Unmarshal(args[0], &pubkeyStr); err != nil {
		return nil, InvalidParamsError("invalid pubkey")
	}

	pubkey, err := types.PubkeyFromBase58(pubkeyStr)
	if err != nil {
		return nil, InvalidParamsError("invalid pubkey format")
	}

	// Parse optional config
	var config BalanceConfig
	if len(args) > 1 {
		if err := json.Unmarshal(args[1], &config); err != nil {
			return nil, InvalidParamsError("invalid config")
		}
	}

	currentSlot := s.getAccountsSlot()
	if config.MinContextSlot != nil && *config.MinContextSlot > currentSlot {
		return nil, MinContextSlotError(*config.MinContextSlot, currentSlot)
	}

	// Get account
	account, err := s.accountsDB.GetAccount(pubkey)
	if err != nil {
		if errors.Is(err, accounts.ErrAccountNotFound) {
			return ResponseWithContext{
				Context: Context{Slot: currentSlot},
				Value:   uint64(0),
			}, nil
		}
		return nil, InternalServerErrorf("failed to get account: %v", err)
	}

	return ResponseWithContext{
		Context: Context{Slot: currentSlot},
		Value:   account.Lamports,
	}, nil
}

// getMultipleAccounts retrieves multiple accounts.
func (s *Server) getMultipleAccounts(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing pubkeys parameter")
	}

	var pubkeyStrs []string
	if err := json.Unmarshal(args[0], &pubkeyStrs); err != nil {
		return nil, InvalidParamsError("invalid pubkeys array")
	}

	if len(pubkeyStrs) > 100 {
		return nil, InvalidParamsError("too many pubkeys (max 100)")
	}

	// Parse optional config
	var config MultipleAccountsConfig
	if len(args) > 1 {
		if err := json.Unmarshal(args[1], &config); err != nil {
			return nil, InvalidParamsError("invalid config")
		}
	}

	if config.Encoding == "" {
		config.Encoding = EncodingBase64
	}

	currentSlot := s.getAccountsSlot()
	if config.MinContextSlot != nil && *config.MinContextSlot > currentSlot {
		return nil, MinContextSlotError(*config.MinContextSlot, currentSlot)
	}

	// Get all accounts
	accountInfos := make([]*AccountInfo, len(pubkeyStrs))
	for i, pubkeyStr := range pubkeyStrs {
		pubkey, err := types.PubkeyFromBase58(pubkeyStr)
		if err != nil {
			return nil, InvalidParamsErrorf("invalid pubkey at index %d", i)
		}

		account, err := s.accountsDB.GetAccount(pubkey)
		if err != nil {
			if errors.Is(err, accounts.ErrAccountNotFound) {
				accountInfos[i] = nil
				continue
			}
			return nil, InternalServerErrorf("failed to get account: %v", err)
		}

		info, rpcErr := s.accountToAccountInfo(account, config.Encoding, config.DataSlice)
		if rpcErr != nil {
			return nil, rpcErr
		}
		accountInfos[i] = info
	}

	return ResponseWithContext{
		Context: Context{Slot: currentSlot},
		Value:   accountInfos,
	}, nil
}

// getProgramAccounts retrieves accounts owned by a program.
func (s *Server) getProgramAccounts(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing program ID parameter")
	}

	var programIDStr string
	if err := json.Unmarshal(args[0], &programIDStr); err != nil {
		return nil, InvalidParamsError("invalid program ID")
	}

	programID, err := types.PubkeyFromBase58(programIDStr)
	if err != nil {
		return nil, InvalidParamsError("invalid program ID format")
	}

	// Parse optional config
	var config ProgramAccountsConfig
	if len(args) > 1 {
		if err := json.Unmarshal(args[1], &config); err != nil {
			return nil, InvalidParamsError("invalid config")
		}
	}

	if config.Encoding == "" {
		config.Encoding = EncodingBase64
	}

	currentSlot := s.getAccountsSlot()
	if config.MinContextSlot != nil && *config.MinContextSlot > currentSlot {
		return nil, MinContextSlotError(*config.MinContextSlot, currentSlot)
	}

	// Get accounts owned by program - requires iteration
	// This is a potentially expensive operation
	var results []KeyedAccountInfo

	// Check if we have a BadgerDB for iteration
	if iterDB, ok := s.accountsDB.(*accounts.BadgerDB); ok {
		err := iterDB.IterateAccounts(func(pubkey types.Pubkey, account *accounts.Account) error {
			// Check owner
			if account.Owner != programID {
				return nil
			}

			// Apply filters
			if !s.matchesFilters(account, config.Filters) {
				return nil
			}

			info, _ := s.accountToAccountInfo(account, config.Encoding, config.DataSlice)
			if info != nil {
				results = append(results, KeyedAccountInfo{
					Pubkey:  pubkey.String(),
					Account: info,
				})
			}
			return nil
		})
		if err != nil {
			return nil, InternalServerErrorf("iteration failed: %v", err)
		}
	} else {
		// Fallback for non-iterable databases (like MemoryDB in tests)
		// Return empty result as we can't iterate efficiently
		results = []KeyedAccountInfo{}
	}

	if config.WithContext {
		return ResponseWithContext{
			Context: Context{Slot: currentSlot},
			Value:   results,
		}, nil
	}

	return results, nil
}

// Block Methods

// getBlock retrieves a block by slot.
func (s *Server) getBlock(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing slot parameter")
	}

	var slot uint64
	if err := json.Unmarshal(args[0], &slot); err != nil {
		return nil, InvalidParamsError("invalid slot")
	}

	// Parse optional config
	var config BlockConfig
	if len(args) > 1 {
		if err := json.Unmarshal(args[1], &config); err != nil {
			return nil, InvalidParamsError("invalid config")
		}
	}

	if config.Encoding == "" {
		config.Encoding = EncodingBase64
	}

	// Get block
	block, err := s.blockstore.GetBlock(slot)
	if err != nil {
		if errors.Is(err, blockstore.ErrBlockNotFound) {
			// Check if slot is beyond latest
			latestSlot := s.blockstore.GetLatestSlot()
			if slot > latestSlot {
				return nil, nil // Return null for future slots
			}
			return nil, SlotSkippedError(slot)
		}
		return nil, InternalServerErrorf("failed to get block: %v", err)
	}

	return s.blockToBlockResponse(block, config), nil
}

// getBlockHeight returns the current block height.
func (s *Server) getBlockHeight(params json.RawMessage) (interface{}, *RPCError) {
	// Get latest block
	latestSlot := s.blockstore.GetLatestSlot()
	if latestSlot == 0 {
		return uint64(0), nil
	}

	block, err := s.blockstore.GetBlock(latestSlot)
	if err != nil {
		// Return slot as approximation if block not found
		return latestSlot, nil
	}

	if block.BlockHeight != nil {
		return *block.BlockHeight, nil
	}

	return latestSlot, nil
}

// getBlockTime returns the block time for a slot.
func (s *Server) getBlockTime(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing slot parameter")
	}

	var slot uint64
	if err := json.Unmarshal(args[0], &slot); err != nil {
		return nil, InvalidParamsError("invalid slot")
	}

	block, err := s.blockstore.GetBlock(slot)
	if err != nil {
		if errors.Is(err, blockstore.ErrBlockNotFound) {
			return nil, nil // Return null for missing blocks
		}
		return nil, InternalServerErrorf("failed to get block: %v", err)
	}

	return block.BlockTime, nil
}

// getBlocks returns a list of confirmed blocks.
func (s *Server) getBlocks(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing start slot parameter")
	}

	var startSlot uint64
	if err := json.Unmarshal(args[0], &startSlot); err != nil {
		return nil, InvalidParamsError("invalid start slot")
	}

	// End slot is optional, defaults to latest
	endSlot := s.blockstore.GetLatestSlot()
	if len(args) > 1 {
		var end uint64
		if err := json.Unmarshal(args[1], &end); err == nil {
			endSlot = end
		}
	}

	// Limit range to 500000 slots
	const maxRange = 500000
	if endSlot-startSlot > maxRange {
		endSlot = startSlot + maxRange
	}

	// Get confirmed blocks in range
	var slots []uint64
	for slot := startSlot; slot <= endSlot; slot++ {
		if s.blockstore.HasBlock(slot) {
			slots = append(slots, slot)
		}
	}

	return slots, nil
}

// getBlocksWithLimit returns a list of confirmed blocks with a limit.
func (s *Server) getBlocksWithLimit(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 2 {
		return nil, InvalidParamsError("missing parameters")
	}

	var startSlot uint64
	if err := json.Unmarshal(args[0], &startSlot); err != nil {
		return nil, InvalidParamsError("invalid start slot")
	}

	var limit uint64
	if err := json.Unmarshal(args[1], &limit); err != nil {
		return nil, InvalidParamsError("invalid limit")
	}

	// Cap limit
	const maxLimit = 500000
	if limit > maxLimit {
		limit = maxLimit
	}

	// Get blocks
	var slots []uint64
	latestSlot := s.blockstore.GetLatestSlot()
	for slot := startSlot; slot <= latestSlot && uint64(len(slots)) < limit; slot++ {
		if s.blockstore.HasBlock(slot) {
			slots = append(slots, slot)
		}
	}

	return slots, nil
}

// Transaction Methods

// getTransaction retrieves a transaction by signature.
func (s *Server) getTransaction(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing signature parameter")
	}

	var sigStr string
	if err := json.Unmarshal(args[0], &sigStr); err != nil {
		return nil, InvalidParamsError("invalid signature")
	}

	sig, err := types.SignatureFromBase58(sigStr)
	if err != nil {
		return nil, InvalidParamsError("invalid signature format")
	}

	// Parse optional config
	var config TransactionConfig
	if len(args) > 1 {
		if err := json.Unmarshal(args[1], &config); err != nil {
			return nil, InvalidParamsError("invalid config")
		}
	}

	if config.Encoding == "" {
		config.Encoding = EncodingBase64
	}

	// Get transaction
	tx, err := s.blockstore.GetTransaction(sig)
	if err != nil {
		if errors.Is(err, blockstore.ErrTransactionNotFound) {
			return nil, nil // Return null for not found
		}
		return nil, InternalServerErrorf("failed to get transaction: %v", err)
	}

	// Get block for block time
	var blockTime *int64
	block, err := s.blockstore.GetBlock(tx.Slot)
	if err == nil {
		blockTime = block.BlockTime
	}

	return s.transactionToResponse(tx, blockTime, config), nil
}

// getSignaturesForAddress retrieves signatures for transactions involving an address.
func (s *Server) getSignaturesForAddress(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing address parameter")
	}

	var addrStr string
	if err := json.Unmarshal(args[0], &addrStr); err != nil {
		return nil, InvalidParamsError("invalid address")
	}

	addr, err := types.PubkeyFromBase58(addrStr)
	if err != nil {
		return nil, InvalidParamsError("invalid address format")
	}

	// Parse optional config
	var config SignaturesForAddressConfig
	if len(args) > 1 {
		if err := json.Unmarshal(args[1], &config); err != nil {
			return nil, InvalidParamsError("invalid config")
		}
	}

	// Set defaults
	if config.Limit == 0 {
		config.Limit = 1000
	}
	if config.Limit > 1000 {
		config.Limit = 1000
	}

	// Build query options
	opts := &blockstore.SignatureQueryOptions{
		Limit: config.Limit,
	}

	if config.Before != "" {
		sig, err := types.SignatureFromBase58(config.Before)
		if err == nil {
			opts.Before = &sig
		}
	}

	// Get signatures
	signatures, err := s.blockstore.GetSignaturesForAddress(addr, opts)
	if err != nil {
		return nil, InternalServerErrorf("failed to get signatures: %v", err)
	}

	// Convert to response format
	results := make([]SignatureInfo, len(signatures))
	for i, sig := range signatures {
		results[i] = SignatureInfo{
			Signature:          sig.Signature.String(),
			Slot:               sig.Slot,
			BlockTime:          sig.BlockTime,
			Memo:               sig.Memo,
			ConfirmationStatus: "finalized",
		}
		if sig.Err != nil {
			results[i].Err = map[string]interface{}{
				"code":    sig.Err.Code,
				"message": sig.Err.Message,
			}
		}
	}

	return results, nil
}

// getSignatureStatuses retrieves the status of signatures.
func (s *Server) getSignatureStatuses(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing signatures parameter")
	}

	var sigStrs []string
	if err := json.Unmarshal(args[0], &sigStrs); err != nil {
		return nil, InvalidParamsError("invalid signatures array")
	}

	if len(sigStrs) > 256 {
		return nil, InvalidParamsError("too many signatures (max 256)")
	}

	currentSlot := s.blockstore.GetLatestSlot()

	// Get status for each signature
	statuses := make([]*SignatureStatus, len(sigStrs))
	for i, sigStr := range sigStrs {
		sig, err := types.SignatureFromBase58(sigStr)
		if err != nil {
			statuses[i] = nil
			continue
		}

		status, err := s.blockstore.GetTransactionStatus(sig)
		if err != nil {
			statuses[i] = nil
			continue
		}

		statuses[i] = &SignatureStatus{
			Slot:               status.Slot,
			Confirmations:      status.Confirmations,
			ConfirmationStatus: status.ConfirmationStatus.String(),
		}
		if status.Err != nil {
			statuses[i].Err = map[string]interface{}{
				"code":    status.Err.Code,
				"message": status.Err.Message,
			}
		}
	}

	return ResponseWithContext{
		Context: Context{Slot: currentSlot},
		Value:   statuses,
	}, nil
}

// Cluster Methods

// getSlot returns the current slot.
func (s *Server) getSlot(params json.RawMessage) (interface{}, *RPCError) {
	var config SlotConfig
	if len(params) > 0 {
		var args []json.RawMessage
		if err := json.Unmarshal(params, &args); err == nil && len(args) > 0 {
			json.Unmarshal(args[0], &config)
		}
	}

	currentSlot := s.getAccountsSlot()
	if config.MinContextSlot != nil && *config.MinContextSlot > currentSlot {
		return nil, MinContextSlotError(*config.MinContextSlot, currentSlot)
	}

	// Return slot based on commitment
	switch config.Commitment {
	case CommitmentFinalized:
		return s.blockstore.GetFinalizedSlot(), nil
	case CommitmentConfirmed:
		return s.blockstore.GetConfirmedSlot(), nil
	default:
		return s.blockstore.GetLatestSlot(), nil
	}
}

// getSlotLeader returns the current slot leader (not applicable for verifier).
func (s *Server) getSlotLeader(params json.RawMessage) (interface{}, *RPCError) {
	// Return a placeholder as we don't track leaders
	return types.SystemProgramAddr.String(), nil
}

// getHealth returns the node health status.
func (s *Server) getHealth(params json.RawMessage) (interface{}, *RPCError) {
	if !s.healthy {
		return nil, ErrNodeUnhealthy
	}
	return "ok", nil
}

// getVersion returns the node version.
func (s *Server) getVersion(params json.RawMessage) (interface{}, *RPCError) {
	return VersionInfo{
		SolanaCore: SolanaCore,
		FeatureSet: FeatureSet,
	}, nil
}

// getGenesisHash returns the genesis hash.
func (s *Server) getGenesisHash(params json.RawMessage) (interface{}, *RPCError) {
	return s.genesisHash.String(), nil
}

// getIdentity returns the node identity.
func (s *Server) getIdentity(params json.RawMessage) (interface{}, *RPCError) {
	return Identity{
		Identity: s.identity.String(),
	}, nil
}

// getEpochInfo returns epoch information.
func (s *Server) getEpochInfo(params json.RawMessage) (interface{}, *RPCError) {
	slot := s.blockstore.GetLatestSlot()

	// Approximate epoch info (X1 uses 432000 slots per epoch like Solana)
	const slotsPerEpoch = 432000
	epoch := slot / slotsPerEpoch
	slotIndex := slot % slotsPerEpoch

	var blockHeight uint64
	block, err := s.blockstore.GetBlock(slot)
	if err == nil && block.BlockHeight != nil {
		blockHeight = *block.BlockHeight
	}

	return EpochInfo{
		AbsoluteSlot: slot,
		BlockHeight:  blockHeight,
		Epoch:        epoch,
		SlotIndex:    slotIndex,
		SlotsInEpoch: slotsPerEpoch,
	}, nil
}

// getEpochSchedule returns the epoch schedule.
func (s *Server) getEpochSchedule(params json.RawMessage) (interface{}, *RPCError) {
	return EpochSchedule{
		SlotsPerEpoch:            432000,
		LeaderScheduleSlotOffset: 432000,
		Warmup:                   false,
		FirstNormalEpoch:         0,
		FirstNormalSlot:          0,
	}, nil
}

// getLatestBlockhash returns the latest blockhash.
func (s *Server) getLatestBlockhash(params json.RawMessage) (interface{}, *RPCError) {
	latestSlot := s.blockstore.GetLatestSlot()
	block, err := s.blockstore.GetBlock(latestSlot)
	if err != nil {
		return nil, InternalServerErrorf("failed to get latest block: %v", err)
	}

	// Last valid block height is approximately 150 blocks ahead
	var lastValidBlockHeight uint64
	if block.BlockHeight != nil {
		lastValidBlockHeight = *block.BlockHeight + 150
	} else {
		lastValidBlockHeight = latestSlot + 150
	}

	return ResponseWithContext{
		Context: Context{Slot: latestSlot},
		Value: LatestBlockhash{
			Blockhash:            block.Blockhash.String(),
			LastValidBlockHeight: lastValidBlockHeight,
		},
	}, nil
}

// getMinimumBalanceForRentExemption returns the minimum balance for rent exemption.
func (s *Server) getMinimumBalanceForRentExemption(params json.RawMessage) (interface{}, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, InvalidParamsError("invalid params")
	}

	if len(args) < 1 {
		return nil, InvalidParamsError("missing data length parameter")
	}

	var dataLen uint64
	if err := json.Unmarshal(args[0], &dataLen); err != nil {
		return nil, InvalidParamsError("invalid data length")
	}

	// Rent exemption formula: (128 + dataLen) * 2 years of rent
	// 1 year = 365.25 * 24 * 60 * 60 / 0.4 seconds per slot = 78892314 slots
	// Rent per byte per epoch: 3480 lamports
	// Simplified: (128 + dataLen) * 6960
	const lamportsPerByteYear = 3480
	rentExempt := (128 + dataLen) * lamportsPerByteYear * 2

	return rentExempt, nil
}

// Helper methods

// getAccountsSlot returns the current slot from accounts DB.
func (s *Server) getAccountsSlot() uint64 {
	return s.accountsDB.GetSlot()
}

// accountToAccountInfo converts an internal account to RPC AccountInfo.
func (s *Server) accountToAccountInfo(account *accounts.Account, encoding Encoding, dataSlice *DataSlice) (*AccountInfo, *RPCError) {
	// Apply data slice if specified
	data := account.Data
	if dataSlice != nil {
		data = ApplyDataSlice(data, dataSlice)
	}

	// Encode data
	encodedData, err := EncodeAccountData(data, encoding)
	if err != nil {
		return nil, InternalServerErrorf("failed to encode data: %v", err)
	}

	return &AccountInfo{
		Data:       encodedData,
		Executable: account.Executable,
		Lamports:   account.Lamports,
		Owner:      account.Owner.String(),
		RentEpoch:  account.RentEpoch,
		Space:      uint64(len(account.Data)),
	}, nil
}

// matchesFilters checks if an account matches the given filters.
func (s *Server) matchesFilters(account *accounts.Account, filters []ProgramAccountFilter) bool {
	for _, filter := range filters {
		if filter.DataSize != nil {
			if uint64(len(account.Data)) != *filter.DataSize {
				return false
			}
		}

		if filter.Memcmp != nil {
			offset := filter.Memcmp.Offset
			if offset >= uint64(len(account.Data)) {
				return false
			}

			// Decode the comparison bytes
			var cmpBytes []byte
			var err error

			switch filter.Memcmp.Encoding {
			case EncodingBase58:
				cmpBytes, err = DecodeBase58(filter.Memcmp.Bytes)
			default:
				cmpBytes, err = DecodeBase64(filter.Memcmp.Bytes)
			}

			if err != nil {
				return false
			}

			// Check if comparison would exceed data length
			if offset+uint64(len(cmpBytes)) > uint64(len(account.Data)) {
				return false
			}

			// Compare bytes
			for i, b := range cmpBytes {
				if account.Data[offset+uint64(i)] != b {
					return false
				}
			}
		}
	}

	return true
}

// blockToBlockResponse converts an internal block to RPC BlockResponse.
func (s *Server) blockToBlockResponse(block *blockstore.Block, config BlockConfig) *BlockResponse {
	resp := &BlockResponse{
		Blockhash:         block.Blockhash.String(),
		PreviousBlockhash: block.PreviousBlockhash.String(),
		ParentSlot:        block.ParentSlot,
		BlockTime:         block.BlockTime,
		BlockHeight:       block.BlockHeight,
	}

	// Include rewards if not disabled
	includeRewards := config.Rewards == nil || *config.Rewards
	if includeRewards && len(block.Rewards) > 0 {
		resp.Rewards = make([]RewardInfo, len(block.Rewards))
		for i, r := range block.Rewards {
			resp.Rewards[i] = RewardInfo{
				Pubkey:      r.Pubkey.String(),
				Lamports:    r.Lamports,
				PostBalance: r.PostBalance,
				RewardType:  rewardTypeToString(r.RewardType),
				Commission:  r.Commission,
			}
		}
	}

	// Handle transaction details
	switch config.TransactionDetails {
	case "none":
		// Don't include transactions
	case "signatures":
		resp.Signatures = make([]string, len(block.Transactions))
		for i, tx := range block.Transactions {
			resp.Signatures[i] = tx.Signature.String()
		}
	default: // "full" or empty
		resp.Transactions = make([]TransactionWithMeta, len(block.Transactions))
		for i, tx := range block.Transactions {
			resp.Transactions[i] = s.transactionWithMeta(&tx, config.Encoding)
		}
	}

	return resp
}

// transactionWithMeta converts a transaction to TransactionWithMeta.
func (s *Server) transactionWithMeta(tx *blockstore.Transaction, encoding Encoding) TransactionWithMeta {
	result := TransactionWithMeta{
		Version: "legacy", // Default to legacy
	}

	// Check for versioned transaction
	if len(tx.Message.AddressTableLookups) > 0 {
		result.Version = 0
	}

	// Encode transaction based on encoding
	switch encoding {
	case EncodingJSONParsed:
		result.Transaction = s.parsedTransaction(tx)
	default:
		// Return as base64 encoded message data
		result.Transaction = []string{
			EncodeBase64(serializeTransaction(tx)),
			string(EncodingBase64),
		}
	}

	// Add metadata if present
	if tx.Meta != nil {
		meta := &TransactionMeta{
			Fee:          tx.Meta.Fee,
			PreBalances:  tx.Meta.PreBalances,
			PostBalances: tx.Meta.PostBalances,
			LogMessages:  tx.Meta.LogMessages,
		}

		if tx.Meta.ComputeUnitsConsumed > 0 {
			consumed := tx.Meta.ComputeUnitsConsumed
			meta.ComputeUnitsConsumed = &consumed
		}

		if tx.Meta.Err != nil {
			meta.Err = map[string]interface{}{
				"InstructionError": []interface{}{
					tx.Meta.Err.Code,
					tx.Meta.Err.Message,
				},
			}
		}

		// Convert inner instructions
		if len(tx.Meta.InnerInstructions) > 0 {
			meta.InnerInstructions = make([]InnerInstructionSet, len(tx.Meta.InnerInstructions))
			for i, ii := range tx.Meta.InnerInstructions {
				meta.InnerInstructions[i] = InnerInstructionSet{
					Index: ii.Index,
				}
				meta.InnerInstructions[i].Instructions = make([]InnerInstruction, len(ii.Instructions))
				for j, ix := range ii.Instructions {
					meta.InnerInstructions[i].Instructions[j] = InnerInstruction{
						ProgramIDIndex: ix.ProgramIDIndex,
						Accounts:       ix.AccountIndexes,
						Data:           EncodeBase58(ix.Data),
					}
				}
			}
		}

		// Convert token balances
		if len(tx.Meta.PreTokenBalances) > 0 {
			meta.PreTokenBalances = make([]TokenBalance, len(tx.Meta.PreTokenBalances))
			for i, tb := range tx.Meta.PreTokenBalances {
				meta.PreTokenBalances[i] = tokenBalanceToRPC(&tb)
			}
		}
		if len(tx.Meta.PostTokenBalances) > 0 {
			meta.PostTokenBalances = make([]TokenBalance, len(tx.Meta.PostTokenBalances))
			for i, tb := range tx.Meta.PostTokenBalances {
				meta.PostTokenBalances[i] = tokenBalanceToRPC(&tb)
			}
		}

		// Convert loaded addresses
		if tx.Meta.LoadedAddresses != nil {
			meta.LoadedAddresses = &LoadedAddresses{
				Writable: pubkeysToStrings(tx.Meta.LoadedAddresses.Writable),
				Readonly: pubkeysToStrings(tx.Meta.LoadedAddresses.Readonly),
			}
		}

		result.Meta = meta
	}

	return result
}

// parsedTransaction returns a parsed transaction representation.
func (s *Server) parsedTransaction(tx *blockstore.Transaction) *ParsedTransaction {
	parsed := &ParsedTransaction{
		Signatures: make([]string, len(tx.Signatures)),
	}

	for i, sig := range tx.Signatures {
		parsed.Signatures[i] = sig.String()
	}

	parsed.Message = ParsedMessage{
		RecentBlockhash: tx.Message.RecentBlockhash.String(),
	}

	// Add account keys with metadata
	parsed.Message.AccountKeys = make([]ParsedAccountKey, len(tx.Message.AccountKeys))
	for i, key := range tx.Message.AccountKeys {
		parsed.Message.AccountKeys[i] = ParsedAccountKey{
			Pubkey:   key.String(),
			Signer:   i < int(tx.Message.Header.NumRequiredSignatures),
			Writable: isWritable(i, &tx.Message),
			Source:   "transaction",
		}
	}

	// Add instructions
	parsed.Message.Instructions = make([]ParsedInstruction, len(tx.Message.Instructions))
	for i, ix := range tx.Message.Instructions {
		programID := tx.Message.AccountKeys[ix.ProgramIDIndex].String()
		parsed.Message.Instructions[i] = ParsedInstruction{
			ProgramID: programID,
			Accounts:  indexesToPubkeys(ix.AccountIndexes, tx.Message.AccountKeys),
			Data:      EncodeBase58(ix.Data),
		}
	}

	// Add address table lookups
	if len(tx.Message.AddressTableLookups) > 0 {
		parsed.Message.AddressTableLookups = make([]AddressTableLookup, len(tx.Message.AddressTableLookups))
		for i, atl := range tx.Message.AddressTableLookups {
			parsed.Message.AddressTableLookups[i] = AddressTableLookup{
				AccountKey:      atl.AccountKey.String(),
				WritableIndexes: atl.WritableIndexes,
				ReadonlyIndexes: atl.ReadonlyIndexes,
			}
		}
	}

	return parsed
}

// transactionToResponse converts a transaction to TransactionResponse.
func (s *Server) transactionToResponse(tx *blockstore.Transaction, blockTime *int64, config TransactionConfig) *TransactionResponse {
	resp := &TransactionResponse{
		Slot:      tx.Slot,
		BlockTime: blockTime,
		Version:   "legacy",
	}

	// Check for versioned transaction
	if len(tx.Message.AddressTableLookups) > 0 {
		resp.Version = 0
	}

	// Encode transaction
	switch config.Encoding {
	case EncodingJSONParsed:
		resp.Transaction = s.parsedTransaction(tx)
	default:
		resp.Transaction = []string{
			EncodeBase64(serializeTransaction(tx)),
			string(EncodingBase64),
		}
	}

	// Add metadata
	if tx.Meta != nil {
		meta := &TransactionMeta{
			Fee:          tx.Meta.Fee,
			PreBalances:  tx.Meta.PreBalances,
			PostBalances: tx.Meta.PostBalances,
			LogMessages:  tx.Meta.LogMessages,
		}

		if tx.Meta.ComputeUnitsConsumed > 0 {
			consumed := tx.Meta.ComputeUnitsConsumed
			meta.ComputeUnitsConsumed = &consumed
		}

		if tx.Meta.Err != nil {
			meta.Err = map[string]interface{}{
				"InstructionError": []interface{}{
					tx.Meta.Err.Code,
					tx.Meta.Err.Message,
				},
			}
		}

		resp.Meta = meta
	}

	return resp
}

// Helper functions

func rewardTypeToString(rt blockstore.RewardType) string {
	switch rt {
	case blockstore.RewardTypeFee:
		return "Fee"
	case blockstore.RewardTypeRent:
		return "Rent"
	case blockstore.RewardTypeStaking:
		return "Staking"
	case blockstore.RewardTypeVoting:
		return "Voting"
	default:
		return ""
	}
}

func tokenBalanceToRPC(tb *blockstore.TokenBalance) TokenBalance {
	return TokenBalance{
		AccountIndex: tb.AccountIndex,
		Mint:         tb.Mint.String(),
		Owner:        tb.Owner.String(),
		ProgramID:    tb.ProgramID.String(),
		UITokenAmount: UITokenAmount{
			Amount:         tb.UITokenAmount.Amount,
			Decimals:       tb.UITokenAmount.Decimals,
			UIAmount:       tb.UITokenAmount.UIAmount,
			UIAmountString: tb.UITokenAmount.UIAmountString,
		},
	}
}

func pubkeysToStrings(pubkeys []types.Pubkey) []string {
	result := make([]string, len(pubkeys))
	for i, pk := range pubkeys {
		result[i] = pk.String()
	}
	return result
}

func indexesToPubkeys(indexes []uint8, keys []types.Pubkey) []string {
	result := make([]string, len(indexes))
	for i, idx := range indexes {
		if int(idx) < len(keys) {
			result[i] = keys[idx].String()
		}
	}
	return result
}

func isWritable(index int, msg *blockstore.TransactionMessage) bool {
	header := &msg.Header
	numRequiredSignatures := int(header.NumRequiredSignatures)
	numReadonlySignedAccounts := int(header.NumReadonlySignedAccounts)
	numReadonlyUnsignedAccounts := int(header.NumReadonlyUnsignedAccounts)
	numAccounts := len(msg.AccountKeys)

	// Signers that are not readonly
	if index < numRequiredSignatures-numReadonlySignedAccounts {
		return true
	}
	// Non-signers that are not readonly
	if index >= numRequiredSignatures && index < numAccounts-numReadonlyUnsignedAccounts {
		return true
	}
	return false
}

// serializeTransaction creates a simplified serialization of the transaction.
// In a production implementation, this would create proper bincode serialization.
func serializeTransaction(tx *blockstore.Transaction) []byte {
	// For now, return a placeholder - proper implementation would serialize
	// the transaction in Solana's wire format
	// This is sufficient for base64 encoding but won't be deserializable
	data := make([]byte, 0, 256)

	// Signatures count (compact-u16)
	data = append(data, byte(len(tx.Signatures)))
	for _, sig := range tx.Signatures {
		data = append(data, sig[:]...)
	}

	// Message header
	data = append(data, tx.Message.Header.NumRequiredSignatures)
	data = append(data, tx.Message.Header.NumReadonlySignedAccounts)
	data = append(data, tx.Message.Header.NumReadonlyUnsignedAccounts)

	// Account keys count and keys
	data = append(data, byte(len(tx.Message.AccountKeys)))
	for _, key := range tx.Message.AccountKeys {
		data = append(data, key[:]...)
	}

	// Recent blockhash
	data = append(data, tx.Message.RecentBlockhash[:]...)

	// Instructions count
	data = append(data, byte(len(tx.Message.Instructions)))
	for _, ix := range tx.Message.Instructions {
		data = append(data, ix.ProgramIDIndex)
		data = append(data, byte(len(ix.AccountIndexes)))
		data = append(data, ix.AccountIndexes...)
		// Data length as compact-u16 (simplified)
		if len(ix.Data) < 128 {
			data = append(data, byte(len(ix.Data)))
		} else {
			data = append(data, byte(len(ix.Data)&0x7f|0x80), byte(len(ix.Data)>>7))
		}
		data = append(data, ix.Data...)
	}

	return data
}
