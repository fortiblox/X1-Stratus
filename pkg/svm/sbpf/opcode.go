// Package sbpf defines sBPF opcodes and instruction encoding.
package sbpf

// Instruction class bits (bits 0-2).
const (
	ClassLd  = 0x00 // Load
	ClassLdx = 0x01 // Load extended
	ClassSt  = 0x02 // Store immediate
	ClassStx = 0x03 // Store register
	ClassAlu = 0x04 // 32-bit ALU
	ClassJmp = 0x05 // 64-bit jump
	ClassJmp32 = 0x06 // 32-bit jump
	ClassAlu64 = 0x07 // 64-bit ALU
)

// Source bits (bit 3).
const (
	SrcK = 0x00 // Immediate
	SrcX = 0x08 // Register
)

// ALU operation codes (bits 4-7).
const (
	AluAdd  = 0x00
	AluSub  = 0x10
	AluMul  = 0x20
	AluDiv  = 0x30
	AluOr   = 0x40
	AluAnd  = 0x50
	AluLsh  = 0x60
	AluRsh  = 0x70
	AluNeg  = 0x80
	AluMod  = 0x90
	AluXor  = 0xa0
	AluMov  = 0xb0
	AluArsh = 0xc0
	AluEnd  = 0xd0
)

// Memory size (bits 3-4 for load/store).
const (
	SizeW  = 0x00 // 32-bit word
	SizeH  = 0x08 // 16-bit half-word
	SizeB  = 0x10 // 8-bit byte
	SizeDW = 0x18 // 64-bit double-word
)

// Memory mode (bits 5-7 for load/store).
const (
	ModeImm = 0x00 // Immediate
	ModeAbs = 0x20 // Absolute (deprecated)
	ModeInd = 0x40 // Indirect (deprecated)
	ModeMem = 0x60 // Memory
)

// Jump operation codes (bits 4-7).
const (
	JmpJa   = 0x00 // Unconditional
	JmpJeq  = 0x10 // ==
	JmpJgt  = 0x20 // > (unsigned)
	JmpJge  = 0x30 // >= (unsigned)
	JmpJset = 0x40 // &
	JmpJne  = 0x50 // !=
	JmpJsgt = 0x60 // > (signed)
	JmpJsge = 0x70 // >= (signed)
	JmpCall = 0x80 // Function call
	JmpExit = 0x90 // Exit
	JmpJlt  = 0xa0 // < (unsigned)
	JmpJle  = 0xb0 // <= (unsigned)
	JmpJslt = 0xc0 // < (signed)
	JmpJsle = 0xd0 // <= (signed)
)

// Composed opcodes for 64-bit ALU with immediate.
const (
	OpAdd64Imm  = ClassAlu64 | SrcK | AluAdd  // 0x07
	OpSub64Imm  = ClassAlu64 | SrcK | AluSub  // 0x17
	OpMul64Imm  = ClassAlu64 | SrcK | AluMul  // 0x27
	OpDiv64Imm  = ClassAlu64 | SrcK | AluDiv  // 0x37
	OpOr64Imm   = ClassAlu64 | SrcK | AluOr   // 0x47
	OpAnd64Imm  = ClassAlu64 | SrcK | AluAnd  // 0x57
	OpLsh64Imm  = ClassAlu64 | SrcK | AluLsh  // 0x67
	OpRsh64Imm  = ClassAlu64 | SrcK | AluRsh  // 0x77
	OpNeg64     = ClassAlu64 | AluNeg         // 0x87
	OpMod64Imm  = ClassAlu64 | SrcK | AluMod  // 0x97
	OpXor64Imm  = ClassAlu64 | SrcK | AluXor  // 0xa7
	OpMov64Imm  = ClassAlu64 | SrcK | AluMov  // 0xb7
	OpArsh64Imm = ClassAlu64 | SrcK | AluArsh // 0xc7
)

