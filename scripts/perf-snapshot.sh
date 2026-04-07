#!/usr/bin/env bash
# perf-snapshot.sh — Take a comprehensive performance snapshot.
#
# Runs every category of benchmark (warm exec, cold compile+run,
# pure compile, host primitives) with statistically meaningful
# settings, then writes the results into tmp/perf/<label>/ for
# later comparison via perf-compare.sh.
#
# Usage:
#   ./scripts/perf-snapshot.sh [<label>]
#
# If no label is given, uses the current git short hash. The
# resulting snapshot directory always contains:
#   - meta.txt           git rev, date, host CPU
#   - exec.txt           pre-compiled RunWith benchmarks (BenchmarkExec*)
#   - end_to_end.txt     compile + RunWith benchmarks
#   - compile.txt        pure compile benchmarks (no RunWith)
#   - check.txt          type checker / instance / unify micro
#   - parse.txt          parser / lexer micro
#   - runtime_micro.txt  budget / value op micro
#
# These files are benchstat-compatible. Use:
#   ./scripts/perf-compare.sh <baseline-label> <new-label>
# to diff two snapshots.

set -euo pipefail

LABEL="${1:-$(git rev-parse --short HEAD)}"
OUT="tmp/perf/${LABEL}"
COUNT="${PERF_COUNT:-5}"
BENCHTIME="${PERF_BENCHTIME:-2s}"

mkdir -p "$OUT"

run_bench() {
  local outfile="$1"; shift
  local label="$1"; shift
  local pattern="$1"; shift
  local pkg="$1"; shift
  printf "  ▸ %-22s " "$label"
  if go test -run='^$' -bench="$pattern" -benchmem \
       -count="$COUNT" -benchtime="$BENCHTIME" \
       "$@" "$pkg" > "$outfile" 2>/dev/null; then
    printf "\033[32mok\033[0m\n"
  else
    printf "\033[31mFAIL\033[0m (see %s)\n" "$outfile"
  fi
}

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  perf snapshot: $LABEL"
echo "  count=$COUNT  benchtime=$BENCHTIME"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Metadata.
{
  echo "label: $LABEL"
  echo "git: $(git rev-parse HEAD)"
  echo "branch: $(git rev-parse --abbrev-ref HEAD)"
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "go: $(go version)"
  echo "host: $(uname -srm)"
  echo "count: $COUNT"
  echo "benchtime: $BENCHTIME"
} > "$OUT/meta.txt"

# Pre-compiled execution: warm steady-state runtime cost.
run_bench "$OUT/exec.txt" "warm exec" \
  '^BenchmarkExec' ./internal/app/engine/

# End-to-end (compile + run inside loop): cold start cost.
run_bench "$OUT/end_to_end.txt" "cold end-to-end" \
  '^BenchmarkEngineEndToEnd|^BenchmarkEndToEnd' ./internal/app/engine/

# Pure compile (NewRuntime, no RunWith).
run_bench "$OUT/compile.txt" "compile" \
  '^BenchmarkEngineCompile|^BenchmarkEngineNewRuntime' ./internal/app/engine/

# Type checker / instance resolution / unify.
run_bench "$OUT/check.txt" "check (semantic)" \
  '.' ./internal/compiler/check/...

# Parser / lexer.
run_bench "$OUT/parse.txt" "parse" \
  '.' ./internal/compiler/parse/

# Optimizer micro.
run_bench "$OUT/optimize.txt" "optimize" \
  '.' ./internal/compiler/optimize/

# Runtime micro: budget, value ops.
run_bench "$OUT/runtime_micro.txt" "runtime micro" \
  '.' ./internal/infra/budget/ ./internal/lang/types/

echo ""
echo "  snapshot written to $OUT/"
echo ""
