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
	// Valid: Bind{Generated: GenAutoForce, Comp: Force{Var{$lz_0}}}
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Bind{
				Generated: GenAutoForce,
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
	// Invalid: Bind{Generated: GenAutoForce, Comp: Force{Lit{42}}}
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Bind{
				Generated: GenAutoForce,
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

func TestVerifyV2_ForceAnyVarName(t *testing.T) {
	// Valid: Bind{Generated: GenAutoForce, Comp: Force{Var{x}}} — the Var's name is
	// immaterial; the structural pattern (Generated + Force + Var) is the invariant.
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Bind{
				Generated: GenAutoForce,
				Comp:      &Force{Expr: &Var{Name: "x"}},
				Var:       "y",
				Body:      &Var{Name: "y"},
			}},
		},
	}
	errs := VerifyProgram(prog)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors for valid auto-force, got %d: %v", len(errs), errs)
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
	// An unannotated Var with Key == "" should trip the V5b check when
	// we hand VerifyAnnotations an empty side table (no traversal has
	// populated the key yet).
	prog := &Program{
		Bindings: []Binding{
			{Name: "main", Expr: &Var{Name: "x", Key: ""}},
		},
	}
	errs := VerifyAnnotations(prog, NewFVAnnotations())
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
	errs := VerifyAnnotations(prog, NewFVAnnotations())
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
					Comp: &Force{Expr: &Var{Name: "t"}},
					Var:  "v",
					Body: &Pure{Expr: &Var{Name: "v"}},
				},
			}},
		},
	}
	annots := AnnotateFreeVarsProgram(prog)
	errs := VerifyAnnotations(prog, annots)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

// V5a: FV coherence checks.

func TestVerifyV5a_LamFVCorrect(t *testing.T) {
	// Lam{param: "x", body: Var{y}} — annotate normally, FV = [y].
	prog := &Program{
		Bindings: []Binding{
			{Name: "f", Expr: &Lam{
				Param: "x",
				Body:  &Var{Name: "y"},
			}},
		},
	}
	annots := AnnotateFreeVarsProgram(prog)
	errs := VerifyAnnotations(prog, annots)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

func TestVerifyV5a_LamFVMismatch(t *testing.T) {
	// Annotate normally then inject a wrong FV into the side table.
	lam := &Lam{
		Param: "x",
		Body:  &Var{Name: "y"},
	}
	prog := &Program{Bindings: []Binding{{Name: "f", Expr: lam}}}
	annots := AnnotateFreeVarsProgram(prog)
	annots.LookupLam(lam).Vars = []string{"z"}

	errs := VerifyAnnotations(prog, annots)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "Lam.FV mismatch") {
		t.Fatalf("expected 'Lam.FV mismatch' message, got %q", errs[0].Message)
	}
}

func TestVerifyV5a_LamFVNilOverflow(t *testing.T) {
	// Overflow=true means the FV computation was truncated — the
	// coherence check should silently skip this node.
	lam := &Lam{
		Param: "x",
		Body:  &Var{Name: "y"},
	}
	prog := &Program{Bindings: []Binding{{Name: "f", Expr: lam}}}
	annots := AnnotateFreeVarsProgram(prog)
	annots.Lams[lam] = &FVInfo{Overflow: true}

	errs := VerifyAnnotations(prog, annots)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors for overflow, got %d: %v", len(errs), errs)
	}
}

func TestVerifyV5a_ThunkFVCorrect(t *testing.T) {
	prog := &Program{
		Bindings: []Binding{
			{Name: "t", Expr: &Thunk{
				Comp: &Pure{Expr: &Var{Name: "x"}},
			}},
		},
	}
	annots := AnnotateFreeVarsProgram(prog)
	errs := VerifyAnnotations(prog, annots)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

func TestVerifyV5a_ThunkFVMismatch(t *testing.T) {
	th := &Thunk{Comp: &Pure{Expr: &Var{Name: "x"}}}
	prog := &Program{Bindings: []Binding{{Name: "t", Expr: th}}}
	annots := AnnotateFreeVarsProgram(prog)
	annots.LookupThunk(th).Vars = []string{} // wrong: should be ["x"]

	errs := VerifyAnnotations(prog, annots)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "Thunk.FV mismatch") {
		t.Fatalf("expected 'Thunk.FV mismatch' message, got %q", errs[0].Message)
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
