package vm

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// compilePatternMatch compiles a Case expression's alternatives into bytecode.
// The scrutinee value is already on TOS.
//
// Strategy: sequential test-and-jump, matching the tree-walker's semantics.
// Each alternative tests the pattern, and on failure jumps to the next.
// The last alternative's failure falls through to MATCH_FAIL.
func compilePatternMatch(c *Compiler, cs *ir.Case, tail bool) {
	numAlts := len(cs.Alts)
	var endJumps []int

	caseSavedLocals := len(c.top().locals)
	scrutSlot := c.allocLocal("$scrut")
	c.emitU16(OpStoreLocal, uint16(scrutSlot))

	allFailPatches := make([][]int, numAlts)

	for i, alt := range cs.Alts {
		savedLocals := len(c.top().locals)

		c.emitU16(OpLoadLocal, uint16(scrutSlot))

		var failPatches []int
		compilePatternCollect(c, alt.Pattern, &failPatches)
		allFailPatches[i] = failPatches

		isLast := i == numAlts-1
		bodyTail := tail && isLast
		c.compileExpr(alt.Body, bodyTail)

		c.popLocals(savedLocals)

		endJumps = append(endJumps, c.emitJump(OpJump))

		if i < numAlts-1 {
			f := c.top()
			target := uint16(len(f.code))
			for _, pos := range failPatches {
				EncodeU16(f.code[pos+3:], target)
			}
		}
	}

	c.emitU16(OpLoadLocal, uint16(scrutSlot))
	matchFailPos := len(c.top().code)
	c.emit(OpMatchFail)
	if numAlts > 0 {
		f := c.top()
		for _, pos := range allFailPatches[numAlts-1] {
			EncodeU16(f.code[pos+3:], uint16(matchFailPos))
		}
	}

	for _, pos := range endJumps {
		c.patchJumpTo(pos)
	}

	c.popLocals(caseSavedLocals)
}

func compilePatternCollect(c *Compiler, pat ir.Pattern, failPatches *[]int) {
	switch p := pat.(type) {
	case *ir.PVar:
		slot := c.allocLocal(p.Name)
		c.emitU16(OpStoreLocal, uint16(slot))
	case *ir.PWild:
		c.emit(OpPop)
	case *ir.PCon:
		compilePCon(c, p, failPatches)
	case *ir.PLit:
		compilePLit(c, p, failPatches)
	case *ir.PRecord:
		compilePRecord(c, p, failPatches)
	default:
		panic("vm/compiler: unhandled pattern type")
	}
}

func compilePCon(c *Compiler, p *ir.PCon, failPatches *[]int) {
	argSlots := make([]int, len(p.Args))
	argNames := make([]string, len(p.Args))
	argGenerated := make([]bool, len(p.Args))
	for i, arg := range p.Args {
		name, gen := patternSlotInfo(arg, i)
		argSlots[i] = c.allocLocal(name)
		argNames[i] = name
		argGenerated[i] = gen
	}

	descIdx := c.addMatchDesc(MatchDesc{
		ConName:      p.Con,
		ArgSlots:     argSlots,
		ArgNames:     argNames,
		ArgGenerated: argGenerated,
	})

	pos := c.emitU16U16(OpMatchCon, descIdx, 0)
	if failPatches != nil {
		*failPatches = append(*failPatches, pos)
	}

	for i, arg := range p.Args {
		if isIrrefutableBinding(arg) {
			continue
		}
		c.emitU16(OpLoadLocal, uint16(argSlots[i]))
		compilePatternCollect(c, arg, failPatches)
	}
}

func compilePLit(c *Compiler, p *ir.PLit, failPatches *[]int) {
	litIdx := c.addConstant(&eval.HostVal{Inner: p.Value})
	pos := c.emitU16U16(OpMatchLit, litIdx, 0)
	if failPatches != nil {
		*failPatches = append(*failPatches, pos)
	}
}

func compilePRecord(c *Compiler, p *ir.PRecord, failPatches *[]int) {
	labels := make([]string, len(p.Fields))
	fieldSlots := make([]int, len(p.Fields))
	for i, f := range p.Fields {
		labels[i] = f.Label
		name, _ := patternSlotInfo(f.Pattern, i)
		fieldSlots[i] = c.allocLocal(name)
	}

	descIdx := c.addMatchDesc(MatchDesc{
		Labels:     labels,
		FieldSlots: fieldSlots,
	})

	pos := c.emitU16U16(OpMatchRecord, descIdx, 0)
	if failPatches != nil {
		*failPatches = append(*failPatches, pos)
	}

	for i, f := range p.Fields {
		if isIrrefutableBinding(f.Pattern) {
			continue
		}
		c.emitU16(OpLoadLocal, uint16(fieldSlots[i]))
		compilePatternCollect(c, f.Pattern, failPatches)
	}
}

// --- helpers ---

func patternSlotInfo(pat ir.Pattern, idx int) (string, bool) {
	switch p := pat.(type) {
	case *ir.PVar:
		return p.Name, p.Generated
	default:
		return fmt.Sprintf("$match_%d", idx), true
	}
}

func isIrrefutableBinding(pat ir.Pattern) bool {
	switch pat.(type) {
	case *ir.PVar, *ir.PWild:
		return true
	default:
		return false
	}
}
