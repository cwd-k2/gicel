// Eval nesting-limit tests — structural nesting depth guard.
// Does NOT cover: step/depth/alloc limits (eval_test.go), PrettyValue (explain tests).
package eval

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/budget"
	"github.com/cwd-k2/gicel/internal/core"
)

// deepApp builds App(id, App(id, ... App(id, Lit(1)) ...)) with the given depth.
// Each App evaluates its Arg in non-tail position, growing Go stack.
func deepApp(depth int) core.Core {
	var expr core.Core = &core.Lit{Value: int64(1)}
	for i := 0; i < depth; i++ {
		expr = &core.App{
			Fun: &core.Lam{Param: "_", Body: &core.Var{Name: "_"}},
			Arg: expr,
		}
	}
	return expr
}

// deepCon builds Con("C", [Con("C", [... Con("C", [Lit(1)]) ...])]) with the given depth.
func deepCon(depth int) core.Core {
	var expr core.Core = &core.Lit{Value: int64(1)}
	for i := 0; i < depth; i++ {
		expr = &core.Con{Name: "C", Args: []core.Core{expr}}
	}
	return expr
}

func nestingBudget(maxNesting int) *budget.Budget {
	b := budget.New(context.Background(), 1_000_000, 1_000)
	b.SetAllocLimit(100 * 1024 * 1024)
	b.SetNestingLimit(maxNesting)
	return b
}

func TestNestingLimit_DeepApp(t *testing.T) {
	// Nesting limit of 50 should reject depth-200 nested App.
	b := nestingBudget(50)
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), deepApp(200))
	if err == nil {
		t.Fatal("expected NestingLimitError for deeply nested App")
	}
	if _, ok := err.(*budget.NestingLimitError); !ok {
		t.Fatalf("expected *NestingLimitError, got %T: %v", err, err)
	}
}

func TestNestingLimit_DeepAppWithinLimit(t *testing.T) {
	// Nesting limit of 50 should allow depth-30 nested App.
	b := nestingBudget(50)
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil, nil)
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), deepApp(30))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(1) {
		t.Errorf("expected HostVal(1), got %v", r.Value)
	}
}

func TestNestingLimit_DeepCon(t *testing.T) {
	// Deeply nested constructor arguments should also be caught.
	b := nestingBudget(50)
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil, nil)
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), deepCon(200))
	if err == nil {
		t.Fatal("expected NestingLimitError for deeply nested Con")
	}
	if _, ok := err.(*budget.NestingLimitError); !ok {
		t.Fatalf("expected *NestingLimitError, got %T: %v", err, err)
	}
}

func TestNestingLimit_Disabled(t *testing.T) {
	// maxNesting=0 means disabled — deep nesting should succeed.
	b := nestingBudget(0)
	ev := NewEvaluator(b, NewPrimRegistry(), nil, nil, nil)
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), deepApp(500))
	if err != nil {
		t.Fatalf("unexpected error with disabled nesting limit: %v", err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(1) {
		t.Errorf("expected HostVal(1), got %v", r.Value)
	}
}
