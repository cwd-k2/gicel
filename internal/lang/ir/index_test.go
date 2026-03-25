// Index assignment tests — de Bruijn local indices and global slot encoding.
// Does NOT cover: FV annotation (free_test.go via annotateFV).

package ir

import "testing"

func TestAssignIndicesNestedLam(t *testing.T) {
	// \x. \y. x → Var("x") gets Index 1, Var("y") would get 0 if referenced.
	xRef := &Var{Name: "x"}
	term := &Lam{
		Param: "x",
		FV:    []string{}, // no free vars on outer lam
		Body: &Lam{
			Param: "y",
			FV:    []string{"x"},
			Body:  xRef,
		},
	}
	AssignIndices(term)
	if xRef.Index != 1 {
		t.Errorf("Var(x) in \\x.\\y.x: want Index 1, got %d", xRef.Index)
	}
}

func TestAssignIndicesFix(t *testing.T) {
	// fix self in \x. self → self at Index 1, x at Index 0.
	selfRef := &Var{Name: "self"}
	lam := &Lam{
		Param: "x",
		FV:    []string{"self"},
		Body:  selfRef,
	}
	term := &Fix{Name: "self", Body: lam}
	AssignIndices(term)
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
	AssignIndices(term)
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
	AssignIndices(term)
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
		FV:    []string{},
		Body:  &PrimOp{Name: "add", Arity: 2, Args: []Core{xRef, &Lit{Value: int64(1)}}},
	}
	AssignIndices(term)
	if xRef.Index != 0 {
		t.Errorf("Var(x) inside PrimOp.Args: want Index 0, got %d", xRef.Index)
	}
}

func TestAssignIndicesFVOverflow(t *testing.T) {
	// When FV is nil (overflow), FVIndices stays nil and body sees
	// all enclosing locals.
	xRef := &Var{Name: "x"}
	term := &Lam{
		Param: "outer",
		FV:    []string{},
		Body: &Lam{
			Param: "y",
			FV:    nil, // simulate FV overflow
			Body:  xRef,
		},
	}
	// Wrap in an outer scope that has "x".
	wrapper := &Lam{
		Param: "x",
		FV:    []string{},
		Body:  term,
	}
	AssignIndices(wrapper)
	// With FV overflow, the inner lam body sees all enclosing locals.
	// The inner lam (FV=nil) does not build a captured scope — it passes
	// enclosing scope + param directly. So x should be found in the scope.
	// Check that FVIndices remained nil (overflow path).
	innerLam := term.Body.(*Lam)
	if innerLam.FVIndices != nil {
		t.Errorf("FV-overflow Lam: FVIndices should be nil, got %v", innerLam.FVIndices)
	}
}
