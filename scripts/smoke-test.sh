#!/usr/bin/env bash
# smoke-test.sh — CLI smoke test suite for gicel.
# Covers normal operation, edge cases, resource limits, and adversarial inputs.
# Usage: ./scripts/smoke-test.sh [path/to/gicel]
# Exit code: 0 if all tests pass, 1 if any fail.

set -euo pipefail

GICEL="${1:-bin/gicel}"
PASS=0
FAIL=0
ERRORS=""

# --- Helpers ---

pass() { PASS=$((PASS + 1)); printf "  \033[32m✓\033[0m %s\n" "$1"; }
fail() { FAIL=$((FAIL + 1)); ERRORS="${ERRORS}\n  - $1"; printf "  \033[31m✗\033[0m %s\n" "$1"; }

# expect_ok: run command, expect exit 0
expect_ok() {
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then pass "$label"; else fail "$label"; fi
}

# expect_fail: run command, expect non-zero exit
expect_fail() {
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then fail "$label (expected failure)"; else pass "$label"; fi
}

# expect_output: run command, expect exit 0 and stdout contains string
expect_output() {
  local label="$1"; local expected="$2"; shift 2
  local out
  if out=$("$@" 2>&1) && echo "$out" | grep -qF "$expected"; then
    pass "$label"
  else
    fail "$label (expected '$expected')"
  fi
}

# expect_error_contains: run command, expect non-zero exit and stderr/stdout contains string
expect_error_contains() {
  local label="$1"; local expected="$2"; shift 2
  local out
  if out=$("$@" 2>&1); then
    fail "$label (expected failure)"
  elif echo "$out" | grep -qF "$expected"; then
    pass "$label"
  else
    fail "$label (missing '$expected' in output)"
  fi
}

# --- Build ---

echo "Building..."
go build -o "$GICEL" ./cmd/gicel/ || { echo "Build failed"; exit 1; }
echo ""

# === Normal Operation ===
echo "Normal operation:"

expect_output "arithmetic" "7" \
  "$GICEL" run -e 'import Prelude; main := 1 + 2 * 3'

expect_output "string concat" '"42!"' \
  "$GICEL" run -e 'import Prelude; main := show 42 <> "!"'

expect_output "type check" "ok" \
  "$GICEL" check -e 'import Prelude; main := 1 + 2'

expect_output "custom entry" "99" \
  "$GICEL" run --entry myMain -e 'import Prelude; myMain := 99'

expect_output "JSON output" '"ok": true' \
  "$GICEL" run --json -e 'import Prelude; main := 42'

expect_output "type family reduction" "MkProxy" \
  "$GICEL" run -e '
import Prelude
data Bool := True | False
type Not (b: Bool) :: Bool := { Not True =: False; Not False =: True }
data Proxy (a: Bool) := MkProxy
f :: Proxy (Not (Not True)) -> Proxy True
f := \x. x
main := f MkProxy
'

expect_output "stateful computation" "11" \
  "$GICEL" run -e '
import Prelude
import Effect.State
main := do { put 10; x <- get; pure (x + 1) }
'

expect_output "recursion" "120" \
  "$GICEL" run --recursion -e '
import Prelude
f := fix (\self n. case n { 0 -> 1; _ -> n * self (n - 1) })
main := f 5
'

expect_output "stdin" "42" \
  bash -c 'echo "import Prelude; main := 7 * 6" | '"$GICEL"' run -'

expect_output "multi-module" '(3, "red", 6)' \
  "$GICEL" run \
    --module Geometry=examples/cli/multi-module/Geometry.gicel \
    --module Color=examples/cli/multi-module/Color.gicel \
    --module MathLib=examples/cli/multi-module/MathLib.gicel \
    examples/cli/multi-module/main.gicel

expect_ok "explain trace" \
  "$GICEL" run --explain -e 'import Prelude; main := 1 + 2'

expect_ok "docs index" \
  "$GICEL" docs

expect_output "docs topic" "Records" \
  "$GICEL" docs features.records

expect_ok "example listing" \
  "$GICEL" example

expect_ok "example view" \
  "$GICEL" example basics.hello

echo ""

# === Error Handling ===
echo "Error handling:"

expect_fail "empty program" \
  "$GICEL" run -e ''

expect_error_contains "no main" "not found" \
  "$GICEL" run -e 'import Prelude; x := 42'

expect_error_contains "type error" "type mismatch" \
  "$GICEL" check -e 'import Prelude; main := "hello" + 1'

expect_error_contains "unknown pack" "unknown pack" \
  "$GICEL" run --use bogus -e 'main := 1'

expect_error_contains "missing file" "no such file" \
  "$GICEL" run --module Foo=nonexistent.gicel -e 'main := 1'

expect_error_contains "fix without recursion" "unbound variable" \
  "$GICEL" run -e 'import Prelude; f := fix (\self n. n); main := f 1'

expect_error_contains "non-exhaustive" "missing" \
  "$GICEL" check -e '
import Prelude
data Color := Red | Green | Blue
f :: Color -> Int
f := \c. case c { Red -> 1; Green -> 2 }
'

echo ""

# === Resource Limits ===
echo "Resource limits:"

expect_error_contains "step limit" "step limit" \
  "$GICEL" run --recursion --max-steps 100 -e '
import Prelude
loop := fix (\self x. self (x + 1))
main := loop 0
'

expect_error_contains "timeout" "step limit" \
  "$GICEL" run --recursion --timeout 100ms -e '
import Prelude
loop := fix (\self x. self (x + 1))
main := loop 0
'

expect_error_contains "alloc limit" "allocation limit" \
  "$GICEL" run --max-alloc 1024 -e '
import Prelude
main := replicate 10000 42
'

expect_error_contains "exponential type family" "too large" \
  "$GICEL" check -e '
import Prelude
data Pair a b := MkPair a b
type Grow (a: Type) :: Type := { Grow a =: Grow (Pair a a) }
f :: Grow Int -> Int
f := \x. x
'

echo ""

# === Adversarial Inputs ===
echo "Adversarial inputs:"

expect_error_contains "null bytes" "unexpected character" \
  bash -c 'printf "import Prelude\x00; main := 1" | '"$GICEL"' run -'

expect_ok "long identifier (10000 chars)" \
  "$GICEL" check -e "import Prelude; $(printf 'a%.0s' $(seq 1 10000)) := 1; main := 1"

expect_ok "deep nesting (200 parens)" \
  "$GICEL" check -e "import Prelude; main := $(printf '(%.0s' $(seq 1 200))1$(printf ')%.0s' $(seq 1 200))"

echo ""

# === Summary ===
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  printf "  Failures:%b\n" "$ERRORS"
  exit 1
else
  echo "  All tests passed."
  exit 0
fi
