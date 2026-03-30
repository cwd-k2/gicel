// Scaling benchmarks — instance resolution, type family reduction, deep do chains.
// Does NOT cover: basic check benchmarks (check_bench_test.go), unify benchmarks (unify/).

package check

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// benchCheck parses and type-checks the given source for benchmarking.
func benchCheck(b *testing.B, source string) {
	b.Helper()
	src := span.NewSource("bench", source)
	es := &diagnostic.Errors{Source: src}
	p := parse.NewParser(context.Background(), src, es)
	ast := p.ParseProgram()
	_, checkErrs := Check(ast, src, nil)
	if checkErrs.HasErrors() {
		b.Fatalf("check failed: %v", checkErrs)
	}
}

// ---------------------------------------------------------------------------
// Instance resolution scaling (Finding 7)
// ---------------------------------------------------------------------------

// instanceScaleSource generates a program with n data types, each with an
// Eq instance, and a main that resolves Eq on the last type.
func instanceScaleSource(n int) string {
	var b strings.Builder
	b.WriteString("form Bool := { True: Bool; False: Bool; }\n")
	b.WriteString("form Eq := \\a. { eq: a -> a -> Bool }\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "form T%d := { C%d: T%d; }\n", i, i, i)
		fmt.Fprintf(&b, "impl Eq T%d := { eq := \\x y. True }\n", i)
	}
	fmt.Fprintf(&b, "main := eq C%d C%d\n", n-1, n-1)
	return b.String()
}

