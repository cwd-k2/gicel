// IR verifier tests — structural and annotation invariant checks.
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

// V4a: double-Force detection.

func TestVerifyV4a_DoubleForce(t *testing.T) {
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Force{Expr: &Force{Expr: &Var{Name: "x"}}}},
		},
	}
	errs := VerifyProgram(prog)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Message, "double Force") {
		t.Fatalf("expected 'double Force' message, got %q", errs[0].Message)
	}
}

func TestVerifyV4a_SingleForce(t *testing.T) {
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Force{Expr: &Var{Name: "t"}}},
		},
	}
	errs := VerifyProgram(prog)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

// V4b: double-Thunk detection (universal, generalizes former V3).

func TestVerifyV4b_DoubleThunkInApp(t *testing.T) {
	// Previously V3: App{Arg: Thunk{Comp: Thunk{...}}} — now caught by V4b on Thunk node.
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

func TestVerifyV4b_DoubleThunkStandalone(t *testing.T) {
	// Double-Thunk outside App context — V3 would miss this, V4b catches it.
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Thunk{Comp: &Thunk{Comp: &Pure{Expr: &Lit{Value: int64(1)}}}}},
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

func TestVerifyV4b_SingleThunk(t *testing.T) {
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Thunk{Comp: &Pure{Expr: &Lit{Value: int64(1)}}}},
		},
	}
	errs := VerifyProgram(prog)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

// V5b: Var.Key annotation check.

func TestVerifyV5b_VarKeyEmpty(t *testing.T) {
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Var{Name: "x", Key: ""}},
		},
	}
	errs := VerifyAnnotations(prog)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Message, "Key is empty") {
		t.Fatalf("expected 'Key is empty' message, got %q", errs[0].Message)
	}
}

func TestVerifyV5b_VarKeyPopulated(t *testing.T) {
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Var{Name: "x", Key: "x"}},
		},
	}
	errs := VerifyAnnotations(prog)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

func TestVerifyAnnotationsClean(t *testing.T) {
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Pure{Expr: &Lit{Value: int64(42)}}},
			{Name: "f", Expr: &Lam{
				Param: "x",
				Body: &Bind{
					Comp: &Force{Expr: &Var{Name: "t", Key: "t"}},
					Var:  "v",
					Body: &Pure{Expr: &Var{Name: "v", Key: "v"}},
				},
			}},
		},
	}
	errs := VerifyAnnotations(prog)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
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
