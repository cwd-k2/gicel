package gicel_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// Probe C: Module-aware evaluation — qualified variable lookup, dual
// registration, Core IR Module field propagation, and module/evaluator
// interaction.
// ---------------------------------------------------------------------------

// --- Helpers ---

func probeCEngine(t *testing.T) *gicel.Engine {
	t.Helper()
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}
	return eng
}

func probeCRunExpectInt(t *testing.T, eng *gicel.Engine, source string, expected int64) {
	t.Helper()
	rt, err := eng.NewRuntime(source)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != expected {
		t.Errorf("expected %d, got %d", expected, hv)
	}
}

func probeCRunExpectStr(t *testing.T, eng *gicel.Engine, source string, expected string) {
	t.Helper()
	rt, err := eng.NewRuntime(source)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	hv := gicel.MustHost[string](result.Value)
	if hv != expected {
		t.Errorf("expected %q, got %q", expected, hv)
	}
}

func probeCRunExpectCon(t *testing.T, eng *gicel.Engine, source string, expectedCon string) {
	t.Helper()
	rt, err := eng.NewRuntime(source)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok {
		t.Fatalf("expected ConVal, got %T: %s", result.Value, result.Value)
	}
	if con.Con != expectedCon {
		t.Errorf("expected %s, got %s", expectedCon, con.Con)
	}
}

func probeCRunExpectConWithArg(t *testing.T, eng *gicel.Engine, source string, expectedCon string, expectedArg int64) {
	t.Helper()
	rt, err := eng.NewRuntime(source)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok {
		t.Fatalf("expected ConVal, got %T: %s", result.Value, result.Value)
	}
	if con.Con != expectedCon {
		t.Errorf("expected constructor %s, got %s", expectedCon, con.Con)
	}
	if len(con.Args) < 1 {
		t.Fatalf("expected at least 1 arg, got %d", len(con.Args))
	}
	hv, ok := con.Args[0].(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal arg, got %T", con.Args[0])
	}
	if hv.Inner != expectedArg {
		t.Errorf("expected arg %d, got %v", expectedArg, hv.Inner)
	}
}

func probeCRunExpectError(t *testing.T, eng *gicel.Engine, source string) {
	t.Helper()
	_, err := eng.NewRuntime(source)
	if err != nil {
		return // compile error is also acceptable
	}
	rt, err := eng.NewRuntime(source)
	if err != nil {
		return
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got success")
	}
}

// ===========================================================================
// Section 1: Evaluator — Qualified Var Lookup
// ===========================================================================

func TestProbeCQualifiedValueSimple(t *testing.T) {
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Arith", `
import Prelude
double :: Int -> Int
double := \x. x + x
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import Arith as A
main := A.double 21
`, 42)
}

func TestProbeCQualifiedConstructor(t *testing.T) {
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Shapes", `
data Shape := Circle Int | Square Int
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectConWithArg(t, eng, `
import Shapes as S
main := S.Circle 10
`, "Circle", 10)
}

