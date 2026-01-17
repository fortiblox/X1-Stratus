package executor

import (
	"testing"

	"github.com/fortiblox/X1-Stratus/internal/types"
	"github.com/fortiblox/X1-Stratus/pkg/accounts"
)

// TestAccountInfoMarkOriginal tests marking and detecting modifications.
func TestAccountInfoMarkOriginal(t *testing.T) {
	acc := &AccountInfo{
		Key:      types.Pubkey{1},
		Lamports: 1000,
		Data:     []byte{1, 2, 3},
	}

	acc.MarkOriginal()

	// Should not be modified initially
	if acc.IsModified() {
		t.Error("Account should not be modified after MarkOriginal")
	}

	// Modify lamports
	acc.Lamports = 2000
	if !acc.IsModified() {
		t.Error("Account should be modified after changing lamports")
	}

	// Reset and test data modification
	acc.Lamports = 1000
	acc.Data[0] = 10
	if !acc.IsModified() {
		t.Error("Account should be modified after changing data")
	}

	// Reset data and test length change
	acc.Data[0] = 1
	acc.Data = append(acc.Data, 4)
	if !acc.IsModified() {
		t.Error("Account should be modified after changing data length")
	}
}

// TestSerializeInput tests input serialization.
func TestSerializeInput(t *testing.T) {
	programID := types.Pubkey{1, 2, 3}
	accounts := []*AccountInfo{
		{
			Key:        types.Pubkey{10},
			Owner:      types.Pubkey{20},
			Lamports:   1000,
			Data:       []byte{1, 2, 3, 4},
			Executable: false,
			RentEpoch:  100,
			IsSigner:   true,
			IsWritable: true,
		},
	}
	data := []byte{0xAA, 0xBB}

	input, err := serializeInput(programID, accounts, data)
	if err != nil {
		t.Fatalf("serializeInput failed: %v", err)
	}

	// Check minimum size
	// 8 (num_accounts) + account_size + 8 (data_len) + 2 (data) + 32 (program_id)
	expectedMinSize := 8 + 8 + 2 + 32
	if len(input) < expectedMinSize {
		t.Errorf("Input size = %d, want at least %d", len(input), expectedMinSize)
	}

	// Verify program ID at end
	endProgramID := input[len(input)-32:]
	for i := 0; i < 32; i++ {
		if endProgramID[i] != programID[i] {
			t.Errorf("Program ID byte %d mismatch", i)
			break
		}
	}
}

// TestDeserializeOutput tests output deserialization.
func TestDeserializeOutput(t *testing.T) {
	programID := types.Pubkey{1}
	accounts := []*AccountInfo{
		{
			Key:        types.Pubkey{10},
			Owner:      types.Pubkey{20},
			Lamports:   1000,
			Data:       []byte{1, 2, 3, 4},
			Executable: false,
			RentEpoch:  100,
			IsSigner:   true,
			IsWritable: true,
		},
	}
	data := []byte{0xAA}

	// Serialize
	input, err := serializeInput(programID, accounts, data)
	if err != nil {
		t.Fatalf("serializeInput failed: %v", err)
	}

	// Modify the serialized lamports (for writable account)
	// The lamports field is after: 8 (num_accounts) + 1 + 1 + 1 + 1 + 4 + 32 + 32 = 80
	lamportsOffset := 8 + 1 + 1 + 1 + 1 + 4 + 32 + 32
	// Write new lamports value (2000 = 0x7D0)
	input[lamportsOffset] = 0xD0
	input[lamportsOffset+1] = 0x07

	// Deserialize
	err = deserializeOutput(input, accounts)
	if err != nil {
		t.Fatalf("deserializeOutput failed: %v", err)
	}

	// Check that lamports was updated
	if accounts[0].Lamports != 2000 {
		t.Errorf("Lamports = %d, want 2000", accounts[0].Lamports)
	}
}

// TestFindModifiedAccounts tests finding modified accounts.
func TestFindModifiedAccounts(t *testing.T) {
	accounts := []*AccountInfo{
		{
			Key:        types.Pubkey{1},
			Lamports:   1000,
			Data:       []byte{1},
			IsWritable: true,
		},
		{
			Key:        types.Pubkey{2},
			Lamports:   2000,
			Data:       []byte{2},
			IsWritable: true,
		},
		{
			Key:        types.Pubkey{3},
			Lamports:   3000,
			Data:       []byte{3},
			IsWritable: false, // Not writable
		},
	}

	// Mark original state
	for _, acc := range accounts {
		acc.MarkOriginal()
	}

	// Modify first and third accounts
	accounts[0].Lamports = 1500
	accounts[2].Lamports = 3500

	modified := findModifiedAccounts(accounts)

	// Only first account should be in the list (third is not writable)
	if len(modified) != 1 {
		t.Errorf("Modified count = %d, want 1", len(modified))
	}

	if modified[0] != accounts[0].Key {
		t.Error("Wrong modified account")
	}
}

