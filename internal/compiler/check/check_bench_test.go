// Benchmarks and inferFreeVarKinds tests.
// Does NOT cover: unify benchmarks (unify/), evidence benchmarks (evidence_sort_stress_test.go).

package check

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/compiler/desugar"
	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func BenchmarkInstanceResolve100(b *testing.B) {
	// Build source with many instances to benchmark resolution.
	source := `form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq Unit := { eq := \x y. True }
main := eq True False`

	for b.Loop() {
		src := span.NewSource("bench", source)
		es := &diagnostic.Errors{Source: src}
		p := parse.NewParser(context.Background(), src, es)
		ast := p.ParseProgram()
		desugar.Program(ast)
		Check(ast, src, nil)
	}
}

func TestQuantifyFreeVarsKindInference(t *testing.T) {
	// Row variable in Computation pre/post should be quantified as Row.
	compTy := testOps.Comp(
		&types.TyVar{Name: "r"},
		&types.TyVar{Name: "r"},
		testOps.Con("Int"),
		nil,
	)
	arrowTy := &types.TyArrow{From: &types.TyVar{Name: "a"}, To: compTy}
	result := quantifyFreeVars(testOps, arrowTy)

	forall1, ok := result.(*types.TyForall)
	if !ok {
		t.Fatalf("expected TyForall, got %T", result)
	}
	// Sorted: "a" first, then "r"
	if forall1.Var != "a" {
		t.Errorf("first quantifier: got %q, want 'a'", forall1.Var)
	}
	if !testOps.Equal(forall1.Kind, types.TypeOfTypes) {
		t.Errorf("'a' kind: got %v, want Type", testOps.PrettyTypeAsKind(forall1.Kind))
	}

	forall2, ok := forall1.Body.(*types.TyForall)
	if !ok {
		t.Fatalf("expected nested TyForall, got %T", forall1.Body)
	}
	if forall2.Var != "r" {
		t.Errorf("second quantifier: got %q, want 'r'", forall2.Var)
	}
	if !testOps.Equal(forall2.Kind, types.TypeOfRows) {
		t.Errorf("'r' kind: got %v, want Row", testOps.PrettyTypeAsKind(forall2.Kind))
	}

	// Pure type variable should get Type.
	pureTy := &types.TyArrow{From: &types.TyVar{Name: "a"}, To: &types.TyVar{Name: "a"}}
	pureResult := quantifyFreeVars(testOps, pureTy)
	pureForall, ok := pureResult.(*types.TyForall)
	if !ok {
		t.Fatalf("expected TyForall, got %T", pureResult)
	}
	if !testOps.Equal(pureForall.Kind, types.TypeOfTypes) {
		t.Errorf("pure 'a' kind: got %v, want Type", testOps.PrettyTypeAsKind(pureForall.Kind))
	}
}

// --- inferFreeVarKinds: additional coverage ---

func TestInferFreeVarKindsThunk(t *testing.T) {
	// Variable in TyCBPV (Thunk) pre/post should get Row.
	fv := map[string]struct{}{"r": {}, "a": {}}
	thunkTy := testOps.Thunk(
		&types.TyVar{Name: "r"},
		&types.TyVar{Name: "r"},
		&types.TyVar{Name: "a"},
		nil,
	)
	kinds := inferFreeVarKinds(thunkTy, fv)
	if !testOps.Equal(kinds["r"], types.TypeOfRows) {
		t.Errorf("'r' in TyCBPV (Thunk) pre/post should be Row, got %v", testOps.PrettyTypeAsKind(kinds["r"]))
	}
	if !testOps.Equal(kinds["a"], types.TypeOfTypes) {
		t.Errorf("'a' in TyCBPV (Thunk) result should be Type, got %v", testOps.PrettyTypeAsKind(kinds["a"]))
	}
}

func TestInferFreeVarKindsBothPositions(t *testing.T) {
	// Variable appearing in both row and type positions should get Row.
	fv := map[string]struct{}{"x": {}}
	ty := testOps.Comp(
		&types.TyVar{Name: "x"}, // row position → Row
		&types.TyVar{Name: "x"},
		&types.TyVar{Name: "x"}, // type position → Type, but Row wins
		nil,
	)
	kinds := inferFreeVarKinds(ty, fv)
	if !testOps.Equal(kinds["x"], types.TypeOfRows) {
		t.Errorf("'x' in both row and type positions should be Row, got %v", testOps.PrettyTypeAsKind(kinds["x"]))
	}
}

func TestInferFreeVarKindsNoFreeVars(t *testing.T) {
	// Empty free variable set should produce empty result.
	fv := map[string]struct{}{}
	ty := &types.TyArrow{From: &types.TyVar{Name: "a"}, To: &types.TyVar{Name: "b"}}
	kinds := inferFreeVarKinds(ty, fv)
	if len(kinds) != 0 {
		t.Errorf("expected empty result for no free vars, got %d entries", len(kinds))
	}
}

func BenchmarkZonkDeepChain(b *testing.B) {
	u := unify.NewUnifier(testOps)
	// Build a deep TyApp chain with no metavariables.
	var ty types.Type = testOps.Con("Base")
	for range 50 {
		ty = &types.TyApp{Fun: testOps.Con("F"), Arg: ty}
	}

	for b.Loop() {
		u.Zonk(ty)
	}
}
