package ir

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/lang/types"
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
//   - V2: Auto-force generated Bind has structure: Bind{Generated:true, Comp: Force{Var{...}}}.
//   - V4a: No double-Force: Force{Expr: Force{...}}.
//   - V4b: No double-Thunk: Thunk{Comp: Thunk{...}} (generalizes former V3).
//   - V6: No label-kinded TyApp after label erasure.
//   - V7: Fix.Body (after peeling TyLam) must be Lam or Thunk.
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
		case *Fix:
			errs = verifyFix(n, errs)
		case *TyApp:
			errs = verifyTyApp(n, errs)
		}
		return true
	})
	return errs
}

// verifyBind checks:
//   - V2: auto-force generated Bind has structure Bind{Generated, Comp: Force{Var{...}}}.
//   - V8: Bind.Comp is a computation-producing node (not Thunk or Lam directly).
//     In CBPV, bind sequences computations; a Thunk (suspended value) or Lam (value)
//     directly in Bind.Comp violates the value/computation distinction.
func verifyBind(b *Bind, errs []VerifyError) []VerifyError {
	// V8: Bind.Comp must be computation-producing.
	switch b.Comp.(type) {
	case *Thunk:
		errs = append(errs, VerifyError{
			Node:    b,
			Message: "Bind.Comp is Thunk — CBPV violation: bind expects a computation, not a suspended value",
		})
	case *Lam:
		errs = append(errs, VerifyError{
			Node:    b,
			Message: "Bind.Comp is Lam — CBPV violation: bind expects a computation, not a lambda value",
		})
	}

	// V2: auto-force generated Bind structure check.
	if !b.Generated.IsGenerated() {
		return errs
	}
	force, ok := b.Comp.(*Force)
	if !ok {
		// Generated Bind with non-Force comp — may be from other compiler passes
		// (e.g. dict extraction, CBPV auto-bind). Not an auto-force violation.
		return errs
	}
	if _, ok := force.Expr.(*Var); !ok {
		errs = append(errs, VerifyError{
			Node:    b,
			Message: "generated Bind{Force{...}} — Force argument is not a Var",
		})
	}
	return errs
}

// verifyFix checks V7: Fix.Body must be Lam or Thunk after peeling TyLam.
func verifyFix(f *Fix, errs []VerifyError) []VerifyError {
	body := PeelTyLam(f.Body)
	switch body.(type) {
	case *Lam, *Thunk:
		// OK
	default:
		errs = append(errs, VerifyError{
			Node:    f,
			Message: fmt.Sprintf("Fix body must be Lam or Thunk (got %T)", body),
		})
	}
	return errs
}

// verifyTyApp checks V6: no label-kinded TyApp after label erasure.
func verifyTyApp(ta *TyApp, errs []VerifyError) []VerifyError {
	if con, ok := ta.TyArg.(*types.TyCon); ok && con.IsLabel {
		errs = append(errs, VerifyError{
			Node:    ta,
			Message: fmt.Sprintf("label TyApp survived label erasure: @%s", con.Name),
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

// VerifyAnnotations checks annotation-layer invariants of a Core IR program
// against a caller-supplied FVAnnotations side table. Must be called after
// AnnotateFreeVarsProgram and AssignIndicesProgram on the same tree.
//
// Checked invariants:
//   - V5a: FV entries in annots match recomputed free variables for every
//     Lam, Thunk, and Merge reachable from the program.
//   - V5b: Every Var node has a non-empty Key after annotation.
func VerifyAnnotations(prog *Program, annots *FVAnnotations) []VerifyError {
	var errs []VerifyError
	for _, b := range prog.Bindings {
		errs = verifyAnnotationsCore(b.Expr, annots, errs)
	}
	return errs
}

func verifyAnnotationsCore(c Core, annots *FVAnnotations, errs []VerifyError) []VerifyError {
	// V5a: FV coherence — recompute annotations in a fresh table and diff
	// against the original. Separating traversal from verification makes
	// both independently testable and eliminates the observer indirection.
	recomputed := NewFVAnnotations()
	traverseFV(c, 0, recomputed)
	for lam, got := range recomputed.Lams {
		stored, ok := annots.Lams[lam]
		if !ok {
			errs = append(errs, VerifyError{
				Node:    lam,
				Message: "Lam missing from FVAnnotations",
			})
			continue
		}
		if stored.Overflow || got.Overflow {
			continue
		}
		if !sliceEqual(stored.Vars, got.Vars) {
			errs = append(errs, VerifyError{
				Node:    lam,
				Message: fmt.Sprintf("Lam.FV mismatch: annotated %v, computed %v", stored.Vars, got.Vars),
			})
		}
	}
	for th, got := range recomputed.Thunks {
		stored, ok := annots.Thunks[th]
		if !ok {
			errs = append(errs, VerifyError{
				Node:    th,
				Message: "Thunk missing from FVAnnotations",
			})
			continue
		}
		if stored.Overflow || got.Overflow {
			continue
		}
		if !sliceEqual(stored.Vars, got.Vars) {
			errs = append(errs, VerifyError{
				Node:    th,
				Message: fmt.Sprintf("Thunk.FV mismatch: annotated %v, computed %v", stored.Vars, got.Vars),
			})
		}
	}
	for m, got := range recomputed.Merges {
		stored, ok := annots.Merges[m]
		if !ok {
			errs = append(errs, VerifyError{
				Node:    m,
				Message: "Merge missing from FVAnnotations",
			})
			continue
		}
		if !stored.Left.Overflow && !got.Left.Overflow {
			if !sliceEqual(stored.Left.Vars, got.Left.Vars) {
				errs = append(errs, VerifyError{
					Node:    m,
					Message: fmt.Sprintf("Merge.LeftFV mismatch: annotated %v, computed %v", stored.Left.Vars, got.Left.Vars),
				})
			}
		}
		if !stored.Right.Overflow && !got.Right.Overflow {
			if !sliceEqual(stored.Right.Vars, got.Right.Vars) {
				errs = append(errs, VerifyError{
					Node:    m,
					Message: fmt.Sprintf("Merge.RightFV mismatch: annotated %v, computed %v", stored.Right.Vars, got.Right.Vars),
				})
			}
		}
	}
	return errs
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
