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
//   - V3: No double-thunk in lazy constructor arguments: App{Arg: Thunk{Comp: Thunk{...}}}.
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
		case *App:
			errs = verifyApp(n, errs)
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

// verifyApp checks V3: no double-thunk in constructor arguments.
func verifyApp(a *App, errs []VerifyError) []VerifyError {
	thunk, ok := a.Arg.(*Thunk)
	if !ok {
		return errs
	}
	if _, ok := thunk.Comp.(*Thunk); ok {
		errs = append(errs, VerifyError{
			Node:    a,
			Message: "double Thunk in App argument: App{Arg: Thunk{Comp: Thunk{...}}}",
		})
	}
	return errs
}
