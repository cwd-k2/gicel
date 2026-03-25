//go:build probe

// Eval probe invariants tests — evaluator limits, trampoline, CapEnv COW, env chains, pattern matching, values.
// Does NOT cover: eval unit tests (eval_test.go), capenv unit tests (capenv_test.go).

package eval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// ===================================================================
// Probe E: Internal evaluator adversarial probing.
// Focus: limit edge cases, trampoline integrity, CapEnv COW,
// env chain, match, explain observer, value representation.
// ===================================================================

// ---------------------------------------------------------------------------
// 1. Limit Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_StepLimitZero(t *testing.T) {
	// maxSteps=0 disables the step limit. Eval should succeed.
	ev := NewEvaluator(budget.New(context.Background(), 0, 100), NewPrimRegistry(), nil, nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Lit{Value: int64(42)})
	if err != nil {
		t.Errorf("maxSteps=0 (disabled) should succeed, got %v", err)
	}
}

func TestProbeE_StepLimitNegative(t *testing.T) {
	// Negative maxSteps is clamped to zero (disabled) by budget.New.
	b := budget.New(context.Background(), -1, 100)
	if b.Max() != 0 {
		t.Fatalf("expected negative maxSteps to be clamped to 0, got %d", b.Max())
	}
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Lit{Value: int64(42)})
	if err != nil {
		t.Errorf("maxSteps=0 (disabled) should succeed, got %v", err)
	}
}

func TestProbeE_DepthLimitZero(t *testing.T) {
	// maxDepth=0 disables the depth limit. Eval should succeed.
	ev := NewEvaluator(budget.New(context.Background(), 1_000_000, 0), NewPrimRegistry(), nil, nil, nil)
	term := &ir.Bind{
		Comp: &ir.Pure{Expr: &ir.Lit{Value: int64(1)}},
		Var:  "_",
		Body: &ir.Lit{Value: int64(2)},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Errorf("maxDepth=0 (disabled) should succeed, got %v", err)
	}
}

func TestProbeE_DepthLimitNegative(t *testing.T) {
	// Negative maxDepth is clamped to zero (disabled) by budget.New.
	b := budget.New(context.Background(), 1_000_000, -1)
	if b.MaxDepth() != 0 {
		t.Fatalf("expected negative maxDepth to be clamped to 0, got %d", b.MaxDepth())
	}
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil, nil)
	term := &ir.Bind{
		Comp: &ir.Pure{Expr: &ir.Lit{Value: int64(1)}},
		Var:  "_",
		Body: &ir.Lit{Value: int64(2)},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Errorf("maxDepth=0 (disabled) should succeed, got %v", err)
	}
}

func TestProbeE_AllocLimitNegative(t *testing.T) {
	// Negative allocLimit is clamped to zero (disabled) by SetAllocLimit.
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(-1)
	if b.MaxAlloc() != 0 {
		t.Fatalf("expected negative allocLimit to be clamped to 0, got %d", b.MaxAlloc())
	}
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Con{Name: "Unit"})
	if err != nil {
		t.Fatalf("allocLimit=0 (disabled) should allow allocation, got: %v", err)
	}
}

func TestProbeE_AllocLimitExactBoundary(t *testing.T) {
	// Set allocLimit to exactly costConBase. One Con should succeed, two should fail.
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(int64(costConBase))
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil, nil)

	// First allocation: exactly at limit.
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Con{Name: "A"})
	if err != nil {
		t.Fatalf("first Con should succeed at exact limit: %v", err)
	}

	// Second allocation: over limit.
	_, err = ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Con{Name: "B"})
	if err == nil {
		t.Fatal("second Con should exceed alloc limit")
	}
	var allocErr *budget.AllocLimitError
	if !errors.As(err, &allocErr) {
		t.Fatalf("expected AllocLimitError, got %T: %v", err, err)
	}
}

func TestProbeE_AllocLimitOverflowSafe(t *testing.T) {
	// Very large allocations shouldn't cause int64 overflow issues.
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(100)
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil, nil)

	// Try to allocate a huge record. The alloc check should fire before overflow.
	fields := make([]ir.RecordField, 1000)
	for i := range fields {
		fields[i] = ir.RecordField{
			Label: fmt.Sprintf("f%d", i),
			Value: &ir.Lit{Value: int64(i)},
		}
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.RecordLit{Fields: fields})
	if err == nil {
		t.Fatal("expected AllocLimitError for huge record")
	}
}

