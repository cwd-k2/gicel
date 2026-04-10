// Optimizer benchmarks — noop pass cost, beta-heavy, rewrite rules.
// Does NOT cover: algebraic simplification correctness (optimize_test.go).

package optimize

import (
	"context"
	"fmt"
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// buildDeepTree creates a Core tree of depth n.
// Structure: nested App(Lam ... (Var "x"), Var "y") — no reducible redexes.
func buildDeepTree(n int) ir.Core {
	var c ir.Core = &ir.Var{Name: "leaf"}
	for range n {
		c = &ir.App{
			Fun: &ir.Var{Name: fmt.Sprintf("f%d", n)},
			Arg: c,
		}
	}
	return c
}

// buildBetaChain creates n nested beta-redexes: ((\x.(\y...body)) arg)...
func buildBetaChain(n int) ir.Core {
	var body ir.Core = &ir.Var{Name: "x0"}
	for i := n - 1; i >= 0; i-- {
		body = &ir.Lam{
			Param: fmt.Sprintf("x%d", i),
			Body:  body,
		}
	}
	// Apply arguments.
	for i := range n {
		body = &ir.App{
			Fun: body,
			Arg: &ir.Var{Name: fmt.Sprintf("arg%d", i)},
		}
	}
	return body
}

// BenchmarkOptimizeNoopSmall measures baseline cost on a small tree with no
// optimization opportunities. This reveals the fixed tax of the pass architecture.
// Safe to share across iterations: no redexes fire, so TransformMut leaves the tree intact.
func BenchmarkOptimizeNoopSmall(b *testing.B) {
	tree := buildDeepTree(50)
	b.ResetTimer()
	for b.Loop() {
		optimize(context.Background(), tree, nil)
	}
}

// BenchmarkOptimizeNoopLarge measures baseline cost on a larger tree.
// Safe to share across iterations: no redexes fire, so TransformMut leaves the tree intact.
func BenchmarkOptimizeNoopLarge(b *testing.B) {
	tree := buildDeepTree(500)
	b.ResetTimer()
	for b.Loop() {
		optimize(context.Background(), tree, nil)
	}
}

// BenchmarkOptimizeBetaHeavy measures cost when many beta reductions fire.
// Tree is rebuilt each iteration because optimize uses TransformMut (in-place).
func BenchmarkOptimizeBetaHeavy(b *testing.B) {
	for b.Loop() {
		tree := buildBetaChain(20)
		optimize(context.Background(), tree, nil)
	}
}

// BenchmarkOptimizeRewriteRules measures cost with registered rewrite rules.
// Safe to share across iterations: noop rules return c unchanged, no mutation occurs.
func BenchmarkOptimizeRewriteRules(b *testing.B) {
	tree := buildDeepTree(200)
	// Dummy rule that never fires — measures rule dispatch overhead.
	noop := func(c ir.Core) ir.Core { return c }
	rules := []func(ir.Core) ir.Core{noop, noop, noop}
	b.ResetTimer()
	for b.Loop() {
		optimize(context.Background(), tree, rules)
	}
}
