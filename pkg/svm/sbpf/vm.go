// Package sbpf implements the Solana Berkeley Packet Filter virtual machine.
//
// sBPF is a register-based virtual machine with 11 64-bit registers (R0-R10),
// where R10 is a read-only frame pointer. The instruction set is based on eBPF
// with Solana-specific extensions.
//
// Memory is organized into four regions:
// - Program (0x100000000): Read-only executable code
// - Stack   (0x200000000): Read-write stack frames
// - Heap    (0x300000000): Read-write heap memory
// - Input   (0x400000000): Read-only input parameters
package sbpf

import (
	"errors"
	"fmt"
)

// Virtual memory region base addresses.
const (
	VaddrProgram = uint64(0x1_0000_0000) // Read-only program code
	VaddrStack   = uint64(0x2_0000_0000) // Stack memory
	VaddrHeap    = uint64(0x3_0000_0000) // Heap memory
	VaddrInput   = uint64(0x4_0000_0000) // Input parameters
)

// Stack and heap constants.
const (
	StackFrameSize = 4096     // 4 KB per frame
	StackDepth     = 64       // Max call depth
	StackGap       = 4096     // Gap between frames
	HeapDefault    = 32768    // 32 KB default heap
	HeapMax        = 262144   // 256 KB max heap
)

// Errors.
var (
	ErrComputeExceeded     = errors.New("compute budget exceeded")
	ErrInvalidMemoryAccess = errors.New("invalid memory access")
	ErrInvalidInstruction  = errors.New("invalid instruction")
	ErrCallDepthExceeded   = errors.New("call depth exceeded")
	ErrDivisionByZero      = errors.New("division by zero")
	ErrUnknownSyscall      = errors.New("unknown syscall")
	ErrExitCalled          = errors.New("exit called")
)

// sBPF instruction costs.
const (
	CostALU   = uint64(1)  // Simple ALU operations
	CostMul   = uint64(4)  // Multiplication
	CostDiv   = uint64(12) // Division/modulo
	CostLoad  = uint64(2)  // Memory load
	CostStore = uint64(2)  // Memory store
	CostLddw  = uint64(2)  // 64-bit immediate load
	CostJump  = uint64(1)  // Jump instructions
	CostCall  = uint64(5)  // Function calls
	CostExit  = uint64(1)  // Exit/return
)

// instructionCost returns the compute cost for an opcode.
func instructionCost(op uint8) uint64 {
	class := op & 0x07
	aluOp := op & 0xF0

	switch class {
	case ClassAlu, ClassAlu64:
		switch aluOp {
		case AluMul:
			return CostMul
		case AluDiv, AluMod:
			return CostDiv
		default:
			return CostALU
		}

	case ClassLd, ClassLdx:
		if op == OpLddw {
			return CostLddw
		}
		return CostLoad

	case ClassSt, ClassStx:
		return CostStore

	case ClassJmp, ClassJmp32:
		jmpOp := op & 0xF0
		switch jmpOp {
		case JmpCall:
			return CostCall
		case JmpExit:
			return CostExit
		default:
			return CostJump
		}

	default:
		return CostALU
	}
}

// VM is the sBPF virtual machine interface.
type VM interface {
	// VMContext returns the execution context.
	VMContext() interface{}

	// Memory access
	Read(addr uint64, p []byte) error
	Read8(addr uint64) (uint8, error)
	Read16(addr uint64) (uint16, error)
	Read32(addr uint64) (uint32, error)
	Read64(addr uint64) (uint64, error)

	Write(addr uint64, p []byte) error
	Write8(addr uint64, x uint8) error
	Write16(addr uint64, x uint16) error
	Write32(addr uint64, x uint64) error
	Write64(addr uint64, x uint64) error

	// Memory translation
	Translate(addr uint64, size uint64, write bool) ([]byte, error)

	// Heap management
	HeapMax() uint64
	HeapSize() uint64
	UpdateHeapSize(size uint64)

	// Compute metering
	ComputeMeter() *ComputeMeter
}

// ComputeMeter tracks compute unit consumption.
type ComputeMeter struct {
	remaining uint64
	limit     uint64
}

// NewComputeMeter creates a new compute meter.
func NewComputeMeter(limit uint64) *ComputeMeter {
	return &ComputeMeter{
		remaining: limit,
		limit:     limit,
	}
}

