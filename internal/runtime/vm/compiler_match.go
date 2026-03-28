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
func compilePatternMatch(e *emitter, cs *ir.Case, tail bool) {
	numAlts := len(cs.Alts)
	var endJumps []int

	// Save scrutinee in a local slot so sub-patterns can reload it on failure.
	scrutSlot := e.allocLocal("$scrut")
	e.emitU16(OpStoreLocal, uint16(scrutSlot))

	allFailPatches := make([][]int, numAlts)

	for i, alt := range cs.Alts {
		savedLocals := len(e.locals)

		// Load scrutinee onto stack for this alt's pattern.
		e.emitU16(OpLoadLocal, uint16(scrutSlot))

		var failPatches []int
		compilePatternCollect(e, alt.Pattern, &failPatches)
		allFailPatches[i] = failPatches

		// Compile body.
		isLast := i == numAlts-1
		bodyTail := tail && isLast
		e.compileExpr(alt.Body, bodyTail)

		// Pop pattern-bound locals.
		e.popLocals(savedLocals)

		if isLast {
			// Last alt: no jump needed (falls through to MATCH_FAIL or RETURN).
			// But if the body didn't end with a tail call, we need to jump past MATCH_FAIL.
			endJumps = append(endJumps, e.emitJump(OpJump))
		} else {
			endJumps = append(endJumps, e.emitJump(OpJump))
		}

		if i < numAlts-1 {
			// Patch fail offsets to here (start of next alternative).
			target := uint16(len(e.code))
			for _, pos := range failPatches {
				EncodeU16(e.code[pos+3:], target)
			}
		}
	}

	// Emit MATCH_FAIL for non-exhaustive matches.
	// Patch last alt's fail offsets to point here.
	matchFailPos := len(e.code)
	e.emit(OpMatchFail)
	if numAlts > 0 {
		for _, pos := range allFailPatches[numAlts-1] {
			EncodeU16(e.code[pos+3:], uint16(matchFailPos))
		}
	}

	// Patch all end jumps.
	for _, pos := range endJumps {
		e.patchJumpTo(pos)
	}
}

// compilePatternCollect compiles a single pattern test. The value to match is on TOS.
// On match: extracts bindings into local slots, pops TOS.
// On fail: emits MATCH_* with placeholder fail offset. If failPatches is non-nil,
// the positions of all emitted MATCH_* instructions are appended for later patching.
func compilePatternCollect(e *emitter, pat ir.Pattern, failPatches *[]int) {
	switch p := pat.(type) {
	case *ir.PVar:
		slot := e.allocLocal(p.Name)
		e.emitU16(OpStoreLocal, uint16(slot))

	case *ir.PWild:
		e.emit(OpPop)

	case *ir.PCon:
		compilePCon(e, p, failPatches)

	case *ir.PLit:
		compilePLit(e, p, failPatches)

	case *ir.PRecord:
		compilePRecord(e, p, failPatches)

	default:
		panic("vm/compiler: unhandled pattern type")
	}
}

// compilePCon compiles a constructor pattern.
// TOS = scrutinee. On match: args are stored in local slots.
func compilePCon(e *emitter, p *ir.PCon, failPatches *[]int) {
	argSlots := make([]int, len(p.Args))
	for i, arg := range p.Args {
		name := patternSlotName(arg, i)
		argSlots[i] = e.allocLocal(name)
	}

	descIdx := e.addMatchDesc(MatchDesc{
		ConName:  p.Con,
		ArgSlots: argSlots,
	})

	pos := e.emitU16U16(OpMatchCon, descIdx, 0) // 0 = placeholder
	if failPatches != nil {
		*failPatches = append(*failPatches, pos)
	}

	// Compile sub-patterns for nested matching.
	for i, arg := range p.Args {
		if isIrrefutableBinding(arg) {
			continue
		}
		e.emitU16(OpLoadLocal, uint16(argSlots[i]))
		compilePatternCollect(e, arg, failPatches)
	}
}

// compilePLit compiles a literal pattern.
func compilePLit(e *emitter, p *ir.PLit, failPatches *[]int) {
	litIdx := e.addConstant(&eval.HostVal{Inner: p.Value})
	pos := e.emitU16U16(OpMatchLit, litIdx, 0)
	if failPatches != nil {
		*failPatches = append(*failPatches, pos)
	}
}

// compilePRecord compiles a record pattern.
func compilePRecord(e *emitter, p *ir.PRecord, failPatches *[]int) {
	labels := make([]string, len(p.Fields))
	fieldSlots := make([]int, len(p.Fields))
	for i, f := range p.Fields {
		labels[i] = f.Label
		name := patternSlotName(f.Pattern, i)
		fieldSlots[i] = e.allocLocal(name)
	}

	descIdx := e.addMatchDesc(MatchDesc{
		Labels:     labels,
		FieldSlots: fieldSlots,
	})

	pos := e.emitU16U16(OpMatchRecord, descIdx, 0)
	if failPatches != nil {
		*failPatches = append(*failPatches, pos)
	}

	for i, f := range p.Fields {
		if isIrrefutableBinding(f.Pattern) {
			continue
		}
		e.emitU16(OpLoadLocal, uint16(fieldSlots[i]))
		compilePatternCollect(e, f.Pattern, failPatches)
	}
}

// --- helpers ---

// patternSlotName returns a name for a pattern variable slot.
func patternSlotName(pat ir.Pattern, idx int) string {
	switch p := pat.(type) {
	case *ir.PVar:
		return p.Name
	default:
		// Synthesize a name for intermediate slots.
		return fmt.Sprintf("$match_%d", idx)
	}
}

// isIrrefutablePattern returns true if the pattern always matches.
func isIrrefutablePattern(pat ir.Pattern) bool {
	switch pat.(type) {
	case *ir.PVar, *ir.PWild:
		return true
	default:
		return false
	}
}

// isIrrefutableBinding returns true if the pattern is a simple variable or wildcard
// (no further matching needed beyond the initial extraction).
func isIrrefutableBinding(pat ir.Pattern) bool {
	switch pat.(type) {
	case *ir.PVar, *ir.PWild:
		return true
	default:
		return false
	}
}
