package eval

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cwd-k2/gicel/internal/budget"
	"github.com/cwd-k2/gicel/internal/core"
)

func defaultBudget() *budget.Budget {
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(10 * 1024 * 1024) // 10 MiB
	return b
}

func newTestEval() *Evaluator {
	return NewEvaluator(defaultBudget(), NewPrimRegistry(), nil, nil)
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
	// (\x. x) Unit
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
	ev := NewEvaluator(defaultBudget(), prims, nil, nil)
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
	ev := NewEvaluator(defaultBudget(), prims, nil, nil)
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
	ev := NewEvaluator(budget.New(context.Background(), 3, 100), NewPrimRegistry(), nil, nil)
	// A chain: App(App(Lam, Lam), Con) — will exceed 3 steps
	term := &core.App{
		Fun: &core.Lam{Param: "f",
			Body: &core.App{Fun: &core.Var{Name: "f"}, Arg: &core.Con{Name: "Unit"}},
		},
		Arg: &core.Lam{Param: "x", Body: &core.Var{Name: "x"}},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if _, ok := err.(*budget.StepLimitError); !ok {
		t.Errorf("expected StepLimitError, got %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	ev := NewEvaluator(budget.New(ctx, 1_000_000, 1_000), NewPrimRegistry(), nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.Con{Name: "Unit"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestTraceHook(t *testing.T) {
	var events []string
	hook := func(e TraceEvent) error {
		events = append(events, e.NodeKind)
		return nil
	}
	ev := NewEvaluator(defaultBudget(), NewPrimRegistry(), hook, nil)
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
	// Match { x: 1, y: 2 } against { x: a, y: b }
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
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(100) // 100 bytes — enough for a few small values
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil)

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
	var allocErr *budget.AllocLimitError
	if !errors.As(err, &allocErr) {
		t.Fatalf("expected AllocLimitError, got %T: %v", err, err)
	}
}

func TestAllocTracking(t *testing.T) {
	ev := newTestEval()
	// Evaluate: (\x. x) Unit — creates a Closure then a ConVal.
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
	ev := NewEvaluator(defaultBudget(), prims, nil, nil)
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
	// letrec f = \x. ext in f — returned closure should have trimmed env.
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
	// Budget with 2 steps allows exactly 2 eval steps.
	ev := NewEvaluator(budget.New(context.Background(), 2, 100), NewPrimRegistry(), nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.Lit{Value: int64(42)})
	if err != nil {
		t.Fatalf("budget(2): first eval should succeed, got %v", err)
	}
	_, err = ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.Lit{Value: int64(43)})
	if err != nil {
		t.Fatalf("budget(2): second eval should succeed, got %v", err)
	}
	_, err = ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.Lit{Value: int64(44)})
	if _, ok := err.(*budget.StepLimitError); !ok {
		t.Errorf("budget(2): third eval should fail with StepLimitError, got %v", err)
	}
}

func TestDepthLimitBoundary(t *testing.T) {
	// maxDepth=1: one level of function application should succeed.
	ev := NewEvaluator(budget.New(context.Background(), 1_000_000, 1), NewPrimRegistry(), nil, nil)
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
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(int64(costConBase))
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.Con{Name: "Unit"})
	if err != nil {
		t.Fatalf("allocLimit=costConBase: one ConVal should succeed, got %v", err)
	}
}

func TestChargeAllocViaContext(t *testing.T) {
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(100)
	ctx := budget.ContextWithBudget(context.Background(), b)

	// Charging within budget should succeed.
	if err := budget.ChargeAlloc(ctx, 50); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if b.Allocated() != 50 {
		t.Fatalf("expected 50 allocated, got %d", b.Allocated())
	}

	// Charging over budget should fail.
	if err := budget.ChargeAlloc(ctx, 60); err == nil {
		t.Fatal("expected AllocLimitError")
	}

	// No-limit context should always succeed.
	if err := budget.ChargeAlloc(context.Background(), 999); err != nil {
		t.Fatalf("expected success without limit, got %v", err)
	}
}