// Consume attempts to consume compute units.
func (cm *ComputeMeter) Consume(cost uint64) error {
	if cm.remaining < cost {
		cm.remaining = 0
		return ErrComputeExceeded
	}
	cm.remaining -= cost
	return nil
}

// Remaining returns remaining compute units.
func (cm *ComputeMeter) Remaining() uint64 {
	return cm.remaining
}

// Syscall is the interface for host functions callable from sBPF programs.
type Syscall interface {
	// Invoke executes the syscall with the given arguments.
	// Arguments are passed in r1-r5, return value goes in r0.
	Invoke(vm VM, r1, r2, r3, r4, r5 uint64) (uint64, error)
}

// SyscallFunc is a function that implements Syscall.
type SyscallFunc func(vm VM, r1, r2, r3, r4, r5 uint64) (uint64, error)

// Invoke implements Syscall.
func (f SyscallFunc) Invoke(vm VM, r1, r2, r3, r4, r5 uint64) (uint64, error) {
	return f(vm, r1, r2, r3, r4, r5)
}

// SyscallRegistry maps syscall hashes to implementations.
type SyscallRegistry func(hash uint32) (Syscall, bool)

// Frame represents a call stack frame.
type Frame struct {
	FramePtr uint64    // Frame pointer (R10 value)
	NVRegs   [4]uint64 // Callee-saved registers (R6-R9)
	RetAddr  int64     // Return address (program counter)
}

// Stack manages the call stack.
type Stack struct {
	mem    []byte  // Stack memory
	frames []Frame // Frame metadata
	sp     uint64  // Stack pointer
}

// NewStack creates a new stack.
func NewStack() *Stack {
	return &Stack{
		mem:    make([]byte, StackFrameSize*StackDepth),
		frames: make([]Frame, 0, StackDepth),
		sp:     0,
	}
}

// Push pushes a new frame onto the stack.
func (s *Stack) Push(regs []uint64, retAddr int64) error {
	if len(s.frames) >= StackDepth {
		return ErrCallDepthExceeded
	}

	frame := Frame{
		FramePtr: regs[10],
		RetAddr:  retAddr,
	}
	copy(frame.NVRegs[:], regs[6:10])
	s.frames = append(s.frames, frame)

	// Update frame pointer for new frame
	regs[10] += StackFrameSize + StackGap

	return nil
}

// Pop pops a frame from the stack.
func (s *Stack) Pop(regs []uint64) (int64, bool) {
	if len(s.frames) == 0 {
		return 0, false
	}

	frame := s.frames[len(s.frames)-1]
	s.frames = s.frames[:len(s.frames)-1]

	// Restore callee-saved registers
	copy(regs[6:10], frame.NVRegs[:])
	regs[10] = frame.FramePtr

	return frame.RetAddr, true
}

// GetFrame returns the memory slice for a stack address.
func (s *Stack) GetFrame(addr uint32) []byte {
	frameIdx := addr / (StackFrameSize + StackGap)
	offset := addr % (StackFrameSize + StackGap)
	if offset >= StackFrameSize {
		return nil // In gap
	}
	base := frameIdx * StackFrameSize
	if int(base+offset) >= len(s.mem) {
		return nil
	}
	return s.mem[base+offset:]
}

// Depth returns the current call depth.
func (s *Stack) Depth() int {
	return len(s.frames)
}

// Interpreter executes sBPF programs.
type Interpreter struct {
	// Program
	text   []uint64 // Program instructions (each instruction is 64 bits)
	ro     []byte   // Read-only data segment
	entry  uint64   // Entry point

	// Runtime state
	stack  *Stack   // Call stack
	heap   []byte   // Heap memory
	input  []byte   // Input parameters

	// Configuration
	heapSize     uint64
	computeMeter *ComputeMeter
	syscalls     SyscallRegistry
	functions    map[uint32]uint64 // Internal function registry
	vmContext    interface{}

	// Execution state
	halted bool
}

// InterpreterOpts configures the interpreter.
type InterpreterOpts struct {
	HeapSize     uint64
	MaxCU        uint64
	Syscalls     SyscallRegistry
	Context      interface{}
}

