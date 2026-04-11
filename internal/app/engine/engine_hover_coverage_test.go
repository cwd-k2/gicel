// Hover coverage tests — exhaustive hover check on all example tokens.
// Does NOT cover: hover content correctness (server_test.go), data structure (hoverindex_test.go).

package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/app/header"
	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/span"
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"
)

// tokenClass classifies tokens into groups for coverage reporting.
type tokenClass int

const (
	classIdentifier  tokenClass = iota // TokLower, TokUpper
	classOperator                      // TokOp, TokDot (infix)
	classLiteral                       // TokIntLit, TokDoubleLit, TokStrLit, TokRuneLit
	classKeyword                       // keywords
	classPunctuation                   // delimiters, arrows, etc.
)

func classifyToken(k syn.TokenKind) tokenClass {
	switch k {
	case syn.TokLower, syn.TokUpper:
		return classIdentifier
	case syn.TokOp, syn.TokDot, syn.TokDotHash:
		return classOperator
	case syn.TokIntLit, syn.TokDoubleLit, syn.TokStrLit, syn.TokRuneLit, syn.TokLabelLit:
		return classLiteral
	case syn.TokAs, syn.TokAssumption, syn.TokCase, syn.TokDo, syn.TokForm,
		syn.TokLazy, syn.TokType, syn.TokInfixl, syn.TokInfixr, syn.TokInfixn,
		syn.TokImpl, syn.TokImport, syn.TokIf, syn.TokThen, syn.TokElse:
		return classKeyword
	default:
		return classPunctuation
	}
}

func (c tokenClass) String() string {
	switch c {
	case classIdentifier:
		return "identifier"
	case classOperator:
		return "operator"
	case classLiteral:
		return "literal"
	case classKeyword:
		return "keyword"
	case classPunctuation:
		return "punctuation"
	default:
		return "unknown"
	}
}

// tokenize returns all tokens from a source string.
func tokenize(source string) []syn.Token {
	src := span.NewSource("<test>", source)
	scanner := parse.NewScanner(src)
	var tokens []syn.Token
	for {
		tok := scanner.Next()
		if tok.Kind == syn.TokEOF {
			break
		}
		tokens = append(tokens, tok)
	}
	return tokens
}

type coverageStats struct {
	total int
	hit   int
}

// TestHoverCoverage_AllExamples runs hover at every token position in all examples
// and reports per-class coverage statistics.
func TestHoverCoverage_AllExamples(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples", "gicel")
	var files []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".gicel") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk examples: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no example files found")
	}

	// Aggregate stats across all files.
	aggregate := make(map[tokenClass]*coverageStats)
	for _, c := range []tokenClass{classIdentifier, classOperator, classLiteral, classKeyword, classPunctuation} {
		aggregate[c] = &coverageStats{}
	}

	for _, f := range files {
		rel, _ := filepath.Rel(root, f)
		t.Run(rel, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read file: %v", err)
			}
			source := string(data)

			eng := NewEngine()
			allPacks(eng)
			eng.EnableHoverIndex()

			res, err := header.Resolve(source, f)
			if err == nil {
				if res.Recursion {
					eng.EnableRecursion()
				}
				for _, mod := range res.Modules {
					if err := eng.RegisterModule(mod.Name, mod.Source); err != nil {
						t.Logf("module %s: %v", mod.Name, err)
					}
				}
			}

			ar := eng.Analyze(context.Background(), source)
			if ar.HoverIndex == nil {
				t.Skip("HoverIndex is nil (compile errors)")
				return
			}

			tokens := tokenize(source)
			perClass := make(map[tokenClass]*coverageStats)
			for _, c := range []tokenClass{classIdentifier, classOperator, classLiteral, classKeyword, classPunctuation} {
				perClass[c] = &coverageStats{}
			}

			for _, tok := range tokens {
				mid := (tok.S.Start + tok.S.End) / 2
				hover := ar.HoverIndex.HoverAt(span.Pos(mid))
				cls := classifyToken(tok.Kind)
				perClass[cls].total++
				aggregate[cls].total++
				if hover != "" {
					perClass[cls].hit++
					aggregate[cls].hit++
				}
			}

			// Log per-file stats.
			for _, cls := range []tokenClass{classIdentifier, classOperator, classLiteral} {
				s := perClass[cls]
				if s.total > 0 {
					pct := float64(s.hit) / float64(s.total) * 100
					t.Logf("  %s: %d/%d (%.0f%%)", cls, s.hit, s.total, pct)
				}
			}
		})
	}

	// Report aggregate stats.
	t.Log("\n=== Aggregate Hover Coverage ===")
	for _, cls := range []tokenClass{classIdentifier, classOperator, classLiteral, classKeyword, classPunctuation} {
		s := aggregate[cls]
		if s.total > 0 {
			pct := float64(s.hit) / float64(s.total) * 100
			t.Logf("  %-12s %4d / %4d  (%.1f%%)", cls, s.hit, s.total, pct)
		}
	}

	// Minimum coverage thresholds.
	idStats := aggregate[classIdentifier]
	if idStats.total > 0 {
		idPct := float64(idStats.hit) / float64(idStats.total) * 100
		if idPct < 50 {
			t.Errorf("identifier hover coverage too low: %.1f%% < 50%%", idPct)
		}
	}
	opStats := aggregate[classOperator]
	if opStats.total > 0 {
		opPct := float64(opStats.hit) / float64(opStats.total) * 100
		if opPct < 30 {
			t.Errorf("operator hover coverage too low: %.1f%% < 30%%", opPct)
		}
	}
	litStats := aggregate[classLiteral]
	if litStats.total > 0 {
		litPct := float64(litStats.hit) / float64(litStats.total) * 100
		if litPct < 50 {
			t.Errorf("literal hover coverage too low: %.1f%% < 50%%", litPct)
		}
	}
}