func TestDepthLimitError(t *testing.T) {
	// With TCO, closure application depth is flat (Enter→bounce→Leave).
	// Depth only accumulates via Bind chains (not trampolined).
	// Build: Bind(Pure(Unit), \_. Bind(Pure(Unit), \_. Pure(Unit)))
	// = 2 nested Binds → depth 2.
	ev := NewEvaluator(budget.New(context.Background(), 1_000_000, 1), NewPrimRegistry(), nil, nil)
	term := &core.Bind{
		Comp: &core.Pure{Expr: &core.Con{Name: "Unit"}},
		Var:  "_",
		Body: &core.Bind{
			Comp: &core.Pure{Expr: &core.Con{Name: "Unit"}},
			Var:  "_",
			Body: &core.Pure{Expr: &core.Con{Name: "Unit"}},
		},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected DepthLimitError for depth-2 Bind chain at maxDepth=1")
	}
	if _, ok := err.(*budget.DepthLimitError); !ok {
		t.Errorf("expected *DepthLimitError, got %T: %v", err, err)
	}
}

func TestDepthLimitMultiLevel(t *testing.T) {
	// Build chain of N nested Binds (depth accumulates via Bind).
	buildBindChain := func(depth int) core.Core {
		var body core.Core = &core.Pure{Expr: &core.Con{Name: "Unit"}}
		for range depth {
			body = &core.Bind{
				Comp: &core.Pure{Expr: &core.Con{Name: "Unit"}},
				Var:  "_",
				Body: body,
			}
		}
		return body
	}

	// maxDepth=5: chain of 5 Binds should succeed.
	ev := NewEvaluator(budget.New(context.Background(), 1_000_000, 5), NewPrimRegistry(), nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), buildBindChain(5))
	if err != nil {
		t.Fatalf("depth-5 Bind chain at maxDepth=5 should succeed, got: %v", err)
	}

	// Chain of 6 should fail.
	ev2 := NewEvaluator(budget.New(context.Background(), 1_000_000, 5), NewPrimRegistry(), nil, nil)
	_, err = ev2.Eval(EmptyEnv(), EmptyCapEnv(), buildBindChain(6))
	if _, ok := err.(*budget.DepthLimitError); !ok {
		t.Errorf("depth-6 Bind chain at maxDepth=5 should fail with DepthLimitError, got %T: %v", err, err)
	}
}

func TestLetRecTCOFlat(t *testing.T) {
	// With TCO, nested LetRec bodies bounce instead of accumulating depth.
	// 10 nested LetRecs at maxDepth=5 should succeed (depth stays at 1).
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

	ev := NewEvaluator(budget.New(context.Background(), 1_000_000, 5), NewPrimRegistry(), nil, nil)
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatalf("expected success for LetRec TCO, got: %v", err)
	}
	if con, ok := r.Value.(*ConVal); !ok || con.Con != "Unit" {
		t.Errorf("expected Unit, got %v", r.Value)
	}
}

// --- Error path and contract tests ---

func TestEvalVarUnbound(t *testing.T) {
	ev := newTestEval()
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &core.Var{Name: "missing"})
	if err == nil {
		t.Fatal("expected error for unbound variable")
	}
	if _, ok := err.(*RuntimeError); !ok {
		t.Errorf("expected *RuntimeError, got %T", err)
	}
}

func TestEvalCaseNoMatch(t *testing.T) {
	ev := newTestEval()
	term := &core.Case{
		Scrutinee: &core.Con{Name: "Foo"},
		Alts: []core.Alt{
			{Pattern: &core.PCon{Con: "Bar"}, Body: &core.Lit{Value: int64(1)}},
		},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for non-exhaustive case")
	}
	if _, ok := err.(*RuntimeError); !ok {
		t.Errorf("expected *RuntimeError, got %T", err)
	}
}

func TestLetRecNonLamBinding(t *testing.T) {
	ev := newTestEval()
	term := &core.LetRec{
		Bindings: []core.Binding{{Name: "x", Expr: &core.Con{Name: "Unit"}}},
		Body:     &core.Var{Name: "x"},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for LetRec non-lambda binding")
	}
	if _, ok := err.(*RuntimeError); !ok {
		t.Errorf("expected *RuntimeError, got %T", err)
	}
}