// ---------------------------------------------------------------------------
// 2. Trampoline / Bounce Integrity
// ---------------------------------------------------------------------------

func TestProbeE_BounceValNeverEscapes(t *testing.T) {
	// The Eval trampoline should resolve all bounceVals.
	// Even deeply nested case expressions should resolve.
	ev := newTestEval()
	// 10 nested cases: case True of True -> case True of True -> ... -> 42
	var term ir.Core = &ir.Lit{Value: int64(42)}
	for range 10 {
		term = &ir.Case{
			Scrutinee: &ir.Con{Name: "True"},
			Alts: []ir.Alt{
				{Pattern: &ir.PCon{Con: "True"}, Body: term},
			},
		}
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	// Must not be a bounceVal.
	if _, ok := r.Value.(*bounceVal); ok {
		t.Fatal("bounceVal escaped trampoline")
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected 42, got %v", r.Value)
	}
}

func TestProbeE_FixEval(t *testing.T) {
	// Fix produces a self-referential closure.
	ev := newTestEval()
	term := &ir.Fix{
		Name: "f",
		Body: &ir.Lam{Param: "x", Body: &ir.Var{Index: -1, Name: "x"}},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Value.(*Closure); !ok {
		t.Errorf("expected Closure from Fix, got %T", r.Value)
	}
}

func TestProbeE_BounceValFromForce(t *testing.T) {
	// Force bounces the thunk body. Verify it resolves.
	ev := newTestEval()
	term := &ir.Force{
		Expr: &ir.Thunk{Comp: &ir.Lit{Value: int64(77)}},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Value.(*bounceVal); ok {
		t.Fatal("bounceVal escaped from Force")
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(77) {
		t.Errorf("expected 77, got %v", r.Value)
	}
}

func TestProbeE_ApplyResolvedNoBounce(t *testing.T) {
	// applyResolved (used by Applier) must resolve bounces.
	ev := newTestEval()
	clo := &Closure{Env: EmptyEnv(), Param: "x", Body: &ir.Var{Index: -1, Name: "x"}}
	r, err := ev.applyResolved(EmptyCapEnv(), clo, &HostVal{Inner: int64(42)}, &ir.App{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Value.(*bounceVal); ok {
		t.Fatal("bounceVal escaped from applyResolved")
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected 42, got %v", r.Value)
	}
}

// ---------------------------------------------------------------------------
// 3. CapEnv COW Integrity
// ---------------------------------------------------------------------------

func TestProbeE_CapEnvCOWMultipleWrites(t *testing.T) {
	// After MarkShared, multiple Sets should each copy.
	orig := map[string]any{"x": 1}
	ce := NewCapEnv(orig)
	shared := ce.MarkShared()

	ce1 := shared.Set("a", 10)
	ce2 := shared.Set("b", 20)

	// shared should not have a or b.
	if _, ok := shared.Get("a"); ok {
		t.Error("shared should not have 'a'")
	}
	if _, ok := shared.Get("b"); ok {
		t.Error("shared should not have 'b'")
	}
	// ce1 should have a but not b.
	if _, ok := ce1.Get("a"); !ok {
		t.Error("ce1 should have 'a'")
	}
	if _, ok := ce1.Get("b"); ok {
		t.Error("ce1 should not have 'b'")
	}
	// ce2 should have b but not a.
	if _, ok := ce2.Get("b"); !ok {
		t.Error("ce2 should have 'b'")
	}
	if _, ok := ce2.Get("a"); ok {
		t.Error("ce2 should not have 'a'")
	}
	// Original map untouched.
	if _, ok := orig["a"]; ok {
		t.Error("original map was modified")
	}
}

func TestProbeE_CapEnvEmptyOperations(t *testing.T) {
	// Operations on empty CapEnv should not panic.
	ce := EmptyCapEnv()
	labels := ce.Labels()
	if len(labels) != 0 {
		t.Errorf("expected empty labels, got %v", labels)
	}
	ce2 := ce.Delete("nonexistent")
	if len(ce2.Labels()) != 0 {
		t.Error("delete of nonexistent on empty should still be empty")
	}
	_, ok := ce.Get("anything")
	if ok {
		t.Error("Get on empty should return false")
	}
}

func TestProbeE_CapEnvMarkSharedIdempotent(t *testing.T) {
	// Calling MarkShared multiple times should be safe.
	ce := EmptyCapEnv().Set("x", 1)
	s1 := ce.MarkShared()
	s2 := s1.MarkShared()
	s3 := s2.MarkShared()
	// All should still have x.
	if _, ok := s3.Get("x"); !ok {
		t.Error("triple MarkShared lost data")
	}
	// Set should work on triple-shared.
	s4 := s3.Set("y", 2)
	if _, ok := s4.Get("y"); !ok {
		t.Error("Set after triple MarkShared should work")
	}
}

func TestProbeE_CapEnvSetAfterCOWIsNotShared(t *testing.T) {
	// After a COW copy, the new CapEnv should not be in COW mode.
	orig := map[string]any{"a": 1}
	ce := NewCapEnv(orig) // cow=true
	ce2 := ce.Set("b", 2) // triggers copy, cow=false on ce2
	// Set on ce2 should mutate in place (not copy again).
	ce3 := ce2.Set("c", 3)
	// ce2 and ce3 share the same map (ce2 was not marked shared).
	// This is the expected behavior: after COW copy, mutations are in-place.
	if _, ok := ce3.Get("c"); !ok {
		t.Error("ce3 should have 'c'")
	}
}

// ---------------------------------------------------------------------------
// 4. Env Chain Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_EnvEmptyLookup(t *testing.T) {
	env := EmptyEnv()
	_, ok := env.Lookup("anything")
	if ok {
		t.Error("Lookup on empty env should return false")
	}
}

func TestProbeE_EnvTrimToEmpty(t *testing.T) {
	env := EmptyEnv().Extend("x", &HostVal{Inner: 1}).Extend("y", &HostVal{Inner: 2})
	trimmed := env.TrimTo(nil)
	if trimmed.Len() != 0 {
		t.Errorf("TrimTo(nil) should produce empty env, got len %d", trimmed.Len())
	}
}

func TestProbeE_EnvTrimToNonexistent(t *testing.T) {
	env := EmptyEnv().Extend("x", &HostVal{Inner: 1})
	trimmed := env.TrimTo([]string{"x", "missing"})
	// "x" should be present, "missing" should not.
	if _, ok := trimmed.Lookup("x"); !ok {
		t.Error("trimmed should have 'x'")
	}
	if _, ok := trimmed.Lookup("missing"); ok {
		t.Error("trimmed should not have 'missing'")
	}
	// Size should reflect only found bindings.
	if trimmed.Len() != 1 {
		t.Errorf("expected len 1, got %d", trimmed.Len())
	}
}

func TestProbeE_EnvFlattenThreshold(t *testing.T) {
	// Build an env chain just under and at the flatten threshold.
	env := EmptyEnv()
	for i := 0; i < flatThreshold; i++ {
		env = env.Extend(fmt.Sprintf("v%d", i), &HostVal{Inner: i})
	}
	// At threshold, lookup should still work.
	v, ok := env.Lookup("v0")
	if !ok || v.(*HostVal).Inner != 0 {
		t.Error("lookup at threshold should work")
	}
	// One more extension triggers flatten.
	env = env.Extend("extra", &HostVal{Inner: 999})
	v, ok = env.Lookup("extra")
	if !ok || v.(*HostVal).Inner != 999 {
		t.Error("lookup after threshold should work")
	}
	v, ok = env.Lookup("v0")
	if !ok || v.(*HostVal).Inner != 0 {
		t.Error("lookup of deep binding after flatten should work")
	}
}

func TestProbeE_EnvExtendManyShadowing(t *testing.T) {
	env := EmptyEnv().Extend("x", &HostVal{Inner: 1})
	env = env.ExtendMany(map[string]Value{"x": &HostVal{Inner: 2}, "y": &HostVal{Inner: 3}})
	v, ok := env.Lookup("x")
	if !ok || v.(*HostVal).Inner != 2 {
		t.Errorf("ExtendMany should shadow: expected 2, got %v", v)
	}
}

func TestProbeE_EnvExtendManyEmpty(t *testing.T) {
	env := EmptyEnv().Extend("x", &HostVal{Inner: 1})
	env2 := env.ExtendMany(map[string]Value{})
	if env2 != env {
		t.Error("ExtendMany with empty map should return same env")
	}
}

// ---------------------------------------------------------------------------
// 5. Match Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_MatchLiteral(t *testing.T) {
	val := &HostVal{Inner: int64(42)}
	pat := &ir.PLit{Value: int64(42)}
	bindings := Match(val, pat)
	if bindings == nil {
		t.Fatal("literal match should succeed")
	}
	if len(bindings) != 0 {
		t.Errorf("literal match should produce no bindings, got %d", len(bindings))
	}
}

func TestProbeE_MatchLiteralMismatch(t *testing.T) {
	val := &HostVal{Inner: int64(42)}
	pat := &ir.PLit{Value: int64(99)}
	bindings := Match(val, pat)
	if bindings != nil {
		t.Error("mismatched literal should not match")
	}
}

func TestProbeE_MatchLiteralOnNonHost(t *testing.T) {
	// PLit should fail on non-HostVal.
	val := &ConVal{Con: "Foo"}
	pat := &ir.PLit{Value: int64(42)}
	bindings := Match(val, pat)
	if bindings != nil {
		t.Error("PLit on ConVal should not match")
	}
}

func TestProbeE_MatchWildcard(t *testing.T) {
	val := &HostVal{Inner: int64(42)}
	pat := &ir.PWild{}
	bindings := Match(val, pat)
	if bindings == nil {
		t.Fatal("wildcard should always match")
	}
	if len(bindings) != 0 {
		t.Errorf("wildcard should produce no bindings, got %d", len(bindings))
	}
}

func TestProbeE_MatchNestedCon(t *testing.T) {
	// Just (Cons 1 Nil) matched against Just (Cons x _)
	val := &ConVal{Con: "Just", Args: []Value{
		&ConVal{Con: "Cons", Args: []Value{
			&HostVal{Inner: int64(1)},
			&ConVal{Con: "Nil"},
		}},
	}}
	pat := &ir.PCon{Con: "Just", Args: []ir.Pattern{
		&ir.PCon{Con: "Cons", Args: []ir.Pattern{
			&ir.PVar{Name: "x"},
			&ir.PWild{},
		}},
	}}
	bindings := Match(val, pat)
	if bindings == nil {
		t.Fatal("nested match should succeed")
	}
	if bindings["x"].(*HostVal).Inner != int64(1) {
		t.Errorf("expected x=1, got %v", bindings["x"])
	}
}

func TestProbeE_MatchEmptyRecord(t *testing.T) {
	// Match empty record against empty record pattern.
	val := &RecordVal{Fields: map[string]Value{}}
	pat := &ir.PRecord{Fields: nil}
	bindings := Match(val, pat)
	if bindings == nil {
		t.Fatal("empty record match should succeed")
	}
}

// ---------------------------------------------------------------------------
// 6. ForceEffectful Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_ForceEffectfulNonPrimVal(t *testing.T) {
	// Non-PrimVal should be returned unchanged.
	ev := newTestEval()
	input := EvalResult{Value: &HostVal{Inner: int64(42)}, CapEnv: EmptyCapEnv()}
	result, err := ev.ForceEffectful(input, span.Span{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Value != input.Value {
		t.Error("non-PrimVal should be returned unchanged")
	}
}

func TestProbeE_ForceEffectfulUnsaturated(t *testing.T) {
	// PrimVal with fewer args than arity should not be forced.
	ev := newTestEval()
	pv := &PrimVal{Name: "test", Arity: 2, Effectful: true, Args: []Value{&HostVal{Inner: int64(1)}}}
	input := EvalResult{Value: pv, CapEnv: EmptyCapEnv()}
	result, err := ev.ForceEffectful(input, span.Span{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Value != pv {
		t.Error("unsaturated PrimVal should not be forced")
	}
}

func TestProbeE_ForceEffectfulNonEffectful(t *testing.T) {
	// Non-effectful PrimVal should not be forced.
	ev := newTestEval()
	pv := &PrimVal{Name: "test", Arity: 0, Effectful: false}
	input := EvalResult{Value: pv, CapEnv: EmptyCapEnv()}
	result, err := ev.ForceEffectful(input, span.Span{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Value != pv {
		t.Error("non-effectful PrimVal should not be forced")
	}
}

func TestProbeE_ForceEffectfulMissingPrim(t *testing.T) {
	// Effectful saturated PrimVal with no registered impl should error.
	ev := newTestEval()
	pv := &PrimVal{Name: "missing_prim", Arity: 0, Effectful: true}
	input := EvalResult{Value: pv, CapEnv: EmptyCapEnv()}
	_, err := ev.ForceEffectful(input, span.Span{})
	if err == nil {
		t.Fatal("expected error for missing primitive")
	}
}

// ---------------------------------------------------------------------------
// 7. callPrim Panic Recovery
// ---------------------------------------------------------------------------

func TestProbeE_CallPrimPanicRecovery(t *testing.T) {
	panicImpl := func(_ context.Context, ce CapEnv, args []Value, _ Applier) (Value, CapEnv, error) {
		panic("intentional panic in prim")
	}
	val, newCap, err := callPrim(context.Background(), panicImpl, EmptyCapEnv(), nil, nil)
	if err == nil {
		t.Fatal("expected error from panicking prim")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("expected 'panicked' in error, got: %v", err)
	}
	if val != nil {
		t.Error("value should be nil after panic")
	}
	_ = newCap
}

func TestProbeE_CallPrimNilReturn(t *testing.T) {
	nilImpl := func(_ context.Context, ce CapEnv, args []Value, _ Applier) (Value, CapEnv, error) {
		return nil, ce, nil
	}
	_, _, err := callPrim(context.Background(), nilImpl, EmptyCapEnv(), nil, nil)
	if err == nil {
		t.Fatal("expected error from nil return")
	}
	if !strings.Contains(err.Error(), "nil value") {
		t.Errorf("expected 'nil value' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 8. Value String() Methods
// ---------------------------------------------------------------------------

func TestProbeE_ValueStringMethods(t *testing.T) {
	// Ensure String() methods don't panic on edge cases.
	tests := []struct {
		name string
		val  Value
	}{
		{"HostVal nil", &HostVal{Inner: nil}},
		{"HostVal string", &HostVal{Inner: "hello"}},
		{"HostVal int64", &HostVal{Inner: int64(42)}},
		{"ConVal empty", &ConVal{Con: "Nil"}},
		{"ConVal with args", &ConVal{Con: "Cons", Args: []Value{&HostVal{Inner: int64(1)}, &ConVal{Con: "Nil"}}}},
		{"Closure", &Closure{Env: EmptyEnv(), Param: "x", Body: &ir.Var{Index: -1, Name: "x"}}},
		{"ThunkVal", &ThunkVal{Env: EmptyEnv(), Comp: &ir.Lit{Value: int64(0)}}},
		{"PrimVal empty", &PrimVal{Name: "test", Arity: 2}},
		{"PrimVal with args", &PrimVal{Name: "test", Arity: 2, Args: []Value{&HostVal{Inner: int64(1)}}}},
		{"RecordVal empty", &RecordVal{Fields: map[string]Value{}}},
		{"RecordVal with fields", &RecordVal{Fields: map[string]Value{"x": &HostVal{Inner: int64(1)}}}},
		{"IndirectVal nil", &IndirectVal{Ref: nil}},
		{"IndirectVal set", func() Value {
			var v Value = &HostVal{Inner: int64(42)}
			return &IndirectVal{Ref: &v}
		}()},
		{"bounceVal", &bounceVal{env: EmptyEnv(), capEnv: EmptyCapEnv(), expr: &ir.Lit{Value: int64(0)}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.val.String()
			if s == "" {
				t.Error("String() should not return empty string")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 9. PrettyValue Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_PrettyValueTuple(t *testing.T) {
	// Tuple: {_1: 1, _2: 2} should be "(1, 2)"
	rv := &RecordVal{Fields: map[string]Value{
		"_1": &HostVal{Inner: int64(1)},
		"_2": &HostVal{Inner: int64(2)},
	}}
	s := PrettyValue(rv)
	if s != "(1, 2)" {
		t.Errorf("expected '(1, 2)', got %q", s)
	}
}

func TestProbeE_PrettyValueUnit(t *testing.T) {
	rv := &RecordVal{Fields: map[string]Value{}}
	s := PrettyValue(rv)
	if s != "()" {
		t.Errorf("expected '()', got %q", s)
	}
}

func TestProbeE_PrettyValueNonTupleRecord(t *testing.T) {
	rv := &RecordVal{Fields: map[string]Value{
		"name": &HostVal{Inner: "Alice"},
		"age":  &HostVal{Inner: int64(30)},
	}}
	s := PrettyValue(rv)
	// Should contain field names.
	if !strings.Contains(s, "name") || !strings.Contains(s, "age") {
		t.Errorf("expected record fields in output, got %q", s)
	}
}

func TestProbeE_PrettyValueIndirectNil(t *testing.T) {
	iv := &IndirectVal{Ref: nil}
	s := PrettyValue(iv)
	if s != "<uninitialized>" {
		t.Errorf("expected '<uninitialized>', got %q", s)
	}
}

func TestProbeE_PrettyValueHostNil(t *testing.T) {
	hv := &HostVal{Inner: nil}
	s := PrettyValue(hv)
	if s != "()" {
		t.Errorf("expected '()', got %q", s)
	}
}

func TestProbeE_PrettyValueHostRune(t *testing.T) {
	hv := &HostVal{Inner: 'A'}
	s := PrettyValue(hv)
	if s != "'A'" {
		t.Errorf("expected 'A', got %q", s)
	}
}

func TestProbeE_PrettyValueConWithSpaceArgs(t *testing.T) {
	// Con with multi-word args should parenthesize them.
	cv := &ConVal{Con: "Pair", Args: []Value{
		&ConVal{Con: "Just", Args: []Value{&HostVal{Inner: int64(1)}}},
		&HostVal{Inner: int64(2)},
	}}
	s := PrettyValue(cv)
	// "Pair (Just 1) 2"
	if !strings.Contains(s, "(Just 1)") {
		t.Errorf("expected parenthesized arg, got %q", s)
	}
}

// ---------------------------------------------------------------------------
// 10. Explain Observer Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_ExplainObserverNilSafe(t *testing.T) {
	// All methods on nil observer should be no-ops.
	var obs *ExplainObserver
	if obs.Active() {
		t.Error("nil observer should not be active")
	}
	obs.Section("test")   // should not panic
	obs.Result("test")    // should not panic
	obs.MarkInternal("x") // should not panic
	if obs.IsInternal("x") {
		t.Error("nil observer IsInternal should return false")
	}
}

func TestProbeE_ExplainObserverSuppression(t *testing.T) {
	var steps []ExplainStep
	hook := func(s ExplainStep) { steps = append(steps, s) }
	obs := NewExplainObserver(hook, nil)

	if !obs.Active() {
		t.Error("new observer should be active")
	}
	obs.EnterInternal()
	if obs.Active() {
		t.Error("observer should be suppressed after EnterInternal")
	}
	obs.LeaveInternal()
	if !obs.Active() {
		t.Error("observer should be active after LeaveInternal")
	}
}

func TestProbeE_ExplainObserverAllMode(t *testing.T) {
	var steps []ExplainStep
	hook := func(s ExplainStep) { steps = append(steps, s) }
	obs := NewExplainObserver(hook, nil)
	obs.SetAll(true)

	obs.EnterInternal()
	if !obs.Active() {
		t.Error("with SetAll(true), observer should be active even when suppressed")
	}
	obs.LeaveInternal()
}

func TestProbeE_ExplainObserverDeepSuppression(t *testing.T) {
	// Multiple EnterInternal calls should require matching LeaveInternal calls.
	var steps []ExplainStep
	hook := func(s ExplainStep) { steps = append(steps, s) }
	obs := NewExplainObserver(hook, nil)

	obs.EnterInternal()
	obs.EnterInternal()
	obs.EnterInternal()
	if obs.Active() {
		t.Error("triple suppressed should not be active")
	}
	obs.LeaveInternal()
	obs.LeaveInternal()
	if obs.Active() {
		t.Error("still suppressed after 2 leaves")
	}
	obs.LeaveInternal()
	if !obs.Active() {
		t.Error("should be active after all leaves")
	}
}

// ---------------------------------------------------------------------------
// 11. ChargeAlloc via Context
// ---------------------------------------------------------------------------

func TestProbeE_ChargeAllocNoLimit(t *testing.T) {
	// Context without limit should always succeed.
	err := budget.ChargeAlloc(context.Background(), 999999999)
	if err != nil {
		t.Fatalf("expected success without limit, got %v", err)
	}
}

func TestProbeE_ChargeAllocZeroBytes(t *testing.T) {
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(100)
	ctx := budget.ContextWithBudget(context.Background(), b)
	err := budget.ChargeAlloc(ctx, 0)
	if err != nil {
		t.Fatalf("charging 0 bytes should succeed: %v", err)
	}
	if b.Allocated() != 0 {
		t.Errorf("expected 0 allocated, got %d", b.Allocated())
	}
}

func TestProbeE_ChargeAllocNegativeBytes(t *testing.T) {
	// Charging negative bytes: the int64 addition would decrease allocated.
	// This is an edge case - could underflow the counter.
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(100)
	ctx := budget.ContextWithBudget(context.Background(), b)

	// First charge some bytes.
	_ = budget.ChargeAlloc(ctx, 50)
	// Then charge negative. This would decrease allocated to 40.
	err := budget.ChargeAlloc(ctx, -10)
	if err != nil {
		t.Fatalf("negative charge should succeed (no underflow check): %v", err)
	}
	// NOTE: There is no guard against negative charges. This allows
	// a malicious primitive to "reclaim" allocation budget. Not necessarily
	// a bug since primitives are trusted code, but worth noting.
	if b.Allocated() != 40 {
		t.Errorf("expected 40, got %d", b.Allocated())
	}
}

// ---------------------------------------------------------------------------
// 12. Eval of nil/unknown Core node
// ---------------------------------------------------------------------------

func TestProbeE_EvalNilCore(t *testing.T) {
	// nil Core should produce an error, not panic.
	ev := newTestEval()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("eval of nil Core panicked: %v", r)
		}
	}()
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), nil)
	if err == nil {
		t.Fatal("expected error for nil Core node")
	}
}

// ---------------------------------------------------------------------------
// 13. Fix node
// ---------------------------------------------------------------------------

func TestProbeE_FixApply(t *testing.T) {
	// Fix produces identity closure; applying it returns the argument.
	ev := newTestEval()
	fix := &ir.Fix{
		Name: "f",
		Body: &ir.Lam{Param: "x", Body: &ir.Var{Index: -1, Name: "x"}},
	}
	term := &ir.App{Fun: fix, Arg: &ir.Lit{Value: int64(42)}}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected 42, got %v", r.Value)
	}
}

// ---------------------------------------------------------------------------
// 14. IndirectVal dereferencing
// ---------------------------------------------------------------------------

func TestProbeE_IndirectValChain(t *testing.T) {
	// IndirectVal pointing to another IndirectVal: should only deref once.
	ev := newTestEval()
	var innerVal Value = &HostVal{Inner: int64(42)}
	innerInd := &IndirectVal{Ref: &innerVal}
	var outerVal Value = innerInd
	outerInd := &IndirectVal{Ref: &outerVal}
	env := EmptyEnv().Extend("x", outerInd)
	r, err := ev.Eval(env, EmptyCapEnv(), &ir.Var{Index: -1, Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// Should get the inner IndirectVal (only one level of deref).
	if _, ok := r.Value.(*IndirectVal); !ok {
		// Or it might be the HostVal if it derefs recursively.
		hv, ok := r.Value.(*HostVal)
		if !ok || hv.Inner != int64(42) {
			t.Errorf("expected IndirectVal or HostVal(42), got %T: %v", r.Value, r.Value)
		}
	}
}

// ---------------------------------------------------------------------------
// 15. PrimRegistry Clone
// ---------------------------------------------------------------------------

func TestProbeE_PrimRegistryCloneIsolation(t *testing.T) {
	reg := NewPrimRegistry()
	reg.Register("original", func(_ context.Context, ce CapEnv, _ []Value, _ Applier) (Value, CapEnv, error) {
		return &HostVal{Inner: "original"}, ce, nil
	})
	clone := reg.Clone()
	// Add to clone.
	clone.Register("cloned", func(_ context.Context, ce CapEnv, _ []Value, _ Applier) (Value, CapEnv, error) {
		return &HostVal{Inner: "cloned"}, ce, nil
	})
	// Original should not have "cloned".
	if _, ok := reg.Lookup("cloned"); ok {
		t.Error("Clone mutation leaked to original")
	}
	// Clone should have both.
	if _, ok := clone.Lookup("original"); !ok {
		t.Error("Clone should have inherited 'original'")
	}
	if _, ok := clone.Lookup("cloned"); !ok {
		t.Error("Clone should have 'cloned'")
	}
}

// ---------------------------------------------------------------------------
// 16. Bind depth tracking
// ---------------------------------------------------------------------------

func TestProbeE_BindDepthUnwind(t *testing.T) {
	// Bind is trampolined (TCO) — it does not consume depth at all.
	// Depth should be unchanged after a Bind.
	ev := newTestEval()
	depthBefore := ev.budget.Depth()
	term := &ir.Bind{
		Comp: &ir.Pure{Expr: &ir.Lit{Value: int64(1)}},
		Var:  "_",
		Body: &ir.Pure{Expr: &ir.Lit{Value: int64(2)}},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	depthAfter := ev.budget.Depth()
	if depthAfter != depthBefore {
		t.Errorf("depth should be restored after Bind: before=%d, after=%d", depthBefore, depthAfter)
	}
}

func TestProbeE_ForceDepthUnwind(t *testing.T) {
	// After Force, depth should return to pre-Force level.
	ev := newTestEval()
	depthBefore := ev.budget.Depth()
	term := &ir.Force{
		Expr: &ir.Thunk{Comp: &ir.Pure{Expr: &ir.Lit{Value: int64(1)}}},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	depthAfter := ev.budget.Depth()
	if depthAfter != depthBefore {
		t.Errorf("depth should be restored after Force: before=%d, after=%d", depthBefore, depthAfter)
	}
}

// ---------------------------------------------------------------------------
// 17. Record edge cases in evaluator
// ---------------------------------------------------------------------------

func TestProbeE_RecordUpdateAddsNewField(t *testing.T) {
	// RecordUpdate can add a field that didn't exist in the original.
	ev := newTestEval()
	term := &ir.RecordUpdate{
		Record: &ir.RecordLit{Fields: []ir.RecordField{
			{Label: "x", Value: &ir.Lit{Value: int64(1)}},
		}},
		Updates: []ir.RecordField{
			{Label: "y", Value: &ir.Lit{Value: int64(2)}},
		},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	rv := r.Value.(*RecordVal)
	if len(rv.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(rv.Fields))
	}
	if rv.Fields["y"].(*HostVal).Inner != int64(2) {
		t.Error("new field y should be 2")
	}
}

func TestProbeE_RecordEmptyUpdate(t *testing.T) {
	// Update with no updates should produce equivalent record.
	ev := newTestEval()
	term := &ir.RecordUpdate{
		Record: &ir.RecordLit{Fields: []ir.RecordField{
			{Label: "x", Value: &ir.Lit{Value: int64(1)}},
		}},
		Updates: nil,
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	rv := r.Value.(*RecordVal)
	if len(rv.Fields) != 1 || rv.Fields["x"].(*HostVal).Inner != int64(1) {
		t.Error("empty update should preserve original fields")
	}
}

// ---------------------------------------------------------------------------
// 18. CapEnv threading through evaluation
// ---------------------------------------------------------------------------

func TestProbeE_CapEnvThreadingThroughConArgs(t *testing.T) {
	// CapEnv changes in constructor arg evaluation should propagate.
	prims := NewPrimRegistry()
	prims.Register("markCap", func(_ context.Context, ce CapEnv, args []Value, _ Applier) (Value, CapEnv, error) {
		return &HostVal{Inner: int64(1)}, ce.Set("marked", true), nil
	})
	ev := NewEvaluator(defaultBudget(), prims, nil, nil, nil)
	term := &ir.Con{
		Name: "Pair",
		Args: []ir.Core{
			&ir.PrimOp{Name: "markCap", Arity: 0, Args: nil},
			&ir.Lit{Value: int64(2)},
		},
	}
	// markCap is non-effectful with arity 0 and has args (none), so it should be called.
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	// CapEnv should have the "marked" capability from the first arg.
	if _, ok := r.CapEnv.Get("marked"); !ok {
		t.Error("CapEnv should carry 'marked' from constructor arg evaluation")
	}
}