// TestExecutionContext tests the execution context implementation.
func TestExecutionContext(t *testing.T) {
	db := accounts.NewMemoryDB()
	programID := types.Pubkey{1, 2, 3}
	accs := []*AccountInfo{}

	ctx := newExecutionContext(db, programID, accs)

	// Test logging
	ctx.Log("test message")
	if len(ctx.logs) != 1 || ctx.logs[0] != "test message" {
		t.Error("Log not recorded correctly")
	}

	// Test LogData
	ctx.LogData([][]byte{{1, 2}, {3, 4}})
	if len(ctx.logs) != 3 {
		t.Error("LogData not recorded correctly")
	}

	// Test return data
	var retID [32]byte
	copy(retID[:], programID[:])
	err := ctx.SetReturnData(retID, []byte{5, 6, 7})
	if err != nil {
		t.Errorf("SetReturnData failed: %v", err)
	}

	id, data := ctx.GetReturnData()
	if id != retID {
		t.Error("Return data program ID mismatch")
	}
	if len(data) != 3 || data[0] != 5 {
		t.Error("Return data content mismatch")
	}

	// Test compute metering
	ctx.cuLimit = 1000
	ctx.cuConsumed = 0

	err = ctx.ConsumeCU(100)
	if err != nil {
		t.Errorf("ConsumeCU failed: %v", err)
	}

	if ctx.RemainingCU() != 900 {
		t.Errorf("RemainingCU = %d, want 900", ctx.RemainingCU())
	}

	// Test exceeding limit
	err = ctx.ConsumeCU(1000)
	if err == nil {
		t.Error("ConsumeCU should fail when exceeding limit")
	}

	// Test GetProgramID
	gotID := ctx.GetProgramID()
	for i := 0; i < 32; i++ {
		if gotID[i] != programID[i] {
			t.Error("GetProgramID mismatch")
			break
		}
	}

	// Test stack height
	if ctx.GetStackHeight() != 1 {
		t.Errorf("GetStackHeight = %d, want 1", ctx.GetStackHeight())
	}
}

// TestBPFExecutorClearCache tests cache clearing.
func TestBPFExecutorClearCache(t *testing.T) {
	db := accounts.NewMemoryDB()
	executor := NewBPFExecutor(db)

	// Manually add to cache
	executor.programCache[types.Pubkey{1}] = nil

	if len(executor.programCache) != 1 {
		t.Fatal("Cache should have 1 entry")
	}

	executor.ClearCache()

	if len(executor.programCache) != 0 {
		t.Error("Cache should be empty after ClearCache")
	}
}

// TestAccountInfoFields tests AccountInfo field access.
func TestAccountInfoFields(t *testing.T) {
	acc := &AccountInfo{
		Key:        types.Pubkey{1, 2, 3},
		Owner:      types.Pubkey{4, 5, 6},
		Lamports:   12345,
		Data:       []byte{7, 8, 9},
		Executable: true,
		RentEpoch:  100,
		IsSigner:   true,
		IsWritable: false,
	}

	// Verify all fields are accessible
	if acc.Key[0] != 1 {
		t.Error("Key field incorrect")
	}
	if acc.Owner[0] != 4 {
		t.Error("Owner field incorrect")
	}
	if acc.Lamports != 12345 {
		t.Error("Lamports field incorrect")
	}
	if len(acc.Data) != 3 {
		t.Error("Data field incorrect")
	}
	if !acc.Executable {
		t.Error("Executable field incorrect")
	}
	if acc.RentEpoch != 100 {
		t.Error("RentEpoch field incorrect")
	}
	if !acc.IsSigner {
		t.Error("IsSigner field incorrect")
	}
	if acc.IsWritable {
		t.Error("IsWritable field incorrect")
	}
}

// TestExecutionResultFields tests ExecutionResult field access.
func TestExecutionResultFields(t *testing.T) {
	result := &ExecutionResult{
		Success:          true,
		ReturnValue:      42,
		Error:            "",
		ComputeUnitsUsed: 1000,
		Logs:             []string{"log1", "log2"},
		ReturnData:       []byte{1, 2, 3},
		ModifiedAccounts: []types.Pubkey{{1}, {2}},
	}

	if !result.Success {
		t.Error("Success field incorrect")
	}
	if result.ReturnValue != 42 {
		t.Error("ReturnValue field incorrect")
	}
	if result.ComputeUnitsUsed != 1000 {
		t.Error("ComputeUnitsUsed field incorrect")
	}
	if len(result.Logs) != 2 {
		t.Error("Logs field incorrect")
	}
	if len(result.ReturnData) != 3 {
		t.Error("ReturnData field incorrect")
	}
	if len(result.ModifiedAccounts) != 2 {
		t.Error("ModifiedAccounts field incorrect")
	}
}

// TestLoadProgramNotFound tests loading a non-existent program.
func TestLoadProgramNotFound(t *testing.T) {
	db := accounts.NewMemoryDB()
	executor := NewBPFExecutor(db)

	_, err := executor.loadProgram(types.Pubkey{99})
	if err != ErrProgramNotFound {
		t.Errorf("loadProgram error = %v, want ErrProgramNotFound", err)
	}
}

// TestLoadProgramNotExecutable tests loading a non-executable account.
func TestLoadProgramNotExecutable(t *testing.T) {
	db := accounts.NewMemoryDB()

	// Add a non-executable account
	programID := types.Pubkey{1}
	db.SetAccount(programID, &accounts.Account{
		Lamports:   1000,
		Data:       []byte{1, 2, 3},
		Executable: false, // Not executable
	})

	executor := NewBPFExecutor(db)

	_, err := executor.loadProgram(programID)
	if err != ErrProgramNotExecutable {
		t.Errorf("loadProgram error = %v, want ErrProgramNotExecutable", err)
	}
}
