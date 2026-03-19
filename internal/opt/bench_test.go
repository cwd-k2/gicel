// Optimizer benchmarks — noop pass cost, beta-heavy, rewrite rules.
// Does NOT cover: algebraic simplification correctness (optimize_test.go).

package opt

import (
	"fmt"
	"testing"

	"github.com/cwd-k2/gicel/internal/core"
)

// buildDeepTree creates a Core tree of depth n.
// Structure: nested App(Lam ... (Var "x"), Var "y") — no reducible redexes.
func buildDeepTree(n int) core.Core {
	var c core.Core = &core.Var{Name: "leaf"}
	for range n {
		c = &core.App{
			Fun: &core.Var{Name: fmt.Sprintf("f%d", n)},
			Arg: c,
		}
	}
	return c
}

// buildBetaChain creates n nested beta-redexes: ((\x.(\y...body)) arg)...
func buildBetaChain(n int) core.Core {
	var body core.Core = &core.Var{Name: "x0"}
	for i := n - 1; i >= 0; i-- {
		body = &core.Lam{
			Param: fmt.Sprintf("x%d", i),
			Body:  body,
		}
	}
	// Apply arguments.
	for i := 0; i < n; i++ {
		body = &core.App{
			Fun: body,
			Arg: &core.Var{Name: fmt.Sprintf("arg%d", i)},
		}
	}
	return body
}

// BenchmarkOptimizeNoopSmall measures baseline cost on a small tree with no
// optimization opportunities. This reveals the fixed tax of the pass architecture.
func BenchmarkOptimizeNoopSmall(b *testing.B) {
	tree := buildDeepTree(50)
	b.ResetTimer()
	for b.Loop() {
		optimize(tree, nil)
	}
}

// BenchmarkOptimizeNoopLarge measures baseline cost on a larger tree.
func BenchmarkOptimizeNoopLarge(b *testing.B) {
	tree := buildDeepTree(500)
	b.ResetTimer()
	for b.Loop() {
		optimize(tree, nil)
	}
}

// BenchmarkOptimizeBetaHeavy measures cost when many beta reductions fire.
func BenchmarkOptimizeBetaHeavy(b *testing.B) {
	tree := buildBetaChain(20)
	b.ResetTimer()
	for b.Loop() {
		optimize(tree, nil)
	}
}

// BenchmarkOptimizeRewriteRules measures cost with registered rewrite rules.
func BenchmarkOptimizeRewriteRules(b *testing.B) {
	tree := buildDeepTree(200)
	// Dummy rule that never fires — measures rule dispatch overhead.
	noop := func(c core.Core) core.Core { return c }
	rules := []func(core.Core) core.Core{noop, noop, noop}
	b.ResetTimer()
	for b.Loop() {
		optimize(tree, rules)
	}
}