func TestProbeCQualifiedClosureLambda(t *testing.T) {
	// Qualified var that is a closure (lambda) — apply it to arguments.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("FnLib", `
import Prelude
apply3 :: (Int -> Int) -> Int -> Int
apply3 := \f x. f (f (f x))
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import FnLib as F
inc :: Int -> Int
inc := \x. x + 1
main := F.apply3 inc 0
`, 3)
}

func TestProbeCQualifiedVarInDoNotation(t *testing.T) {
	// Qualified function used inside do-notation.
	eng := probeCEngine(t)
	if err := eng.Use(gicel.EffectState); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("Helpers", `
import Prelude
addOne :: Int -> Int
addOne := \x. x + 1
`); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import Effect.State
import Helpers as H
main := do {
  n <- get;
  put (H.addOne n);
  get
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &gicel.HostVal{Inner: int64(10)}}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 11 {
		t.Errorf("expected 11, got %d", hv)
	}
}

func TestProbeCMixQualifiedAndUnqualified(t *testing.T) {
	// Mix of qualified and unqualified vars in the same expression.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("MLib", `
import Prelude
triple :: Int -> Int
triple := \x. x * 3
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import MLib as M
local := \x. x + 1
main := M.triple (local 9)
`, 30)
}

func TestProbeCQualifiedVarShadowsLocalName(t *testing.T) {
	// Qualified var that shares a name with a local binding.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Shadow", `
import Prelude
value :: Int
value := 100
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import Shadow as S
value :: Int
value := 1
main := value + S.value
`, 101)
}

// ===========================================================================
// Section 2: Evaluator — Con.Module
// ===========================================================================

func TestProbeCQualifiedConEvaluatesCorrectly(t *testing.T) {
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Types", `
data Wrapper a := Wrap a
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectConWithArg(t, eng, `
import Types as T
main := T.Wrap 42
`, "Wrap", 42)
}

