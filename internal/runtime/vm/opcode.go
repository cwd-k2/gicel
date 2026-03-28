// Package vm implements a bytecode virtual machine for GICEL Core IR.
//
// The VM replaces the tree-walking evaluator's trampoline loop with a flat
// fetch-decode-execute cycle, achieving natural TCO and eliminating the
// structural nesting limit imposed by Go's call stack.
package vm

import "encoding/binary"

// Opcode identifies a single bytecode instruction.
type Opcode uint8

const (
	// --- Variables ---

	// OpLoadLocal pushes the local variable at the given frame-relative slot.
	// Operand: u16 slot index.
	OpLoadLocal Opcode = iota
	// OpLoadGlobal pushes the global variable at the given slot.
	// Dereferences IndirectVal. Operand: u16 slot index.
	OpLoadGlobal
	// OpStoreLocal pops TOS into the given frame-relative slot.
	// Operand: u16 slot index.
	OpStoreLocal

	// --- Constants ---

	// OpConst pushes a value from the constant pool.
	// Operand: u16 constant pool index.
	OpConst
	// OpConstUnit pushes UnitVal (zero operands).
	OpConstUnit

	// --- Functions ---

	// OpClosure builds a VMClosure from a prototype.
	// Operand: u16 prototype index.
	OpClosure
	// OpApply pops [fn, arg], dispatches on fn type (VMClosure/ConVal/PrimVal).
	OpApply
	// OpTailApply is the tail-call variant of OpApply; reuses the current frame.
	OpTailApply
	// OpReturn pops the result, pops the frame, and pushes the result onto
	// the caller's operand stack.
	OpReturn

	// --- CBPV ---

	// OpThunk builds a VMThunkVal from a prototype.
	// Operand: u16 prototype index.
	OpThunk
	// OpForce pops a ThunkVal, pushes a new frame for its computation.
	OpForce
	// OpForceTail is the tail-call variant of OpForce.
	OpForceTail
	// OpForceEffectful auto-forces AutoForce ThunkVals and invokes
	// saturated effectful PrimVals against the current CapEnv.
	OpForceEffectful

	// --- Bind ---

	// OpBind performs ForceEffectful on TOS, stores result in a local slot,
	// and updates the frame's CapEnv. Operand: u16 slot index.
	OpBind

	// --- Data construction ---

	// OpCon builds a ConVal. Operand: u16 name (string pool), u8 arity.
	// Pops arity values (first arg deepest).
	OpCon
	// OpRecord builds a RecordVal. Operand: u16 descriptor index.
	// Descriptor encodes field count + label indices.
	OpRecord
	// OpRecordProj projects a field from a RecordVal.
	// Operand: u16 label (string pool index).
	OpRecordProj
	// OpRecordUpdate performs copy-on-write record update.
	// Operand: u16 descriptor index (encodes update labels).
	OpRecordUpdate

	// --- Pattern matching ---

	// OpMatchCon tests TOS against a constructor.
	// Operand: u16 match descriptor index, u16 fail offset.
	// On match: extracts args into local slots per descriptor.
	// On fail: jumps to fail offset (absolute).
	OpMatchCon
	// OpMatchLit tests TOS against a literal.
	// Operand: u16 constant pool index, u16 fail offset.
	OpMatchLit
	// OpMatchRecord tests TOS against a record pattern.
	// Operand: u16 match descriptor index, u16 fail offset.
	OpMatchRecord
	// OpMatchWild always succeeds. No operands.
	OpMatchWild
	// OpMatchFail raises a non-exhaustive pattern match error.
	OpMatchFail
	// OpJump performs an unconditional relative jump.
	// Operand: i16 signed offset (added to ip after this instruction).
	OpJump
	// OpPop discards TOS.
	OpPop

	// --- Fix ---

	// OpFixClosure builds a self-referential VMClosure.
	// Operand: u16 prototype index.
	OpFixClosure
	// OpFixThunk builds a self-referential VMThunkVal (AutoForce=true).
	// Operand: u16 prototype index.
	OpFixThunk

	// --- Primitives ---

	// OpPrim invokes a fully-applied non-effectful primitive.
	// Operand: u16 name (string pool), u8 arity. Pops arity args.
	OpPrim
	// OpPrimPartial pushes a PrimVal stub from the constant pool.
	// Operand: u16 constant pool index.
	OpPrimPartial

	// --- Budget ---

	// OpStep charges one evaluation step against the budget.
	OpStep

	opcodeCount // sentinel: total number of opcodes
)

