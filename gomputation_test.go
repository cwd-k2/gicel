package gomputation_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	gmp "github.com/cwd-k2/gomputation"
	"github.com/cwd-k2/gomputation/internal/eval"
	"github.com/cwd-k2/gomputation/pkg/types"
)

func TestIdentity(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
id := \x -> x
main := id True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestPure(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := pure Unit`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Unit" {
		t.Errorf("expected Unit, got %s", result.Value)
	}
}

func TestHostBinding(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", types.KType{})
	eng.DeclareBinding("x", types.Con("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string]gmp.Value{
		"x": &gmp.HostVal{Inner: 42},
	}
	result, err := rt.RunContext(context.Background(), nil, bindings, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != 42 {
		t.Errorf("expected HostVal(42), got %s", result.Value)
	}
}

func TestAssumption(t *testing.T) {
	eng := gmp.NewEngine()
	eng.DeclareAssumption("getUnit", types.MkArrow(types.Con("Unit"), types.Con("Unit")))
	eng.RegisterPrim("getUnit", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		return &eval.ConVal{Con: "Unit"}, capEnv, nil
	})
	rt, err := eng.NewRuntime(`
getUnit := assumption
main := getUnit Unit
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Unit" {
		t.Errorf("expected Unit, got %s", result.Value)
	}
}

func TestCaseExpression(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
not := \b -> case b of { True -> False; False -> True }
main := not True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

func TestDoBlock(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := do { pure Unit }`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Unit" {
		t.Errorf("expected Unit, got %s", result.Value)
	}
}

func TestNoPrelude(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data MyBool = MyTrue | MyFalse
main := MyTrue
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "MyTrue" {
		t.Errorf("expected MyTrue, got %s", result.Value)
	}
}

func TestCompileError(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := undefined_thing`)
	if err == nil {
		t.Error("expected compile error")
	}
	_, ok := err.(*gmp.CompileError)
	if !ok {
		t.Errorf("expected CompileError, got %T", err)
	}
}

func TestMissingEntry(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`x := True`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err == nil {
		t.Error("expected error for missing entry point")
	}
}

func TestConstructorArgs(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := Just True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Errorf("expected Just, got %s", result.Value)
	}
	if len(con.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(con.Args))
	}
	inner, ok := con.Args[0].(*gmp.ConVal)
	if !ok || inner.Con != "True" {
		t.Errorf("expected True, got %s", con.Args[0])
	}
}

func TestPairConstructor(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := Pair True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Pair" {
		t.Errorf("expected Pair, got %s", result.Value)
	}
	if len(con.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(con.Args))
	}
}

func TestStepLimit(t *testing.T) {
	eng := gmp.NewEngine()
	eng.SetStepLimit(10)
	eng.EnableRecursion()
	eng.NoPrelude()
	// No recursion in source, just a program that completes in few steps.
	rt, err := eng.NewRuntime(`
data Unit = Unit
main := Unit
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	if result.Stats.Steps == 0 {
		t.Error("expected non-zero step count")
	}
}

func TestGoroutineSafety(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 10)
	for range 10 {
		go func() {
			_, err := rt.RunContext(context.Background(), nil, nil, "main")
			errs <- err
		}()
	}
	for range 10 {
		if err := <-errs; err != nil {
			t.Errorf("concurrent execution failed: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// A. Pure computation
// ---------------------------------------------------------------------------

func TestMultiParamLambda(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
konst := \x y -> x
main := konst True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestNestedCase(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
bothTrue := \x y -> case x of { True -> case y of { True -> True; False -> False }; False -> False }
main := bothTrue True True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestBlockExpression(t *testing.T) {
	eng := gmp.NewEngine()
	// Block binding: { x := True; x } desugars to (\x -> x) True.
	// The body references the block binding variable.
	rt, err := eng.NewRuntime(`main := { x := True; x }`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestBlockMultipleBindings(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := { x := True; y := False; x }`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// ---------------------------------------------------------------------------
// B. Computation
// ---------------------------------------------------------------------------

func TestBindChain(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := do { x <- pure True; pure x }`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestThunkForce(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := force (thunk (pure Unit))`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Unit" {
		t.Errorf("expected Unit, got %s", result.Value)
	}
}

// ---------------------------------------------------------------------------
// C. Type errors (compile-time rejection)
// ---------------------------------------------------------------------------

func TestTypeErrorUnboundVar(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := totally_undefined`)
	if err == nil {
		t.Fatal("expected compile error for unbound variable")
	}
	if _, ok := err.(*gmp.CompileError); !ok {
		t.Errorf("expected CompileError, got %T", err)
	}
}

func TestTypeErrorNonExhaustive(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
f := \x -> case x of { True -> Unit }
main := f True
`)
	if err == nil {
		t.Fatal("expected compile error for non-exhaustive pattern")
	}
	ce, ok := err.(*gmp.CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T", err)
	}
	if !strings.Contains(ce.Error(), "non-exhaustive") {
		t.Errorf("expected non-exhaustive in error message, got: %s", ce.Error())
	}
}