func BenchmarkResolveInstances10(b *testing.B) {
	source := instanceScaleSource(10)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

func BenchmarkResolveInstances50(b *testing.B) {
	source := instanceScaleSource(50)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

func BenchmarkResolveInstances100(b *testing.B) {
	source := instanceScaleSource(100)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

// ---------------------------------------------------------------------------
// Superclass depth scaling (Finding 7)
// ---------------------------------------------------------------------------

// superclassChainSource generates a chain of n superclasses:
// form C0 := \a. { m0: a -> Bool }, form C1 := \a. C0 a => { m1: a -> Bool }, ...
func superclassChainSource(depth int) string {
	var b strings.Builder
	b.WriteString("form Bool := { True: Bool; False: Bool; }\n")
	b.WriteString("form C0 := \\a. { m0: a -> Bool }\n")
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&b, "form C%d := \\a. C%d a => { m%d: a -> Bool }\n", i, i-1, i)
	}
	b.WriteString("form X := { X: X; }\n")
	// Provide instances for the full chain.
	for i := 0; i <= depth; i++ {
		fmt.Fprintf(&b, "impl C%d X := { m%d := \\x. True }\n", i, i)
	}
	fmt.Fprintf(&b, "main := m%d X\n", depth)
	return b.String()
}

func BenchmarkSuperclassDepth5(b *testing.B) {
	source := superclassChainSource(5)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

func BenchmarkSuperclassDepth10(b *testing.B) {
	source := superclassChainSource(10)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

// ---------------------------------------------------------------------------
// Type family reduction scaling (Finding 9)
// ---------------------------------------------------------------------------

// typeFamilyLinearSource generates a closed type family with n equations.
func typeFamilyLinearSource(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "form T%d := { C%d: T%d; }\n", i, i, i)
	}
	b.WriteString("type F :: Type := \\(a: Type). case a {\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  T%d => T%d", i, (i+1)%n)
		if i < n-1 {
			b.WriteString(";")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	// Force reduction of the first equation.
	fmt.Fprintf(&b, "f :: F T0 -> T%d\nf := \\x. x\nmain := f C%d\n", 1%n, 1%n)
	return b.String()
}

func BenchmarkTypeFamilyReduceLinear10(b *testing.B) {
	source := typeFamilyLinearSource(10)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

func BenchmarkTypeFamilyReduceLinear50(b *testing.B) {
	source := typeFamilyLinearSource(50)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

// ---------------------------------------------------------------------------
// Large program check (checker throughput proxy)
// ---------------------------------------------------------------------------

// largeDeclSource generates n independent function declarations.
func largeDeclSource(n int) string {
	var b strings.Builder
	b.WriteString("form Bool := { True: Bool; False: Bool; }\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "f%d :: Bool -> Bool\nf%d := \\x. x\n", i, i)
	}
	b.WriteString("main := f0 True\n")
	return b.String()
}

func BenchmarkCheckLargeDecl50(b *testing.B) {
	source := largeDeclSource(50)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

func BenchmarkCheckLargeDecl200(b *testing.B) {
	source := largeDeclSource(200)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

func BenchmarkCheckLargeDecl500(b *testing.B) {
	source := largeDeclSource(500)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

// ---------------------------------------------------------------------------
// Do-block bind chain type inference scaling
// ---------------------------------------------------------------------------

// doBlockSource generates a program with n sequential get/put binds in a do-block.
// Tests state type propagation through infer-mode bind chains.
func doBlockSource(n int) string {
	var b strings.Builder
	b.WriteString("form Bool := { True: Bool; False: Bool; }\n")
	b.WriteString("form Unit := { MkUnit: Unit }\n")
	b.WriteString("form Computation := \\(s: Type) (t: Type) (a: Type). { MkComp: (s -> (a, t)) -> Computation s t a }\n")
	b.WriteString("form IxMonad := \\(m: Type -> Type -> Type -> Type). {\n")
	b.WriteString("  ixpure: \\a s. a -> m s s a;\n")
	b.WriteString("  ixbind: \\a b s t u. m s t a -> (a -> m t u b) -> m s u b\n}\n")
	b.WriteString("impl IxMonad Computation := {\n")
	b.WriteString("  ixpure := \\a. MkComp (\\s. (a, s));\n")
	b.WriteString("  ixbind := \\ma f. MkComp (\\s. case ma { MkComp run => case run s { (a, s2) => case f a { MkComp run2 => run2 s2 } } })\n}\n")
	b.WriteString("myGet :: Computation Bool Bool Bool\nmyGet := MkComp (\\s. (s, s))\n")
	b.WriteString("myPut :: Bool -> Computation Bool Bool Unit\nmyPut := \\v. MkComp (\\s. (MkUnit, v))\n")
	b.WriteString("compute := do {\n  myPut True;\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  x%d <- myGet; myPut x%d;\n", i, i)
	}
	b.WriteString("  myGet\n}\nmain := compute\n")
	return b.String()
}

func BenchmarkCheckDoBlock5(b *testing.B) {
	source := doBlockSource(5)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

func BenchmarkCheckDoBlock15(b *testing.B) {
	source := doBlockSource(15)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

func BenchmarkCheckDoBlock30(b *testing.B) {
	source := doBlockSource(30)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

// ---------------------------------------------------------------------------
// Instance resolution with overlap (backtracking cost)
// ---------------------------------------------------------------------------

// overlappingInstanceSource generates a program where n types share the same
// class, then resolves via a constrained function that triggers overlap checking.
func overlappingInstanceSource(n int) string {
	var b strings.Builder
	b.WriteString("form Bool := { True: Bool; False: Bool; }\n")
	b.WriteString("form Show := \\a. { show: a -> Bool }\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "form T%d := { C%d: T%d; }\n", i, i, i)
		fmt.Fprintf(&b, "impl Show T%d := { show := \\x. True }\n", i)
	}
	// Resolve Show for the middle type — triggers linear scan over instances.
	mid := n / 2
	fmt.Fprintf(&b, "main := show C%d\n", mid)
	return b.String()
}

func BenchmarkResolveOverlap50(b *testing.B) {
	source := overlappingInstanceSource(50)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}

func BenchmarkResolveOverlap200(b *testing.B) {
	source := overlappingInstanceSource(200)
	b.ResetTimer()
	for b.Loop() {
		benchCheck(b, source)
	}
}
