// Walk/Transform tests — concurrent safety and bind chain handling.
// Does NOT cover: index_test.go, core_test.go, verify_test.go.
package ir

import (
	"sync"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// TestTransform_BindChain_ConcurrentRead verifies that Transform on a
// Bind chain does not mutate the original tree's Body pointers, which
// would be a data race if a concurrent reader (e.g., LSP hover) is
// traversing the same tree. Run with -race to detect violations.
func TestTransform_BindChain_ConcurrentRead(t *testing.T) {
	// Build a Bind chain: Bind{a, Bind{b, Bind{c, leaf}}}
	leaf := &Lit{Value: int64(1), S: span.Span{Start: 0, End: 1}}
	chain := buildBindChain(3, leaf)

	// Snapshot original Body pointers.
	origBodies := collectBindBodies(chain)

	// Transform with identity function — should not mutate original.
	identity := func(c Core) Core { return c }

	var wg sync.WaitGroup
	const readers = 8
	const iters = 1000

	// Concurrent readers: continuously verify Body pointers are stable.
	for range readers {
		wg.Go(func() {
			for range iters {
				bodies := collectBindBodies(chain)
				for i, b := range bodies {
					if b != origBodies[i] {
						t.Errorf("Body[%d] changed during Transform: got %p, want %p", i, b, origBodies[i])
						return
					}
				}
			}
		})
	}

	// Writer: repeatedly Transform the chain.
	wg.Go(func() {
		for range iters {
			Transform(chain, identity)
		}
	})

	wg.Wait()
}

// TestTransform_BindChain_NoMutation verifies that Transform with an
// identity function returns the original tree without allocating new nodes.
func TestTransform_BindChain_NoMutation(t *testing.T) {
	leaf := &Lit{Value: int64(1), S: span.Span{Start: 0, End: 1}}
	chain := buildBindChain(5, leaf)

	result := Transform(chain, func(c Core) Core { return c })
	if result != chain {
		t.Error("identity Transform should return original pointer for unchanged Bind chain")
	}
}

// TestTransform_BindChain_Rewrite verifies that Transform correctly
// rebuilds a Bind chain when f rewrites a node.
func TestTransform_BindChain_Rewrite(t *testing.T) {
	leaf := &Lit{Value: int64(1), S: span.Span{Start: 0, End: 1}}
	chain := buildBindChain(3, leaf)

	// Rewrite: replace the leaf literal with a different one.
	replacement := &Lit{Value: int64(99), S: span.Span{Start: 0, End: 1}}
	result := Transform(chain, func(c Core) Core {
		if lit, ok := c.(*Lit); ok && lit.Value == int64(1) {
			return replacement
		}
		return c
	})

	if result == chain {
		t.Error("Transform should return a new tree when f rewrites a node")
	}

	// Verify original is unchanged.
	origLeaf := findLeaf(chain)
	if origLeaf != leaf {
		t.Error("original tree's leaf should be unchanged after Transform")
	}
}

func buildBindChain(n int, tail Core) Core {
	cur := tail
	for i := n - 1; i >= 0; i-- {
		cur = &Bind{
			Comp: &Lit{Value: int64(i), S: span.Span{Start: 0, End: 1}},
			Var:  "x",
			Body: cur,
			S:    span.Span{Start: 0, End: 1},
		}
	}
	return cur
}

func collectBindBodies(c Core) []Core {
	var bodies []Core
	for {
		b, ok := c.(*Bind)
		if !ok {
			break
		}
		bodies = append(bodies, b.Body)
		c = b.Body
	}
	return bodies
}

func findLeaf(c Core) Core {
	for {
		b, ok := c.(*Bind)
		if !ok {
			return c
		}
		c = b.Body
	}
}