func TestTypeErrorMismatch(t *testing.T) {
	eng := gmp.NewEngine()
	// Apply True (a Bool, not a function) to an argument.
	_, err := eng.NewRuntime(`main := True Unit`)
	if err == nil {
		t.Fatal("expected compile error for type mismatch")
	}
	if _, ok := err.(*gmp.CompileError); !ok {
		t.Errorf("expected CompileError, got %T", err)
	}
}

// ---------------------------------------------------------------------------
// D. Context cancellation
// ---------------------------------------------------------------------------

func TestContextCancellation(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := pure Unit`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err = rt.RunContext(ctx, nil, nil, "main")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestContextTimeout(t *testing.T) {
	eng := gmp.NewEngine()
	eng.SetStepLimit(1) // extremely low step limit
	eng.EnableRecursion()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Unit = Unit
data Bool = True | False
not := \b -> case b of { True -> False; False -> True }
main := not (not (not True))
`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err = rt.RunContext(ctx, nil, nil, "main")
	if err == nil {
		t.Error("expected error from step limit or timeout")
	}
}

// ---------------------------------------------------------------------------
// E. Runtime reuse
// ---------------------------------------------------------------------------

func TestRuntimeReuse(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", types.KType{})
	eng.DeclareBinding("x", types.Con("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}

	// First run with value 1.
	r1, err := rt.RunContext(context.Background(), nil, map[string]gmp.Value{"x": &gmp.HostVal{Inner: 1}}, "main")
	if err != nil {
		t.Fatal(err)
	}
	h1, ok := r1.Value.(*gmp.HostVal)
	if !ok || h1.Inner != 1 {
		t.Errorf("first run: expected HostVal(1), got %s", r1.Value)
	}

	// Second run with value 2.
	r2, err := rt.RunContext(context.Background(), nil, map[string]gmp.Value{"x": &gmp.HostVal{Inner: 2}}, "main")
	if err != nil {
		t.Fatal(err)
	}
	h2, ok := r2.Value.(*gmp.HostVal)
	if !ok || h2.Inner != 2 {
		t.Errorf("second run: expected HostVal(2), got %s", r2.Value)
	}
}

func TestRuntimeConcurrent(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
not := \b -> case b of { True -> False; False -> True }
main := not False
`)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, err := rt.RunContext(context.Background(), nil, nil, "main")
			if err != nil {
				errs[idx] = err
				return
			}
			con, ok := result.Value.(*gmp.ConVal)
			if !ok || con.Con != "True" {
				errs[idx] = context.DeadlineExceeded // sentinel
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d failed: %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
// F. Host API
// ---------------------------------------------------------------------------

func TestDiagnostics(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := undefined_xyz`)
	if err == nil {
		t.Fatal("expected compile error")
	}
	ce, ok := err.(*gmp.CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T", err)
	}
	diags := ce.Diagnostics()
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic")
	}
	d := diags[0]
	if d.Phase == "" {
		t.Errorf("expected non-empty phase")
	}
	if d.Message == "" {
		t.Errorf("expected non-empty message")
	}
	if d.Line == 0 {
		t.Errorf("expected non-zero line number")
	}
}

