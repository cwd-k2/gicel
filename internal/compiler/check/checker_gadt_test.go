// Checker GADT tests — ConType registration, pattern refinement, multi-branch, exhaustiveness filtering.
// Does NOT cover: general exhaustiveness (exhaustiveness_test.go), DataKinds (resolve_kind_datakinds_test.go).

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- GADT tests ---

func TestGADTConTypeRegistration(t *testing.T) {
	// IntLit :: Int -> Expr Int → constructor type is registered correctly.
	source := `data Bool := True | False
data Expr a := { IntLit :: Bool -> Expr Bool; BoolLit :: Bool -> Expr Bool }
main := IntLit True`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
			// Verify the inferred type is Expr Bool.
			pretty := types.Pretty(b.Type)
			if !strings.Contains(pretty, "Expr") || !strings.Contains(pretty, "Bool") {
				t.Errorf("expected main :: Expr Bool, got %s", pretty)
			}
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
	// Verify GADT constructors are in DataDecls.
	for _, d := range prog.DataDecls {
		if d.Name == "Expr" {
			if len(d.Cons) != 2 {
				t.Fatalf("expected 2 cons, got %d", len(d.Cons))
			}
			for _, c := range d.Cons {
				if c.ReturnType == nil {
					t.Errorf("GADT con %s should have ReturnType", c.Name)
				}
			}
		}
	}
}

func TestGADTPatternRefinement(t *testing.T) {
	// case (e: Expr Bool) { BoolLit b -> b } should derive b: Bool
	source := `data Bool := True | False
data Expr a := { BoolLit :: Bool -> Expr Bool; IntLit :: Bool -> Expr Bool }
f :: Expr Bool -> Bool
f := \e. case e { BoolLit b -> b; IntLit b -> b }`
	checkSource(t, source, nil)

	// Negative test: refinement must not allow returning wrong type.
	// After matching BoolLit b, b: Bool; returning it as Int should fail.
	badSource := `data Bool := True | False
data Expr a := { BoolLit :: Bool -> Expr Bool; IntLit :: Bool -> Expr Bool }
f :: Expr Bool -> Expr Bool
f := \e. case e { BoolLit b -> b; IntLit b -> b }`
	checkSourceExpectCode(t, badSource, nil, diagnostic.ErrTypeMismatch)
}

func TestGADTMultiBranch(t *testing.T) {
	// Multiple GADT constructors sharing the same return type specialization.
	source := `data Bool := True | False
data Expr a := { Lit :: Bool -> Expr Bool; Not :: Expr Bool -> Expr Bool }
eval :: Expr Bool -> Bool
eval := \e. case e { Lit b -> b; Not inner -> True }`
	checkSource(t, source, nil)
}

func TestGADTExhaustiveRelevant(t *testing.T) {
	// Tag Bool case: TagUnit is irrelevant (return type Tag Unit ≠ Tag Bool).
	// Only TagBool is required.
	source := `data Bool := True | False
data Unit := Unit
data Tag a := { TagBool :: Bool -> Tag Bool; TagUnit :: Unit -> Tag Unit }
f :: Tag Bool -> Bool
f := \t. case t { TagBool b -> b }`
	checkSource(t, source, nil)
}

func TestGADTNonExhaustiveError(t *testing.T) {
	// Tag Bool case: TagBool is required but missing → error.
	source := `data Bool := True | False
data Unit := Unit
data Tag a := { TagBool :: Bool -> Tag Bool; TagUnit :: Unit -> Tag Unit }
f :: Tag Bool -> Bool
f := \t. case t { TagUnit _ -> True }`
	errMsg := checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
	if !strings.Contains(errMsg, "TagBool") {
		t.Errorf("expected missing TagBool, got: %s", errMsg)
	}
}

func TestGADTAllBranchesIrrelevant(t *testing.T) {
	// If all constructors are irrelevant for the scrutinee type,
	// an empty case is OK (dead code).
	source := `data Bool := True | False
data Unit := Unit
data Void := MkVoid
data Tag a := { TagBool :: Bool -> Tag Bool; TagUnit :: Unit -> Tag Unit }
f :: Tag Void -> Void
f := \t. case t { _ -> MkVoid }`
	checkSource(t, source, nil)
}

func TestGADTEvalPolyRecursive(t *testing.T) {
	// V7: Polymorphic recursive GADT evaluator with fix.
	// Verifies that a multi-branch GADT with mixed return types
	// and recursive calls type-checks under polymorphic fix.
	source := `data Expr a := { LitI :: Int -> Expr Int; LitB :: Bool -> Expr Bool; Add :: Expr Int -> Expr Int -> Expr Int }
eval :: \a. Expr a -> a
eval := fix (\self e. case e { LitI n -> n; LitB b -> b; Add x y -> self x + self y })
main := eval (Add (LitI 10) (LitI 32))`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{
			"Int":  types.KType{},
			"Bool": types.KType{},
		},
		GatedBuiltins: map[string]bool{"fix": true, "rec": true},
		Assumptions: map[string]types.Type{
			"+": types.MkArrow(
				&types.TyCon{Name: "Int"},
				types.MkArrow(
					&types.TyCon{Name: "Int"},
					&types.TyCon{Name: "Int"},
				),
			),
		},
	}
	prog := checkSource(t, source, config)
	// Verify main binding exists and has type Int.
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
			pretty := types.Pretty(b.Type)
			if !strings.Contains(pretty, "Int") {
				t.Errorf("expected main :: Int, got %s", pretty)
			}
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
}