// TestHoverCoverage_OperatorDetail verifies that operator hover includes
// type information (not just the expression result type).
func TestHoverCoverage_OperatorDetail(t *testing.T) {
	source := `import Prelude
main := 1 + 2`

	eng := NewEngine()
	allPacks(eng)
	eng.EnableHoverIndex()

	ar := eng.Analyze(context.Background(), source)
	if ar.HoverIndex == nil {
		t.Fatal("HoverIndex is nil")
	}

	tokens := tokenize(source)
	var opToken syn.Token
	for _, tok := range tokens {
		if tok.Kind == syn.TokOp && tok.Text == "+" {
			opToken = tok
			break
		}
	}
	if opToken.Kind == syn.TokEOF {
		t.Fatal("no + operator token found")
	}

	mid := (opToken.S.Start + opToken.S.End) / 2
	hover := ar.HoverIndex.HoverAt(span.Pos(mid))
	if hover == "" {
		t.Fatal("no hover for + operator")
	}
	t.Logf("+ hover: %s", hover)
	if !strings.Contains(hover, "(") || !strings.Contains(hover, "::") {
		t.Errorf("operator hover should include (op) :: type format, got: %s", hover)
	}
	if !strings.Contains(hover, "infixl") {
		t.Errorf("operator hover should include fixity info, got: %s", hover)
	}
}

// TestHoverCoverage_LambdaParam verifies that lambda parameter hover works.
func TestHoverCoverage_LambdaParam(t *testing.T) {
	source := `import Prelude
main := (\x. x + 1) 42`

	eng := NewEngine()
	allPacks(eng)
	eng.EnableHoverIndex()

	ar := eng.Analyze(context.Background(), source)
	if ar.HoverIndex == nil {
		t.Fatal("HoverIndex is nil")
	}

	tokens := tokenize(source)
	// Find the 'x' after backslash (the lambda parameter).
	var paramToken syn.Token
	foundBackslash := false
	for _, tok := range tokens {
		if tok.Kind == syn.TokBackslash {
			foundBackslash = true
			continue
		}
		if foundBackslash && tok.Kind == syn.TokLower && tok.Text == "x" {
			paramToken = tok
			break
		}
		foundBackslash = false
	}
	if paramToken.Kind == syn.TokEOF {
		t.Fatal("no lambda param token found")
	}

	mid := (paramToken.S.Start + paramToken.S.End) / 2
	hover := ar.HoverIndex.HoverAt(span.Pos(mid))
	if hover == "" {
		t.Fatal("no hover for lambda parameter")
	}
	t.Logf("lambda param hover: %s", hover)
	if !strings.Contains(hover, "Int") {
		t.Errorf("lambda param should show inferred type Int, got: %s", hover)
	}
}

// TestHoverCoverage_MissingSummary is a diagnostic test that reports which
// specific tokens are missing hover. Run with -v to see the output.
func TestHoverCoverage_MissingSummary(t *testing.T) {
	source := `import Prelude

add :: Int -> Int -> Int
add := \x y. x + y

double := \n. n + n

main := add (double 3) 4`

	eng := NewEngine()
	allPacks(eng)
	eng.EnableHoverIndex()

	ar := eng.Analyze(context.Background(), source)
	if ar.HoverIndex == nil {
		t.Fatal("HoverIndex is nil")
	}

	tokens := tokenize(source)
	type missingInfo struct {
		text  string
		kind  syn.TokenKind
		class tokenClass
	}
	var missing []missingInfo

	for _, tok := range tokens {
		cls := classifyToken(tok.Kind)
		if cls == classPunctuation || cls == classKeyword {
			continue
		}
		mid := (tok.S.Start + tok.S.End) / 2
		hover := ar.HoverIndex.HoverAt(span.Pos(mid))
		if hover == "" {
			missing = append(missing, missingInfo{text: tok.Text, kind: tok.Kind, class: cls})
		}
	}

	if len(missing) > 0 {
		// Group by class.
		groups := make(map[tokenClass][]string)
		for _, m := range missing {
			groups[m.class] = append(groups[m.class], m.text)
		}
		sorted := make([]tokenClass, 0, len(groups))
		for c := range groups {
			sorted = append(sorted, c)
		}
		slices.Sort(sorted)
		for _, c := range sorted {
			texts := groups[c]
			t.Logf("missing %s hover: %s", c, strings.Join(unique(texts), ", "))
		}
	}
	t.Logf("total semantic tokens: %d, missing hover: %d", len(tokens), len(missing))
}

