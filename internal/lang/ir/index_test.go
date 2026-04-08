// Index assignment tests — de Bruijn local indices and global slot encoding.
// Does NOT cover: FV annotation (free_test.go via annotateFV).

package ir

import "testing"

// annotateAndAssign runs the full annotate→assign pipeline on c and
// returns the freshly computed FVAnnotations for inspection.
func annotateAndAssign(c Core) *FVAnnotations {
	annots := AnnotateFreeVars(c)
	AssignIndices(c, annots)
	return annots
}

func TestAssignIndicesNestedLam(t *testing.T) {
	// \x. \y. x → Var("x") gets Index 1, Var("y") would get 0 if referenced.
	xRef := &Var{Name: "x"}
	term := &Lam{
		Param: "x",
		Body: &Lam{
			Param: "y",
			Body:  xRef,
		},
	}
	annotateAndAssign(term)
	if xRef.Index != 1 {
		t.Errorf("Var(x) in \\x.\\y.x: want Index 1, got %d", xRef.Index)
	}
}

func TestAssignIndicesFix(t *testing.T) {
	// fix self in \x. self → self at Index 1, x at Index 0.
	selfRef := &Var{Name: "self"}
	lam := &Lam{
		Param: "x",
		Body:  selfRef,
	}
	term := &Fix{Name: "self", Body: lam}
	annotateAndAssign(term)
	if selfRef.Index != 1 {
		t.Errorf("Var(self) in fix self \\x. self: want Index 1, got %d", selfRef.Index)
	}
}

func TestAssignIndicesBind(t *testing.T) {
	// bind comp "x" (Var "x") → Var("x") in body gets Index 0.
	xRef := &Var{Name: "x"}
	term := &Bind{
		Comp: &Lit{Value: int64(42)},
		Var:  "x",
		Body: xRef,
	}
	annotateAndAssign(term)
	if xRef.Index != 0 {
		t.Errorf("Var(x) in bind body: want Index 0, got %d", xRef.Index)
	}
}

func TestAssignIndicesCasePatternVars(t *testing.T) {
	// case scrutinee of { Pair a b -> a }
	// Pattern bindings: [a, b] → a at Index 1, b at Index 0.
	aRef := &Var{Name: "a"}
	term := &Case{
		Scrutinee: &Con{Name: "Pair", Args: []Core{&Lit{Value: int64(1)}, &Lit{Value: int64(2)}}},
		Alts: []Alt{{
			Pattern: &PCon{Con: "Pair", Args: []Pattern{&PVar{Name: "a"}, &PVar{Name: "b"}}},
			Body:    aRef,
		}},
	}
	annotateAndAssign(term)
	if aRef.Index != 1 {
		t.Errorf("Var(a) in case alt body: want Index 1, got %d", aRef.Index)
	}
}

// Global slot encoding tests removed: assignGlobalSlots IR mutation
// was replaced by name-based resolution in the evaluator. Global Var
// nodes remain at Index == -1 permanently; the evaluator resolves
// them via globalSlots map at eval time.

func TestAssignIndicesPrimOpArgs(t *testing.T) {
	// PrimOp with Var args — after #1 fix, args must be traversed.
	xRef := &Var{Name: "x"}
	term := &Lam{
		Param: "x",
		Body:  &PrimOp{Name: "add", Arity: 2, Args: []Core{xRef, &Lit{Value: int64(1)}}},
	}
	annotateAndAssign(term)
	if xRef.Index != 0 {
		t.Errorf("Var(x) inside PrimOp.Args: want Index 0, got %d", xRef.Index)
	}
}

func TestAssignIndicesFVOverflow(t *testing.T) {
	// When FV overflow is reported, Indices stays nil and body sees
	// all enclosing locals. Force overflow by poking the annotation
	// table directly — the depth-limit machinery is hard to trigger
	// from a small synthetic tree.
	xRef := &Var{Name: "x"}
	innerLam := &Lam{
		Param: "y",
		Body:  xRef,
	}
	term := &Lam{
		Param: "outer",
		Body:  innerLam,
	}
	// Wrap in an outer scope that has "x".
	wrapper := &Lam{
		Param: "x",
		Body:  term,
	}
	annots := AnnotateFreeVars(wrapper)
	// Override the inner lam to simulate FV overflow.
	annots.Lams[innerLam] = &FVInfo{Overflow: true}
	AssignIndices(wrapper, annots)
	// With FV overflow, the inner lam body sees all enclosing locals.
	// The inner lam (Overflow=true) does not build a captured scope — it
	// passes enclosing scope + param directly. So x should be found in
	// the scope. Check that Indices remained nil (overflow path).
	info := annots.LookupLam(innerLam)
	if info.Indices != nil {
		t.Errorf("FV-overflow Lam: Indices should be nil, got %v", info.Indices)
	}
}

// TestFVAnnotations_Roundtrip verifies that AnnotateFreeVars →
// AssignIndices → VerifyAnnotations is a self-consistent cycle on a
// tree rich enough to exercise every annotation-point variant.
func TestFVAnnotations_Roundtrip(t *testing.T) {
	// \f. \x. f (thunk (f x))
	// Outer Lam f → FV = []
	// Inner Lam x → FV = [f]
	// Thunk       → FV = [f, x]
	expr := &Lam{
		Param: "f",
		Body: &Lam{
			Param: "x",
			Body: &App{
				Fun: &Var{Name: "f"},
				Arg: &Thunk{
					Comp: &App{
						Fun: &Var{Name: "f"},
						Arg: &Var{Name: "x"},
					},
				},
			},
		},
	}
	annots := AnnotateFreeVars(expr)
	AssignIndices(expr, annots)

	// Program wrapper so we can reuse VerifyAnnotations.
	prog := &Program{Bindings: []Binding{{Name: "main", Expr: expr}}}
	if errs := VerifyAnnotations(prog, annots); len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("verify: %s", e.Message)
		}
	}

	// Spot-check the inner Lam's FV — should be [f].
	innerLam := expr.Body.(*Lam)
	innerInfo := annots.LookupLam(innerLam)
	if innerInfo.Overflow {
		t.Fatalf("inner Lam unexpectedly overflow")
	}
	if len(innerInfo.Vars) != 1 || innerInfo.Vars[0] != "f" {
		t.Errorf("inner Lam FV: want [f], got %v", innerInfo.Vars)
	}
}
