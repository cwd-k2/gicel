// Budget tests — nesting limit.
// Does NOT cover: step, depth, alloc limits (tested via eval integration tests).
package budget

import (
	"context"
	"testing"
)

func TestNestUnest(t *testing.T) {
	b := New(context.Background(), 0, 0)
	b.SetNestingLimit(3)

	// Should succeed up to limit.
	for i := 0; i < 3; i++ {
		if err := b.Nest(); err != nil {
			t.Fatalf("Nest() #%d: unexpected error: %v", i+1, err)
		}
	}
	if b.Nesting() != 3 {
		t.Fatalf("expected nesting=3, got %d", b.Nesting())
	}

	// Exceeding limit should error.
	err := b.Nest()
	if err == nil {
		t.Fatal("expected NestingLimitError")
	}
	if _, ok := err.(*NestingLimitError); !ok {
		t.Fatalf("expected *NestingLimitError, got %T: %v", err, err)
	}

	// Exceeding Nest still incremented counter (consistent with Enter).
	// Unnest twice to get back within limit.
	b.Unnest()
	b.Unnest()
	if b.Nesting() != 2 {
		t.Fatalf("expected nesting=2 after two Unnest, got %d", b.Nesting())
	}

	// After unnest, Nest should succeed again.
	if err := b.Nest(); err != nil {
		t.Fatalf("Nest() after Unnest: unexpected error: %v", err)
	}
}

func TestNestingLimitZeroDisabled(t *testing.T) {
	b := New(context.Background(), 0, 0)
	// maxNesting=0 means disabled.
	for i := 0; i < 10000; i++ {
		if err := b.Nest(); err != nil {
			t.Fatalf("Nest() #%d: unexpected error with disabled limit: %v", i+1, err)
		}
	}
}

func TestNestingLimitNegativeClamped(t *testing.T) {
	b := New(context.Background(), 0, 0)
	b.SetNestingLimit(-5)
	if b.MaxNesting() != 0 {
		t.Fatalf("expected maxNesting=0 after negative input, got %d", b.MaxNesting())
	}
}