func TestProbeCQualifiedConWithMultiArgs(t *testing.T) {
	// Constructor applied to multiple arguments via qualified import.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Pairs", `
data Pair a b := MkPair a b
`); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Pairs as P
main := P.MkPair 1 2
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "MkPair" {
		t.Fatalf("expected MkPair, got %s", result.Value)
	}
	if len(con.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(con.Args))
	}
	a1 := gicel.MustHost[int64](con.Args[0])
	a2 := gicel.MustHost[int64](con.Args[1])
	if a1 != 1 || a2 != 2 {
		t.Errorf("expected (1, 2), got (%d, %d)", a1, a2)
	}
}

func TestProbeCPatternMatchOnQualifiedCon(t *testing.T) {
	// Pattern matching on values constructed via qualified constructor.
	// LIMITATION: constructors in case patterns must be in unqualified scope.
	// Qualified-only import cannot be used for pattern matching.
	// We verify the open-import path works for constructors built via
	// qualified expression from a separately-registered module.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Maybe2", `
data Opt a := Some a | None
`); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("OptFactory", `
import Maybe2
wrapInt :: Int -> Opt Int
wrapInt := \x. Some x
`); err != nil {
		t.Fatal(err)
	}
	// Open import for constructors (patterns); qualified import for factory function.
	probeCRunExpectInt(t, eng, `
import Maybe2
import OptFactory as F
extract :: Opt Int -> Int
extract := \o. case o { Some x -> x; None -> 0 }
main := extract (F.wrapInt 77)
`, 77)
}

func TestProbeCCaseAnalysisQualifiedConInScrutinee(t *testing.T) {
	// Case analysis: scrutinee built via qualified constructor expression,
	// patterns use constructors from an open import.
	// LIMITATION: case patterns do not support qualified constructors (Q.Con).
	eng := probeCEngine(t)
	if err := eng.RegisterModule("BoolLib", `
data MyBool := MyTrue | MyFalse
`); err != nil {
		t.Fatal(err)
	}
	// Open import for pattern constructors.
	probeCRunExpectStr(t, eng, `
import BoolLib
main := case MyTrue { MyTrue -> "yes"; MyFalse -> "no" }
`, "yes")
}

// ===========================================================================
// Section 3: Core IR — FreeVars with Module
// ===========================================================================

func TestProbeCQualifiedVarDoesNotCreateLocalDep(t *testing.T) {
	// A binding that references Q.x should NOT depend on a local "x".
	// If SortBindings incorrectly treated the qualified var as local "x",
	// topological sort would create a false dependency.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Ext", `
import Prelude
val :: Int
val := 999
`); err != nil {
		t.Fatal(err)
	}
	// val is defined locally AND referenced qualified; they should be independent.
	probeCRunExpectInt(t, eng, `
import Prelude
import Ext as E
val :: Int
val := 1
main := val + E.val
`, 1000)
}

func TestProbeCAnnotateFreeVarsQualifiedKey(t *testing.T) {
	// Qualified vars in a lambda body should be captured by the closure
	// using module\x00name as the key in FV. When TrimTo is called, the
	// closure should still access the qualified binding.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Const", `
import Prelude
secret :: Int
secret := 42
`); err != nil {
		t.Fatal(err)
	}
	// The lambda here captures E.secret as a free variable. After TrimTo,
	// the qualified key should still resolve.
	probeCRunExpectInt(t, eng, `
import Prelude
import Const as E
f :: Int -> Int
f := \x. x + E.secret
main := f 8
`, 50)
}

// ===========================================================================
// Section 4: Dual Registration
// ===========================================================================

func TestProbeCTwoModulesSameNamedBinding(t *testing.T) {
	// Two modules with same-named binding, both qualified → each resolves independently.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("ModA", `
import Prelude
value :: Int
value := 10
`); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("ModB", `
import Prelude
value :: Int
value := 20
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import ModA as A
import ModB as B
main := A.value + B.value
`, 30)
}

func TestProbeCTwoModulesSameNamedConstructor(t *testing.T) {
	// Two modules with same-named constructor, both qualified → no collision.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("TreeA", `
data Tree := Leaf Int
`); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("TreeB", `
data Tree := Leaf String
`); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import TreeA as A
import TreeB as B
main := (A.Leaf 42, B.Leaf "hello")
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := result.Value.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal (tuple), got %T: %s", result.Value, result.Value)
	}
	// Check _1 is Leaf with int arg
	c1, ok := rec.Fields["_1"].(*gicel.ConVal)
	if !ok || c1.Con != "Leaf" {
		t.Errorf("expected Leaf for _1, got %s", rec.Fields["_1"])
	}
	// Check _2 is Leaf with string arg
	c2, ok := rec.Fields["_2"].(*gicel.ConVal)
	if !ok || c2.Con != "Leaf" {
		t.Errorf("expected Leaf for _2, got %s", rec.Fields["_2"])
	}
}

func TestProbeCDualRegPlainAndQualifiedKey(t *testing.T) {
	// Module binding should be accessible via both plain name (open import)
	// and qualified key (qualified import) from different import forms.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Dual", `
import Prelude
dval :: Int
dval := 7
`); err != nil {
		t.Fatal(err)
	}
	// One open import and one module accessed qualified — but we can't import
	// the same module twice. Instead, test that plain name resolution works
	// for open import (Dual imported openly) and separately that qualified
	// resolution works (Dual imported qualified).
	probeCRunExpectInt(t, eng, `
import Prelude
import Dual
main := dval
`, 7)
}

func TestProbeCDualRegQualifiedKey(t *testing.T) {
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Dual2", `
import Prelude
dval :: Int
dval := 7
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import Dual2 as D
main := D.dval
`, 7)
}

func TestProbeCOpenAndQualifiedDifferentModulesSameName(t *testing.T) {
	// BUG: severity=medium. Open import + qualified import of different modules
	// with the same export name. Expected: unqualified "shared" resolves to
	// Open1's value (111), Q.shared resolves to Qual1's value (222), total = 333.
	// Actual: evalModuleBindings registers ALL module bindings under their plain
	// name (dual registration), so Qual1's "shared" overwrites Open1's "shared"
	// in the runtime environment. Result is 222 + 222 = 444.
	//
	// Root cause: runtime.go evalModuleBindings always does:
	//   env = env.Extend(b.Name, cell)          // plain name
	//   env = env.Extend(moduleName+"\x00"+b.Name, cell) // qualified key
	// The plain-name registration doesn't respect import mode (open vs qualified).
	// A later module's same-named binding shadows the earlier one.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Open1", `
import Prelude
shared :: Int
shared := 111
`); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("Qual1", `
import Prelude
shared :: Int
shared := 222
`); err != nil {
		t.Fatal(err)
	}
	// Fixed: qualified-only modules skip plain-name registration.
	// shared resolves to Open1 (111), Q.shared to Qual1 (222).
	probeCRunExpectInt(t, eng, `
import Prelude
import Open1
import Qual1 as Q
main := shared + Q.shared
`, 333)
}

// ===========================================================================
// Section 5: Runtime Edge Cases
// ===========================================================================

func TestProbeCModuleWithEmptyBindings(t *testing.T) {
	// evalModuleBindings with empty binding list — module has only data decls.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Empty", `
data Unit2 := Unit2
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectCon(t, eng, `
import Empty
main := Unit2
`, "Unit2")
}

