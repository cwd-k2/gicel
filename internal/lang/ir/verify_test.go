// IR verifier tests — structural invariant checks.
// Does NOT cover: type-level verification (future work).
package ir

import (
	"strings"
	"testing"
)

func TestVerifyV1_ErrorNodeDetected(t *testing.T) {
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Error{}},
		},
	}
	errs := VerifyProgram(prog)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Message, "ir.Error") {
		t.Fatalf("expected ir.Error message, got %q", errs[0].Message)
	}
}

func TestVerifyV1_NestedErrorNode(t *testing.T) {
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &App{Fun: &Var{Name: "f"}, Arg: &Error{}}},
		},
	}
	errs := VerifyProgram(prog)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
}

func TestVerifyV2_ValidAutoForce(t *testing.T) {
	// Valid: Bind{Generated: true, Comp: Force{Var{$lz_0}}}
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Bind{
				Generated: true,
				Comp:      &Force{Expr: &Var{Name: "$lz_0"}},
				Var:       "x",
				Body:      &Var{Name: "x"},
			}},
		},
	}
	errs := VerifyProgram(prog)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors for valid auto-force, got %d: %v", len(errs), errs)
	}
}

func TestVerifyV2_ForceNonVar(t *testing.T) {
	// Invalid: Bind{Generated: true, Comp: Force{Lit{42}}}
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Bind{
				Generated: true,
				Comp:      &Force{Expr: &Lit{Value: int64(42)}},
				Var:       "x",
				Body:      &Var{Name: "x"},
			}},
		},
	}
	errs := VerifyProgram(prog)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Message, "not a Var") {
		t.Fatalf("expected 'not a Var' message, got %q", errs[0].Message)
	}
}

func TestVerifyV2_ForceBadPrefix(t *testing.T) {
	// Invalid: Bind{Generated: true, Comp: Force{Var{x}}} — not $lz prefix
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Bind{
				Generated: true,
				Comp:      &Force{Expr: &Var{Name: "x"}},
				Var:       "y",
				Body:      &Var{Name: "y"},
			}},
		},
	}
	errs := VerifyProgram(prog)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Message, "$lz prefix") {
		t.Fatalf("expected '$lz prefix' message, got %q", errs[0].Message)
	}
}

func TestVerifyV3_DoubleThunk(t *testing.T) {
	// Invalid: App{Arg: Thunk{Comp: Thunk{...}}}
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &App{
				Fun: &Var{Name: "f"},
				Arg: &Thunk{Comp: &Thunk{Comp: &Var{Name: "x"}}},
			}},
		},
	}
	errs := VerifyProgram(prog)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Message, "double Thunk") {
		t.Fatalf("expected 'double Thunk' message, got %q", errs[0].Message)
	}
}

func TestVerifyCleanProgram(t *testing.T) {
	// Valid program with no violations.
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Pure{Expr: &Lit{Value: int64(42)}}},
			{Name: "f", Expr: &Lam{
				Param: "x",
				Body: &Bind{
					Comp: &Force{Expr: &Var{Name: "t"}},
					Var:  "v",
					Body: &Pure{Expr: &Var{Name: "v"}},
				},
			}},
		},
	}
	errs := VerifyProgram(prog)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}