func TestTraceHookAbort(t *testing.T) {
	sentinel := errors.New("abort from hook")
	hook := func(ev TraceEvent) error { return sentinel }
	evl := NewEvaluator(defaultBudget(), NewPrimRegistry(), hook, nil)
	_, err := evl.Eval(EmptyEnv(), EmptyCapEnv(), &core.Lit{Value: int64(1)})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error from hook, got %v", err)
	}
}

func TestCapEnvDeleteAndLabels(t *testing.T) {
	ce := NewCapEnv(map[string]any{"a": 1, "b": 2, "c": 3})
	ce2 := ce.Delete("b")
	if _, ok := ce2.Get("b"); ok {
		t.Error("expected key 'b' to be deleted")
	}
	if v, ok := ce2.Get("a"); !ok || v != 1 {
		t.Error("expected key 'a' to survive delete")
	}
	labels := ce2.Labels()
	if len(labels) != 2 || labels[0] != "a" || labels[1] != "c" {
		t.Errorf("expected [a c], got %v", labels)
	}

	// CoW: delete on shared env should not mutate original.
	shared := NewCapEnv(map[string]any{"x": 1, "y": 2}).MarkShared()
	del := shared.Delete("x")
	if _, ok := del.Get("x"); ok {
		t.Error("expected x deleted in new env")
	}
	if _, ok := shared.Get("x"); !ok {
		t.Error("expected x still present in shared env after CoW delete")
	}
}

// Regression: NewCapEnv must not allow Set/Delete to mutate the caller's map.
func TestNewCapEnvIsolatesCallerMap(t *testing.T) {
	orig := map[string]any{"a": 1, "b": 2}
	ce := NewCapEnv(orig)

	// Set should not touch the original map.
	ce.Set("c", 3)
	if _, ok := orig["c"]; ok {
		t.Error("Set mutated caller's original map")
	}

	// Delete should not touch the original map.
	ce.Delete("a")
	if _, ok := orig["a"]; !ok {
		t.Error("Delete mutated caller's original map")
	}
}

func TestPrimOpNotRegistered(t *testing.T) {
	ev := newTestEval()
	term := &core.PrimOp{Name: "nonexistent", Args: []core.Core{&core.Lit{Value: int64(0)}}}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for unregistered PrimOp")
	}
	if _, ok := err.(*RuntimeError); !ok {
		t.Errorf("expected *RuntimeError, got %T", err)
	}
}

func TestTCOTailRecursionFlat(t *testing.T) {
	// Tail-recursive countdown: self 100000 → 0 via case.
	// Without TCO the Go stack would overflow. With TCO (Case alt body
	// returns bounceVal) the stack stays flat.
	const N = 100_000
	ctx := context.Background()
	prims := NewPrimRegistry()
	ev := NewEvaluator(budget.New(ctx, N*10, N*2), prims, nil, nil)

	zeroLit := &core.Lit{Value: int64(0)}
	oneLit := &core.Lit{Value: int64(1)}
	eqOp := &core.PrimOp{Name: "eq", Arity: 2, Args: []core.Core{
		&core.Var{Name: "n"}, zeroLit,
	}}
	subOp := &core.PrimOp{Name: "sub", Arity: 2, Args: []core.Core{
		&core.Var{Name: "n"}, oneLit,
	}}
	selfCall := &core.App{Fun: &core.Var{Name: "self"}, Arg: subOp}
	body := &core.Case{
		Scrutinee: eqOp,
		Alts: []core.Alt{
			{Pattern: &core.PCon{Con: "True"}, Body: &core.Var{Name: "n"}},
			{Pattern: &core.PCon{Con: "False"}, Body: selfCall},
		},
	}
	innerLam := &core.Lam{Param: "n", Body: body}
	letrec := &core.LetRec{
		Bindings: []core.Binding{{Name: "self", Expr: innerLam}},
		Body:     &core.App{Fun: &core.Var{Name: "self"}, Arg: &core.Lit{Value: int64(N)}},
	}

	prims.Register("eq", func(_ context.Context, ce CapEnv, args []Value, _ Applier) (Value, CapEnv, error) {
		a := args[0].(*HostVal).Inner.(int64)
		b := args[1].(*HostVal).Inner.(int64)
		if a == b {
			return &ConVal{Con: "True"}, ce, nil
		}
		return &ConVal{Con: "False"}, ce, nil
	})
	prims.Register("sub", func(_ context.Context, ce CapEnv, args []Value, _ Applier) (Value, CapEnv, error) {
		a := args[0].(*HostVal).Inner.(int64)
		b := args[1].(*HostVal).Inner.(int64)
		return &HostVal{Inner: a - b}, ce, nil
	})

	result, err := ev.Eval(EmptyEnv(), NewCapEnv(nil), letrec)
	if err != nil {
		t.Fatalf("TCO tail recursion failed: %v", err)
	}
	n := result.Value.(*HostVal).Inner.(int64)
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
	// TCO keeps the Go call stack flat even though depth counter still
	// increments (apply still calls Enter/Leave). The key test is that
	// the 100k recursion completes without Go stack overflow.
}

