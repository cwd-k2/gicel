package ir

import (
	"fmt"
	"sort"
	"strings"
)

// VerifyError represents a structural invariant violation in Core IR.
type VerifyError struct {
	Node    Core
	Message string
}

func (e *VerifyError) Error() string {
	return fmt.Sprintf("IR verify: %s", e.Message)
}

// VerifyProgram checks structural invariants of a Core IR program.
// Returns a list of violations. An empty list means the program is sound
// with respect to the checked invariants.
//
// Checked invariants:
//   - V1: No ir.Error nodes survive to post-check.
//   - V2: Auto-force generated Bind has structure: Bind{Generated:true, Comp: Force{Var{$lz...}}}.
//   - V4a: No double-Force: Force{Expr: Force{...}}.
//   - V4b: No double-Thunk: Thunk{Comp: Thunk{...}} (generalizes former V3).
func VerifyProgram(prog *Program) []VerifyError {
	var errs []VerifyError
	for _, b := range prog.Bindings {
		errs = verifyCore(b.Expr, errs)
	}
	return errs
}

func verifyCore(c Core, errs []VerifyError) []VerifyError {
	Walk(c, func(node Core) bool {
		switch n := node.(type) {
		case *Error:
			// V1: Error nodes should not survive to post-check.
			errs = append(errs, VerifyError{
				Node:    n,
				Message: "ir.Error node present in post-check IR",
			})
		case *Bind:
			errs = verifyBind(n, errs)
		case *Force:
			errs = verifyForce(n, errs)
		case *Thunk:
			errs = verifyThunk(n, errs)
		}
		return true
	})
	return errs
}

// verifyBind checks V2: auto-force generated Bind structure.
func verifyBind(b *Bind, errs []VerifyError) []VerifyError {
	if !b.Generated {
		return errs
	}
	// Generated Bind from autoForceLazy has Comp = Force{Expr: Var{Name: "$lz..."}}.
	force, ok := b.Comp.(*Force)
	if !ok {
		// Generated Bind with non-Force comp — may be from other compiler passes
		// (e.g. dict extraction). Only flag if it looks like a botched auto-force.
		return errs
	}
	v, ok := force.Expr.(*Var)
	if !ok {
		errs = append(errs, VerifyError{
			Node:    b,
			Message: "generated Bind{Force{...}} — Force argument is not a Var",
		})
		return errs
	}
	if !strings.HasPrefix(v.Name, "$lz") {
		errs = append(errs, VerifyError{
			Node:    b,
			Message: fmt.Sprintf("generated Bind{Force{Var{%q}}} — expected $lz prefix", v.Name),
		})
	}
	return errs
}

// verifyForce checks V4a: no double-Force.
func verifyForce(f *Force, errs []VerifyError) []VerifyError {
	if _, ok := f.Expr.(*Force); ok {
		errs = append(errs, VerifyError{
			Node:    f,
			Message: "double Force: Force{Expr: Force{...}}",
		})
	}
	return errs
}

// verifyThunk checks V4b: no double-Thunk anywhere.
func verifyThunk(th *Thunk, errs []VerifyError) []VerifyError {
	if _, ok := th.Comp.(*Thunk); ok {
		errs = append(errs, VerifyError{
			Node:    th,
			Message: "double Thunk: Thunk{Comp: Thunk{...}}",
		})
	}
	return errs
}

// VerifyAnnotations checks annotation-layer invariants of a Core IR program.
// Must be called after AnnotateFreeVarsProgram and AssignIndicesProgram.
//
// Checked invariants:
//   - V5a: Lam.FV, Thunk.FV, Merge.LeftFV/RightFV match recomputed free variables.
//   - V5b: Every Var node has a non-empty Key after annotation.
func VerifyAnnotations(prog *Program) []VerifyError {
	var errs []VerifyError
	for _, b := range prog.Bindings {
		errs = verifyAnnotationsCore(b.Expr, errs)
	}
	return errs
}

func verifyAnnotationsCore(c Core, errs []VerifyError) []VerifyError {
	Walk(c, func(node Core) bool {
		switch n := node.(type) {
		case *Var:
			// V5b: Var.Key must be populated after AnnotateFreeVars.
			if n.Key == "" {
				errs = append(errs, VerifyError{
					Node:    n,
					Message: fmt.Sprintf("Var{%q}.Key is empty after annotation", n.Name),
				})
			}
		case *Lam:
			// V5a: Lam.FV coherence.
			errs = verifyFVCoherence(n, n.FV, "Lam", n.Param, errs)
		case *Thunk:
			// V5a: Thunk.FV coherence.
			errs = verifyFVCoherenceThunk(n, errs)
		case *Merge:
			// V5a: Merge.LeftFV/RightFV coherence.
			errs = verifyFVCoherenceMerge(n, errs)
		}
		return true
	})
	return errs
}

// verifyFVCoherence checks that a Lam node's FV annotation matches a
// recomputed free variable set. Skips if FV is nil (depth overflow).
func verifyFVCoherence(node Core, annotatedFV []string, kind, param string, errs []VerifyError) []VerifyError {
	if annotatedFV == nil {
		return errs // overflow — skip
	}
	lam := node.(*Lam)
	computed := computeFV(lam.Body, 0)
	if computed.overflow {
		return errs
	}
	computed.delete(param)
	expected := sortedKeys(computed.vars)
	if !sliceEqual(annotatedFV, expected) {
		errs = append(errs, VerifyError{
			Node:    node,
			Message: fmt.Sprintf("%s.FV mismatch: annotated %v, computed %v", kind, annotatedFV, expected),
		})
	}
	return errs
}