func unique(ss []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// -- script-like output for CI/local use --

// TestHoverCoverage_Report produces a compact coverage report suitable
// for piping into scripts or CI logs. Run with: go test -run TestHoverCoverage_Report -v
func TestHoverCoverage_Report(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples", "gicel")
	var files []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".gicel") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk examples: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no example files found")
	}

	type fileReport struct {
		name string
		id   coverageStats
		op   coverageStats
		lit  coverageStats
	}
	var reports []fileReport

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		source := string(data)
		rel, _ := filepath.Rel(root, f)

		eng := NewEngine()
		allPacks(eng)
		eng.EnableHoverIndex()

		res, err := header.Resolve(source, f)
		if err == nil {
			if res.Recursion {
				eng.EnableRecursion()
			}
			for _, mod := range res.Modules {
				_ = eng.RegisterModule(mod.Name, mod.Source)
			}
		}

		ar := eng.Analyze(context.Background(), source)
		if ar.HoverIndex == nil {
			continue
		}

		tokens := tokenize(source)
		var rep fileReport
		rep.name = rel

		for _, tok := range tokens {
			mid := (tok.S.Start + tok.S.End) / 2
			hover := ar.HoverIndex.HoverAt(span.Pos(mid))
			cls := classifyToken(tok.Kind)
			hit := 0
			if hover != "" {
				hit = 1
			}
			switch cls {
			case classIdentifier:
				rep.id.total++
				rep.id.hit += hit
			case classOperator:
				rep.op.total++
				rep.op.hit += hit
			case classLiteral:
				rep.lit.total++
				rep.lit.hit += hit
			}
		}
		reports = append(reports, rep)
	}

	// Print report.
	t.Logf("%-45s  %8s  %8s  %8s", "FILE", "IDENT", "OPER", "LIT")
	t.Logf("%s", strings.Repeat("-", 80))

	var totalID, totalOp, totalLit coverageStats
	for _, r := range reports {
		idStr := fmtPct(r.id)
		opStr := fmtPct(r.op)
		litStr := fmtPct(r.lit)
		t.Logf("%-45s  %8s  %8s  %8s", r.name, idStr, opStr, litStr)
		totalID.hit += r.id.hit
		totalID.total += r.id.total
		totalOp.hit += r.op.hit
		totalOp.total += r.op.total
		totalLit.hit += r.lit.hit
		totalLit.total += r.lit.total
	}
	t.Logf("%s", strings.Repeat("-", 80))
	t.Logf("%-45s  %8s  %8s  %8s", "TOTAL", fmtPct(totalID), fmtPct(totalOp), fmtPct(totalLit))
}

// TestHoverCoverage_ClassMethodAndDoc diagnoses class method and doc comment hover.
func TestHoverCoverage_ClassMethodAndDoc(t *testing.T) {
	source := `import Prelude

-- Add two values.
myAdd :: Int -> Int -> Int
myAdd := \x y. x + y

main := do {
  a <- pure (eq 1 2);
  b <- pure (myAdd 3 4);
  pure (a, b)
}`

	eng := NewEngine()
	allPacks(eng)
	eng.EnableHoverIndex()

	ar := eng.Analyze(context.Background(), source)
	if ar.HoverIndex == nil {
		t.Fatal("HoverIndex is nil")
	}

	tokens := tokenize(source)
	for _, tok := range tokens {
		if tok.Kind != syn.TokLower {
			continue
		}
		mid := (tok.S.Start + tok.S.End) / 2
		hover := ar.HoverIndex.HoverAt(span.Pos(mid))
		if tok.Text == "eq" || tok.Text == "myAdd" || tok.Text == "pure" {
			t.Logf("%-8s → %q", tok.Text, hover)
		}
	}
}

func fmtPct(s coverageStats) string {
	if s.total == 0 {
		return "-"
	}
	pct := float64(s.hit) / float64(s.total) * 100
	return fmt.Sprintf("%d/%d %2.0f%%", s.hit, s.total, pct)
}
