// Equality constraint parse tests — TyExprEq in type annotations.
// Does NOT cover: checker integration (check package).

package parse

import (
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive
)

func TestParseEqualityConstraintSimple(t *testing.T) {
	// a ~ Int => a -> Int
	prog := parseMustSucceed(t, `f :: a ~ Int => a -> Int`)
	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(prog.Decls))
	}
	ann, ok := prog.Decls[0].(*DeclTypeAnn)
	if !ok {
		t.Fatalf("expected DeclTypeAnn, got %T", prog.Decls[0])
	}
	qual, ok := ann.Type.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual, got %T", ann.Type)
	}
	eq, ok := qual.Constraint.(*TyExprEq)
	if !ok {
		t.Fatalf("expected TyExprEq constraint, got %T", qual.Constraint)
	}
	lhs, ok := eq.Lhs.(*TyExprVar)
	if !ok || lhs.Name != "a" {
		t.Errorf("expected lhs 'a', got %T", eq.Lhs)
	}
	rhs, ok := eq.Rhs.(*TyExprCon)
	if !ok || rhs.Name != "Int" {
		t.Errorf("expected rhs 'Int', got %T", eq.Rhs)
	}
}

func TestParseEqualityConstraintTuple(t *testing.T) {
	// (a ~ Int, Eq a) => a -> Bool
	prog := parseMustSucceed(t, `f :: (a ~ Int, Eq a) => a -> Bool`)
	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(prog.Decls))
	}
	ann := prog.Decls[0].(*DeclTypeAnn)
	// Should desugar to: (a ~ Int) => Eq a => a -> Bool
	qual1, ok := ann.Type.(*TyExprQual)
	if !ok {
		t.Fatalf("expected outer TyExprQual, got %T", ann.Type)
	}
	_, ok = qual1.Constraint.(*TyExprEq)
	if !ok {
		t.Fatalf("expected first constraint to be TyExprEq, got %T", qual1.Constraint)
	}
	qual2, ok := qual1.Body.(*TyExprQual)
	if !ok {
		t.Fatalf("expected inner TyExprQual, got %T", qual1.Body)
	}
	_, ok = qual2.Constraint.(*TyExprApp)
	if !ok {
		t.Fatalf("expected second constraint to be TyExprApp (Eq a), got %T", qual2.Constraint)
	}
}

func TestParseEqualityConstraintWithArrow(t *testing.T) {
	// ~ binds tighter than ->, so a ~ Int -> Bool is (a ~ Int) -> Bool (not a ~ (Int -> Bool))
	prog := parseMustSucceed(t, `f :: a ~ Int -> Bool`)
	ann := prog.Decls[0].(*DeclTypeAnn)
	arrow, ok := ann.Type.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow, got %T", ann.Type)
	}
	_, ok = arrow.From.(*TyExprEq)
	if !ok {
		t.Fatalf("expected TyExprEq as arrow from, got %T", arrow.From)
	}
}

func TestParseEqualityConstraintTypeFamilyApp(t *testing.T) {
	// ElemOf c ~ a => ...
	prog := parseMustSucceed(t, `f :: ElemOf c ~ a => a -> a`)
	ann := prog.Decls[0].(*DeclTypeAnn)
	qual, ok := ann.Type.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual, got %T", ann.Type)
	}
	eq, ok := qual.Constraint.(*TyExprEq)
	if !ok {
		t.Fatalf("expected TyExprEq, got %T", qual.Constraint)
	}
	// LHS should be TyExprApp (ElemOf c)
	_, ok = eq.Lhs.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp as LHS, got %T", eq.Lhs)
	}
}
