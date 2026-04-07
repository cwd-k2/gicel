#!/usr/bin/env bash
# perf-profile.sh — Take CPU + alloc profiles for one benchmark.
#
# Designed for deep investigation of a specific hot path. The default
# benchtime of 10s gives enough samples to overcome GC noise.
#
# Usage:
#   ./scripts/perf-profile.sh <bench-pattern> [pkg]
#
# Examples:
#   ./scripts/perf-profile.sh BenchmarkExecMapInsert50
#   ./scripts/perf-profile.sh BenchmarkEngineCompileLarge ./internal/app/engine/
#
# Outputs (in tmp/perf/profile/<bench>/):
#   bench.txt   the bench output
#   cpu.prof    CPU profile (use: go tool pprof tmp/perf/profile/<bench>/cpu.prof)
#   mem.prof    memory profile (alloc_space)
#   block.prof  blocking profile
#
# Quick views:
#   go tool pprof -top -cum -nodecount=20 tmp/perf/profile/<bench>/cpu.prof
#   go tool pprof -top -nodecount=20 tmp/perf/profile/<bench>/mem.prof

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: $0 <bench-pattern> [pkg]" >&2
  exit 1
fi

PATTERN="$1"
PKG="${2:-./internal/app/engine/}"
BENCHTIME="${PERF_PROFILE_BENCHTIME:-10s}"

# Slug for directory: strip ^ $ and replace problematic chars.
SLUG=$(echo "$PATTERN" | sed -E 's/[^A-Za-z0-9._-]/_/g')
OUT="tmp/perf/profile/${SLUG}"
mkdir -p "$OUT"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  profile: $PATTERN"
echo "  pkg:     $PKG"
echo "  out:     $OUT"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

go test -run='^$' \
  -bench="^${PATTERN}$" \
  -benchmem \
  -count=1 \
  -benchtime="$BENCHTIME" \
  -cpuprofile="$OUT/cpu.prof" \
  -memprofile="$OUT/mem.prof" \
  -blockprofile="$OUT/block.prof" \
  -memprofilerate=1 \
  "$PKG" | tee "$OUT/bench.txt"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Top 10 CPU (cum):"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
go tool pprof -top -cum -nodecount=10 "$OUT/cpu.prof" 2>/dev/null | grep -E '^\s+[0-9]'  || true

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Top 10 alloc:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
go tool pprof -top -nodecount=10 "$OUT/mem.prof" 2>/dev/null | grep -E '^\s+[0-9]'  || true

echo ""
echo "  Inspect interactively:"
echo "    go tool pprof -http :8080 $OUT/cpu.prof"
echo "    go tool pprof -http :8081 $OUT/mem.prof"
echo ""
