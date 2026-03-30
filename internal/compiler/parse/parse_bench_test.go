// Parse throughput benchmarks — program-scale parsing at increasing sizes.
// Does NOT cover: recovery (parser_recovery_probe_test.go), pathological (parse_pathological_test.go).

package parse

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// generateProgram builds a synthetic GICEL program with n data + value declarations.
func generateProgram(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "form T%d = C%d Int\n", i, i)
		fmt.Fprintf(&b, "f%d := \\x. x\n", i)
	}
	fmt.Fprintf(&b, "main := f0 (C0 42)\n")
	return b.String()
}

func benchParse(b *testing.B, source string) {
	b.Helper()
	src := span.NewSource("bench", source)
	b.ResetTimer()
	for range b.N {
		es := &diagnostic.Errors{Source: src}
		p := NewParser(context.Background(), src, es)
		_ = p.ParseProgram()
	}
}

func BenchmarkParseSmall(b *testing.B) {
	benchParse(b, generateProgram(20))
}

func BenchmarkParseMedium(b *testing.B) {
	benchParse(b, generateProgram(100))
}

func BenchmarkParseLarge(b *testing.B) {
	benchParse(b, generateProgram(500))
}

func BenchmarkParseExprDeep(b *testing.B) {
	// Build deeply nested expression: ((((... x ...))))
	var sb strings.Builder
	sb.WriteString("main := ")
	depth := 100
	for range depth {
		sb.WriteString("(\\x. ")
	}
	sb.WriteString("x")
	for range depth {
		sb.WriteString(")")
	}
	sb.WriteString("\n")
	benchParse(b, sb.String())
}

func BenchmarkLexTokenize(b *testing.B) {
	source := generateProgram(200)
	src := span.NewSource("bench", source)
	b.ResetTimer()
	for range b.N {
		s := NewScanner(src)
		for s.Next().Kind != 0 {
		}
	}
}