// Instruction encoding helpers.
// All multi-byte operands are little-endian.

// EncodeU16 appends a u16 operand in little-endian.
func EncodeU16(buf []byte, v uint16) {
	binary.LittleEndian.PutUint16(buf, v)
}

// DecodeU16 reads a u16 operand in little-endian.
func DecodeU16(code []byte, offset int) uint16 {
	return binary.LittleEndian.Uint16(code[offset:])
}

// DecodeI16 reads a signed i16 operand in little-endian.
func DecodeI16(code []byte, offset int) int16 {
	return int16(binary.LittleEndian.Uint16(code[offset:]))
}

// InstructionSize returns the total size in bytes of the instruction
// starting at code[offset] (opcode + operands).
func InstructionSize(op Opcode) int {
	switch op {
	case OpLoadLocal, OpLoadGlobal, OpStoreLocal,
		OpConst, OpClosure, OpThunk,
		OpRecordProj, OpRecord, OpRecordUpdate,
		OpFixClosure, OpFixThunk,
		OpPrimPartial,
		OpBind:
		return 3 // opcode + u16

	case OpCon, OpPrim:
		return 4 // opcode + u16 + u8

	case OpMatchCon, OpMatchRecord:
		return 5 // opcode + u16 + u16

	case OpMatchLit:
		return 5 // opcode + u16 + u16

	case OpJump:
		return 3 // opcode + i16

	case OpApply, OpTailApply, OpReturn,
		OpForce, OpForceTail, OpForceEffectful,
		OpConstUnit,
		OpMatchWild, OpMatchFail,
		OpPop,
		OpStep:
		return 1 // opcode only

	default:
		return 1
	}
}

// opNames maps opcodes to their string representations.
var opNames = [opcodeCount]string{
	OpLoadLocal:      "LOAD_LOCAL",
	OpLoadGlobal:     "LOAD_GLOBAL",
	OpStoreLocal:     "STORE_LOCAL",
	OpConst:          "CONST",
	OpConstUnit:      "CONST_UNIT",
	OpClosure:        "CLOSURE",
	OpApply:          "APPLY",
	OpTailApply:      "TAIL_APPLY",
	OpReturn:         "RETURN",
	OpThunk:          "THUNK",
	OpForce:          "FORCE",
	OpForceTail:      "FORCE_TAIL",
	OpForceEffectful: "FORCE_EFFECTFUL",
	OpBind:           "BIND",
	OpCon:            "CON",
	OpRecord:         "RECORD",
	OpRecordProj:     "RECORD_PROJ",
	OpRecordUpdate:   "RECORD_UPDATE",
	OpMatchCon:       "MATCH_CON",
	OpMatchLit:       "MATCH_LIT",
	OpMatchRecord:    "MATCH_RECORD",
	OpMatchWild:      "MATCH_WILD",
	OpMatchFail:      "MATCH_FAIL",
	OpJump:           "JUMP",
	OpPop:            "POP",
	OpFixClosure:     "FIX_CLOSURE",
	OpFixThunk:       "FIX_THUNK",
	OpPrim:           "PRIM",
	OpPrimPartial:    "PRIM_PARTIAL",
	OpStep:           "STEP",
}

func (op Opcode) String() string {
	if int(op) < len(opNames) && opNames[op] != "" {
		return opNames[op]
	}
	return "UNKNOWN"
}