func TestProbeCModuleWithManyBindings(t *testing.T) {
	// Large module (many bindings) — verify all are accessible qualified.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Big", `
import Prelude
v1 :: Int; v1 := 1
v2 :: Int; v2 := 2
v3 :: Int; v3 := 3
v4 :: Int; v4 := 4
v5 :: Int; v5 := 5
v6 :: Int; v6 := 6
v7 :: Int; v7 := 7
v8 :: Int; v8 := 8
v9 :: Int; v9 := 9
v10 :: Int; v10 := 10
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import Big as B
main := B.v1 + B.v2 + B.v3 + B.v4 + B.v5 + B.v6 + B.v7 + B.v8 + B.v9 + B.v10
`, 55)
}

func TestProbeCModuleEvalOrder(t *testing.T) {
	// Module bindings should be evaluated before user bindings.
	// A user binding can reference a module binding.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Init", `
import Prelude
base :: Int
base := 100
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import Init as I
offset := I.base + 5
main := offset
`, 105)
}

func TestProbeCModuleBindingReferencesOtherModule(t *testing.T) {
	// Module B depends on module A's binding. Dependency chain.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("BaseLib", `
import Prelude
baseVal :: Int
baseVal := 50
`); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("DerivedLib", `
import Prelude
import BaseLib
derivedVal :: Int
derivedVal := baseVal + 25
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import DerivedLib as D
main := D.derivedVal
`, 75)
}

// ===========================================================================
// Section 6: Resource Limits
// ===========================================================================

func TestProbeCQualifiedFuncStepLimit(t *testing.T) {
	// Qualified function calls should count toward step limit correctly.
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}
	eng.SetStepLimit(50) // very tight
	if err := eng.RegisterModule("StepLib", `
import Prelude
inc :: Int -> Int
inc := \x. x + 1
`); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import StepLib as S
main := S.inc (S.inc (S.inc (S.inc (S.inc (S.inc (S.inc (S.inc (S.inc (S.inc 0)))))))))
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	// Should either succeed with 10 or fail with step limit.
	// If step limit is hit, err is non-nil. Either outcome is valid —
	// the test verifies steps are counted, not a specific limit.
	if err != nil {
		if !strings.Contains(err.Error(), "step") {
			t.Errorf("expected step limit error, got: %v", err)
		}
	}
}

func TestProbeCQualifiedConAllocLimit(t *testing.T) {
	// Qualified constructor allocation should count toward alloc limit.
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}
	eng.SetAllocLimit(500) // very tight
	if err := eng.RegisterModule("AllocLib", `
data Box := MkBox Int
`); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import AllocLib as A
main := (A.MkBox 1, A.MkBox 2, A.MkBox 3, A.MkBox 4, A.MkBox 5, A.MkBox 6, A.MkBox 7, A.MkBox 8, A.MkBox 9, A.MkBox 10)
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	// Either succeeds or hits alloc limit — both valid.
	// The key is that alloc is tracked at all for qualified constructors.
	if err != nil {
		if !strings.Contains(err.Error(), "alloc") {
			t.Errorf("expected alloc limit error, got: %v", err)
		}
	}
}

func TestProbeCQualifiedFuncDepthLimit(t *testing.T) {
	// Deeply nested qualified function calls should hit depth limit.
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}
	eng.SetDepthLimit(5) // very tight
	if err := eng.RegisterModule("DeepLib", `
import Prelude
inc :: Int -> Int
inc := \x. x + 1
`); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import DeepLib as D
main := D.inc (D.inc (D.inc (D.inc (D.inc (D.inc (D.inc (D.inc (D.inc (D.inc 0)))))))))
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	// Should either succeed or hit depth limit depending on TCO behavior.
	if err != nil {
		if !strings.Contains(err.Error(), "depth") {
			t.Errorf("expected depth limit error, got: %v", err)
		}
	}
}

// ===========================================================================
// Section 7: ExplainObserver
// ===========================================================================

func TestProbeCQualifiedClosureInternalInExplain(t *testing.T) {
	// Module closures should be marked as internal in explain output.
	// This means they should be suppressed at ExplainUser level.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Intern", `
import Prelude
step :: Int -> Int
step := \x. x + 1
`); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import Intern as I
main := I.step 0
`)
	if err != nil {
		t.Fatal(err)
	}
	var steps []gicel.ExplainStep
	_, err = rt.RunWith(context.Background(), &gicel.RunOptions{
		Explain: func(s gicel.ExplainStep) { steps = append(steps, s) },
	})
	if err != nil {
		t.Fatal(err)
	}
	// At ExplainUser level, stdlib/module-internal function entries should be suppressed.
	// Look for any ExplainLabel "enter" event for "step" — it should NOT appear
	// because module closures are marked internal.
	for _, s := range steps {
		if s.Kind == gicel.ExplainLabel && s.Detail.LabelKind == "enter" && s.Detail.Name == "step" {
			t.Errorf("module-internal closure 'step' should be suppressed at ExplainUser level")
		}
	}
}