func verifyFVCoherenceThunk(th *Thunk, errs []VerifyError) []VerifyError {
	if th.FV == nil {
		return errs
	}
	computed := computeFV(th.Comp, 0)
	if computed.overflow {
		return errs
	}
	expected := sortedKeys(computed.vars)
	if !sliceEqual(th.FV, expected) {
		errs = append(errs, VerifyError{
			Node:    th,
			Message: fmt.Sprintf("Thunk.FV mismatch: annotated %v, computed %v", th.FV, expected),
		})
	}
	return errs
}

func verifyFVCoherenceMerge(m *Merge, errs []VerifyError) []VerifyError {
	if m.LeftFV != nil {
		computed := computeFV(m.Left, 0)
		if !computed.overflow {
			expected := sortedKeys(computed.vars)
			if !sliceEqual(m.LeftFV, expected) {
				errs = append(errs, VerifyError{
					Node:    m,
					Message: fmt.Sprintf("Merge.LeftFV mismatch: annotated %v, computed %v", m.LeftFV, expected),
				})
			}
		}
	}
	if m.RightFV != nil {
		computed := computeFV(m.Right, 0)
		if !computed.overflow {
			expected := sortedKeys(computed.vars)
			if !sliceEqual(m.RightFV, expected) {
				errs = append(errs, VerifyError{
					Node:    m,
					Message: fmt.Sprintf("Merge.RightFV mismatch: annotated %v, computed %v", m.RightFV, expected),
				})
			}
		}
	}
	return errs
}

// computeFV mirrors annotateFV's logic in read-only mode: computes free variables
// without mutating any IR node. Used by V5a to independently verify FV annotations.
func computeFV(c Core, depth int) fvResult {
	if depth > maxTraversalDepth {
		return fvOverflow
	}
	switch n := c.(type) {
	case *Var:
		key := n.Key
		if key == "" {
			key = varKey(n)
		}
		return fvResult{vars: map[string]struct{}{key: {}}}
	case *Lam:
		bodyFV := computeFV(n.Body, depth+1)
		if bodyFV.overflow {
			return fvOverflow
		}
		bodyFV.delete(n.Param)
		return bodyFV
	case *App:
		return computeFVLeftSpine(n, depth)
	case *TyApp:
		return computeFV(n.Expr, depth+1)
	case *TyLam:
		return computeFV(n.Body, depth+1)
	case *Con:
		var result fvResult
		for _, arg := range n.Args {
			result = mergeFV(result, computeFV(arg, depth+1))
		}
		return result
	case *Case:
		result := computeFV(n.Scrutinee, depth+1)
		for _, alt := range n.Alts {
			altFV := computeFV(alt.Body, depth+1)
			for _, name := range alt.Pattern.Bindings() {
				altFV.delete(name)
			}
			result = mergeFV(result, altFV)
		}
		return result
	case *Fix:
		result := computeFV(n.Body, depth+1)
		result.delete(n.Name)
		return result
	case *Pure:
		return computeFV(n.Expr, depth+1)
	case *Bind:
		compFV := computeFV(n.Comp, depth+1)
		bodyFV := computeFV(n.Body, depth+1)
		bodyFV.delete(n.Var)
		return mergeFV(compFV, bodyFV)
	case *Thunk:
		return computeFV(n.Comp, depth+1)
	case *Force:
		return computeFV(n.Expr, depth+1)
	case *Merge:
		return mergeFV(computeFV(n.Left, depth+1), computeFV(n.Right, depth+1))
	case *PrimOp:
		var result fvResult
		for _, arg := range n.Args {
			result = mergeFV(result, computeFV(arg, depth+1))
		}
		return result
	case *Lit:
		return fvResult{}
	case *Error:
		return fvResult{}
	case *RecordLit:
		var result fvResult
		for _, f := range n.Fields {
			result = mergeFV(result, computeFV(f.Value, depth+1))
		}
		return result
	case *RecordProj:
		return computeFV(n.Record, depth+1)
	case *RecordUpdate:
		result := computeFV(n.Record, depth+1)
		for _, f := range n.Updates {
			result = mergeFV(result, computeFV(f.Value, depth+1))
		}
		return result
	default:
		panic(fmt.Sprintf("computeFV: unhandled Core node %T", c))
	}
}

func computeFVLeftSpine(app *App, depth int) fvResult {
	var rights []Core
	cur := Core(app)
	for {
		switch n := cur.(type) {
		case *App:
			rights = append(rights, n.Arg)
			cur = n.Fun
			continue
		case *TyApp:
			cur = n.Expr
			continue
		case *TyLam:
			cur = n.Body
			continue
		default:
		}
		break
	}
	result := computeFV(cur, depth+1)
	for i := len(rights) - 1; i >= 0; i-- {
		result = mergeFV(result, computeFV(rights[i], depth+1))
	}
	return result
}

func sortedKeys(s map[string]struct{}) []string {
	if len(s) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(s))
	for k := range s {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