// Composed opcodes for 64-bit ALU with register.
const (
	OpAdd64Reg  = ClassAlu64 | SrcX | AluAdd  // 0x0f
	OpSub64Reg  = ClassAlu64 | SrcX | AluSub  // 0x1f
	OpMul64Reg  = ClassAlu64 | SrcX | AluMul  // 0x2f
	OpDiv64Reg  = ClassAlu64 | SrcX | AluDiv  // 0x3f
	OpOr64Reg   = ClassAlu64 | SrcX | AluOr   // 0x4f
	OpAnd64Reg  = ClassAlu64 | SrcX | AluAnd  // 0x5f
	OpLsh64Reg  = ClassAlu64 | SrcX | AluLsh  // 0x6f
	OpRsh64Reg  = ClassAlu64 | SrcX | AluRsh  // 0x7f
	OpMod64Reg  = ClassAlu64 | SrcX | AluMod  // 0x9f
	OpXor64Reg  = ClassAlu64 | SrcX | AluXor  // 0xaf
	OpMov64Reg  = ClassAlu64 | SrcX | AluMov  // 0xbf
	OpArsh64Reg = ClassAlu64 | SrcX | AluArsh // 0xcf
)

// Composed opcodes for 32-bit ALU with immediate.
const (
	OpAdd32Imm  = ClassAlu | SrcK | AluAdd  // 0x04
	OpSub32Imm  = ClassAlu | SrcK | AluSub  // 0x14
	OpMul32Imm  = ClassAlu | SrcK | AluMul  // 0x24
	OpDiv32Imm  = ClassAlu | SrcK | AluDiv  // 0x34
	OpOr32Imm   = ClassAlu | SrcK | AluOr   // 0x44
	OpAnd32Imm  = ClassAlu | SrcK | AluAnd  // 0x54
	OpLsh32Imm  = ClassAlu | SrcK | AluLsh  // 0x64
	OpRsh32Imm  = ClassAlu | SrcK | AluRsh  // 0x74
	OpNeg32     = ClassAlu | AluNeg         // 0x84
	OpMod32Imm  = ClassAlu | SrcK | AluMod  // 0x94
	OpXor32Imm  = ClassAlu | SrcK | AluXor  // 0xa4
	OpMov32Imm  = ClassAlu | SrcK | AluMov  // 0xb4
	OpArsh32Imm = ClassAlu | SrcK | AluArsh // 0xc4
)

// Composed opcodes for 32-bit ALU with register.
const (
	OpAdd32Reg  = ClassAlu | SrcX | AluAdd  // 0x0c
	OpSub32Reg  = ClassAlu | SrcX | AluSub  // 0x1c
	OpMul32Reg  = ClassAlu | SrcX | AluMul  // 0x2c
	OpDiv32Reg  = ClassAlu | SrcX | AluDiv  // 0x3c
	OpOr32Reg   = ClassAlu | SrcX | AluOr   // 0x4c
	OpAnd32Reg  = ClassAlu | SrcX | AluAnd  // 0x5c
	OpLsh32Reg  = ClassAlu | SrcX | AluLsh  // 0x6c
	OpRsh32Reg  = ClassAlu | SrcX | AluRsh  // 0x7c
	OpMod32Reg  = ClassAlu | SrcX | AluMod  // 0x9c
	OpXor32Reg  = ClassAlu | SrcX | AluXor  // 0xac
	OpMov32Reg  = ClassAlu | SrcX | AluMov  // 0xbc
	OpArsh32Reg = ClassAlu | SrcX | AluArsh // 0xcc
)

// Memory load opcodes.
const (
	OpLdxb  = ClassLdx | ModeMem | SizeB  // 0x71 - load byte
	OpLdxh  = ClassLdx | ModeMem | SizeH  // 0x69 - load half-word
	OpLdxw  = ClassLdx | ModeMem | SizeW  // 0x61 - load word
	OpLdxdw = ClassLdx | ModeMem | SizeDW // 0x79 - load double-word
)

// Memory store immediate opcodes.
const (
	OpStb  = ClassSt | ModeMem | SizeB  // 0x72 - store byte immediate
	OpSth  = ClassSt | ModeMem | SizeH  // 0x6a - store half-word immediate
	OpStw  = ClassSt | ModeMem | SizeW  // 0x62 - store word immediate
	OpStdw = ClassSt | ModeMem | SizeDW // 0x7a - store double-word immediate
)