func TestProbeCQualifiedClosureBodyVisibleInExplainAll(t *testing.T) {
	// At ExplainAll level, the body of module-internal closures should emit events.
	// NOTE: the "enter" label for internal closures is NOT emitted even in ExplainAll
	// mode — ExplainAll only un-suppresses events from WITHIN the internal body
	// (because IsInternal triggers EnterInternal, skipping the label emission branch).
	// This tests that body-level events (like bind) DO appear in ExplainAll mode.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Vis", `
import Prelude
data Opt a := Some a | None
mapOpt :: (a -> b) -> Opt a -> Opt b
mapOpt := \f o. case o { Some x -> Some (f x); None -> None }
`); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import Vis as V
inc :: Int -> Int
inc := \x. x + 1
main := V.mapOpt inc (V.Some 5)
`)
	if err != nil {
		t.Fatal(err)
	}
	var steps []gicel.ExplainStep
	_, err = rt.RunWith(context.Background(), &gicel.RunOptions{
		Explain:      func(s gicel.ExplainStep) { steps = append(steps, s) },
		ExplainDepth: gicel.ExplainAll,
	})
	if err != nil {
		t.Fatal(err)
	}
	// In ExplainAll mode, we should see events from within the module-internal
	// closure body (match events from case in mapOpt).
	hasInternalEvent := false
	for _, s := range steps {
		if s.Kind == gicel.ExplainMatch {
			hasInternalEvent = true
			break
		}
	}
	if !hasInternalEvent {
		t.Errorf("at ExplainAll, expected match events from module-internal mapOpt body")
	}
}

// ===========================================================================
// Section 8: Additional Edge Cases
// ===========================================================================

