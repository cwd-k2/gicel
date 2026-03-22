package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// TestInstanceMethodReferencesRegularBinding verifies that an instance
// method can reference a regular (non-assumption) annotated binding.
// This requires both:
//   - Phase 7.5 (type-level: annotated bindings visible to instance methods)
//   - SortBindings (runtime: dependency-ordered evaluation)
func TestInstanceMethodReferencesRegularBinding(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data Wrap := \f. { wrap: \ a. a -> f a }

myJust: \ a. a -> Maybe a
myJust := \x. Just x

impl Wrap Maybe := { wrap := myJust }

main := wrap True
`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	r, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	con, ok := r.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just True, got %v", r.Value)
	}
}

// TestInstanceMethodReferencesChain verifies a chain:
// assumption -> helper -> instance method -> user code.
func TestInstanceMethodReferencesChain(t *testing.T) {
	eng := NewEngine()
	_ = stdlib.Prelude(eng)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

data Scale := \a. { scale: Int -> a -> a }

scaleInt: Int -> Int -> Int
scaleInt := \n x. n * x

impl Scale Int := { scale := scaleInt }

main := scale 3 10
`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	r, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	hv, ok := r.Value.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", r.Value)
	}
	if hv.Inner.(int64) != 30 {
		t.Fatalf("expected 30, got %v", hv.Inner)
	}
}