// NewInterpreter creates a new sBPF interpreter.
func NewInterpreter(program *Program, input []byte, opts InterpreterOpts) *Interpreter {
	heapSize := opts.HeapSize
	if heapSize == 0 {
		heapSize = HeapDefault
	}
	if heapSize > HeapMax {
		heapSize = HeapMax
	}

	// Copy function registry from program
	functions := make(map[uint32]uint64)
	if program.Functions != nil {
		for k, v := range program.Functions {
			functions[k] = v
		}
	}

	return &Interpreter{
		text:         program.Text,
		ro:           program.RO,
		entry:        program.Entry,
		stack:        NewStack(),
		heap:         make([]byte, heapSize),
		input:        input,
		heapSize:     heapSize,
		computeMeter: NewComputeMeter(opts.MaxCU),
		syscalls:     opts.Syscalls,
		functions:    functions,
		vmContext:    opts.Context,
	}
}

// Run executes the program until completion or error.
func (ip *Interpreter) Run() (r0 uint64, err error) {
	// Initialize registers
	var r [11]uint64
	r[1] = VaddrInput                        // Input pointer in R1
	r[10] = VaddrStack + StackFrameSize - 1  // Frame pointer in R10

	pc := int64(ip.entry)

	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("vm panic: %v", rec)
		}
	}()

	for !ip.halted {
		if pc < 0 || pc >= int64(len(ip.text)) {
			return 0, fmt.Errorf("program counter out of bounds: %d", pc)
		}

		ins := ip.text[pc]
		op := uint8(ins & 0xFF)
		dst := uint8((ins >> 8) & 0x0F)
		src := uint8((ins >> 12) & 0x0F)
		off := int16(ins >> 16)
		imm := int32(ins >> 32)

		// Consume variable compute units based on instruction type
		cost := instructionCost(op)
		if err := ip.computeMeter.Consume(cost); err != nil {
			return 0, err
		}

		// Validate register indices (sBPF has 11 registers: R0-R10)
		if dst > 10 || src > 10 {
			return 0, fmt.Errorf("%w: invalid register index dst=%d src=%d", ErrInvalidInstruction, dst, src)
		}

		// Execute instruction
		switch op {
		// 64-bit immediate load (uses two instruction slots)
		case OpLddw:
			if pc+1 >= int64(len(ip.text)) {
				return 0, fmt.Errorf("%w: incomplete lddw at pc %d", ErrInvalidInstruction, pc)
			}
			if dst == 10 {
				return 0, fmt.Errorf("%w: cannot write to R10", ErrInvalidInstruction)
			}
			nextIns := ip.text[pc+1]
			imm64 := uint64(uint32(imm)) | (uint64(nextIns>>32) << 32)
			r[dst] = imm64
			pc++ // Skip the second instruction slot
		// ALU64 immediate
		case OpAdd64Imm:
			r[dst] += uint64(imm)
		case OpSub64Imm:
			r[dst] -= uint64(imm)
		case OpMul64Imm:
			r[dst] *= uint64(imm)
		case OpDiv64Imm:
			if imm == 0 {
				return 0, ErrDivisionByZero
			}
			r[dst] /= uint64(uint32(imm))
		case OpOr64Imm:
			r[dst] |= uint64(imm)
		case OpAnd64Imm:
			r[dst] &= uint64(imm)
		case OpLsh64Imm:
			r[dst] <<= uint64(imm) & 63
		case OpRsh64Imm:
			r[dst] >>= uint64(imm) & 63
		case OpNeg64:
			r[dst] = uint64(-int64(r[dst]))
		case OpMod64Imm:
			if imm == 0 {
				return 0, ErrDivisionByZero
			}
			r[dst] %= uint64(uint32(imm))
		case OpXor64Imm:
			r[dst] ^= uint64(imm)
		case OpMov64Imm:
			r[dst] = uint64(imm)
		case OpArsh64Imm:
			r[dst] = uint64(int64(r[dst]) >> (uint64(imm) & 63))

		// ALU64 register
		case OpAdd64Reg:
			r[dst] += r[src]
		case OpSub64Reg:
			r[dst] -= r[src]
		case OpMul64Reg:
			r[dst] *= r[src]
		case OpDiv64Reg:
			if r[src] == 0 {
				return 0, ErrDivisionByZero
			}
			r[dst] /= r[src]
		case OpOr64Reg:
			r[dst] |= r[src]
		case OpAnd64Reg:
			r[dst] &= r[src]
		case OpLsh64Reg:
			r[dst] <<= r[src] & 63
		case OpRsh64Reg:
			r[dst] >>= r[src] & 63
		case OpMod64Reg:
			if r[src] == 0 {
				return 0, ErrDivisionByZero
			}
			r[dst] %= r[src]
		case OpXor64Reg:
			r[dst] ^= r[src]
		case OpMov64Reg:
			r[dst] = r[src]
		case OpArsh64Reg:
			r[dst] = uint64(int64(r[dst]) >> (r[src] & 63))

		// ALU32 immediate
		case OpAdd32Imm:
			r[dst] = uint64(uint32(r[dst]) + uint32(imm))
		case OpSub32Imm:
			r[dst] = uint64(uint32(r[dst]) - uint32(imm))
		case OpMul32Imm:
			r[dst] = uint64(uint32(r[dst]) * uint32(imm))
		case OpDiv32Imm:
			if imm == 0 {
				return 0, ErrDivisionByZero
			}
			r[dst] = uint64(uint32(r[dst]) / uint32(imm))
		case OpOr32Imm:
			r[dst] = uint64(uint32(r[dst]) | uint32(imm))
		case OpAnd32Imm:
			r[dst] = uint64(uint32(r[dst]) & uint32(imm))
		case OpLsh32Imm:
			r[dst] = uint64(uint32(r[dst]) << (uint32(imm) & 31))
		case OpRsh32Imm:
			r[dst] = uint64(uint32(r[dst]) >> (uint32(imm) & 31))
		case OpMod32Imm:
			if imm == 0 {
				return 0, ErrDivisionByZero
			}
			r[dst] = uint64(uint32(r[dst]) % uint32(imm))
		case OpXor32Imm:
			r[dst] = uint64(uint32(r[dst]) ^ uint32(imm))
		case OpMov32Imm:
			r[dst] = uint64(uint32(imm))
		case OpNeg32:
			r[dst] = uint64(uint32(-int32(r[dst])))
		case OpArsh32Imm:
			r[dst] = uint64(uint32(int32(r[dst]) >> (uint32(imm) & 31)))

		// ALU32 register
		case OpAdd32Reg:
			r[dst] = uint64(uint32(r[dst]) + uint32(r[src]))
		case OpSub32Reg:
			r[dst] = uint64(uint32(r[dst]) - uint32(r[src]))
		case OpMul32Reg:
			r[dst] = uint64(uint32(r[dst]) * uint32(r[src]))
		case OpDiv32Reg:
			if uint32(r[src]) == 0 {
				return 0, ErrDivisionByZero
			}
			r[dst] = uint64(uint32(r[dst]) / uint32(r[src]))
		case OpOr32Reg:
			r[dst] = uint64(uint32(r[dst]) | uint32(r[src]))
		case OpAnd32Reg:
			r[dst] = uint64(uint32(r[dst]) & uint32(r[src]))
		case OpLsh32Reg:
			r[dst] = uint64(uint32(r[dst]) << (uint32(r[src]) & 31))
		case OpRsh32Reg:
			r[dst] = uint64(uint32(r[dst]) >> (uint32(r[src]) & 31))
		case OpMod32Reg:
			if uint32(r[src]) == 0 {
				return 0, ErrDivisionByZero
			}
			r[dst] = uint64(uint32(r[dst]) % uint32(r[src]))
		case OpXor32Reg:
			r[dst] = uint64(uint32(r[dst]) ^ uint32(r[src]))
		case OpMov32Reg:
			r[dst] = uint64(uint32(r[src]))
		case OpArsh32Reg:
			r[dst] = uint64(uint32(int32(r[dst]) >> (uint32(r[src]) & 31)))

		// Memory load
		case OpLdxb:
			val, err := ip.Read8(r[src] + uint64(off))
			if err != nil {
				return 0, err
			}
			r[dst] = uint64(val)
		case OpLdxh:
			val, err := ip.Read16(r[src] + uint64(off))
			if err != nil {
				return 0, err
			}
			r[dst] = uint64(val)
		case OpLdxw:
			val, err := ip.Read32(r[src] + uint64(off))
			if err != nil {
				return 0, err
			}
			r[dst] = uint64(val)
		case OpLdxdw:
			val, err := ip.Read64(r[src] + uint64(off))
			if err != nil {
				return 0, err
			}
			r[dst] = val

		// Memory store
		case OpStb:
			if err := ip.Write8(r[dst]+uint64(off), uint8(imm)); err != nil {
				return 0, err
			}
		case OpSth:
			if err := ip.Write16(r[dst]+uint64(off), uint16(imm)); err != nil {
				return 0, err
			}
		case OpStw:
			if err := ip.Write32(r[dst]+uint64(off), uint32(imm)); err != nil {
				return 0, err
			}
		case OpStdw:
			if err := ip.Write64(r[dst]+uint64(off), uint64(imm)); err != nil {
				return 0, err
			}
		case OpStxb:
			if err := ip.Write8(r[dst]+uint64(off), uint8(r[src])); err != nil {
				return 0, err
			}
		case OpStxh:
			if err := ip.Write16(r[dst]+uint64(off), uint16(r[src])); err != nil {
				return 0, err
			}
		case OpStxw:
			if err := ip.Write32(r[dst]+uint64(off), uint32(r[src])); err != nil {
				return 0, err
			}
		case OpStxdw:
			if err := ip.Write64(r[dst]+uint64(off), r[src]); err != nil {
				return 0, err
			}

		// Jump unconditional
		case OpJa:
			pc += int64(off)

		// Jump conditional (64-bit)
		case OpJeqImm:
			if r[dst] == uint64(imm) {
				pc += int64(off)
			}
		case OpJeqReg:
			if r[dst] == r[src] {
				pc += int64(off)
			}
		case OpJgtImm:
			if r[dst] > uint64(imm) {
				pc += int64(off)
			}
		case OpJgtReg:
			if r[dst] > r[src] {
				pc += int64(off)
			}
		case OpJgeImm:
			if r[dst] >= uint64(imm) {
				pc += int64(off)
			}
		case OpJgeReg:
			if r[dst] >= r[src] {
				pc += int64(off)
			}
		case OpJltImm:
			if r[dst] < uint64(imm) {
				pc += int64(off)
			}
		case OpJltReg:
			if r[dst] < r[src] {
				pc += int64(off)
			}
		case OpJleImm:
			if r[dst] <= uint64(imm) {
				pc += int64(off)
			}
		case OpJleReg:
			if r[dst] <= r[src] {
				pc += int64(off)
			}
		case OpJneImm:
			if r[dst] != uint64(imm) {
				pc += int64(off)
			}
		case OpJneReg:
			if r[dst] != r[src] {
				pc += int64(off)
			}
		case OpJsetImm:
			if r[dst]&uint64(imm) != 0 {
				pc += int64(off)
			}
		case OpJsetReg:
			if r[dst]&r[src] != 0 {
				pc += int64(off)
			}
		case OpJsgtImm:
			if int64(r[dst]) > int64(int32(imm)) {
				pc += int64(off)
			}
		case OpJsgtReg:
			if int64(r[dst]) > int64(r[src]) {
				pc += int64(off)
			}
		case OpJsgeImm:
			if int64(r[dst]) >= int64(int32(imm)) {
				pc += int64(off)
			}
		case OpJsgeReg:
			if int64(r[dst]) >= int64(r[src]) {
				pc += int64(off)
			}
		case OpJsltImm:
			if int64(r[dst]) < int64(int32(imm)) {
				pc += int64(off)
			}
		case OpJsltReg:
			if int64(r[dst]) < int64(r[src]) {
				pc += int64(off)
			}
		case OpJsleImm:
			if int64(r[dst]) <= int64(int32(imm)) {
				pc += int64(off)
			}
		case OpJsleReg:
			if int64(r[dst]) <= int64(r[src]) {
				pc += int64(off)
			}

		// 32-bit jump conditional (compare 32-bit values)
		case OpJeq32Imm:
			if uint32(r[dst]) == uint32(imm) {
				pc += int64(off)
			}
		case OpJeq32Reg:
			if uint32(r[dst]) == uint32(r[src]) {
				pc += int64(off)
			}
		case OpJgt32Imm:
			if uint32(r[dst]) > uint32(imm) {
				pc += int64(off)
			}
		case OpJgt32Reg:
			if uint32(r[dst]) > uint32(r[src]) {
				pc += int64(off)
			}
		case OpJge32Imm:
			if uint32(r[dst]) >= uint32(imm) {
				pc += int64(off)
			}
		case OpJge32Reg:
			if uint32(r[dst]) >= uint32(r[src]) {
				pc += int64(off)
			}
		case OpJlt32Imm:
			if uint32(r[dst]) < uint32(imm) {
				pc += int64(off)
			}
		case OpJlt32Reg:
			if uint32(r[dst]) < uint32(r[src]) {
				pc += int64(off)
			}
		case OpJle32Imm:
			if uint32(r[dst]) <= uint32(imm) {
				pc += int64(off)
			}
		case OpJle32Reg:
			if uint32(r[dst]) <= uint32(r[src]) {
				pc += int64(off)
			}
		case OpJne32Imm:
			if uint32(r[dst]) != uint32(imm) {
				pc += int64(off)
			}
		case OpJne32Reg:
			if uint32(r[dst]) != uint32(r[src]) {
				pc += int64(off)
			}
		case OpJset32Imm:
			if uint32(r[dst])&uint32(imm) != 0 {
				pc += int64(off)
			}
		case OpJset32Reg:
			if uint32(r[dst])&uint32(r[src]) != 0 {
				pc += int64(off)
			}
		case OpJsgt32Imm:
			if int32(r[dst]) > int32(imm) {
				pc += int64(off)
			}
		case OpJsgt32Reg:
			if int32(r[dst]) > int32(r[src]) {
				pc += int64(off)
			}
		case OpJsge32Imm:
			if int32(r[dst]) >= int32(imm) {
				pc += int64(off)
			}
		case OpJsge32Reg:
			if int32(r[dst]) >= int32(r[src]) {
				pc += int64(off)
			}
		case OpJslt32Imm:
			if int32(r[dst]) < int32(imm) {
				pc += int64(off)
			}
		case OpJslt32Reg:
			if int32(r[dst]) < int32(r[src]) {
				pc += int64(off)
			}
		case OpJsle32Imm:
			if int32(r[dst]) <= int32(imm) {
				pc += int64(off)
			}
		case OpJsle32Reg:
			if int32(r[dst]) <= int32(r[src]) {
				pc += int64(off)
			}

		// Call and exit
		case OpCall:
			hash := uint32(imm)
			if sc, ok := ip.syscalls(hash); ok {
				// Syscall
				result, err := sc.Invoke(ip, r[1], r[2], r[3], r[4], r[5])
				if err != nil {
					return 0, err
				}
				r[0] = result
			} else if targetPC, ok := ip.functions[hash]; ok {
				// Internal BPF-to-BPF function call
				// Push current frame with return address
				if err := ip.stack.Push(r[:], pc+1); err != nil {
					return 0, err
				}
				// Jump to function entry point
				pc = int64(targetPC)
				continue // Don't increment PC
			} else {
				// Unknown function - check if it's a relative call (src == 1)
				// In sBPF, when src register is 1, imm is a relative offset
				if src == 1 {
					// Relative call: target = pc + imm + 1
					if err := ip.stack.Push(r[:], pc+1); err != nil {
						return 0, err
					}
					pc = pc + int64(imm) + 1
					continue
				}
				return 0, fmt.Errorf("%w: unknown function 0x%x", ErrUnknownSyscall, hash)
			}

		case OpExit:
			retAddr, ok := ip.stack.Pop(r[:])
			if !ok {
				// No more frames, exit program
				ip.halted = true
				return r[0], nil
			}
			pc = retAddr
			continue // Don't increment PC

		default:
			return 0, fmt.Errorf("%w: opcode 0x%02x", ErrInvalidInstruction, op)
		}

		pc++
	}

	return r[0], nil
}

// VM interface implementation

func (ip *Interpreter) VMContext() interface{} {
	return ip.vmContext
}

func (ip *Interpreter) ComputeMeter() *ComputeMeter {
	return ip.computeMeter
}

func (ip *Interpreter) HeapMax() uint64 {
	return HeapMax
}

func (ip *Interpreter) HeapSize() uint64 {
	return ip.heapSize
}

func (ip *Interpreter) UpdateHeapSize(size uint64) {
	if size <= HeapMax && size > ip.heapSize {
		newHeap := make([]byte, size)
		copy(newHeap, ip.heap)
		ip.heap = newHeap
		ip.heapSize = size
	}
}

// Program represents a loaded sBPF program.
type Program struct {
	Text      []uint64          // Instructions
	RO        []byte            // Read-only data
	Entry     uint64            // Entry point
	Functions map[uint32]uint64 // Function registry: hash -> PC offset
}
