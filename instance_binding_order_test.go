package gicel_test

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel"
)

// TestInstanceMethodReferencesRegularBinding verifies that an instance
// method can reference a regular (non-assumption) annotated binding.
// This requires both:
//   - Phase 7.5 (type-level: annotated bindings visible to instance methods)
//   - SortBindings (runtime: dependency-ordered evaluation)
func TestInstanceMethodReferencesRegularBinding(t *testing.T) {
	eng := gicel.NewEngine()
	rt, err := eng.NewRuntime(`
class Wrap f { wrap :: \ a. a -> f a }

myJust :: \ a. a -> Maybe a
myJust := \x -> Just x

instance Wrap Maybe { wrap := myJust }

main := wrap True
`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	r, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	con, ok := r.Value.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just True, got %v", r.Value)
	}
}

// TestInstanceMethodReferencesChain verifies a chain:
// assumption -> helper -> instance method -> user code.
func TestInstanceMethodReferencesChain(t *testing.T) {
	eng := gicel.NewEngine()
	_ = gicel.Num(eng)
	rt, err := eng.NewRuntime(`
import Std.Num

class Scale a { scale :: Int -> a -> a }

scaleInt :: Int -> Int -> Int
scaleInt := \n -> \x -> n * x

instance Scale Int { scale := scaleInt }

main := scale 3 10
`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	r, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	hv, ok := r.Value.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", r.Value)
	}
	if hv.Inner.(int64) != 30 {
		t.Fatalf("expected 30, got %v", hv.Inner)
	}
}
