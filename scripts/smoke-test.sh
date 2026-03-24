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
  "$GICEL" run --show -e 'import Prelude; main := 1 + 2 * 3'

expect_output "string concat" '"42!"' \
  "$GICEL" run --show -e 'import Prelude; main := show 42 <> "!"'

expect_output "type check" "ok" \
  "$GICEL" check -e 'import Prelude; main := 1 + 2'

expect_output "custom entry" "99" \
  "$GICEL" run --show --entry myMain -e 'import Prelude; myMain := 99'

expect_output "JSON output" '"ok": true' \
  "$GICEL" run --json -e 'import Prelude; main := 42'

expect_output "type family reduction" "MkProxy" \
  "$GICEL" run --show -e '
import Prelude
form Bool2 := True2 | False2
type Not :: Bool2 := \(b: Bool2). case b { True2 => False2; False2 => True2 }
form Proxy := \(a: Bool2). { MkProxy: Proxy a }
f :: Proxy (Not (Not True2)) -> Proxy True2
f := \x. x
main := f MkProxy
'

expect_output "stateful computation" "11" \
  "$GICEL" run --show -e '
import Prelude
import Effect.State
main := do { put 10; x <- get; pure (x + 1) }
'

expect_output "recursion" "120" \
  "$GICEL" run --show --recursion -e '
import Prelude
f := fix (\self n. case n { 0 => 1; _ => n * self (n - 1) })
main := f 5
'

expect_output "stdin" "42" \
  bash -c 'echo "import Prelude; main := 7 * 6" | '"$GICEL"' run --show -'

expect_output "multi-module" '(3, "red", 6)' \
  "$GICEL" run --show \
    --module Geometry=examples/cli/multi-module/Geometry.gicel \
    --module Color=examples/cli/multi-module/Color.gicel \
    --module MathLib=examples/cli/multi-module/MathLib.gicel \
    examples/cli/multi-module/main.gicel

expect_ok "explain trace" \
  "$GICEL" run --show --explain -e 'import Prelude; main := 1 + 2'

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
  "$GICEL" run --show -e ''

expect_error_contains "no main" "not found" \
  "$GICEL" run --show -e 'import Prelude; x := 42'

expect_error_contains "type error" "type mismatch" \
  "$GICEL" check -e 'import Prelude; main := "hello" + 1'

expect_error_contains "unknown pack" "unknown pack" \
  "$GICEL" run --packs bogus -e 'main := 1'

expect_error_contains "missing file" "no such file" \
  "$GICEL" run --module Foo=nonexistent.gicel -e 'main := 1'

expect_error_contains "fix without recursion" "unbound variable" \
  "$GICEL" run --show -e 'import Prelude; f := fix (\self n. n); main := f 1'

expect_error_contains "non-exhaustive" "missing" \
  "$GICEL" check -e '
import Prelude
form Color := Red | Green | Blue
f :: Color -> Int
f := \c. case c { Red => 1; Green => 2 }
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
form Pair := \a b. { MkPair: a -> b -> Pair a b }
type Grow :: Type := \(a: Type). case a { a => Grow (Pair a a) }
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

# === List Patterns & Pretty Print ===
echo "List patterns & pretty print:"

expect_output "list pattern [x, y]" "7" \
  "$GICEL" run --show -e 'import Prelude; f := \xs. case xs { [x, y] => x + y; _ => 0 }; main := f [3, 4]'

expect_output "list pattern []" '"empty"' \
  "$GICEL" run --show -e 'import Prelude; f := \xs. case xs { [] => "empty"; _ => "other" }; main := f ([] :: List Int)'

expect_output "list pattern nested" '"match"' \
  "$GICEL" run --show -e 'import Prelude; f := \xs. case xs { [[1], [2, 3]] => "match"; _ => "no" }; main := f [[1], [2, 3]]'

expect_output "list pattern with constructor" "99" \
  "$GICEL" run --show -e 'import Prelude; f := \xs. case xs { [Just x, Nothing] => x; _ => 0 }; main := f [Just 99, Nothing]'

expect_output "list pattern literal" '"yes"' \
  "$GICEL" run --show -e 'import Prelude; f := \xs. case xs { [1, 2, 3] => "yes"; _ => "no" }; main := f [1, 2, 3]'

expect_output "mixing Cons and []" "42" \
  "$GICEL" run --show -e 'import Prelude; f := \xs. case xs { Cons x [] => x; _ => 0 }; main := f [42]'

expect_output "pretty list" "[1, 2, 3]" \
  "$GICEL" run --show -e 'import Prelude; main := [1, 2, 3]'

expect_output "pretty nested list" "[[1, 2], [3]]" \
  "$GICEL" run --show -e 'import Prelude; main := [[1, 2], [3]]'

expect_output "pretty empty list" "[]" \
  "$GICEL" run --show -e 'import Prelude; main := ([] :: List Int)'

