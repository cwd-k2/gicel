//go:build scale

// Engine scale tests — O(N) scaling verification for compile pipeline.
// Does NOT cover: engine_bench_test.go (throughput benchmarks).
package engine

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/cwd-k2/gicel/internal/compiler/check"
	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// scaleResult holds measurements for a single scale point.
type scaleResult struct {
	N           int
	SourceBytes int
	LexParse    time.Duration
	Check       time.Duration
	PostCheck   time.Duration
	Total       time.Duration
	Allocs      int64
	Bytes       int64
}

// measureScale runs the full compile pipeline on largeSource(n) and returns
// per-stage timing and allocation counts. Module cache is pre-warmed.
func measureScale(t *testing.T, n int, runs int) scaleResult {
	t.Helper()
	source := largeSource(n)

	// Warm module cache.
	warm := NewEngine()
	stdlib.Prelude(warm)

	var res scaleResult
	res.N = n
	res.SourceBytes = len(source)

	for range runs {
		eng := NewEngine()
		stdlib.Prelude(eng)
		pc := eng.pipeline(eng.compileCtx)

		runtime.GC()
		var m1, m2 runtime.MemStats
		runtime.ReadMemStats(&m1)

		// Stage 1: lex + parse
		t0 := time.Now()
		ast, src, err := pc.lexAndParse("<input>", source, pc.store.Has("Core"))
		if err != nil {
			t.Fatal(err)
		}
		t1 := time.Now()

		// Stage 2: type check
		cfg := pc.makeCheckConfig()
		cfg.Context = pc.ctx
		cfg.EntryPoint = "main"
		prog, checkErrs := check.Check(ast, src, cfg)
		if checkErrs.HasErrors() {
			t.Fatal(checkErrs.Format())
		}
		t2 := time.Now()

		// Stage 3: post-check (optimize + annotate + index)
		pc.postCheck(prog)
		t3 := time.Now()

		runtime.ReadMemStats(&m2)

		res.LexParse += t1.Sub(t0)
		res.Check += t2.Sub(t1)
		res.PostCheck += t3.Sub(t2)
		res.Total += t3.Sub(t0)
		res.Allocs += int64(m2.Mallocs - m1.Mallocs)
		res.Bytes += int64(m2.TotalAlloc - m1.TotalAlloc)
	}

	div := time.Duration(runs)
	res.LexParse /= div
	res.Check /= div
	res.PostCheck /= div
	res.Total /= div
	res.Allocs /= int64(runs)
	res.Bytes /= int64(runs)
	return res
}

func TestScalePolymorphicDecls(t *testing.T) {
	sizes := []int{1, 10, 50, 100, 200, 500, 1000}
	const runs = 5

	t.Logf("| N | src bytes | lex+parse | check | post | total | allocs | MB |")
	t.Logf("|---|-----------|-----------|-------|------|-------|--------|----|")

	var results []scaleResult
	for _, n := range sizes {
		r := measureScale(t, n, runs)
		results = append(results, r)
		t.Logf("| %d | %d | %v | %v | %v | %v | %d | %.2f |",
			r.N, r.SourceBytes, r.LexParse, r.Check, r.PostCheck, r.Total, r.Allocs, float64(r.Bytes)/(1024*1024))
	}

	// Scaling ratio: N=1000 vs N=1
	first, last := results[0], results[len(results)-1]
	inputRatio := float64(last.N) / float64(first.N)
	t.Logf("")
	t.Logf("Scaling (N=%d / N=%d, input ratio %.0fx):", last.N, first.N, inputRatio)
	t.Logf("  total:     %.1fx", float64(last.Total)/float64(first.Total))
	t.Logf("  lex+parse: %.1fx", float64(last.LexParse)/float64(first.LexParse))
	t.Logf("  check:     %.1fx", float64(last.Check)/float64(first.Check))
	t.Logf("  post:      %.1fx", float64(last.PostCheck)/float64(first.PostCheck))
	t.Logf("  allocs:    %.1fx", float64(last.Allocs)/float64(first.Allocs))
	t.Logf("  bytes:     %.1fx", float64(last.Bytes)/float64(first.Bytes))

	// Per-decl marginal cost (last two data points)
	prev := results[len(results)-2]
	dn := float64(last.N - prev.N)
	t.Logf("")
	t.Logf("Marginal cost per decl (N=%d→%d):", prev.N, last.N)
	t.Logf("  time:   %.1fµs/decl", float64(last.Total-prev.Total)/dn*1e-3)
	t.Logf("  allocs: %.0f/decl", float64(last.Allocs-prev.Allocs)/dn)
	t.Logf("  bytes:  %.0f/decl", float64(last.Bytes-prev.Bytes)/dn)
}

// TestScaleParserOnly isolates lex+parse scaling without type checking.
func TestScaleParserOnly(t *testing.T) {
	sizes := []int{1, 10, 50, 100, 200, 500, 1000}
	const runs = 10

	t.Logf("| N | src bytes | lex+parse | allocs |")
	t.Logf("|---|-----------|-----------|--------|")

	for _, n := range sizes {
		source := largeSource(n)
		var totalTime time.Duration
		var totalAllocs int64

		for range runs {
			runtime.GC()
			var m1, m2 runtime.MemStats
			runtime.ReadMemStats(&m1)
			start := time.Now()

			src := span.NewSource("<input>", source)
			parseErrs := &diagnostic.Errors{Source: src}
			p := parse.NewParser(context.Background(), src, parseErrs)
			_ = p.ParseProgram()
			if p.LexErrors().HasErrors() {
				t.Fatal(p.LexErrors().Format())
			}

			elapsed := time.Since(start)
			runtime.ReadMemStats(&m2)
			totalTime += elapsed
			totalAllocs += int64(m2.Mallocs - m1.Mallocs)
		}

		t.Logf("| %d | %d | %v | %d |",
			n, len(source), totalTime/time.Duration(runs), totalAllocs/int64(runs))
	}
}
