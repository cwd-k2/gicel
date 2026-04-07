// Compiler — bytecode emission primitives.
// Does NOT cover: IR node compilation (compiler_expr.go),
//                 child proto builders (compiler_closure.go),
//                 pattern matching (compiler_match.go).

package vm

import (
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// irNodeKind returns a short human-readable name for the Core IR node type.
func irNodeKind(n ir.Core) string {
	switch n.(type) {
	case *ir.Var:
		return "Var"
	case *ir.Lit:
		return "Lit"
	case *ir.Lam:
		return "Lam"
	case *ir.App:
		return "App"
	case *ir.Con:
		return "Con"
	case *ir.Case:
		return "Case"
	case *ir.Fix:
		return "Fix"
	case *ir.Bind:
		return "Bind"
	case *ir.Thunk:
		return "Thunk"
	case *ir.Force:
		return "Force"
	case *ir.Merge:
		return "Merge"
	case *ir.PrimOp:
		return "PrimOp"
	case *ir.RecordLit:
		return types.TyConRecord
	case *ir.RecordProj:
		return "RecordProj"
	case *ir.RecordUpdate:
		return "RecordUpdate"
	default:
		return "?"
	}
}

// --- bytecode emission ---

func (c *Compiler) emitStep(expr ir.Core) {
	kind := irNodeKind(expr)
	idx := c.addString(kind)
	c.emitU16(OpStep, idx)
}

// irNodeKind returns a short human-readable name for the Core IR node type.
func (c *Compiler) emit(op Opcode) int {
	f := c.top()
	pos := len(f.code)
	f.code = append(f.code, byte(op))
	return pos
}
func (c *Compiler) emitU16(op Opcode, operand uint16) int {
	f := c.top()
	pos := len(f.code)
	f.code = append(f.code, byte(op), 0, 0)
	EncodeU16(f.code[pos+1:], operand)
	return pos
}
func (c *Compiler) emitU8(op Opcode, operand uint8) int {
	f := c.top()
	pos := len(f.code)
	f.code = append(f.code, byte(op), operand)
	return pos
}
func (c *Compiler) emitU16U8(op Opcode, a uint16, b uint8) int {
	f := c.top()
	pos := len(f.code)
	f.code = append(f.code, byte(op), 0, 0, b)
	EncodeU16(f.code[pos+1:], a)
	return pos
}
func (c *Compiler) emitU16U16(op Opcode, a, b uint16) int {
	f := c.top()
	pos := len(f.code)
	f.code = append(f.code, byte(op), 0, 0, 0, 0)
	EncodeU16(f.code[pos+1:], a)
	EncodeU16(f.code[pos+3:], b)
	return pos
}
func (c *Compiler) emitJump(op Opcode) int {
	f := c.top()
	pos := len(f.code)
	f.code = append(f.code, byte(op), 0, 0)
	return pos
}
func (c *Compiler) patchJumpTo(pos int) {
	f := c.top()
	offset := int16(len(f.code) - pos - 3)
	EncodeU16(f.code[pos+1:], uint16(offset))
}

// --- constant / string pool ---