// Memory store register opcodes.
const (
	OpStxb  = ClassStx | ModeMem | SizeB  // 0x73 - store byte
	OpStxh  = ClassStx | ModeMem | SizeH  // 0x6b - store half-word
	OpStxw  = ClassStx | ModeMem | SizeW  // 0x63 - store word
	OpStxdw = ClassStx | ModeMem | SizeDW // 0x7b - store double-word
)

// Jump opcodes.
const (
	OpJa      = ClassJmp | JmpJa            // 0x05 - unconditional jump
	OpJeqImm  = ClassJmp | SrcK | JmpJeq    // 0x15 - jump if equal (imm)
	OpJeqReg  = ClassJmp | SrcX | JmpJeq    // 0x1d - jump if equal (reg)
	OpJgtImm  = ClassJmp | SrcK | JmpJgt    // 0x25 - jump if greater (imm, unsigned)
	OpJgtReg  = ClassJmp | SrcX | JmpJgt    // 0x2d - jump if greater (reg, unsigned)
	OpJgeImm  = ClassJmp | SrcK | JmpJge    // 0x35 - jump if greater or equal (imm, unsigned)
	OpJgeReg  = ClassJmp | SrcX | JmpJge    // 0x3d - jump if greater or equal (reg, unsigned)
	OpJsetImm = ClassJmp | SrcK | JmpJset   // 0x45 - jump if set (imm)
	OpJsetReg = ClassJmp | SrcX | JmpJset   // 0x4d - jump if set (reg)
	OpJneImm  = ClassJmp | SrcK | JmpJne    // 0x55 - jump if not equal (imm)
	OpJneReg  = ClassJmp | SrcX | JmpJne    // 0x5d - jump if not equal (reg)
	OpJsgtImm = ClassJmp | SrcK | JmpJsgt   // 0x65 - jump if greater (imm, signed)
	OpJsgtReg = ClassJmp | SrcX | JmpJsgt   // 0x6d - jump if greater (reg, signed)
	OpJsgeImm = ClassJmp | SrcK | JmpJsge   // 0x75 - jump if greater or equal (imm, signed)
	OpJsgeReg = ClassJmp | SrcX | JmpJsge   // 0x7d - jump if greater or equal (reg, signed)
	OpCall    = ClassJmp | JmpCall          // 0x85 - function call
	OpExit    = ClassJmp | JmpExit          // 0x95 - exit
	OpJltImm  = ClassJmp | SrcK | JmpJlt    // 0xa5 - jump if less (imm, unsigned)
	OpJltReg  = ClassJmp | SrcX | JmpJlt    // 0xad - jump if less (reg, unsigned)
	OpJleImm  = ClassJmp | SrcK | JmpJle    // 0xb5 - jump if less or equal (imm, unsigned)
	OpJleReg  = ClassJmp | SrcX | JmpJle    // 0xbd - jump if less or equal (reg, unsigned)
	OpJsltImm = ClassJmp | SrcK | JmpJslt   // 0xc5 - jump if less (imm, signed)
	OpJsltReg = ClassJmp | SrcX | JmpJslt   // 0xcd - jump if less (reg, signed)
	OpJsleImm = ClassJmp | SrcK | JmpJsle   // 0xd5 - jump if less or equal (imm, signed)
	OpJsleReg = ClassJmp | SrcX | JmpJsle   // 0xdd - jump if less or equal (reg, signed)
)

