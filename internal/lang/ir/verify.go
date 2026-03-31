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
//   - V5b: Every Var node has a non-empty Key after annotation.
func VerifyAnnotations(prog *Program) []VerifyError {
	var errs []VerifyError
	for _, b := range prog.Bindings {
		Walk(b.Expr, func(node Core) bool {
			if v, ok := node.(*Var); ok {
				if v.Key == "" {
					errs = append(errs, VerifyError{
						Node:    v,
						Message: fmt.Sprintf("Var{%q}.Key is empty after annotation", v.Name),
					})
				}
			}
			return true
		})
	}
	return errs
}