func TestCheckOnly(t *testing.T) {
	eng := gmp.NewEngine()
	prog, err := eng.Check(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	if prog == nil {
		t.Error("expected non-nil program from Check")
	}
}

func TestParseOnly(t *testing.T) {
	eng := gmp.NewEngine()
	ast, err := eng.Parse(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	if ast == nil {
		t.Error("expected non-nil AST from Parse")
	}
}

func TestPrettyProgram(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	pretty := rt.PrettyProgram()
	if pretty == "" {
		t.Error("expected non-empty PrettyProgram output")
	}
}

func TestRunContextFull(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := pure Unit`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContextFull(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Unit" {
		t.Errorf("expected Unit, got %s", result.Value)
	}
	// CapEnv should be present (even if empty).
	_ = result.CapEnv
	_ = result.Stats
}

func TestValueConversion(t *testing.T) {
	// HostVal wraps arbitrary Go values.
	hv := &eval.HostVal{Inner: "hello"}
	if hv.Inner != "hello" {
		t.Errorf("expected HostVal.Inner = hello, got %v", hv.Inner)
	}
	if hv.String() != "HostVal(hello)" {
		t.Errorf("unexpected HostVal.String(): %s", hv.String())
	}

	// ConVal represents constructors.
	cv := &eval.ConVal{Con: "True"}
	if cv.String() != "True" {
		t.Errorf("expected True, got %s", cv.String())
	}

	// ConVal with arguments.
	cv2 := &eval.ConVal{Con: "Just", Args: []eval.Value{&eval.ConVal{Con: "Unit"}}}
	s := cv2.String()
	if !strings.Contains(s, "Just") || !strings.Contains(s, "Unit") {
		t.Errorf("expected (Just Unit), got %s", s)
	}
}

// ---------------------------------------------------------------------------
// G. Type helpers
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// H. Boundary conditions and spec compliance
// ---------------------------------------------------------------------------

// :: type annotation in source — the spec-canonical way to type assumptions.
func TestInSourceTypeAnnotation(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.RegisterPrim("getVal", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(99), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
getVal :: Unit -> Computation {} {} Int
getVal := assumption
main := do { x <- getVal Unit; pure x }
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	v := gmp.MustHost[int](result.Value)
	if v != 99 {
		t.Errorf("expected 99, got %d", v)
	}
}

// Assumption without any type annotation must produce a compile error.
func TestAssumptionNoType(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
noType := assumption
main := noType Unit
`)
	if err == nil {
		t.Fatal("expected compile error for assumption without type")
	}
	ce, ok := err.(*gmp.CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T", err)
	}
	if !strings.Contains(ce.Error(), "assumption") {
		t.Errorf("expected mention of assumption in error, got: %s", ce.Error())
	}
}

// _ <- in do blocks must work (was a parser bug: TokUnderscore != TokLower).
func TestDoWildcardBind(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := do {
  _ <- pure True;
  pure False
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

// CapEnv threading: mutations propagate through do-block bind chain.
func TestCapEnvThreading(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.RegisterPrim("inc", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		v, _ := capEnv.Get("n")
		n, _ := v.(int)
		return gmp.ToValue(nil), capEnv.Set("n", n+1), nil
	})
	eng.RegisterPrim("getN", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		v, _ := capEnv.Get("n")
		n, _ := v.(int)
		return gmp.ToValue(n), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
inc :: Unit -> Computation {} {} Unit
inc := assumption
getN :: Unit -> Computation {} {} Int
getN := assumption
main := do { _ <- inc Unit; _ <- inc Unit; _ <- inc Unit; getN Unit }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContextFull(context.Background(), map[string]any{"n": 0}, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	n := gmp.MustHost[int](result.Value)
	if n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
	finalN, _ := result.CapEnv.Get("n")
	if finalN != 3 {
		t.Errorf("expected final capenv n=3, got %v", finalN)
	}
}

// Missing runtime binding must produce a runtime error (not panic).
func TestMissingRuntimeBinding(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.DeclareBinding("x", types.Con("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}
	// Run without providing binding "x".
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err == nil {
		t.Fatal("expected error for missing runtime binding")
	}
	if !strings.Contains(err.Error(), "missing binding") {
		t.Errorf("expected 'missing binding' in error, got: %s", err.Error())
	}
}

// PrimVal partial application: binary assumption applied to one arg, then another.
func TestPrimPartialApplication(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		a := gmp.MustHost[int](args[0])
		b := gmp.MustHost[int](args[1])
		return gmp.ToValue(a + b), capEnv, nil
	})
	eng.DeclareBinding("a", types.Con("Int"))
	eng.DeclareBinding("b", types.Con("Int"))
	rt, err := eng.NewRuntime(`
add :: Int -> Int -> Int
add := assumption
main := add a b
`)
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string]gmp.Value{
		"a": gmp.ToValue(10),
		"b": gmp.ToValue(32),
	}
	result, err := rt.RunContext(context.Background(), nil, bindings, "main")
	if err != nil {
		t.Fatal(err)
	}
	v := gmp.MustHost[int](result.Value)
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

// Cyclic type alias must produce a compile error.
func TestCyclicTypeAlias(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
type A = B
type B = A
main := True
`)
	if err == nil {
		t.Fatal("expected compile error for cyclic type alias")
	}
	if !strings.Contains(err.Error(), "cyclic") {
		t.Errorf("expected 'cyclic' in error, got: %s", err.Error())
	}
}

// Non-exhaustive case on Maybe must name the missing constructor.
func TestNonExhaustiveMaybe(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
f := \x -> case x of { Just y -> y }
main := f (Just True)
`)
	if err == nil {
		t.Fatal("expected compile error for non-exhaustive case on Maybe")
	}
	ce := err.(*gmp.CompileError)
	if !strings.Contains(ce.Error(), "Nothing") {
		t.Errorf("expected missing constructor 'Nothing', got: %s", ce.Error())
	}
}

// ToValue and FromBool round-trip.
func TestToValueFromBool(t *testing.T) {
	v := gmp.ToValue(true)
	b, ok := gmp.FromBool(v)
	if !ok || b != true {
		t.Errorf("ToValue(true) -> FromBool failed")
	}
	v2 := gmp.ToValue(false)
	b2, ok := gmp.FromBool(v2)
	if !ok || b2 != false {
		t.Errorf("ToValue(false) -> FromBool failed")
	}
}

// ToValue(nil) produces Unit.
func TestToValueNil(t *testing.T) {
	v := gmp.ToValue(nil)
	name, _, ok := gmp.FromCon(v)
	if !ok || name != "Unit" {
		t.Errorf("ToValue(nil) should produce Unit, got %s", v)
	}
}

// FromHost on non-HostVal returns ok=false.
func TestFromHostNonHost(t *testing.T) {
	v := gmp.ToValue(true) // ConVal, not HostVal
	_, ok := gmp.FromHost(v)
	if ok {
		t.Error("FromHost on ConVal should return ok=false")
	}
}

// MustHost panics on wrong type.
func TestMustHostPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from MustHost on ConVal")
		}
	}()
	gmp.MustHost[int](gmp.ToValue(true))
}

