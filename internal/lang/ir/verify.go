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
		case *TyApp:
			errs = verifyTyApp(n, errs)
		}
		return true
	})
	return errs
}

// verifyBind checks V2: auto-force generated Bind structure.
func verifyBind(b *Bind, errs []VerifyError) []VerifyError {
	if !b.Generated.IsGenerated() {
		return errs
	}
	// Generated Bind from autoForceLazy has Comp = Force{Expr: Var{...}}.
	// The structural pattern (Generated + Force + Var) is the invariant;
	// the Var's name is an implementation detail of autoForceLazy.
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
	// V5b: Var.Key must be populated after AnnotateFreeVars.
	Walk(c, func(node Core) bool {
		if v, ok := node.(*Var); ok && v.Key == "" {
			errs = append(errs, VerifyError{
				Node:    v,
				Message: fmt.Sprintf("Var{%q}.Key is empty after annotation", v.Name),
			})
		}
		return true
	})
	// V5a: FV coherence — single bottom-up pass via traverseFV (O(N)).
	obs := &verifyObs{annots: annots}
	traverseFV(c, 0, obs)
	errs = append(errs, obs.errs...)
	return errs
}

// verifyObs checks FV annotations at each annotation point by comparing
// the side-table entry with the recomputed free variable set.
type verifyObs struct {
	annots *FVAnnotations
	errs   []VerifyError
}

func (v *verifyObs) OnLam(lam *Lam, bodyFV fvResult) {
	info, ok := v.annots.Lams[lam]
	if !ok {
		v.errs = append(v.errs, VerifyError{
			Node:    lam,
			Message: "Lam missing from FVAnnotations",
		})
		return
	}
	if info.Overflow || bodyFV.overflow {
		return
	}
	expected := fvResultToSlice(bodyFV)
	if !sliceEqual(info.Vars, expected) {
		v.errs = append(v.errs, VerifyError{
			Node:    lam,
			Message: fmt.Sprintf("Lam.FV mismatch: annotated %v, computed %v", info.Vars, expected),
		})
	}
}

func (v *verifyObs) OnThunk(th *Thunk, compFV fvResult) {
	info, ok := v.annots.Thunks[th]
	if !ok {
		v.errs = append(v.errs, VerifyError{
			Node:    th,
			Message: "Thunk missing from FVAnnotations",
		})
		return
	}
	if info.Overflow || compFV.overflow {
		return
	}
	expected := fvResultToSlice(compFV)
	if !sliceEqual(info.Vars, expected) {
		v.errs = append(v.errs, VerifyError{
			Node:    th,
			Message: fmt.Sprintf("Thunk.FV mismatch: annotated %v, computed %v", info.Vars, expected),
		})
	}
}

func (v *verifyObs) OnMerge(m *Merge, leftFV, rightFV fvResult) {
	info, ok := v.annots.Merges[m]
	if !ok {
		v.errs = append(v.errs, VerifyError{
			Node:    m,
			Message: "Merge missing from FVAnnotations",
		})
		return
	}
	if !info.Left.Overflow && !leftFV.overflow {
		expected := fvResultToSlice(leftFV)
		if !sliceEqual(info.Left.Vars, expected) {
			v.errs = append(v.errs, VerifyError{
				Node:    m,
				Message: fmt.Sprintf("Merge.LeftFV mismatch: annotated %v, computed %v", info.Left.Vars, expected),
			})
		}
	}
	if !info.Right.Overflow && !rightFV.overflow {
		expected := fvResultToSlice(rightFV)
		if !sliceEqual(info.Right.Vars, expected) {
			v.errs = append(v.errs, VerifyError{
				Node:    m,
				Message: fmt.Sprintf("Merge.RightFV mismatch: annotated %v, computed %v", info.Right.Vars, expected),
			})
		}
	}
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