expect_output "JSON list array" '"value": [' \
  "$GICEL" run --json -e 'import Prelude; main := [1, 2, 3]'

expect_output "JSON empty list" '"value": []' \
  "$GICEL" run --json -e 'import Prelude; main := ([] :: List Int)'

echo ""

# === JSON Contract ===
echo "JSON contract:"

expect_error_contains "JSON runtime error has message" '"message":' \
  "$GICEL" run --json -e 'import Prelude; import Effect.Fail; main := fail'

expect_error_contains "JSON preflight error phase" '"phase": "preflight"' \
  "$GICEL" run --json --max-steps -1 -e 'main := 1'

expect_output "JSON success has allocated" '"allocated":' \
  "$GICEL" run --json -e 'import Prelude; main := 42'

echo ""

# === Malformed Inputs ===
echo "Malformed inputs:"

expect_error_contains "operator +.+" "expected expression" \
  "$GICEL" check -e 'import Prelude; main := 1 +.+ 2'

expect_error_contains "operator =:= (reserved :=)" "expected operator" \
  "$GICEL" check -e 'import Prelude; infixl 5 =:=; (=:=) :: Int -> Int -> Int; (=:=) := \x y. x; main := 0'

expect_error_contains "reserved ->" "expected declaration" \
  "$GICEL" check -e 'import Prelude; main := 1 -> 2'

expect_error_contains "reserved <-" "expected declaration" \
  "$GICEL" check -e 'import Prelude; main := 1 <- 2'

expect_error_contains "1a1 (digit-ident boundary)" "unbound variable" \
  "$GICEL" check -e 'main := 1a1'

expect_error_contains '123abc (digit-ident run)' "unbound variable" \
  "$GICEL" check -e 'main := 123abc'

expect_error_contains 'unterminated string' "unterminated" \
  "$GICEL" check -e 'main := "hello'

expect_error_contains 'unterminated rune' "unterminated" \
  "$GICEL" check -e "main := 'a"

expect_error_contains "huge integer literal" "invalid integer literal" \
  "$GICEL" check -e 'main := 99999999999999999999999999999999999999999'

expect_error_contains "unclosed list pattern" "expected ]" \
  "$GICEL" check -e 'import Prelude; f := \xs. case xs { [x, y -> x }; main := 0'

expect_error_contains "list pattern trailing comma" "expected pattern" \
  "$GICEL" check -e 'import Prelude; f := \xs. case xs { [x,] => x; _ => 0 }; main := 0'

expect_error_contains "double comma in list" "expected expression" \
  "$GICEL" check -e 'import Prelude; main := [1,,2]'

expect_error_contains "list type mismatch" "type mismatch" \
  "$GICEL" check -e 'import Prelude; f :: List Int -> Int; f := \xs. case xs { ["hello"] => 0; _ => 1 }; main := 0'

expect_error_contains "list pattern on non-list" "type mismatch" \
  "$GICEL" check -e 'import Prelude; f :: Int -> Int; f := \x. case x { [a] => a; _ => 0 }; main := 0'

expect_error_contains "special chars @#$" "expected declaration" \
  "$GICEL" check -e '@#$%'

expect_ok "1000 semicolons" \
  "$GICEL" check -e "$(printf ';%.0s' $(seq 1 1000))"

expect_ok "semicolons around decls" \
  "$GICEL" run --show -e ';;;import Prelude;;;;main := 1 + 2;;;;'

echo ""

# === New Features (v0.17) ===

expect_output "if-then-else basic" "42" \
  "$GICEL" run --show -e 'import Prelude; main := if True then 42 else 0'

expect_output "if-then-else with variable" "1" \
  "$GICEL" run --show -e 'import Prelude; x := True; main := if x then 1 else 2'

expect_output "pattern binding in block" "30" \
  "$GICEL" run --show -e 'import Prelude; main := { (a, b) := (10, 20); a + b }'

expect_output "pattern binding in do" "7" \
  "$GICEL" run --show -e 'import Prelude; main := do { (x, y) <- pure (3, 4); pure (x + y) }'

expect_output "mmap pack loads" "2" \
  "$GICEL" run --show --packs prelude,mmap -e 'import Prelude; import Effect.Map as MM; main := do { m <- MM.new; MM.insert 1 "a" m; MM.insert 2 "b" m; MM.size m }'

expect_output "mset pack loads" "3" \
  "$GICEL" run --show --packs prelude,mset -e 'import Prelude; import Effect.Set as MS; main := do { s <- MS.new; MS.insert 1 s; MS.insert 2 s; MS.insert 3 s; MS.size s }'

expect_output "Bool JSON output" '"value": true' \
  "$GICEL" run --json -e 'import Prelude; main := True'

expect_error_contains "module name validation error" "must start with an uppercase letter" \
  "$GICEL" run --module "bad=file.gicel" -e 'main := 1'

expect_output "seq combinator" "42" \
  "$GICEL" run --show -e 'import Prelude; main := do { seq (pure 1) (pure 42) }'

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
