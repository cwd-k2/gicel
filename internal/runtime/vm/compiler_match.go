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
	// failJumps[i] = patch position for alt i's fail jump.
	failJumps := make([]int, numAlts)
	// endJumps collects patch positions for "jump to end" after each alt body.
	var endJumps []int

	for i, alt := range cs.Alts {
		savedLocals := len(e.locals)

		if i < numAlts-1 {
			// Not the last alternative: compile pattern with fail jump.
			compilePattern(e, alt.Pattern, &failJumps[i])
		} else {
			// Last alternative: fail goes to MATCH_FAIL.
			compilePattern(e, alt.Pattern, nil)
		}

		// Compile body.
		isLast := i == numAlts-1
		e.compileExpr(alt.Body, tail && isLast)

		// Pop pattern-bound locals.
		e.popLocals(savedLocals)

		if !isLast && !tail {
			// Jump to end after body.
			endJumps = append(endJumps, e.emitJump(OpJump))
		} else if !isLast && tail {
			// Tail position: body already returned (TAIL_APPLY/RETURN).
			// But we still need the jump for non-tail bodies that fall through.
			endJumps = append(endJumps, e.emitJump(OpJump))
		}

		if i < numAlts-1 {
			// Patch fail jump to here (start of next alternative).
			e.patchJumpTo(failJumps[i])
		}
	}

	// If the last alt's pattern can fail (not a wildcard/var), emit MATCH_FAIL.
	if numAlts > 0 {
		lastPat := cs.Alts[numAlts-1].Pattern
		if !isIrrefutablePattern(lastPat) {
			e.emit(OpMatchFail)
		}
	} else {
		e.emit(OpMatchFail)
	}

	// Patch all end jumps.
	for _, pos := range endJumps {
		e.patchJumpTo(pos)
	}
}

// compilePattern compiles a single pattern test. The value to match is on TOS.
// On match: extracts bindings into local slots, pops TOS.
// On fail: jumps to *failPatch (if non-nil) or falls through to MATCH_FAIL.
func compilePattern(e *emitter, pat ir.Pattern, failPatch *int) {
	switch p := pat.(type) {
	case *ir.PVar:
		// Variable pattern: always matches, bind the value.
		slot := e.allocLocal(p.Name)
		e.emitU16(OpStoreLocal, uint16(slot))

	case *ir.PWild:
		// Wildcard: always matches, discard.
		e.emit(OpPop)

	case *ir.PCon:
		compilePCon(e, p, failPatch)

	case *ir.PLit:
		compilePLit(e, p, failPatch)

	case *ir.PRecord:
		compilePRecord(e, p, failPatch)

	default:
		panic("vm/compiler: unhandled pattern type")
	}
}

// compilePCon compiles a constructor pattern.
// TOS = scrutinee. On match: args are stored in local slots.
func compilePCon(e *emitter, p *ir.PCon, failPatch *int) {
	// Allocate slots for constructor args.
	argSlots := make([]int, len(p.Args))
	for i, arg := range p.Args {
		// Each arg gets a temporary slot; nested patterns will further test.
		name := patternSlotName(arg, i)
		argSlots[i] = e.allocLocal(name)
	}

	descIdx := e.addMatchDesc(MatchDesc{
		ConName:  p.Con,
		ArgSlots: argSlots,
	})

	if failPatch != nil {
		*failPatch = e.emitU16U16(OpMatchCon, descIdx, 0) // 0 = placeholder offset
	} else {
		// Last alt: fail offset points to MATCH_FAIL which follows.
		e.emitU16U16(OpMatchCon, descIdx, 0)
	}

	// Compile sub-patterns for nested matching.
	for i, arg := range p.Args {
		if isIrrefutableBinding(arg) {
			continue // PVar/PWild already handled by MATCH_CON's arg extraction.
		}
		// Load the extracted arg and match further.
		e.emitU16(OpLoadLocal, uint16(argSlots[i]))
		compilePattern(e, arg, failPatch)
	}
}

// compilePLit compiles a literal pattern.
func compilePLit(e *emitter, p *ir.PLit, failPatch *int) {
	litIdx := e.addConstant(&eval.HostVal{Inner: p.Value})
	if failPatch != nil {
		*failPatch = e.emitU16U16(OpMatchLit, litIdx, 0)
	} else {
		e.emitU16U16(OpMatchLit, litIdx, 0)
	}
}

// compilePRecord compiles a record pattern.
func compilePRecord(e *emitter, p *ir.PRecord, failPatch *int) {
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

	if failPatch != nil {
		*failPatch = e.emitU16U16(OpMatchRecord, descIdx, 0)
	} else {
		e.emitU16U16(OpMatchRecord, descIdx, 0)
	}

	// Sub-patterns.
	for i, f := range p.Fields {
		if isIrrefutableBinding(f.Pattern) {
			continue
		}
		e.emitU16(OpLoadLocal, uint16(fieldSlots[i]))
		compilePattern(e, f.Pattern, failPatch)
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
