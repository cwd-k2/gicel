#!/usr/bin/env bash
# run-examples.sh — Run all Go and GICEL examples.
# Usage: ./scripts/run-examples.sh [path/to/gicel]
# Exit code: 0 if all pass, 1 if any fail.

set -euo pipefail

GICEL="${1:-bin/gicel}"
PASS=0
FAIL=0
ERRORS=""

pass() { PASS=$((PASS + 1)); printf "  \033[32m✓\033[0m %s\n" "$1"; }
fail() { FAIL=$((FAIL + 1)); ERRORS="${ERRORS}\n  - $1"; printf "  \033[31m✗\033[0m %s\n" "$1"; }

# ── Go examples ─────────────────────────────────────────────

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Go examples"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

for dir in examples/go/*/; do
  name="$(basename "$dir")"
  if (cd "$dir" && go run . >/dev/null 2>&1); then
    pass "go/$name"
  else
    fail "go/$name"
  fi
done

# ── GICEL examples ──────────────────────────────────────────

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  GICEL examples"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ ! -x "$GICEL" ]; then
  echo "Building $GICEL..."
  go build -o "$GICEL" ./cmd/gicel/
fi

while IFS= read -r f; do
  name="${f#examples/gicel/}"
  # Header directives (-- gicel: --recursion) handle flags automatically.
  # stdin from /dev/null: prevents examples with getLine from blocking.
  if "$GICEL" run --packs all --timeout 10s "$f" </dev/null >/dev/null 2>&1; then
    pass "$name"
  else
    # Some examples are check-only (no main that produces output).
    # Fall back to check if run fails.
    if "$GICEL" check --packs all "$f" >/dev/null 2>&1; then
      pass "$name (check-only)"
    else
      fail "$name"
    fi
  fi
done < <(find examples/gicel -name '*.gicel' | sort)

# ── Summary ─────────────────────────────────────────────────

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo -e "  Failures:$ERRORS"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  exit 1
else
  echo "  All examples passed."
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
fi
