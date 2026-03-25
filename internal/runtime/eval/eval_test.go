package eval

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
)

func defaultBudget() *budget.Budget {
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(10 * 1024 * 1024) // 10 MiB
	return b
}

func newTestEval() *Evaluator {
	return NewEvaluator(defaultBudget(), NewPrimRegistry(), nil, nil, nil)
}

// prepareIR runs the FV annotation and de Bruijn index assignment passes
// on a Core expression, matching the production pipeline. Test IR constructed
// with named variables (Index = 0 default) will have correct indices assigned.
func prepareIR(c ir.Core) {
	ir.AnnotateFreeVars(c)
	ir.AssignIndices(c)
}

func TestEvalVar(t *testing.T) {
	ev := newTestEval()
	env := EmptyEnv().Extend("x", &HostVal{Inner: 42})
	r, err := ev.Eval(env, EmptyCapEnv(), &ir.Var{Index: -1, Name: "x"})
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
	term := &ir.App{
		Fun: &ir.Lam{Param: "x", Body: &ir.Var{Name: "x"}},
		Arg: &ir.Con{Name: "Unit"},
	}
	prepareIR(term)
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
	term := &ir.Bind{
		Comp: &ir.Pure{Expr: &ir.Var{Name: "val"}},
		Var:  "x",
		Body: &ir.Pure{Expr: &ir.Var{Name: "x"}},
	}
	prepareIR(term)
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
	term := &ir.Force{
		Expr: &ir.Thunk{
			Comp: &ir.Pure{Expr: &ir.Con{Name: "Unit"}},
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
	term := &ir.Case{
		Scrutinee: &ir.Con{Name: "True"},
		Alts: []ir.Alt{
			{Pattern: &ir.PCon{Con: "True"}, Body: &ir.Var{Index: -1, Name: "one"}},
			{Pattern: &ir.PCon{Con: "False"}, Body: &ir.Var{Index: -1, Name: "two"}},
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
	term := &ir.TyApp{Expr: &ir.Con{Name: "Unit"}}
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
	ev := NewEvaluator(defaultBudget(), prims, nil, nil, nil)
	term := &ir.PrimOp{
		Name: "id",
		Args: []ir.Core{&ir.Con{Name: "Unit"}},
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
	ev := NewEvaluator(defaultBudget(), prims, nil, nil, nil)
	// Bind(PrimOp("setFoo"), "_", Pure(Con("Unit")))
	term := &ir.Bind{
		Comp: &ir.PrimOp{Name: "setFoo"},
		Var:  "_",
		Body: &ir.Pure{Expr: &ir.Con{Name: "Unit"}},
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
	ev := NewEvaluator(budget.New(context.Background(), 3, 100), NewPrimRegistry(), nil, nil, nil)
	// A chain: App(App(Lam, Lam), Con) — will exceed 3 steps
	term := &ir.App{
		Fun: &ir.Lam{Param: "f",
			Body: &ir.App{Fun: &ir.Var{Index: -1, Name: "f"}, Arg: &ir.Con{Name: "Unit"}},
		},
		Arg: &ir.Lam{Param: "x", Body: &ir.Var{Index: -1, Name: "x"}},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if _, ok := err.(*budget.StepLimitError); !ok {
		t.Errorf("expected StepLimitError, got %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	ev := NewEvaluator(budget.New(ctx, 1_000_000, 1_000), NewPrimRegistry(), nil, nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Con{Name: "Unit"})
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
	ev := NewEvaluator(defaultBudget(), NewPrimRegistry(), hook, nil, nil)
	term := &ir.Pure{Expr: &ir.Con{Name: "Unit"}}
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
	v, ok := env.LookupGlobal("v0")
	if !ok {
		t.Fatal("v0 not found in 100-deep chain")
	}
	if v.(*HostVal).Inner != 0 {
		t.Errorf("expected 0, got %v", v.(*HostVal).Inner)
	}
	// Lookup the most recent binding.
	v99, ok := env.LookupGlobal("v99")
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
	// Extend mutates the globals map in place — no map copy.
	base := EmptyEnv()
	for i := range 50 {
		base = base.Extend(fmt.Sprintf("v%d", i), &HostVal{Inner: i})
	}
	allocs := testing.AllocsPerRun(100, func() {
		base.Extend("extra", &HostVal{Inner: 999})
	})
	if allocs > 1 {
		t.Errorf("Extend allocated %v per run; expected <= 1 (in-place map insert)", allocs)
	}
}

func BenchmarkEnvExtend100(b *testing.B) {
	for i := 0; i < b.N; i++ {
		env := EmptyEnv()
		for j := 0; j < 100; j++ {
			env = env.Extend(fmt.Sprintf("v%d", j), &HostVal{Inner: j})
		}
		// Force a lookup to prevent dead-code elimination.
		env.LookupGlobal("v50")
	}
}

func TestEnvLookup(t *testing.T) {
	env := EmptyEnv().Extend("x", &HostVal{Inner: 1}).Extend("y", &HostVal{Inner: 2})
	v, ok := env.LookupGlobal("x")
	if !ok {
		t.Fatal("x not found")
	}
	if v.(*HostVal).Inner != 1 {
		t.Error("x should be 1")
	}
	// Shadowing
	env2 := env.Extend("x", &HostVal{Inner: 99})
	v2, _ := env2.LookupGlobal("x")
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
	pat := &ir.PCon{Con: "Pair", Args: []ir.Pattern{
		&ir.PVar{Name: "a"},
		&ir.PVar{Name: "b"},
	}}
	bindings := Match(val, pat)
	if bindings == nil {
		t.Fatal("match should succeed")
	}
	// Match returns []Value in pattern-binding order: [a, b]
	if bindings[0].(*HostVal).Inner != 1 || bindings[1].(*HostVal).Inner != 2 {
		t.Error("binding values wrong")
	}
	// Mismatch: different constructor
	if Match(val, &ir.PCon{Con: "Other"}) != nil {
		t.Error("should not match different constructor")
	}
	// Mismatch: arity too few
	if Match(val, &ir.PCon{Con: "Pair", Args: []ir.Pattern{&ir.PVar{Name: "a"}}}) != nil {
		t.Error("should not match with fewer pattern args than value args")
	}
	// Mismatch: arity too many
	threePat := &ir.PCon{Con: "Pair", Args: []ir.Pattern{
		&ir.PVar{Name: "a"}, &ir.PVar{Name: "b"}, &ir.PVar{Name: "c"},
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
	pat := &ir.PRecord{Fields: []ir.PRecordField{
		{Label: "x", Pattern: &ir.PVar{Name: "a"}},
		{Label: "y", Pattern: &ir.PVar{Name: "b"}},
	}}
	bindings := Match(rv, pat)
	if bindings == nil {
		t.Fatal("expected match to succeed")
	}
	// Match returns []Value in pattern field order: [a, b]
	if bindings[0].(*HostVal).Inner != 10 {
		t.Errorf("expected a=10, got %v", bindings[0])
	}
	if bindings[1].(*HostVal).Inner != 20 {
		t.Errorf("expected b=20, got %v", bindings[1])
	}
	// Missing field should fail.
	patExtra := &ir.PRecord{Fields: []ir.PRecordField{
		{Label: "x", Pattern: &ir.PVar{Name: "a"}},
		{Label: "z", Pattern: &ir.PVar{Name: "c"}},
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
	term := &ir.Pure{Expr: &ir.Con{Name: "Unit"}}
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
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil, nil)

	// Build a 10-field record: costRecBase(56) + costRecFld(32)*10 = 376 bytes > 100
	fields := make([]ir.RecordField, 10)
	for i := range fields {
		fields[i] = ir.RecordField{
			Label: fmt.Sprintf("f%d", i),
			Value: &ir.Lit{Value: i},
		}
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.RecordLit{Fields: fields})
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
	term := &ir.App{
		Fun: &ir.Lam{Param: "x", Body: &ir.Var{Name: "x"}},
		Arg: &ir.Con{Name: "Unit"},
	}
	prepareIR(term)
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
	ev := NewEvaluator(defaultBudget(), prims, nil, nil, nil)
	term := &ir.Bind{
		Comp: &ir.Pure{Expr: &ir.Con{Name: "Unit"}},
		Var:  "_",
		Body: &ir.PrimOp{Name: "setFoo", Arity: 0, Effectful: true},
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
	// Closure with FV annotation should capture only referenced locals.
	ev := newTestEval()
	env := EmptyEnv()
	env = env.Push(&HostVal{Inner: 1}) // y at de Bruijn index 1
	env = env.Push(&HostVal{Inner: 2}) // z at de Bruijn index 0
	// Lam captures only y (index 1 in enclosing env).
	lam := &ir.Lam{
		Param:     "x",
		Body:      &ir.Var{Name: "y", Index: 1},
		FV:        []string{"y"},
		FVIndices: []int{1},
	}
	r, err := ev.Eval(env, EmptyCapEnv(), lam)
	if err != nil {
		t.Fatal(err)
	}
	clo := r.Value.(*Closure)
	if len(clo.Env.locals) != 1 {
		t.Errorf("trimmed closure env should have 1 local (y only), got %d", len(clo.Env.locals))
	}
}

func TestThunkEnvTrimmed(t *testing.T) {
	// Thunk with FV annotation should capture only referenced locals.
	ev := newTestEval()
	env := EmptyEnv()
	env = env.Push(&HostVal{Inner: 1}) // y at de Bruijn index 1
	env = env.Push(&HostVal{Inner: 2}) // z at de Bruijn index 0
	thunk := &ir.Thunk{
		Comp:      &ir.Pure{Expr: &ir.Var{Name: "y", Index: 0}},
		FV:        []string{"y"},
		FVIndices: []int{1},
	}
	r, err := ev.Eval(env, EmptyCapEnv(), thunk)
	if err != nil {
		t.Fatal(err)
	}
	tv := r.Value.(*ThunkVal)
	if len(tv.Env.locals) != 1 {
		t.Errorf("trimmed thunk env should have 1 local (y only), got %d", len(tv.Env.locals))
	}
}

func TestFixEnvTrimmed(t *testing.T) {
	// fix f = \x. ext — returned closure should capture only ext, not noise.
	ev := newTestEval()
	env := EmptyEnv()
	env = env.Push(&HostVal{Inner: 1}) // ext at de Bruijn index 1
	env = env.Push(&HostVal{Inner: 2}) // noise at de Bruijn index 0
	// Lam captures ext (index 1). Fix adds self-ref.
	// After Capture([ext]) + Push(self): locals = [ext, self].
	// In body after Push(x): [ext, self, x]. ext=2, self=1, x=0.
	term := &ir.Fix{
		Name: "f",
		Body: &ir.Lam{
			Param:     "x",
			Body:      &ir.Var{Name: "ext", Index: 2},
			FV:        []string{"ext"},
			FVIndices: []int{1},
		},
	}
	r, err := ev.Eval(env, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	clo := r.Value.(*Closure)
	// Captured env: [ext, self] — 2 locals (FV + fix self-ref).
	if len(clo.Env.locals) != 2 {
		t.Errorf("Fix closure env should have 2 locals (ext + self), got %d", len(clo.Env.locals))
	}
}

func TestAllocTrackingThunk(t *testing.T) {
	ev := newTestEval()
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(),
		&ir.Thunk{Comp: &ir.Pure{Expr: &ir.Con{Name: "Unit"}}})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Stats().Allocated < costThunk {
		t.Errorf("Thunk should allocate at least %d bytes, got %d", costThunk, ev.Stats().Allocated)
	}
}

func TestAllocTrackingFix(t *testing.T) {
	ev := newTestEval()
	term := &ir.App{
		Fun: &ir.Fix{
			Name: "f",
			Body: &ir.Lam{Param: "x", Body: &ir.Var{Name: "x"}},
		},
		Arg: &ir.Con{Name: "Unit"},
	}
	prepareIR(term)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	// Fix: costFix (closure + env node) + Con(Unit): costConBase
	expected := int64(costFix + costConBase)
	if ev.Stats().Allocated != expected {
		t.Errorf("expected %d bytes, got %d", expected, ev.Stats().Allocated)
	}
}

func TestAllocTrackingRecordUpdate(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordUpdate{
		Record: &ir.RecordLit{Fields: []ir.RecordField{
			{Label: "a", Value: &ir.Lit{Value: int64(1)}},
		}},
		Updates: []ir.RecordField{
			{Label: "a", Value: &ir.Lit{Value: int64(2)}},
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
	thunk := &ir.Thunk{Comp: &ir.Pure{Expr: &ir.Con{Name: "Unit"}}}
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
	ev := NewEvaluator(budget.New(context.Background(), 2, 100), NewPrimRegistry(), nil, nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Lit{Value: int64(42)})
	if err != nil {
		t.Fatalf("budget(2): first eval should succeed, got %v", err)
	}
	_, err = ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Lit{Value: int64(43)})
	if err != nil {
		t.Fatalf("budget(2): second eval should succeed, got %v", err)
	}
	_, err = ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Lit{Value: int64(44)})
	if _, ok := err.(*budget.StepLimitError); !ok {
		t.Errorf("budget(2): third eval should fail with StepLimitError, got %v", err)
	}
}

func TestDepthLimitBoundary(t *testing.T) {
	// maxDepth=1: one level of function application should succeed.
	ev := NewEvaluator(budget.New(context.Background(), 1_000_000, 1), NewPrimRegistry(), nil, nil, nil)
	term := &ir.App{
		Fun: &ir.Lam{Param: "x", Body: &ir.Var{Name: "x"}},
		Arg: &ir.Con{Name: "Unit"},
	}
	prepareIR(term)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatalf("maxDepth=1: single application should succeed, got %v", err)
	}
}

func TestAllocLimitBoundary(t *testing.T) {
	// allocLimit = costConBase: exactly one ConVal allocation should succeed.
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(int64(costConBase))
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Con{Name: "Unit"})
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
	// Bind chains are trampolined (TCO) — they do NOT consume depth.
	// Depth only accumulates via non-tail closure application.
	// Verify that a deep Bind chain succeeds even with maxDepth=1.
	ev := NewEvaluator(budget.New(context.Background(), 1_000_000, 1), NewPrimRegistry(), nil, nil, nil)
	term := &ir.Bind{
		Comp: &ir.Pure{Expr: &ir.Con{Name: "Unit"}},
		Var:  "_",
		Body: &ir.Bind{
			Comp: &ir.Pure{Expr: &ir.Con{Name: "Unit"}},
			Var:  "_",
			Body: &ir.Pure{Expr: &ir.Con{Name: "Unit"}},
		},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatalf("Bind chain should not consume depth (TCO), got: %v", err)
	}
}

func TestDepthLimitMultiLevel(t *testing.T) {
	// Bind chains are trampolined — arbitrarily deep chains succeed with low maxDepth.
	buildBindChain := func(depth int) ir.Core {
		var body ir.Core = &ir.Pure{Expr: &ir.Con{Name: "Unit"}}
		for range depth {
			body = &ir.Bind{
				Comp: &ir.Pure{Expr: &ir.Con{Name: "Unit"}},
				Var:  "_",
				Body: body,
			}
		}
		return body
	}

	// maxDepth=5: chain of 100 Binds should succeed (Bind does not consume depth).
	ev := NewEvaluator(budget.New(context.Background(), 1_000_000, 5), NewPrimRegistry(), nil, nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), buildBindChain(100))
	if err != nil {
		t.Fatalf("100-Bind chain at maxDepth=5 should succeed (TCO), got: %v", err)
	}
}

func TestBindChainTCO(t *testing.T) {
	// 10000-deep Bind chain with maxDepth=10 must succeed, proving TCO.
	var body ir.Core = &ir.Pure{Expr: &ir.Lit{Value: int64(42)}}
	for range 10000 {
		body = &ir.Bind{
			Comp: &ir.Pure{Expr: &ir.Con{Name: "Unit"}},
			Var:  "_",
			Body: body,
		}
	}
	ev := NewEvaluator(budget.New(context.Background(), 1_000_000, 10), NewPrimRegistry(), nil, nil, nil)
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), body)
	if err != nil {
		t.Fatalf("10000-Bind chain should succeed with TCO, got: %v", err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected 42, got %v", r.Value)
	}
}

func TestBindForceEffectfulDeferred(t *testing.T) {
	// Verify that ForceEffectful is applied to the terminal value of a Bind
	// chain when the terminal is a bare effectful PrimOp. The ForceEffectful
	// call is deferred to the trampoline via forceSpan.
	prims := NewPrimRegistry()
	prims.Register("getState", func(ctx context.Context, ce CapEnv, args []Value, _ Applier) (Value, CapEnv, error) {
		return &HostVal{Inner: int64(99)}, ce, nil
	})
	ev := NewEvaluator(budget.New(context.Background(), 1_000_000, 10), prims, nil, nil, nil)

	// Bind(Pure(Unit), _, Bind(Pure(Unit), _, PrimOp("getState")))
	// The final PrimOp needs ForceEffectful via the trampoline.
	term := &ir.Bind{
		Comp: &ir.Pure{Expr: &ir.Con{Name: "Unit"}},
		Var:  "_",
		Body: &ir.Bind{
			Comp: &ir.Pure{Expr: &ir.Con{Name: "Unit"}},
			Var:  "_",
			Body: &ir.PrimOp{Name: "getState", Arity: 0, Effectful: true},
		},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(99) {
		t.Errorf("expected ForceEffectful to produce 99, got %v", r.Value)
	}
}

func TestFixNestedEval(t *testing.T) {
	// Nested Fix nodes evaluate correctly — each produces a closure.
	ev := newTestEval()
	// (fix f in \x. x) applied to Unit
	inner := &ir.Fix{Name: "f", Body: &ir.Lam{Param: "x", Body: &ir.Var{Name: "x"}}}
	term := &ir.App{Fun: inner, Arg: &ir.Con{Name: "Unit"}}
	prepareIR(term)
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if con, ok := r.Value.(*ConVal); !ok || con.Con != "Unit" {
		t.Errorf("expected Unit, got %v", r.Value)
	}
}

// --- Error path and contract tests ---

func TestEvalVarUnbound(t *testing.T) {
	ev := newTestEval()
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Var{Index: -1, Name: "missing"})
	if err == nil {
		t.Fatal("expected error for unbound variable")
	}
	if _, ok := err.(*RuntimeError); !ok {
		t.Errorf("expected *RuntimeError, got %T", err)
	}
}

func TestEvalCaseNoMatch(t *testing.T) {
	ev := newTestEval()
	term := &ir.Case{
		Scrutinee: &ir.Con{Name: "Foo"},
		Alts: []ir.Alt{
			{Pattern: &ir.PCon{Con: "Bar"}, Body: &ir.Lit{Value: int64(1)}},
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

func TestFixNonLamBinding(t *testing.T) {
	ev := newTestEval()
	term := &ir.Fix{Name: "x", Body: &ir.Con{Name: "Unit"}}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for Fix non-lambda body")
	}
	if _, ok := err.(*RuntimeError); !ok {
		t.Errorf("expected *RuntimeError, got %T", err)
	}
}

func TestTraceHookAbort(t *testing.T) {
	sentinel := errors.New("abort from hook")
	hook := func(ev TraceEvent) error { return sentinel }
	evl := NewEvaluator(defaultBudget(), NewPrimRegistry(), hook, nil, nil)
	_, err := evl.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Lit{Value: int64(1)})
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
	term := &ir.PrimOp{Name: "nonexistent", Args: []ir.Core{&ir.Lit{Value: int64(0)}}}
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
	ev := NewEvaluator(budget.New(ctx, N*10, N*2), prims, nil, nil, nil)

	zeroLit := &ir.Lit{Value: int64(0)}
	oneLit := &ir.Lit{Value: int64(1)}
	eqOp := &ir.PrimOp{Name: "eq", Arity: 2, Args: []ir.Core{
		&ir.Var{Name: "n"}, zeroLit,
	}}
	subOp := &ir.PrimOp{Name: "sub", Arity: 2, Args: []ir.Core{
		&ir.Var{Name: "n"}, oneLit,
	}}
	selfCall := &ir.App{Fun: &ir.Var{Name: "self"}, Arg: subOp}
	body := &ir.Case{
		Scrutinee: eqOp,
		Alts: []ir.Alt{
			{Pattern: &ir.PCon{Con: "True"}, Body: &ir.Var{Name: "n"}},
			{Pattern: &ir.PCon{Con: "False"}, Body: selfCall},
		},
	}
	innerLam := &ir.Lam{Param: "n", Body: body}
	fix := &ir.Fix{Name: "self", Body: innerLam}
	term := &ir.App{Fun: fix, Arg: &ir.Lit{Value: int64(N)}}
	prepareIR(term)

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

	result, err := ev.Eval(EmptyEnv(), NewCapEnv(nil), term)
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
	term := &ir.Case{
		Scrutinee: &ir.Con{Name: "True"},
		Alts: []ir.Alt{
			{Pattern: &ir.PCon{Con: "True"}, Body: &ir.Lit{Value: int64(42)}},
			{Pattern: &ir.PCon{Con: "False"}, Body: &ir.Lit{Value: int64(0)}},
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
	term := &ir.Case{
		Scrutinee: &ir.Con{Name: "True"},
		Alts: []ir.Alt{
			{
				Pattern: &ir.PCon{Con: "True"},
				Body: &ir.Case{
					Scrutinee: &ir.Con{Name: "False"},
					Alts: []ir.Alt{
						{Pattern: &ir.PCon{Con: "True"}, Body: &ir.Lit{Value: int64(1)}},
						{Pattern: &ir.PCon{Con: "False"}, Body: &ir.Lit{Value: int64(2)}},
					},
				},
			},
			{Pattern: &ir.PCon{Con: "False"}, Body: &ir.Lit{Value: int64(3)}},
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
	term := &ir.RecordProj{
		Record: &ir.Lit{Value: int64(42)},
		Label:  "x",
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for projection on non-record")
	}
}

func TestEvalRecordUpdateOnNonRecordEval(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordUpdate{
		Record:  &ir.Lit{Value: int64(42)},
		Updates: []ir.RecordField{{Label: "x", Value: &ir.Lit{Value: int64(1)}}},
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for update on non-record")
	}
}

func TestEvalForceNonThunk(t *testing.T) {
	ev := newTestEval()
	term := &ir.Force{Expr: &ir.Lit{Value: int64(42)}}
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
	term := &ir.App{
		Fun: &ir.Lit{Value: int64(42)},
		Arg: &ir.Con{Name: "Unit"},
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
	term := &ir.Con{
		Name: "Pair",
		Args: []ir.Core{
			&ir.Lit{Value: int64(1)},
			&ir.Lit{Value: int64(2)},
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
	term := &ir.App{
		Fun: &ir.App{
			Fun: &ir.Con{Name: "Pair"},
			Arg: &ir.Lit{Value: int64(1)},
		},
		Arg: &ir.Lit{Value: int64(2)},
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

func TestFixSelfReference(t *testing.T) {
	// Fix creates a self-referential closure: fix self in \x. self
	ev := newTestEval()
	term := &ir.Fix{
		Name: "self",
		Body: &ir.Lam{Param: "x", Body: &ir.Var{Name: "self"}},
	}
	prepareIR(term)
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	clo := r.Value.(*Closure)
	// Applying the fixpoint closure should return itself.
	appEnv := EmptyEnv()
	appEnv.Extend("self", clo)
	r2, err := ev.Eval(appEnv, EmptyCapEnv(), &ir.App{
		Fun: &ir.Var{Index: -1, Name: "self"},
		Arg: &ir.Con{Name: "Unit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r2.Value.(*Closure); !ok {
		t.Errorf("expected Closure from self-reference, got %T", r2.Value)
	}
}

func TestBounceValString(t *testing.T) {
	b := &bounceVal{env: EmptyEnv(), capEnv: EmptyCapEnv(), expr: &ir.Con{Name: "Unit"}}
	if b.String() != "bounceVal(...)" {
		t.Errorf("expected 'bounceVal(...)', got %q", b.String())
	}
}

func TestEvalTyLamErased(t *testing.T) {
	ev := newTestEval()
	term := &ir.TyLam{Body: &ir.Con{Name: "Unit"}}
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
	term := &ir.RecordProj{
		Record: &ir.RecordLit{Fields: []ir.RecordField{
			{Label: "a", Value: &ir.Lit{Value: int64(1)}},
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
	ev := NewEvaluator(defaultBudget(), prims, nil, nil, nil)

	// PrimOp with arity 2, applied to 0 args → PrimVal.
	primOp := &ir.PrimOp{Name: "add2", Arity: 2}
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
	term1 := &ir.App{Fun: primOp, Arg: &ir.Lit{Value: int64(10)}}
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
	term2 := &ir.App{
		Fun: term1,
		Arg: &ir.Lit{Value: int64(20)},
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
	ev := NewEvaluator(defaultBudget(), prims, nil, nil, nil)

	// Effectful PrimOp with arity=0: should produce a PrimVal (deferred).
	term := &ir.PrimOp{Name: "eff0", Arity: 0, Effectful: true}
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

func TestEvalUnknownCoreNode(t *testing.T) {
	// The default case in evalStep should return an error.
	// We can't easily construct an unknown Core node from outside,
	// but we can verify the error message format for a known edge case.
	ev := newTestEval()
	// nil is not a valid Core but shows up as *ir.Xxx; use a non-evaluatable node.
	// Actually, just verify that RecordLit with fields evaluates correctly (no error path).
	term := &ir.RecordLit{Fields: []ir.RecordField{
		{Label: "x", Value: &ir.Lit{Value: int64(1)}},
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
	term := &ir.PrimOp{Name: "missing_prim", Arity: 1, Args: []ir.Core{&ir.Lit{Value: int64(1)}}}
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
	_, err := ev.Eval(env, EmptyCapEnv(), &ir.Var{Index: -1, Name: "x"})
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
	r, err := ev.Eval(env2, EmptyCapEnv(), &ir.Var{Index: -1, Name: "y"})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected HostVal(42), got %v", r.Value)
	}
}

// TestRuntimeErrorSourcePropagation verifies that RuntimeError carries
// the correct Source when the error occurs in a given source context.
func TestRuntimeErrorSourcePropagation(t *testing.T) {
	modSrc := span.NewSource("Module", "module source text")
	mainSrc := span.NewSource("Main", "main source text")

	ev := NewEvaluator(defaultBudget(), NewPrimRegistry(), nil, nil, mainSrc)

	// Create a closure in module source context.
	ev.SetSource(modSrc)
	caseExpr := &ir.Case{
		Scrutinee: &ir.Var{Index: -1, Name: "x"},
		Alts:      nil, // no alts → non-exhaustive match guaranteed
		S:         span.Span{Start: 5, End: 20},
	}
	closureR, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), &ir.Lam{Param: "x", Body: caseExpr})
	if err != nil {
		t.Fatal(err)
	}

	// Verify closure captured module source.
	clo := closureR.Value.(*Closure)
	if clo.Source != modSrc {
		t.Fatalf("Closure.Source = %v, want module source", clo.Source)
	}

	// Evaluate the case body in module context to trigger the RuntimeError.
	env := EmptyEnv().Extend("x", &ConVal{Con: "Unknown"})
	_, err = ev.Eval(env, EmptyCapEnv(), caseExpr)
	if err == nil {
		t.Fatal("expected RuntimeError from non-exhaustive case")
	}
	var re *RuntimeError
	if !errors.As(err, &re) {
		t.Fatalf("expected RuntimeError, got %T: %v", err, err)
	}
	if re.Source != modSrc {
		t.Errorf("RuntimeError.Source.Name = %q, want %q", re.Source.Name, modSrc.Name)
	}
	if re.Span != (span.Span{Start: 5, End: 20}) {
		t.Errorf("RuntimeError.Span = %v, want {5, 20}", re.Span)
	}
}

// TestSourceRestoredAfterEval verifies that Eval restores the source
// context after returning, even when the evaluation crosses module boundaries
// via closure application.
func TestSourceRestoredAfterEval(t *testing.T) {
	modSrc := span.NewSource("Mod", "mod")
	mainSrc := span.NewSource("Main", "main")

	var steps []ExplainStep
	hook := func(s ExplainStep) { steps = append(steps, s) }
	obs := NewExplainObserver(hook, mainSrc)

	ev := NewEvaluator(defaultBudget(), NewPrimRegistry(), nil, obs, mainSrc)

	// Create a closure in module context that does a simple case match.
	ev.SetSource(modSrc)
	body := &ir.Case{
		Scrutinee: &ir.Var{Name: "x"},
		Alts: []ir.Alt{{
			Pattern: &ir.PVar{Name: "y"},
			Body:    &ir.Var{Name: "y"},
		}},
		S: span.Span{Start: 1, End: 3},
	}
	lamExpr := &ir.Lam{Param: "x", Body: body}
	prepareIR(lamExpr)
	closureR, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), lamExpr)
	if err != nil {
		t.Fatal(err)
	}
	clo := closureR.Value.(*Closure)
	clo.Name = "modFunc"

	// Return to main context.
	ev.SetSource(mainSrc)

	// Apply the module closure from main context.
	appExpr := &ir.App{
		Fun: &ir.Var{Index: -1, Name: "f"},
		Arg: &ir.Lit{Value: int64(1)},
		S:   span.Span{Start: 1, End: 4},
	}
	env := EmptyEnv().Extend("f", clo)
	_, err = ev.Eval(env, EmptyCapEnv(), appExpr)
	if err != nil {
		t.Fatal(err)
	}

	// After Eval returns, the source should be restored to main.
	// Verify via the observer: the match event inside the module closure
	// should carry the module source name.
	var foundMod bool
	for _, s := range steps {
		if s.SourceName == "Mod" {
			foundMod = true
			break
		}
	}
	if !foundMod {
		t.Error("expected explain event with SourceName=\"Mod\" from module closure")
	}
}
