package sbpf

import (
	"testing"
)

// TestComputeMeter tests the compute meter.
func TestComputeMeter(t *testing.T) {
	cm := NewComputeMeter(1000)

	// Check initial state
	if cm.Remaining() != 1000 {
		t.Errorf("Remaining() = %d, want 1000", cm.Remaining())
	}

	// Consume some units
	if err := cm.Consume(100); err != nil {
		t.Errorf("Consume(100) failed: %v", err)
	}

	if cm.Remaining() != 900 {
		t.Errorf("Remaining() = %d, want 900", cm.Remaining())
	}

	// Consume remaining
	if err := cm.Consume(900); err != nil {
		t.Errorf("Consume(900) failed: %v", err)
	}

	if cm.Remaining() != 0 {
		t.Errorf("Remaining() = %d, want 0", cm.Remaining())
	}

	// Should fail on next consume
	if err := cm.Consume(1); err != ErrComputeExceeded {
		t.Errorf("Consume(1) = %v, want ErrComputeExceeded", err)
	}
}

// TestStack tests the call stack.
func TestStack(t *testing.T) {
	stack := NewStack()

	// Initial state
	if stack.Depth() != 0 {
		t.Errorf("Depth() = %d, want 0", stack.Depth())
	}

	// Push a frame
	regs := make([]uint64, 11)
	regs[6] = 100
	regs[7] = 200
	regs[8] = 300
	regs[9] = 400
	regs[10] = VaddrStack + StackFrameSize - 1

	if err := stack.Push(regs, 42); err != nil {
		t.Errorf("Push() failed: %v", err)
	}

	if stack.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1", stack.Depth())
	}

	// Frame pointer should be updated
	expectedFP := VaddrStack + StackFrameSize - 1 + StackFrameSize + StackGap
	if regs[10] != expectedFP {
		t.Errorf("Frame pointer = 0x%x, want 0x%x", regs[10], expectedFP)
	}

	// Pop the frame
	retAddr, ok := stack.Pop(regs)
	if !ok {
		t.Error("Pop() failed")
	}

	if retAddr != 42 {
		t.Errorf("Return address = %d, want 42", retAddr)
	}

	if stack.Depth() != 0 {
		t.Errorf("Depth() = %d, want 0", stack.Depth())
	}

	// Registers should be restored
	if regs[6] != 100 || regs[7] != 200 || regs[8] != 300 || regs[9] != 400 {
		t.Error("Callee-saved registers not restored")
	}
}

// TestStackDepthLimit tests the stack depth limit.
func TestStackDepthLimit(t *testing.T) {
	stack := NewStack()
	regs := make([]uint64, 11)
	regs[10] = VaddrStack + StackFrameSize - 1

	// Push up to the limit
	for i := 0; i < StackDepth; i++ {
		if err := stack.Push(regs, int64(i)); err != nil {
			t.Fatalf("Push() failed at depth %d: %v", i, err)
		}
	}

	// Next push should fail
	if err := stack.Push(regs, 100); err != ErrCallDepthExceeded {
		t.Errorf("Push() = %v, want ErrCallDepthExceeded", err)
	}
}

// TestInterpreterALU64 tests 64-bit ALU operations.
func TestInterpreterALU64(t *testing.T) {
	tests := []struct {
		name     string
		program  []uint64
		expected uint64
	}{
		{
			name: "add immediate",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 10),  // r0 = 10
				Encode(OpAdd64Imm, 0, 0, 0, 5),   // r0 += 5
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 15,
		},
		{
			name: "sub immediate",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 10),  // r0 = 10
				Encode(OpSub64Imm, 0, 0, 0, 3),   // r0 -= 3
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 7,
		},
		{
			name: "mul immediate",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 6),   // r0 = 6
				Encode(OpMul64Imm, 0, 0, 0, 7),   // r0 *= 7
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 42,
		},
		{
			name: "div immediate",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 100), // r0 = 100
				Encode(OpDiv64Imm, 0, 0, 0, 10),  // r0 /= 10
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 10,
		},
		{
			name: "or immediate",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 0x0F), // r0 = 0x0F
				Encode(OpOr64Imm, 0, 0, 0, 0xF0),  // r0 |= 0xF0
				Encode(OpExit, 0, 0, 0, 0),        // exit
			},
			expected: 0xFF,
		},
		{
			name: "and immediate",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 0xFF), // r0 = 0xFF
				Encode(OpAnd64Imm, 0, 0, 0, 0x0F), // r0 &= 0x0F
				Encode(OpExit, 0, 0, 0, 0),        // exit
			},
			expected: 0x0F,
		},
		{
			name: "xor immediate",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 0xFF), // r0 = 0xFF
				Encode(OpXor64Imm, 0, 0, 0, 0xF0), // r0 ^= 0xF0
				Encode(OpExit, 0, 0, 0, 0),        // exit
			},
			expected: 0x0F,
		},
		{
			name: "lsh immediate",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 1),  // r0 = 1
				Encode(OpLsh64Imm, 0, 0, 0, 4),  // r0 <<= 4
				Encode(OpExit, 0, 0, 0, 0),      // exit
			},
			expected: 16,
		},
		{
			name: "rsh immediate",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 32), // r0 = 32
				Encode(OpRsh64Imm, 0, 0, 0, 3),  // r0 >>= 3
				Encode(OpExit, 0, 0, 0, 0),      // exit
			},
			expected: 4,
		},
		{
			name: "neg",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 5),  // r0 = 5
				Encode(OpNeg64, 0, 0, 0, 0),     // r0 = -r0
				Encode(OpExit, 0, 0, 0, 0),      // exit
			},
			expected: ^uint64(5) + 1, // Two's complement of 5
		},
		{
			name: "mod immediate",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 17), // r0 = 17
				Encode(OpMod64Imm, 0, 0, 0, 5),  // r0 %= 5
				Encode(OpExit, 0, 0, 0, 0),      // exit
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog := &Program{
				Text:  tt.program,
				Entry: 0,
			}

			ip := NewInterpreter(prog, nil, InterpreterOpts{
				MaxCU: 10000,
				Syscalls: func(hash uint32) (Syscall, bool) {
					return nil, false
				},
			})

			r0, err := ip.Run()
			if err != nil {
				t.Fatalf("Run() failed: %v", err)
			}

			if r0 != tt.expected {
				t.Errorf("r0 = %d (0x%x), want %d (0x%x)", r0, r0, tt.expected, tt.expected)
			}
		})
	}
}