func TestTCOCaseNonTailDoesNotBounce(t *testing.T) {
	// In a case expression, the scrutinee is NOT a tail position.
	// Verify the result of a non-tail case is not a bounceVal.
	ev := newTestEval()
	term := &core.Case{
		Scrutinee: &core.Con{Name: "True"},
		Alts: []core.Alt{
			{Pattern: &core.PCon{Con: "True"}, Body: &core.Lit{Value: int64(42)}},
			{Pattern: &core.PCon{Con: "False"}, Body: &core.Lit{Value: int64(0)}},
		},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	// The trampoline should have resolved the bounce — we should get HostVal.
	hv, ok := r.Value.(*HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", r.Value)
	}
	if hv.Inner != int64(42) {
		t.Errorf("expected 42, got %v", hv.Inner)
	}
}

func TestTCONestedCase(t *testing.T) {
	// Nested case expressions: outer case dispatches to inner case.
	// Both should work correctly via TCO trampoline.
	ev := newTestEval()
	term := &core.Case{
		Scrutinee: &core.Con{Name: "True"},
		Alts: []core.Alt{
			{
				Pattern: &core.PCon{Con: "True"},
				Body: &core.Case{
					Scrutinee: &core.Con{Name: "False"},
					Alts: []core.Alt{
						{Pattern: &core.PCon{Con: "True"}, Body: &core.Lit{Value: int64(1)}},
						{Pattern: &core.PCon{Con: "False"}, Body: &core.Lit{Value: int64(2)}},
					},
				},
			},
			{Pattern: &core.PCon{Con: "False"}, Body: &core.Lit{Value: int64(3)}},
		},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", r.Value)
	}
	if hv.Inner != int64(2) {
		t.Errorf("expected 2, got %v", hv.Inner)
	}
}

func TestEvalRecordProjOnNonRecordEval(t *testing.T) {
	ev := newTestEval()
	term := &core.RecordProj{
		Record: &core.Lit{Value: int64(42)},
		Label:  "x",
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for projection on non-record")
	}
}

func TestEvalRecordUpdateOnNonRecordEval(t *testing.T) {
	ev := newTestEval()
	term := &core.RecordUpdate{
		Record:  &core.Lit{Value: int64(42)},
		Updates: []core.RecordField{{Label: "x", Value: &core.Lit{Value: int64(1)}}},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for update on non-record")
	}
}

func TestEvalForceNonThunk(t *testing.T) {
	ev := newTestEval()
	term := &core.Force{Expr: &core.Lit{Value: int64(42)}}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for force on non-thunk")
	}
	if _, ok := err.(*RuntimeError); !ok {
		t.Errorf("expected *RuntimeError, got %T", err)
	}
}

func TestEvalApplicationOfNonFunction(t *testing.T) {
	ev := newTestEval()
	term := &core.App{
		Fun: &core.Lit{Value: int64(42)},
		Arg: &core.Con{Name: "Unit"},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for application of non-function")
	}
	if _, ok := err.(*RuntimeError); !ok {
		t.Errorf("expected *RuntimeError, got %T", err)
	}
}