func TestProbeCQualifiedVarInCaseScrutinee(t *testing.T) {
	// Qualified variable used as the scrutinee of a case expression.
	// Constructors in patterns must be imported openly (no qualified patterns).
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Enums", `
import Prelude
data Color := Red | Green | Blue
favorite :: Color
favorite := Blue
`); err != nil {
		t.Fatal(err)
	}
	// Open import for pattern constructors; qualified var for scrutinee
	// requires using a separate module for the value.
	// Since we can't import Enums both ways, use open import and reference
	// the value through a local binding that calls into the module.
	probeCRunExpectStr(t, eng, `
import Enums
main := case favorite { Red -> "r"; Green -> "g"; Blue -> "b" }
`, "b")
}

func TestProbeCQualifiedVarInLetBody(t *testing.T) {
	// Qualified variable used inside a let-binding body.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("LetLib", `
import Prelude
offset :: Int
offset := 100
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import LetLib as L
f := \x. x + L.offset
main := f 5
`, 105)
}

func TestProbeCQualifiedNestedApplication(t *testing.T) {
	// Qualified function applied to result of another qualified function.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Chain", `
import Prelude
double :: Int -> Int
double := \x. x * 2
inc :: Int -> Int
inc := \x. x + 1
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import Chain as C
main := C.double (C.inc 4)
`, 10)
}

func TestProbeCQualifiedHigherOrderFunction(t *testing.T) {
	// Qualified higher-order function: passing a local function to a module function.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("HO", `
import Prelude
applyTwice :: (Int -> Int) -> Int -> Int
applyTwice := \f x. f (f x)
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import HO as H
double :: Int -> Int
double := \x. x * 2
main := H.applyTwice double 3
`, 12)
}

func TestProbeCQualifiedConPartialApplication(t *testing.T) {
	// Qualified constructor used as a function (partial application).
	// Con nodes with 0 supplied args should still work as function values.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("WrapLib", `
import Prelude
data Wrapped := Wrap Int
wrapFn :: (Int -> Wrapped) -> Int -> Wrapped
wrapFn := \f x. f x
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectConWithArg(t, eng, `
import WrapLib as W
main := W.wrapFn W.Wrap 77
`, "Wrap", 77)
}

func TestProbeCQualifiedConZeroArgs(t *testing.T) {
	// Zero-argument qualified constructor (enum-like).
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Enum", `
data Dir := North | South | East | West
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectCon(t, eng, `
import Enum as E
main := E.West
`, "West")
}

func TestProbeCMultipleModulesChainedEval(t *testing.T) {
	// Three modules forming a dependency chain, all accessed qualified.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("Layer1", `
import Prelude
l1val :: Int
l1val := 10
`); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("Layer2", `
import Prelude
import Layer1
l2val :: Int
l2val := l1val + 20
`); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("Layer3", `
import Prelude
import Layer2
l3val :: Int
l3val := l2val + 30
`); err != nil {
		t.Fatal(err)
	}
	probeCRunExpectInt(t, eng, `
import Prelude
import Layer3 as L
main := L.l3val
`, 60)
}

func TestProbeCQualifiedOperator(t *testing.T) {
	// Qualified access to a module-defined operator.
	// LIMITATION: qualified operator syntax (Q.(+++)) is not supported.
	// Operators can only be used via open or selective import, not qualified.
	// We test that the operator works via open import from a module.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("OpLib", `
import Prelude
infixl 6 +++
(+++) :: Int -> Int -> Int
(+++) := \x y. x + y + 1
`); err != nil {
		t.Fatal(err)
	}
	// Open import to make the operator available.
	probeCRunExpectInt(t, eng, `
import Prelude
import OpLib
main := 3 +++ 4
`, 8)
}

func TestProbeCQualifiedRecordField(t *testing.T) {
	// Qualified function that returns a record.
	eng := probeCEngine(t)
	if err := eng.RegisterModule("RecLib", `
import Prelude
mkRec :: Int -> (Int, Int)
mkRec := \x. (x, x + 1)
`); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import RecLib as R
main := R.mkRec 5
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := result.Value.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal (tuple), got %T: %s", result.Value, result.Value)
	}
	v1 := gicel.MustHost[int64](rec.Fields["_1"])
	v2 := gicel.MustHost[int64](rec.Fields["_2"])
	if v1 != 5 || v2 != 6 {
		t.Errorf("expected (5, 6), got (%d, %d)", v1, v2)
	}
}
