#!/usr/bin/env bash
# full-check.sh — Run the complete test and validation suite.
# This is the gate for integration and release.
# Usage: ./scripts/full-check.sh
# Exit code: 0 if everything passes, 1 on first failure.

set -euo pipefail

FAIL=0

section() {
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  $1"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

run_step() {
  local label="$1"; shift
  printf "  ▸ %s... " "$label"
  if "$@" >/dev/null 2>&1; then
    printf "\033[32mok\033[0m\n"
  else
    printf "\033[31mFAIL\033[0m\n"
    FAIL=$((FAIL + 1))
  fi
}

# ── Build ───────────────────────────────────────────────────

section "Build"
run_step "go build ./..." go build ./...
run_step "go build -o bin/gicel" go build -o bin/gicel ./cmd/gicel/

# ── Tests ───────────────────────────────────────────────────

section "Tests"
run_step "unit tests" go test ./...
run_step "probe tests" go test -tags probe ./...
run_step "stress tests" go test ./tests/stress/
run_step "bench tests (compile)" go test -bench=. -benchtime=1x ./...

# ── Examples ────────────────────────────────────────────────

section "Examples"
run_step "Go + GICEL examples" ./scripts/run-examples.sh bin/gicel

# ── CLI smoke test ──────────────────────────────────────────

section "Smoke test"
run_step "CLI smoke test" ./scripts/smoke-test.sh bin/gicel

# ── Summary ─────────────────────────────────────────────────

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [ "$FAIL" -gt 0 ]; then
  printf "  \033[31m%d step(s) failed.\033[0m\n" "$FAIL"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  exit 1
else
  printf "  \033[32mAll checks passed.\033[0m\n"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
fi