// TestInterpreterALU64Reg tests 64-bit ALU operations with register source.
func TestInterpreterALU64Reg(t *testing.T) {
	tests := []struct {
		name     string
		program  []uint64
		expected uint64
	}{
		{
			name: "add register",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 10),  // r0 = 10
				Encode(OpMov64Imm, 1, 0, 0, 5),   // r1 = 5
				Encode(OpAdd64Reg, 0, 1, 0, 0),   // r0 += r1
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 15,
		},
		{
			name: "sub register",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 10),  // r0 = 10
				Encode(OpMov64Imm, 1, 0, 0, 3),   // r1 = 3
				Encode(OpSub64Reg, 0, 1, 0, 0),   // r0 -= r1
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 7,
		},
		{
			name: "mul register",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 6),   // r0 = 6
				Encode(OpMov64Imm, 1, 0, 0, 7),   // r1 = 7
				Encode(OpMul64Reg, 0, 1, 0, 0),   // r0 *= r1
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 42,
		},
		{
			name: "mov register",
			program: []uint64{
				Encode(OpMov64Imm, 1, 0, 0, 42),  // r1 = 42
				Encode(OpMov64Reg, 0, 1, 0, 0),   // r0 = r1
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog := &Program{
				Text:  tt.program,
				Entry: 0,
			}

			ip := NewInterpreter(prog, nil, InterpreterOpts{
				MaxCU: 10000,
				Syscalls: func(hash uint32) (Syscall, bool) {
					return nil, false
				},
			})

			r0, err := ip.Run()
			if err != nil {
				t.Fatalf("Run() failed: %v", err)
			}

			if r0 != tt.expected {
				t.Errorf("r0 = %d, want %d", r0, tt.expected)
			}
		})
	}
}

// TestInterpreterJumps tests conditional and unconditional jumps.
func TestInterpreterJumps(t *testing.T) {
	tests := []struct {
		name     string
		program  []uint64
		expected uint64
	}{
		{
			name: "unconditional jump",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 1),   // r0 = 1
				Encode(OpJa, 0, 0, 1, 0),         // jump +1
				Encode(OpMov64Imm, 0, 0, 0, 2),   // r0 = 2 (skipped)
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 1,
		},
		{
			name: "jump if equal (taken)",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 5),   // r0 = 5
				Encode(OpJeqImm, 0, 0, 1, 5),     // if r0 == 5, jump +1
				Encode(OpMov64Imm, 0, 0, 0, 0),   // r0 = 0 (skipped)
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 5,
		},
		{
			name: "jump if equal (not taken)",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 5),   // r0 = 5
				Encode(OpJeqImm, 0, 0, 1, 10),    // if r0 == 10, jump +1
				Encode(OpMov64Imm, 0, 0, 0, 0),   // r0 = 0 (executed)
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 0,
		},
		{
			name: "jump if not equal",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 5),   // r0 = 5
				Encode(OpJneImm, 0, 0, 1, 10),    // if r0 != 10, jump +1
				Encode(OpMov64Imm, 0, 0, 0, 0),   // r0 = 0 (skipped)
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 5,
		},
		{
			name: "jump if greater than",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 10),  // r0 = 10
				Encode(OpJgtImm, 0, 0, 1, 5),     // if r0 > 5, jump +1
				Encode(OpMov64Imm, 0, 0, 0, 0),   // r0 = 0 (skipped)
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 10,
		},
		{
			name: "loop",
			program: []uint64{
				Encode(OpMov64Imm, 0, 0, 0, 0),   // r0 = 0 (counter)
				Encode(OpMov64Imm, 1, 0, 0, 5),   // r1 = 5 (limit)
				Encode(OpAdd64Imm, 0, 0, 0, 1),   // r0 += 1
				Encode(OpJltReg, 0, 1, -2, 0),    // if r0 < r1, jump -2
				Encode(OpExit, 0, 0, 0, 0),       // exit
			},
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog := &Program{
				Text:  tt.program,
				Entry: 0,
			}

			ip := NewInterpreter(prog, nil, InterpreterOpts{
				MaxCU: 10000,
				Syscalls: func(hash uint32) (Syscall, bool) {
					return nil, false
				},
			})

			r0, err := ip.Run()
			if err != nil {
				t.Fatalf("Run() failed: %v", err)
			}

			if r0 != tt.expected {
				t.Errorf("r0 = %d, want %d", r0, tt.expected)
			}
		})
	}
}

