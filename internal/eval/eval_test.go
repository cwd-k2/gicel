package eval

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cwd-k2/gicel/internal/core"
)

func newTestEval() *Evaluator {
	return NewEvaluator(context.Background(), NewPrimRegistry(), DefaultLimit(), nil)
}

func TestEvalVar(t *testing.T) {
	ev := newTestEval()
	env := EmptyEnv().Extend("x", &HostVal{Inner: 42})
	r, err := ev.Eval(env, EmptyCapEnv(), &core.Var{Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != 42 {
		t.Errorf("expected HostVal(42), got %v", r.Value)
	}
}

func TestEvalLamApp(t *testing.T) {
	ev := newTestEval()
	// (\x -> x) Unit
	term := &core.App{
		Fun: &core.Lam{Param: "x", Body: &core.Var{Name: "x"}},
		Arg: &core.Con{Name: "Unit"},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	cv, ok := r.Value.(*ConVal)
	if !ok || cv.Con != "Unit" {
		t.Errorf("expected Unit, got %v", r.Value)
	}
}

func TestEvalPureBind(t *testing.T) {
	ev := newTestEval()
	// Bind(Pure(HostVal(42)), "x", Pure(Var("x")))
	term := &core.Bind{
		Comp: &core.Pure{Expr: &core.Var{Name: "val"}},
		Var:  "x",
		Body: &core.Pure{Expr: &core.Var{Name: "x"}},
	}
	env := EmptyEnv().Extend("val", &HostVal{Inner: 42})
	r, err := ev.Eval(env, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != 42 {
		t.Errorf("expected HostVal(42), got %v", r.Value)
	}
}

func TestEvalThunkForce(t *testing.T) {
	ev := newTestEval()
	// Force(Thunk(Pure(Con("Unit"))))
	term := &core.Force{
		Expr: &core.Thunk{
			Comp: &core.Pure{Expr: &core.Con{Name: "Unit"}},
		},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	cv, ok := r.Value.(*ConVal)
	if !ok || cv.Con != "Unit" {
		t.Errorf("expected Unit, got %v", r.Value)
	}
}

func TestEvalCase(t *testing.T) {
	ev := newTestEval()
	// case True { True -> HostVal(1); False -> HostVal(2) }
	term := &core.Case{
		Scrutinee: &core.Con{Name: "True"},
		Alts: []core.Alt{
			{Pattern: &core.PCon{Con: "True"}, Body: &core.Var{Name: "one"}},
			{Pattern: &core.PCon{Con: "False"}, Body: &core.Var{Name: "two"}},
		},
	}
	env := EmptyEnv().Extend("one", &HostVal{Inner: 1}).Extend("two", &HostVal{Inner: 2})
	r, err := ev.Eval(env, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != 1 {
		t.Errorf("expected HostVal(1), got %v", r.Value)
	}
}

func TestEvalTyAppErased(t *testing.T) {
	ev := newTestEval()
	term := &core.TyApp{Expr: &core.Con{Name: "Unit"}}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	cv, ok := r.Value.(*ConVal)
	if !ok || cv.Con != "Unit" {
		t.Errorf("expected Unit, got %v", r.Value)
	}
}

func TestEvalPrimOp(t *testing.T) {
	prims := NewPrimRegistry()
	prims.Register("id", func(ctx context.Context, cap CapEnv, args []Value, _ Applier) (Value, CapEnv, error) {
		return args[0], cap, nil
	})
	ev := NewEvaluator(context.Background(), prims, DefaultLimit(), nil)
	term := &core.PrimOp{
		Name: "id",
		Args: []core.Core{&core.Con{Name: "Unit"}},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	cv, ok := r.Value.(*ConVal)
	if !ok || cv.Con != "Unit" {
		t.Errorf("expected Unit, got %v", r.Value)
	}
}

func TestEvalCapEnvThreading(t *testing.T) {
	prims := NewPrimRegistry()
	prims.Register("setFoo", func(ctx context.Context, cap CapEnv, args []Value, _ Applier) (Value, CapEnv, error) {
		return &ConVal{Con: "Unit"}, cap.Set("foo", "bar"), nil
	})
	ev := NewEvaluator(context.Background(), prims, DefaultLimit(), nil)
	// Bind(PrimOp("setFoo"), "_", Pure(Con("Unit")))
	term := &core.Bind{
		Comp: &core.PrimOp{Name: "setFoo"},
		Var:  "_",
		Body: &core.Pure{Expr: &core.Con{Name: "Unit"}},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := r.CapEnv.Get("foo")
	if !ok || v != "bar" {
		t.Errorf("expected foo=bar in CapEnv, got %v", v)
	}
}

func TestStepLimit(t *testing.T) {
	ev := NewEvaluator(context.Background(), NewPrimRegistry(), NewLimit(3, 100), nil)
	// A chain: App(App(Lam, Lam), Con) — will exceed 3 steps
	term := &core.App{
		Fun: &core.Lam{Param: "f",
			Body: &core.App{Fun: &core.Var{Name: "f"}, Arg: &core.Con{Name: "Unit"}},
		},
		Arg: &core.Lam{Param: "x", Body: &core.Var{Name: "x"}},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if _, ok := err.(*StepLimitError); !ok {
		t.Errorf("expected StepLimitError, got %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	ev := NewEvaluator(ctx, NewPrimRegistry(), DefaultLimit(), nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.Con{Name: "Unit"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestTraceHook(t *testing.T) {
	var events []string
	hook := func(e TraceEvent) error {
		switch e.Node.(type) {
		case *core.Con:
			events = append(events, "Con")
		case *core.Pure:
			events = append(events, "Pure")
		}
		return nil
	}
	ev := NewEvaluator(context.Background(), NewPrimRegistry(), DefaultLimit(), hook)
	term := &core.Pure{Expr: &core.Con{Name: "Unit"}}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0] != "Pure" || events[1] != "Con" {
		t.Errorf("expected [Pure, Con], got %v", events)
	}
}

func TestEnvParentChainLookup(t *testing.T) {
	// Build a 100-deep chain and verify deepest binding is found.
	env := EmptyEnv()
	for i := 0; i < 100; i++ {
		env = env.Extend(fmt.Sprintf("v%d", i), &HostVal{Inner: i})
	}
	// Lookup the deepest binding.
	v, ok := env.Lookup("v0")
	if !ok {
		t.Fatal("v0 not found in 100-deep chain")
	}
	if v.(*HostVal).Inner != 0 {
		t.Errorf("expected 0, got %v", v.(*HostVal).Inner)
	}
	// Lookup the most recent binding.
	v99, ok := env.Lookup("v99")
	if !ok {
		t.Fatal("v99 not found")
	}
	if v99.(*HostVal).Inner != 99 {
		t.Errorf("expected 99, got %v", v99.(*HostVal).Inner)
	}
	// Verify Len().
	if env.Len() != 100 {
		t.Errorf("expected Len()=100, got %d", env.Len())
	}
}

func TestEnvExtendAlloc(t *testing.T) {
	// Extend should NOT copy the map (O(1) allocation).
	base := EmptyEnv()
	for i := 0; i < 50; i++ {
		base = base.Extend(fmt.Sprintf("v%d", i), &HostVal{Inner: i})
	}
	allocs := testing.AllocsPerRun(100, func() {
		base.Extend("extra", &HostVal{Inner: 999})
	})
	// With parent-chain, Extend allocates 1 Env struct only.
	// Old impl would allocate map + copy (much more).
	if allocs > 3 {
		t.Errorf("Extend allocated %v per run; expected <= 3 (parent-chain O(1))", allocs)
	}
}

func BenchmarkEnvExtend100(b *testing.B) {
	for i := 0; i < b.N; i++ {
		env := EmptyEnv()
		for j := 0; j < 100; j++ {
			env = env.Extend(fmt.Sprintf("v%d", j), &HostVal{Inner: j})
		}
		// Force a lookup to prevent dead-code elimination.
		env.Lookup("v50")
	}
}

func TestEnvLookup(t *testing.T) {
	env := EmptyEnv().Extend("x", &HostVal{Inner: 1}).Extend("y", &HostVal{Inner: 2})
	v, ok := env.Lookup("x")
	if !ok {
		t.Fatal("x not found")
	}
	if v.(*HostVal).Inner != 1 {
		t.Error("x should be 1")
	}
	// Shadowing
	env2 := env.Extend("x", &HostVal{Inner: 99})
	v2, _ := env2.Lookup("x")
	if v2.(*HostVal).Inner != 99 {
		t.Error("shadowed x should be 99")
	}
}

func TestCapEnvCoW(t *testing.T) {
	c1 := EmptyCapEnv().Set("a", 1)
	c2 := c1.MarkShared()
	c3 := c2.Set("b", 2)
	// c2 should still only have "a"
	if _, ok := c2.Get("b"); ok {
		t.Error("c2 should not have 'b' after CoW set")
	}
	if v, ok := c3.Get("b"); !ok || v != 2 {
		t.Error("c3 should have 'b'=2")
	}
}

func TestMatchPatterns(t *testing.T) {
	val := &ConVal{Con: "Pair", Args: []Value{&HostVal{Inner: 1}, &HostVal{Inner: 2}}}
	pat := &core.PCon{Con: "Pair", Args: []core.Pattern{
		&core.PVar{Name: "a"},
		&core.PVar{Name: "b"},
	}}
	bindings := Match(val, pat)
	if bindings == nil {
		t.Fatal("match should succeed")
	}
	if bindings["a"].(*HostVal).Inner != 1 || bindings["b"].(*HostVal).Inner != 2 {
		t.Error("binding values wrong")
	}
	// Mismatch: different constructor
	if Match(val, &core.PCon{Con: "Other"}) != nil {
		t.Error("should not match different constructor")
	}
	// Mismatch: arity too few
	if Match(val, &core.PCon{Con: "Pair", Args: []core.Pattern{&core.PVar{Name: "a"}}}) != nil {
		t.Error("should not match with fewer pattern args than value args")
	}
	// Mismatch: arity too many
	threePat := &core.PCon{Con: "Pair", Args: []core.Pattern{
		&core.PVar{Name: "a"}, &core.PVar{Name: "b"}, &core.PVar{Name: "c"},
	}}
	if Match(val, threePat) != nil {
		t.Error("should not match with more pattern args than value args")
	}
}

func TestMatchRecordPattern(t *testing.T) {
	// Match { x = 1, y = 2 } against { x = a, y = b }
	rv := &RecordVal{Fields: map[string]Value{
		"x": &HostVal{Inner: 10},
		"y": &HostVal{Inner: 20},
	}}
	pat := &core.PRecord{Fields: []core.PRecordField{
		{Label: "x", Pattern: &core.PVar{Name: "a"}},
		{Label: "y", Pattern: &core.PVar{Name: "b"}},
	}}
	bindings := Match(rv, pat)
	if bindings == nil {
		t.Fatal("expected match to succeed")
	}
	if bindings["a"].(*HostVal).Inner != 10 {
		t.Errorf("expected a=10, got %v", bindings["a"])
	}
	if bindings["b"].(*HostVal).Inner != 20 {
		t.Errorf("expected b=20, got %v", bindings["b"])
	}
	// Missing field should fail.
	patExtra := &core.PRecord{Fields: []core.PRecordField{
		{Label: "x", Pattern: &core.PVar{Name: "a"}},
		{Label: "z", Pattern: &core.PVar{Name: "c"}},
	}}
	if Match(rv, patExtra) != nil {
		t.Error("should not match when pattern has label not in value")
	}
	// Non-record value should fail.
	if Match(&HostVal{Inner: 42}, pat) != nil {
		t.Error("should not match non-record value against record pattern")
	}
}

func TestEvalStats(t *testing.T) {
	ev := newTestEval()
	term := &core.Pure{Expr: &core.Con{Name: "Unit"}}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Stats().Steps != 2 { // Pure + Con
		t.Errorf("expected 2 steps, got %d", ev.Stats().Steps)
	}
}

func TestAllocLimit(t *testing.T) {
	// A tight alloc limit should stop evaluation before producing large structures.
	limit := NewLimit(1_000_000, 1_000)
	limit.SetAllocLimit(100) // 100 bytes — enough for a few small values
	ev := NewEvaluator(context.Background(), NewPrimRegistry(), limit, nil)

	// Build a 10-field record: costRecBase(56) + costRecFld(32)*10 = 376 bytes > 100
	fields := make([]core.RecordField, 10)
	for i := range fields {
		fields[i] = core.RecordField{
			Label: fmt.Sprintf("f%d", i),
			Value: &core.Lit{Value: i},
		}
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.RecordLit{Fields: fields})
	if err == nil {
		t.Fatal("expected AllocLimitError, got nil")
	}
	var allocErr *AllocLimitError
	if !errors.As(err, &allocErr) {
		t.Fatalf("expected AllocLimitError, got %T: %v", err, err)
	}
}

func TestAllocTracking(t *testing.T) {
	ev := newTestEval()
	// Evaluate: (\x -> x) Unit — creates a Closure then a ConVal.
	term := &core.App{
		Fun: &core.Lam{Param: "x", Body: &core.Var{Name: "x"}},
		Arg: &core.Con{Name: "Unit"},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	stats := ev.Stats()
	if stats.Allocated <= 0 {
		t.Errorf("expected positive allocation, got %d", stats.Allocated)
	}
	// Closure(40) + ConVal(32) = 72 at minimum
	if stats.Allocated < 72 {
		t.Errorf("expected at least 72 bytes allocated, got %d", stats.Allocated)
	}
}

// --- Mutation-killing tests ---

func TestBindForceEffectfulBody(t *testing.T) {
	// do { _ <- pure Unit; setFoo } where setFoo is effectful 0-arity.
	// Body result is a PrimVal that must be forced via ForceEffectful.
	prims := NewPrimRegistry()
	prims.Register("setFoo", func(ctx context.Context, cap CapEnv, args []Value, _ Applier) (Value, CapEnv, error) {
		return &ConVal{Con: "Unit"}, cap.Set("foo", "done"), nil
	})
	ev := NewEvaluator(context.Background(), prims, DefaultLimit(), nil)
	term := &core.Bind{
		Comp: &core.Pure{Expr: &core.Con{Name: "Unit"}},
		Var:  "_",
		Body: &core.PrimOp{Name: "setFoo", Arity: 0, Effectful: true},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Value.(*PrimVal); ok {
		t.Fatal("body effectful PrimVal should have been forced, got PrimVal")
	}
	v, ok := r.CapEnv.Get("foo")
	if !ok || v != "done" {
		t.Errorf("expected foo=done in CapEnv, got %v (ok=%v)", v, ok)
	}
}

func TestClosureEnvTrimmed(t *testing.T) {
	// Closure with FV annotation should trim env to named vars only.
	ev := newTestEval()
	lam := &core.Lam{Param: "x", Body: &core.Var{Name: "y"}, FV: []string{"y"}}
	env := EmptyEnv().Extend("y", &HostVal{Inner: 1}).Extend("z", &HostVal{Inner: 2})
	r, err := ev.Eval(env, EmptyCapEnv(), lam)
	if err != nil {
		t.Fatal(err)
	}
	clo := r.Value.(*Closure)
	if _, ok := clo.Env.Lookup("z"); ok {
		t.Error("trimmed closure env should not contain 'z'")
	}
	if _, ok := clo.Env.Lookup("y"); !ok {
		t.Error("trimmed closure env should contain 'y'")
	}
}

func TestThunkEnvTrimmed(t *testing.T) {
	// Thunk with FV annotation should trim env.
	ev := newTestEval()
	thunk := &core.Thunk{
		Comp: &core.Pure{Expr: &core.Var{Name: "y"}},
		FV:   []string{"y"},
	}
	env := EmptyEnv().Extend("y", &HostVal{Inner: 1}).Extend("z", &HostVal{Inner: 2})
	r, err := ev.Eval(env, EmptyCapEnv(), thunk)
	if err != nil {
		t.Fatal(err)
	}
	tv := r.Value.(*ThunkVal)
	if _, ok := tv.Env.Lookup("z"); ok {
		t.Error("trimmed thunk env should not contain 'z'")
	}
	if _, ok := tv.Env.Lookup("y"); !ok {
		t.Error("trimmed thunk env should contain 'y'")
	}
}

func TestLetRecEnvTrimmed(t *testing.T) {
	// letrec f = \x -> ext in f — returned closure should have trimmed env.
	ev := newTestEval()
	fLam := &core.Lam{
		Param: "x",
		Body:  &core.Var{Name: "ext"},
		FV:    []string{"ext"},
	}
	term := &core.LetRec{
		Bindings: []core.Binding{{Name: "f", Expr: fLam}},
		Body:     &core.Var{Name: "f"},
	}
	env := EmptyEnv().
		Extend("ext", &HostVal{Inner: 1}).
		Extend("noise", &HostVal{Inner: 2})
	r, err := ev.Eval(env, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	clo := r.Value.(*Closure)
	if _, ok := clo.Env.Lookup("noise"); ok {
		t.Error("LetRec closure env should not contain 'noise'")
	}
	if _, ok := clo.Env.Lookup("ext"); !ok {
		t.Error("LetRec closure env should contain 'ext'")
	}
}

func TestAllocTrackingThunk(t *testing.T) {
	ev := newTestEval()
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(),
		&core.Thunk{Comp: &core.Pure{Expr: &core.Con{Name: "Unit"}}})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Stats().Allocated < costThunk {
		t.Errorf("Thunk should allocate at least %d bytes, got %d", costThunk, ev.Stats().Allocated)
	}
}

func TestAllocTrackingLetRec(t *testing.T) {
	ev := newTestEval()
	term := &core.LetRec{
		Bindings: []core.Binding{
			{Name: "f", Expr: &core.Lam{Param: "x", Body: &core.Var{Name: "x"}}},
		},
		Body: &core.Con{Name: "Unit"},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	// LetRec: costLetRec per binding + ConVal(Unit): costConBase
	expected := int64(costLetRec + costConBase)
	if ev.Stats().Allocated != expected {
		t.Errorf("expected %d bytes, got %d", expected, ev.Stats().Allocated)
	}
}

func TestAllocTrackingRecordUpdate(t *testing.T) {
	ev := newTestEval()
	term := &core.RecordUpdate{
		Record: &core.RecordLit{Fields: []core.RecordField{
			{Label: "a", Value: &core.Lit{Value: int64(1)}},
		}},
		Updates: []core.RecordField{
			{Label: "a", Value: &core.Lit{Value: int64(2)}},
		},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	// RecordLit: costRecBase + costRecFld*1
	// RecordUpdate copy: costRecBase + costRecFld*1
	expected := int64(2 * (costRecBase + costRecFld))
	if ev.Stats().Allocated != expected {
		t.Errorf("expected %d bytes, got %d", expected, ev.Stats().Allocated)
	}
}

func TestThunkCapEnvMarkShared(t *testing.T) {
	// After thunk creation, returned CapEnv should be CoW-protected.
	ev := newTestEval()
	capEnv := EmptyCapEnv().Set("a", 1)
	thunk := &core.Thunk{Comp: &core.Pure{Expr: &core.Con{Name: "Unit"}}}
	r, err := ev.Eval(EmptyEnv(), capEnv, thunk)
	if err != nil {
		t.Fatal(err)
	}
	// Mutate via Set — if MarkShared was applied, map is copied (no leak).
	// If MarkShared was skipped, Set mutates the shared map in place.
	r.CapEnv.Set("a", 999)
	v, _ := r.CapEnv.Get("a")
	if v != 1 {
		t.Errorf("MarkShared should prevent mutation leak: expected a=1, got %v", v)
	}
}

func TestStepLimitBoundary(t *testing.T) {
	// NewLimit(n, ...) allows exactly n eval steps.
	ev := NewEvaluator(context.Background(), NewPrimRegistry(), NewLimit(2, 100), nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.Lit{Value: int64(42)})
	if err != nil {
		t.Fatalf("NewLimit(2): first eval should succeed, got %v", err)
	}
	_, err = ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.Lit{Value: int64(43)})
	if err != nil {
		t.Fatalf("NewLimit(2): second eval should succeed, got %v", err)
	}
	_, err = ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.Lit{Value: int64(44)})
	if _, ok := err.(*StepLimitError); !ok {
		t.Errorf("NewLimit(2): third eval should fail with StepLimitError, got %v", err)
	}
}

func TestDepthLimitBoundary(t *testing.T) {
	// maxDepth=1: one level of function application should succeed.
	ev := NewEvaluator(context.Background(), NewPrimRegistry(), NewLimit(1_000_000, 1), nil)
	term := &core.App{
		Fun: &core.Lam{Param: "x", Body: &core.Var{Name: "x"}},
		Arg: &core.Con{Name: "Unit"},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatalf("maxDepth=1: single application should succeed, got %v", err)
	}
}

func TestAllocLimitBoundary(t *testing.T) {
	// allocLimit = costConBase: exactly one ConVal allocation should succeed.
	limit := NewLimit(1_000_000, 1_000)
	limit.SetAllocLimit(int64(costConBase))
	ev := NewEvaluator(context.Background(), NewPrimRegistry(), limit, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.Con{Name: "Unit"})
	if err != nil {
		t.Fatalf("allocLimit=costConBase: one ConVal should succeed, got %v", err)
	}
}

func TestDepthLimitError(t *testing.T) {
	// To stack depth, the *body* of a function must contain another application.
	// Build: \x -> (\y -> y) x, applied to Unit → depth 2 (outer Enter + inner Enter).
	ev := NewEvaluator(context.Background(), NewPrimRegistry(), NewLimit(1_000_000, 1), nil)
	// (\x -> (\y -> y) x) Unit — body applies identity, giving depth=2.
	term := &core.App{
		Fun: &core.Lam{Param: "x", Body: &core.App{
			Fun: &core.Lam{Param: "y", Body: &core.Var{Name: "y"}},
			Arg: &core.Var{Name: "x"},
		}},
		Arg: &core.Con{Name: "Unit"},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected DepthLimitError for depth-2 chain at maxDepth=1")
	}
	if _, ok := err.(*DepthLimitError); !ok {
		t.Errorf("expected *DepthLimitError, got %T: %v", err, err)
	}
}

func TestDepthLimitMultiLevel(t *testing.T) {
	// Build chain of N depth levels:
	// Level 1: \x -> x
	// Level 2: \x -> (level1) x
	// Level N: \x -> (level(N-1)) x
	// Applying levelN to Unit gives depth=N.
	buildChain := func(depth int) core.Core {
		var fn core.Core = &core.Lam{Param: "x0", Body: &core.Var{Name: "x0"}}
		for i := 1; i < depth; i++ {
			param := fmt.Sprintf("x%d", i)
			fn = &core.Lam{Param: param, Body: &core.App{
				Fun: fn,
				Arg: &core.Var{Name: param},
			}}
		}
		return &core.App{Fun: fn, Arg: &core.Con{Name: "Unit"}}
	}

	// maxDepth=5: chain of 5 should succeed.
	ev := NewEvaluator(context.Background(), NewPrimRegistry(), NewLimit(1_000_000, 5), nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), buildChain(5))
	if err != nil {
		t.Fatalf("depth-5 chain at maxDepth=5 should succeed, got: %v", err)
	}

	// Chain of 6 should fail.
	ev2 := NewEvaluator(context.Background(), NewPrimRegistry(), NewLimit(1_000_000, 5), nil)
	_, err = ev2.Eval(EmptyEnv(), EmptyCapEnv(), buildChain(6))
	if _, ok := err.(*DepthLimitError); !ok {
		t.Errorf("depth-6 chain at maxDepth=5 should fail with DepthLimitError, got %T: %v", err, err)
	}
}

func TestLetRecDepthLimit(t *testing.T) {
	// LetRec body evaluation should consume depth budget.
	// Build nested LetRec: letrec f = \x -> x in (letrec g = \x -> x in ... Unit)
	term := core.Core(&core.Con{Name: "Unit"})
	for i := range 10 {
		name := string(rune('a' + i))
		term = &core.LetRec{
			Bindings: []core.Binding{
				{Name: name, Expr: &core.Lam{Param: "x", Body: &core.Var{Name: "x"}}},
			},
			Body: term,
		}
	}

	// With maxDepth=5, 10 nested LetRecs should hit the depth limit.
	ev := NewEvaluator(context.Background(), NewPrimRegistry(), NewLimit(1_000_000, 5), nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected DepthLimitError for deeply nested LetRec")
	}
	if _, ok := err.(*DepthLimitError); !ok {
		t.Errorf("expected *DepthLimitError, got %T: %v", err, err)
	}
}
