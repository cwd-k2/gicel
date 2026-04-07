#!/usr/bin/env bash
# perf-compare.sh — Diff two perf snapshots via benchstat.
#
# Usage:
#   ./scripts/perf-compare.sh <baseline-label> <new-label> [category]
#
# `category` selects which file to compare. If omitted, compares all
# categories that exist in both snapshots and prints geomean lines.
#
# Examples:
#   ./scripts/perf-compare.sh main HEAD                # all categories
#   ./scripts/perf-compare.sh main HEAD exec           # exec only
#   ./scripts/perf-compare.sh 6ef77e0 9c68906 end_to_end

set -euo pipefail

if [ $# -lt 2 ]; then
  echo "Usage: $0 <baseline-label> <new-label> [category]" >&2
  echo "Available categories: exec, end_to_end, compile, check, parse, optimize, runtime_micro" >&2
  exit 1
fi

BASE_LABEL="$1"
NEW_LABEL="$2"
CATEGORY="${3:-}"

BASE_DIR="tmp/perf/${BASE_LABEL}"
NEW_DIR="tmp/perf/${NEW_LABEL}"

if [ ! -d "$BASE_DIR" ]; then
  echo "Baseline snapshot missing: $BASE_DIR" >&2
  exit 1
fi
if [ ! -d "$NEW_DIR" ]; then
  echo "New snapshot missing: $NEW_DIR" >&2
  exit 1
fi

if ! command -v benchstat >/dev/null 2>&1; then
  echo "benchstat not found. Install with: go install golang.org/x/perf/cmd/benchstat@latest" >&2
  exit 1
fi

categories=(exec end_to_end compile check parse optimize runtime_micro)
if [ -n "$CATEGORY" ]; then
  categories=("$CATEGORY")
fi

for cat in "${categories[@]}"; do
  base_file="$BASE_DIR/$cat.txt"
  new_file="$NEW_DIR/$cat.txt"
  if [ ! -f "$base_file" ] || [ ! -f "$new_file" ]; then
    continue
  fi
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  $cat:  $BASE_LABEL → $NEW_LABEL"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  benchstat "$base_file" "$new_file"
  echo ""
done
