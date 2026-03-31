package ir

import (
	"fmt"
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
	obs := &verifyObs{}
	traverseFV(c, 0, obs)
	errs = append(errs, obs.errs...)
	return errs
}

// verifyObs checks FV annotations at each annotation point by comparing
// the stored annotation with the recomputed free variable set.
type verifyObs struct {
	errs []VerifyError
}

func (v *verifyObs) OnLam(lam *Lam, bodyFV fvResult) {
	if lam.FV == nil || bodyFV.overflow {
		return
	}
	expected := setToSlice(bodyFV.vars)
	if !sliceEqual(lam.FV, expected) {
		v.errs = append(v.errs, VerifyError{
			Node:    lam,
			Message: fmt.Sprintf("Lam.FV mismatch: annotated %v, computed %v", lam.FV, expected),
		})
	}
}

func (v *verifyObs) OnThunk(th *Thunk, compFV fvResult) {
	if th.FV == nil || compFV.overflow {
		return
	}
	expected := setToSlice(compFV.vars)
	if !sliceEqual(th.FV, expected) {
		v.errs = append(v.errs, VerifyError{
			Node:    th,
			Message: fmt.Sprintf("Thunk.FV mismatch: annotated %v, computed %v", th.FV, expected),
		})
	}
}

func (v *verifyObs) OnMerge(m *Merge, leftFV, rightFV fvResult) {
	if m.LeftFV != nil && !leftFV.overflow {
		expected := setToSlice(leftFV.vars)
		if !sliceEqual(m.LeftFV, expected) {
			v.errs = append(v.errs, VerifyError{
				Node:    m,
				Message: fmt.Sprintf("Merge.LeftFV mismatch: annotated %v, computed %v", m.LeftFV, expected),
			})
		}
	}
	if m.RightFV != nil && !rightFV.overflow {
		expected := setToSlice(rightFV.vars)
		if !sliceEqual(m.RightFV, expected) {
			v.errs = append(v.errs, VerifyError{
				Node:    m,
				Message: fmt.Sprintf("Merge.RightFV mismatch: annotated %v, computed %v", m.RightFV, expected),
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
