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
	l := parse.NewLexer(src)
	tokens, _ := l.Tokenize()
	es := &diagnostic.Errors{Source: src}
	p := parse.NewParser(context.Background(), tokens, es)
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
	b.WriteString("data Bool := True | False\n")
	b.WriteString("class Eq a { eq :: a -> a -> Bool }\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "data T%d := C%d\n", i, i)
		fmt.Fprintf(&b, "instance Eq T%d { eq := \\x y. True }\n", i)
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
// class C0 a, class C0 a => C1 a, ..., class C(n-1) a => Cn a
func superclassChainSource(depth int) string {
	var b strings.Builder
	b.WriteString("data Bool := True | False\n")
	b.WriteString("class C0 a { m0 :: a -> Bool }\n")
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&b, "class C%d a => C%d a { m%d :: a -> Bool }\n", i-1, i, i)
	}
	b.WriteString("data X := X\n")
	// Provide instances for the full chain.
	for i := 0; i <= depth; i++ {
		fmt.Fprintf(&b, "instance C%d X { m%d := \\x. True }\n", i, i)
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
// Uses GICEL syntax: type F (a: Type) :: Type := { F T0 =: T1; ... }
func typeFamilyLinearSource(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "data T%d := C%d\n", i, i)
	}
	b.WriteString("type F (a: Type) :: Type := {\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  F T%d =: T%d", i, (i+1)%n)
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
	b.WriteString("data Bool := True | False\n")
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