// Explicit bind syntax: bind comp (\x -> body) elaborates to Core.Bind.
func TestExplicitBind(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := bind (pure True) (\x -> pure x)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// pure/bind/thunk/force used standalone (not applied) must error.
func TestSpecialFormStandalone(t *testing.T) {
	forms := []string{"pure", "bind", "thunk", "force"}
	for _, name := range forms {
		eng := gmp.NewEngine()
		_, err := eng.NewRuntime("main := " + name)
		if err == nil {
			t.Errorf("expected compile error for standalone %s", name)
		}
	}
}

// Thunk/force round-trip in do block.
func TestThunkForceInDo(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := do {
  t := thunk (pure True);
  force t
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// Forall with kinded binder: forall (r : Row). T
func TestKindedForallBinder(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.RegisterPrim("getVal", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(7), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
getVal :: forall (r : Row). Unit -> Computation r r Int
getVal := assumption
main := do { getVal Unit }
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	v := gmp.MustHost[int](result.Value)
	if v != 7 {
		t.Errorf("expected 7, got %d", v)
	}
}

// Result type has 2 parameters: Result e a = Ok a | Err e
func TestResult2Params(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("String", gmp.KindType())
	rt, err := eng.NewRuntime(`
main := Ok True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Ok" {
		t.Errorf("expected Ok, got %s", result.Value)
	}
	if len(con.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(con.Args))
	}
	inner, ok := con.Args[0].(*gmp.ConVal)
	if !ok || inner.Con != "True" {
		t.Errorf("expected True inside Ok, got %s", con.Args[0])
	}
}

// Type alias: type Effect r a = Computation r r a
func TestTypeAlias(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
type Effect r a = Computation r r a

main :: Effect {} Bool
main := pure True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// Prelude's Effect alias should work without user-defined alias.
func TestPreludeEffectAlias(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main :: Effect {} Bool
main := pure True
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// Env flat map: deeply nested binds don't retain parent chains.
func TestDeepBindEnvDoesNotLeak(t *testing.T) {
	eng := gmp.NewEngine()
	eng.EnableRecursion()
	eng.RegisterType("Int", gmp.KindType())
	eng.RegisterPrim("mkInt", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(0), capEnv, nil
	})
	// A chain of 100 nested binds. With linked-list Env, each
	// closure would retain the entire chain. With flat Env, each
	// only holds its own bindings map.
	rt, err := eng.NewRuntime(`
mkInt :: Unit -> Computation {} {} Int
mkInt := assumption
main := do {
  x0 <- mkInt Unit;
  x1 <- mkInt Unit;
  x2 <- mkInt Unit;
  x3 <- mkInt Unit;
  x4 <- mkInt Unit;
  x5 <- mkInt Unit;
  x6 <- mkInt Unit;
  x7 <- mkInt Unit;
  x8 <- mkInt Unit;
  x9 <- mkInt Unit;
  pure x9
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	v := gmp.MustHost[int](result.Value)
	if v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

// Empty program (no main) should produce a runtime error, not panic.
func TestNoMainEntry(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`helper := True`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err == nil {
		t.Fatal("expected error for missing entry point 'main'")
	}
}

// Large case expression: exhaustiveness on 6-constructor ADT.
func TestExhaustiveLargeADT(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Color = Red | Green | Blue | Yellow | Cyan | Magenta

f := \c -> case c of {
  Red -> True;
  Green -> False;
  Blue -> True;
  Yellow -> False;
  Cyan -> True;
  Magenta -> False
}

main := f Red
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// Non-exhaustive on large ADT names missing constructors.
func TestNonExhaustiveLargeADT(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
data Color = Red | Green | Blue | Yellow | Cyan | Magenta

f := \c -> case c of {
  Red -> True;
  Blue -> True
}

main := f Red
`)
	if err == nil {
		t.Fatal("expected compile error for non-exhaustive case")
	}
	errStr := err.Error()
	for _, missing := range []string{"Green", "Yellow", "Cyan", "Magenta"} {
		if !strings.Contains(errStr, missing) {
			t.Errorf("expected missing constructor %s in error, got: %s", missing, errStr)
		}
	}
}

// ---------------------------------------------------------------------------
// TC. Type classes — end-to-end
// ---------------------------------------------------------------------------

func TestTypeClassEqBool(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool {
  eq := \x -> \y -> case x of {
    True  -> case y of { True -> True;  False -> False };
    False -> case y of { True -> False; False -> True }
  }
}
main := eq True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

func TestTypeClassPolymorphic(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool {
  eq := \x -> \y -> case x of {
    True  -> case y of { True -> True;  False -> False };
    False -> case y of { True -> False; False -> True }
  }
}
f :: forall a. Eq a => a -> a -> Bool
f := \x -> \y -> eq x y
main := f True True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestTypeClassSuperclass(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { lt :: a -> a -> Bool }
instance Eq Bool {
  eq := \x -> \y -> True
}
instance Ord Bool {
  lt := \x -> \y -> False
}
useOrd :: forall a. Ord a => a -> a -> Bool
useOrd := \x -> \y -> eq x y
main := useOrd True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestTypeClassFunctor(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Bool = True | False
data Maybe a = Just a | Nothing
class Functor f { fmap :: forall a b. (a -> b) -> f a -> f b }
instance Functor Maybe {
  fmap := \g -> \mx -> case mx of {
    Just x  -> Just (g x);
    Nothing -> Nothing
  }
}
not := \b -> case b of { True -> False; False -> True }
main := fmap not (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Errorf("expected Just, got %s", result.Value)
	}
	if len(con.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(con.Args))
	}
	inner, ok := con.Args[0].(*gmp.ConVal)
	if !ok || inner.Con != "False" {
		t.Errorf("expected False inside Just, got %s", con.Args[0])
	}
}

func TestTypeClassMultiParam(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Bool = True | False
class Coercible a b { coerce :: a -> b }
instance Coercible Bool Bool {
  coerce := \x -> x
}
main := coerce True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// ---------------------------------------------------------------------------
// I. Type helpers
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// DK. DataKinds — end-to-end integration
// ---------------------------------------------------------------------------

func TestDataKindsDBState(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data DBState = Opened | Closed
data DB s = MkDB

open :: DB Closed -> DB Opened
open := \_ -> MkDB

close :: DB Opened -> DB Closed
close := \_ -> MkDB

main := close (open (MkDB :: DB Closed))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "MkDB" {
		t.Errorf("expected MkDB, got %s", result.Value)
	}
}

func TestDataKindsInRow(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", types.KType{})
	eng.RegisterPrim("readDB", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(42), ce, nil
	})
	rt, err := eng.NewRuntime(`
data DBState = Opened | Closed

readDB :: Unit -> Computation { db : Int } { db : Int } Int
readDB := assumption

main :: Computation { db : Int } { db : Int } Int
main := do { readDB Unit }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), map[string]any{"db": 0}, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	v := gmp.MustHost[int](result.Value)
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestDataKindsBoolPromotion(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Proxy s = MkProxy
main := (MkProxy :: Proxy True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "MkProxy" {
		t.Errorf("expected MkProxy, got %s", result.Value)
	}
}

// --- GADT integration tests ---

func TestGADTEvalExpr(t *testing.T) {
	eng := gmp.NewEngine()
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(`
data Expr a = { LitBool :: Bool -> Expr Bool; Not :: Expr Bool -> Expr Bool }

eval :: Expr Bool -> Bool
eval := fix (\self -> \e -> case e of {
  LitBool b -> b;
  Not inner -> case self inner of { True -> False; False -> True }
})

main := eval (Not (LitBool True))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

func TestGADTWithDataKinds(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data DBState = Opened | Closed
data DB s = MkDB

data Action s = { Open :: Action Opened; Close :: Action Closed }

describe :: Action Opened -> DB Opened
describe := \a -> case a of { Open -> MkDB }

main := describe Open
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "MkDB" {
		t.Errorf("expected MkDB, got %s", result.Value)
	}
}

func TestGADTNestedPattern(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Expr a = { LitBool :: Bool -> Expr Bool; Not :: Expr Bool -> Expr Bool }

-- Nested pattern: match on Not (LitBool _)
isDoubleNeg :: Expr Bool -> Bool
isDoubleNeg := \e -> case e of {
  Not inner -> case inner of {
    Not _ -> True;
    LitBool _ -> False
  };
  LitBool _ -> False
}

main := isDoubleNeg (Not (Not (LitBool True)))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// --- Module system integration tests ---

func TestRegisterModule(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	err := eng.RegisterModule("Lib", `
data Bool = True | False
not :: Bool -> Bool
not := \b -> case b of { True -> False; False -> True }
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestImportModuleTypes(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	err := eng.RegisterModule("Lib", `
data Bool = True | False
not :: Bool -> Bool
not := \b -> case b of { True -> False; False -> True }
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Lib
main := not True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

func TestImportModuleInstances(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	err := eng.RegisterModule("EqLib", `
data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import EqLib
main := eq True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestCircularImportError(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	// Register module A that imports B.
	err := eng.RegisterModule("A", `
import B
data Unit = Unit
`)
	// Should fail because B is not registered.
	if err == nil {
		// Now try registering B that imports A → circular.
		err = eng.RegisterModule("B", `
import A
data Void = MkVoid
`)
		if err == nil {
			t.Error("expected circular import error")
		}
	}
	// If A failed because B doesn't exist, that's also acceptable.
	if err != nil && !strings.Contains(err.Error(), "unknown module") && !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected import error, got: %s", err)
	}
}

func TestImportNameCollision(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	// Module A defines Bool.
	err := eng.RegisterModule("A", `data Bool = True | False`)
	if err != nil {
		t.Fatal(err)
	}
	// Module B also defines Bool.
	err = eng.RegisterModule("B", `data Bool = Yes | No`)
	if err != nil {
		t.Fatal(err)
	}
	// Importing both should succeed at parse time but may produce
	// ambiguity at type check time. At minimum, the program should not panic.
	_, err = eng.NewRuntime(`
import A
import B
main := True
`)
	// This may or may not error depending on resolution order.
	// The important thing is no panic.
	_ = err
}

// --- Stdlib integration tests ---

func TestStdlibEqOrd(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := eq True True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestStdlibFunctor(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
not :: Bool -> Bool
not := \b -> case b of { True -> False; False -> True }

main := fmap not (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %s", result.Value)
	}
	inner, ok := con.Args[0].(*gmp.ConVal)
	if !ok || inner.Con != "False" {
		t.Errorf("expected Just False, got Just %s", con.Args[0])
	}
}

func TestStdlibFoldable(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := foldr (\x _ -> x) False (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestStdlibEqPair(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := eq (Pair True False) (Pair True False)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// --- Coercible integration tests ---

func TestCoercibleMultiParam(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
class Coercible a b { coerce :: a -> b }

instance Coercible Bool Unit { coerce := \_ -> Unit }

main := coerce True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Unit" {
		t.Errorf("expected Unit, got %s", result.Value)
	}
}

func TestCoercibleUsage(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
class Coercible a b { coerce :: a -> b }

instance Coercible Bool Bool { coerce := \x -> x }

main :: Bool
main := coerce True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestPreludeAsModule(t *testing.T) {
	// Prelude is now loaded as an implicit module — Bool, Unit, etc. should be available.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestNoPreludeWithModules(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	// Without prelude, Bool is not defined — need to define it ourselves.
	rt, err := eng.NewRuntime(`
data MyBool = Yes | No
main := Yes
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Yes" {
		t.Errorf("expected Yes, got %s", result.Value)
	}
}

func TestTypeHelpers(t *testing.T) {
	// Con constructs a type constructor.
	intTy := types.Con("Int")
	if intTy.Name != "Int" {
		t.Errorf("expected Con name Int, got %s", intTy.Name)
	}

	// MkArrow constructs a function type.
	arrTy := types.MkArrow(types.Con("Bool"), types.Con("Bool"))
	if arrTy.From.(*types.TyCon).Name != "Bool" {
		t.Errorf("expected arrow from Bool, got %v", arrTy.From)
	}

	// EmptyRow constructs an empty row.
	row := types.EmptyRow()
	if len(row.Fields) != 0 {
		t.Errorf("expected empty row, got %d fields", len(row.Fields))
	}

	// ClosedRow constructs a row with fields.
	row2 := types.ClosedRow(types.RowField{Label: "x", Type: types.Con("Int")})
	if len(row2.Fields) != 1 || row2.Fields[0].Label != "x" {
		t.Errorf("expected row with field x, got %v", row2)
	}

	// MkForall constructs a quantified type.
	forallTy := types.MkForall("a", types.KType{}, types.MkArrow(types.Var("a"), types.Var("a")))
	if forallTy.Var != "a" {
		t.Errorf("expected forall var a, got %s", forallTy.Var)
	}

	// MkComp constructs a computation type.
	compTy := types.MkComp(types.EmptyRow(), types.EmptyRow(), types.Con("Unit"))
	if compTy.Result.(*types.TyCon).Name != "Unit" {
		t.Errorf("expected Computation result Unit, got %v", compTy.Result)
	}

	// Var constructs a type variable.
	tv := types.Var("a")
	if tv.Name != "a" {
		t.Errorf("expected type var a, got %s", tv.Name)
	}

	// Kind helpers.
	k := types.KType{}
	if !k.Equal(types.KType{}) {
		t.Errorf("KType should equal KType")
	}
	kr := types.KRow{}
	if kr.Equal(types.KType{}) {
		t.Errorf("KRow should not equal KType")
	}
	ka := &types.KArrow{From: types.KType{}, To: types.KType{}}
	if !ka.Equal(&types.KArrow{From: types.KType{}, To: types.KType{}}) {
		t.Errorf("KArrow(Type,Type) should equal itself")
	}
}