// 32-bit jump opcodes (for comparing 32-bit values).
const (
	OpJeq32Imm  = ClassJmp32 | SrcK | JmpJeq   // 0x16 - jump if equal (32-bit imm)
	OpJeq32Reg  = ClassJmp32 | SrcX | JmpJeq   // 0x1e - jump if equal (32-bit reg)
	OpJgt32Imm  = ClassJmp32 | SrcK | JmpJgt   // 0x26 - jump if greater (32-bit imm, unsigned)
	OpJgt32Reg  = ClassJmp32 | SrcX | JmpJgt   // 0x2e - jump if greater (32-bit reg, unsigned)
	OpJge32Imm  = ClassJmp32 | SrcK | JmpJge   // 0x36 - jump if greater or equal (32-bit imm, unsigned)
	OpJge32Reg  = ClassJmp32 | SrcX | JmpJge   // 0x3e - jump if greater or equal (32-bit reg, unsigned)
	OpJset32Imm = ClassJmp32 | SrcK | JmpJset  // 0x46 - jump if set (32-bit imm)
	OpJset32Reg = ClassJmp32 | SrcX | JmpJset  // 0x4e - jump if set (32-bit reg)
	OpJne32Imm  = ClassJmp32 | SrcK | JmpJne   // 0x56 - jump if not equal (32-bit imm)
	OpJne32Reg  = ClassJmp32 | SrcX | JmpJne   // 0x5e - jump if not equal (32-bit reg)
	OpJsgt32Imm = ClassJmp32 | SrcK | JmpJsgt  // 0x66 - jump if greater (32-bit imm, signed)
	OpJsgt32Reg = ClassJmp32 | SrcX | JmpJsgt  // 0x6e - jump if greater (32-bit reg, signed)
	OpJsge32Imm = ClassJmp32 | SrcK | JmpJsge  // 0x76 - jump if greater or equal (32-bit imm, signed)
	OpJsge32Reg = ClassJmp32 | SrcX | JmpJsge  // 0x7e - jump if greater or equal (32-bit reg, signed)
	OpJlt32Imm  = ClassJmp32 | SrcK | JmpJlt   // 0xa6 - jump if less (32-bit imm, unsigned)
	OpJlt32Reg  = ClassJmp32 | SrcX | JmpJlt   // 0xae - jump if less (32-bit reg, unsigned)
	OpJle32Imm  = ClassJmp32 | SrcK | JmpJle   // 0xb6 - jump if less or equal (32-bit imm, unsigned)
	OpJle32Reg  = ClassJmp32 | SrcX | JmpJle   // 0xbe - jump if less or equal (32-bit reg, unsigned)
	OpJslt32Imm = ClassJmp32 | SrcK | JmpJslt  // 0xc6 - jump if less (32-bit imm, signed)
	OpJslt32Reg = ClassJmp32 | SrcX | JmpJslt  // 0xce - jump if less (32-bit reg, signed)
	OpJsle32Imm = ClassJmp32 | SrcK | JmpJsle  // 0xd6 - jump if less or equal (32-bit imm, signed)
	OpJsle32Reg = ClassJmp32 | SrcX | JmpJsle  // 0xde - jump if less or equal (32-bit reg, signed)
)

// Special opcodes.
const (
	OpLddw = 0x18 // Load 64-bit immediate (uses two instruction slots)
)

// Instruction extracts fields from an encoded instruction.
type Instruction uint64

// Op returns the opcode (bits 0-7).
func (i Instruction) Op() uint8 {
	return uint8(i & 0xFF)
}

// Dst returns the destination register (bits 8-11).
func (i Instruction) Dst() uint8 {
	return uint8((i >> 8) & 0x0F)
}

// Src returns the source register (bits 12-15).
func (i Instruction) Src() uint8 {
	return uint8((i >> 12) & 0x0F)
}

// Off returns the offset (bits 16-31, signed).
func (i Instruction) Off() int16 {
	return int16(i >> 16)
}

// Imm returns the immediate value (bits 32-63, signed).
func (i Instruction) Imm() int32 {
	return int32(i >> 32)
}

// Uimm returns the immediate value as unsigned.
func (i Instruction) Uimm() uint32 {
	return uint32(i >> 32)
}

// Encode creates an instruction from its components.
func Encode(op uint8, dst, src uint8, off int16, imm int32) uint64 {
	return uint64(op) |
		uint64(dst&0x0F)<<8 |
		uint64(src&0x0F)<<12 |
		uint64(uint16(off))<<16 |
		uint64(uint32(imm))<<32
}