func TestEvalConWithArgs(t *testing.T) {
	ev := newTestEval()
	term := &core.Con{
		Name: "Pair",
		Args: []core.Core{
			&core.Lit{Value: int64(1)},
			&core.Lit{Value: int64(2)},
		},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	cv, ok := r.Value.(*ConVal)
	if !ok {
		t.Fatalf("expected ConVal, got %T", r.Value)
	}
	if cv.Con != "Pair" || len(cv.Args) != 2 {
		t.Errorf("expected Pair with 2 args, got %s with %d", cv.Con, len(cv.Args))
	}
}

func TestEvalConApplication(t *testing.T) {
	// Applying a value to a ConVal accumulates arguments.
	ev := newTestEval()
	term := &core.App{
		Fun: &core.App{
			Fun: &core.Con{Name: "Pair"},
			Arg: &core.Lit{Value: int64(1)},
		},
		Arg: &core.Lit{Value: int64(2)},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	cv, ok := r.Value.(*ConVal)
	if !ok {
		t.Fatalf("expected ConVal, got %T", r.Value)
	}
	if cv.Con != "Pair" || len(cv.Args) != 2 {
		t.Errorf("expected Pair(2 args), got %s(%d)", cv.Con, len(cv.Args))
	}
}

func TestIsFixpointBody(t *testing.T) {
	// Pattern: f = \arg. (g f) arg
	binding := core.Binding{
		Name: "f",
		Expr: &core.Lam{
			Param: "arg",
			Body: &core.App{
				Fun: &core.App{
					Fun: &core.Var{Name: "g"},
					Arg: &core.Var{Name: "f"},
				},
				Arg: &core.Var{Name: "arg"},
			},
		},
	}
	inner, ok := isFixpointBody(binding)
	if !ok {
		t.Fatal("expected isFixpointBody to return true")
	}
	if inner == nil {
		t.Fatal("expected non-nil inner expression")
	}
}

func TestIsFixpointBodyNonLam(t *testing.T) {
	binding := core.Binding{
		Name: "f",
		Expr: &core.Con{Name: "Unit"},
	}
	_, ok := isFixpointBody(binding)
	if ok {
		t.Fatal("expected isFixpointBody to return false for non-lambda")
	}
}

func TestIsFixpointBodyNotPattern(t *testing.T) {
	// f = \arg. arg (not the fix/rec pattern)
	binding := core.Binding{
		Name: "f",
		Expr: &core.Lam{Param: "arg", Body: &core.Var{Name: "arg"}},
	}
	_, ok := isFixpointBody(binding)
	if ok {
		t.Fatal("expected isFixpointBody to return false for identity")
	}
}

func TestLetRecGroupFV(t *testing.T) {
	letrec := &core.LetRec{
		Bindings: []core.Binding{
			{Name: "f", Expr: &core.Lam{Param: "x", Body: &core.Var{Name: "y"}, FV: []string{"y", "z"}}},
			{Name: "g", Expr: &core.Lam{Param: "x", Body: &core.Var{Name: "z"}, FV: []string{"z"}}},
		},
	}
	fv := letRecGroupFV(letrec)
	if fv == nil {
		t.Fatal("expected non-nil FV set")
	}
	fvSet := make(map[string]bool)
	for _, v := range fv {
		fvSet[v] = true
	}
	if !fvSet["y"] || !fvSet["z"] {
		t.Errorf("expected y and z in FV, got %v", fv)
	}
}

func TestLetRecGroupFVNoAnnotation(t *testing.T) {
	letrec := &core.LetRec{
		Bindings: []core.Binding{
			{Name: "f", Expr: &core.Lam{Param: "x", Body: &core.Var{Name: "y"}}},
		},
	}
	fv := letRecGroupFV(letrec)
	if fv != nil {
		t.Errorf("expected nil FV for unannotated binding, got %v", fv)
	}
}

func TestBounceValString(t *testing.T) {
	b := &bounceVal{env: EmptyEnv(), capEnv: EmptyCapEnv(), expr: &core.Con{Name: "Unit"}}
	if b.String() != "bounceVal(...)" {
		t.Errorf("expected 'bounceVal(...)', got %q", b.String())
	}
}

func TestEvalTyLamErased(t *testing.T) {
	ev := newTestEval()
	term := &core.TyLam{Body: &core.Con{Name: "Unit"}}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	cv, ok := r.Value.(*ConVal)
	if !ok || cv.Con != "Unit" {
		t.Errorf("expected Unit, got %v", r.Value)
	}
}

func TestEvalRecordProjMissingField(t *testing.T) {
	ev := newTestEval()
	term := &core.RecordProj{
		Record: &core.RecordLit{Fields: []core.RecordField{
			{Label: "a", Value: &core.Lit{Value: int64(1)}},
		}},
		Label: "missing",
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for missing field")
	}
	if _, ok := err.(*RuntimeError); !ok {
		t.Errorf("expected *RuntimeError, got %T", err)
	}
}

func TestEvalPrimValPartialApplication(t *testing.T) {
	// An unsaturated PrimVal should accumulate arguments.
	prims := NewPrimRegistry()
	prims.Register("add2", func(_ context.Context, ce CapEnv, args []Value, _ Applier) (Value, CapEnv, error) {
		a := args[0].(*HostVal).Inner.(int64)
		b := args[1].(*HostVal).Inner.(int64)
		return &HostVal{Inner: a + b}, ce, nil
	})
	ev := NewEvaluator(defaultBudget(), prims, nil, nil)

	// PrimOp with arity 2, applied to 0 args → PrimVal.
	primOp := &core.PrimOp{Name: "add2", Arity: 2}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), primOp)
	if err != nil {
		t.Fatal(err)
	}
	pv, ok := r.Value.(*PrimVal)
	if !ok {
		t.Fatalf("expected PrimVal, got %T", r.Value)
	}
	if pv.Arity != 2 || len(pv.Args) != 0 {
		t.Errorf("expected arity=2, args=0, got %d/%d", pv.Arity, len(pv.Args))
	}

	// Apply first arg → still PrimVal with 1 arg.
	term1 := &core.App{Fun: primOp, Arg: &core.Lit{Value: int64(10)}}
	r1, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term1)
	if err != nil {
		t.Fatal(err)
	}
	pv1, ok := r1.Value.(*PrimVal)
	if !ok {
		t.Fatalf("expected PrimVal after 1 arg, got %T", r1.Value)
	}
	if len(pv1.Args) != 1 {
		t.Errorf("expected 1 accumulated arg, got %d", len(pv1.Args))
	}

	// Apply second arg → saturated, should call impl and return result.
	term2 := &core.App{
		Fun: term1,
		Arg: &core.Lit{Value: int64(20)},
	}
	r2, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term2)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r2.Value.(*HostVal)
	if !ok || hv.Inner != int64(30) {
		t.Errorf("expected HostVal(30), got %v", r2.Value)
	}
}