// TestInterpreterDivisionByZero tests division by zero error.
func TestInterpreterDivisionByZero(t *testing.T) {
	program := []uint64{
		Encode(OpMov64Imm, 0, 0, 0, 10),  // r0 = 10
		Encode(OpDiv64Imm, 0, 0, 0, 0),   // r0 /= 0
		Encode(OpExit, 0, 0, 0, 0),       // exit
	}

	prog := &Program{
		Text:  program,
		Entry: 0,
	}

	ip := NewInterpreter(prog, nil, InterpreterOpts{
		MaxCU: 10000,
		Syscalls: func(hash uint32) (Syscall, bool) {
			return nil, false
		},
	})

	_, err := ip.Run()
	if err != ErrDivisionByZero {
		t.Errorf("Run() = %v, want ErrDivisionByZero", err)
	}
}

// TestInterpreterComputeExhaustion tests compute exhaustion.
func TestInterpreterComputeExhaustion(t *testing.T) {
	// Infinite loop
	program := []uint64{
		Encode(OpJa, 0, 0, -1, 0),  // jump to self
	}

	prog := &Program{
		Text:  program,
		Entry: 0,
	}

	ip := NewInterpreter(prog, nil, InterpreterOpts{
		MaxCU: 10, // Very low limit
		Syscalls: func(hash uint32) (Syscall, bool) {
			return nil, false
		},
	})

	_, err := ip.Run()
	if err != ErrComputeExceeded {
		t.Errorf("Run() = %v, want ErrComputeExceeded", err)
	}
}

// TestInterpreterInternalCall tests internal function calls using the function registry.
func TestInterpreterInternalCall(t *testing.T) {
	// Simple program that uses a registered function
	// Entry at 0, function at PC 4
	program := []uint64{
		// Main (entry at PC 0)
		Encode(OpMov64Imm, 0, 0, 0, 21),  // r0 = 21
		Encode(OpMov64Reg, 1, 0, 0, 0),   // r1 = r0 (argument)
		Encode(OpCall, 0, 0, 0, 0x1234),  // call function hash 0x1234
		Encode(OpExit, 0, 0, 0, 0),       // exit (return r0)

		// Function: double (at PC 4)
		Encode(OpAdd64Reg, 1, 1, 0, 0),   // r1 += r1 (double)
		Encode(OpMov64Reg, 0, 1, 0, 0),   // r0 = r1 (return value)
		Encode(OpExit, 0, 0, 0, 0),       // return
	}

	prog := &Program{
		Text:  program,
		Entry: 0,
		Functions: map[uint32]uint64{
			0x1234: 4, // Map hash 0x1234 to PC 4
		},
	}

	ip := NewInterpreter(prog, nil, InterpreterOpts{
		MaxCU: 10000,
		Syscalls: func(hash uint32) (Syscall, bool) {
			return nil, false
		},
	})

	r0, err := ip.Run()
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	if r0 != 42 {
		t.Errorf("r0 = %d, want 42", r0)
	}
}

// TestInstructionCost tests the instruction cost function.
func TestInstructionCost(t *testing.T) {
	tests := []struct {
		op       uint8
		expected uint64
	}{
		{OpAdd64Imm, CostALU},
		{OpMul64Imm, CostMul},
		{OpDiv64Imm, CostDiv},
		{OpMod64Imm, CostDiv},
		{OpLdxdw, CostLoad},
		{OpStxdw, CostStore},
		{OpLddw, CostLddw},
		{OpJa, CostJump},
		{OpCall, CostCall},
		{OpExit, CostExit},
	}

	for _, tt := range tests {
		cost := instructionCost(tt.op)
		if cost != tt.expected {
			t.Errorf("instructionCost(0x%02x) = %d, want %d", tt.op, cost, tt.expected)
		}
	}
}
