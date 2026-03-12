package eval

import (
	"context"
	"testing"

	"github.com/cwd-k2/gomputation/internal/core"
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
	// case True of { True -> HostVal(1); False -> HostVal(2) }
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
	prims.Register("id", func(ctx context.Context, cap CapEnv, args []Value) (Value, CapEnv, error) {
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
	prims.Register("setFoo", func(ctx context.Context, cap CapEnv, args []Value) (Value, CapEnv, error) {
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
	if err != context.Canceled {
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
	// Mismatch
	if Match(val, &core.PCon{Con: "Other"}) != nil {
		t.Error("should not match different constructor")
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