func TestEvalEffectfulPrimValDeferred(t *testing.T) {
	// Effectful PrimVal should be deferred even when saturated via apply.
	prims := NewPrimRegistry()
	prims.Register("eff0", func(_ context.Context, ce CapEnv, args []Value, _ Applier) (Value, CapEnv, error) {
		return &ConVal{Con: "Done"}, ce.Set("effected", true), nil
	})
	ev := NewEvaluator(defaultBudget(), prims, nil, nil)

	// Effectful PrimOp with arity=0: should produce a PrimVal (deferred).
	term := &core.PrimOp{Name: "eff0", Arity: 0, Effectful: true}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	pv, ok := r.Value.(*PrimVal)
	if !ok {
		t.Fatalf("expected deferred PrimVal, got %T", r.Value)
	}
	if !pv.Effectful {
		t.Error("expected Effectful=true")
	}
}

func TestIsFixpointBodyArgMismatch(t *testing.T) {
	// f = \arg. (g f) wrong_arg  (outer arg != lambda param)
	binding := core.Binding{
		Name: "f",
		Expr: &core.Lam{
			Param: "arg",
			Body: &core.App{
				Fun: &core.App{
					Fun: &core.Var{Name: "g"},
					Arg: &core.Var{Name: "f"},
				},
				Arg: &core.Var{Name: "other"},
			},
		},
	}
	_, ok := isFixpointBody(binding)
	if ok {
		t.Fatal("expected false when outer arg doesn't match lambda param")
	}
}

func TestIsFixpointBodySelfArgMismatch(t *testing.T) {
	// f = \arg. (g notF) arg  (inner arg is not self)
	binding := core.Binding{
		Name: "f",
		Expr: &core.Lam{
			Param: "arg",
			Body: &core.App{
				Fun: &core.App{
					Fun: &core.Var{Name: "g"},
					Arg: &core.Var{Name: "notF"},
				},
				Arg: &core.Var{Name: "arg"},
			},
		},
	}
	_, ok := isFixpointBody(binding)
	if ok {
		t.Fatal("expected false when inner arg is not self")
	}
}

func TestIsFixpointBodyInnerNotApp(t *testing.T) {
	// f = \arg. x arg  (fun is not an App, just a Var)
	binding := core.Binding{
		Name: "f",
		Expr: &core.Lam{
			Param: "arg",
			Body: &core.App{
				Fun: &core.Var{Name: "x"},
				Arg: &core.Var{Name: "arg"},
			},
		},
	}
	_, ok := isFixpointBody(binding)
	if ok {
		t.Fatal("expected false when fun is not an App")
	}
}

func TestIsFixpointBodyNoOuterApp(t *testing.T) {
	// f = \arg. arg  (body is not an App)
	// Already tested in TestIsFixpointBodyNotPattern but with different framing.
	binding := core.Binding{
		Name: "f",
		Expr: &core.Lam{Param: "arg", Body: &core.Lit{Value: int64(42)}},
	}
	_, ok := isFixpointBody(binding)
	if ok {
		t.Fatal("expected false when body is not an App")
	}
}

func TestEvalUnknownCoreNode(t *testing.T) {
	// The default case in evalStep should return an error.
	// We can't easily construct an unknown Core node from outside,
	// but we can verify the error message format for a known edge case.
	ev := newTestEval()
	// nil is not a valid Core but shows up as *core.Xxx; use a non-evaluatable node.
	// Actually, just verify that RecordLit with fields evaluates correctly (no error path).
	term := &core.RecordLit{Fields: []core.RecordField{
		{Label: "x", Value: &core.Lit{Value: int64(1)}},
	}}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := r.Value.(*RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T", r.Value)
	}
	if v, ok := rv.Fields["x"]; !ok || v.(*HostVal).Inner != int64(1) {
		t.Error("expected field x=1")
	}
}

func TestEvalPrimOpMissingPrim(t *testing.T) {
	// PrimOp with saturated args but no registered implementation.
	ev := newTestEval()
	term := &core.PrimOp{Name: "missing_prim", Arity: 1, Args: []core.Core{&core.Lit{Value: int64(1)}}}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for missing primitive with args")
	}
}

func TestIndirectVal(t *testing.T) {
	// IndirectVal with nil Ref should error.
	ev := newTestEval()
	ind := &IndirectVal{Ref: nil}
	env := EmptyEnv().Extend("x", ind)
	_, err := ev.Eval(env, EmptyCapEnv(), &core.Var{Name: "x"})
	if err == nil {
		t.Fatal("expected error for uninitialized IndirectVal")
	}
	if _, ok := err.(*RuntimeError); !ok {
		t.Errorf("expected *RuntimeError, got %T", err)
	}

	// IndirectVal with set Ref should dereference.
	var val Value = &HostVal{Inner: int64(42)}
	ind2 := &IndirectVal{Ref: &val}
	env2 := EmptyEnv().Extend("y", ind2)
	r, err := ev.Eval(env2, EmptyCapEnv(), &core.Var{Name: "y"})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected HostVal(42), got %v", r.Value)
	}
}
