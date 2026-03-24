#!/usr/bin/env bash
# scaling-test.sh — Generate GICEL programs at varying input sizes and
# measure wall-clock time, reporting a scaling table.
#
# Usage:
#   ./scripts/scaling-test.sh            # build + run all suites
#   ./scripts/scaling-test.sh sort       # run one suite
#   ./scripts/scaling-test.sh --no-build # skip binary build
#
# Requires: bin/gicel (built automatically unless --no-build).

set -euo pipefail

GICEL="${GICEL:-bin/gicel}"
TMPDIR_BASE="${TMPDIR:-/tmp}/gicel-scaling-$$"
SKIP_BUILD=false
SUITES=()

for arg in "$@"; do
  case "$arg" in
    --no-build) SKIP_BUILD=true ;;
    *) SUITES+=("$arg") ;;
  esac
done

if [ "$SKIP_BUILD" = false ]; then
  echo "Building bin/gicel..."
  go build -o bin/gicel ./cmd/gicel/
fi

mkdir -p "$TMPDIR_BASE"
trap 'rm -rf "$TMPDIR_BASE"' EXIT

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

run_timed() {
  local label="$1" file="$2"
  shift 2
  local combined real_s first_line
  combined=$( { /usr/bin/time -p "$GICEL" run "$@" "$file" ; } 2>&1 ) || true

  # /usr/bin/time -p prints "real X.XX" on stderr, merged into combined
  real_s=$(echo "$combined" | grep '^real ' | awk '{print $2}')
  first_line=$(echo "$combined" | grep -v '^real \|^user \|^sys ' | head -1)

  if echo "$combined" | grep -q '^error\|^runtime error'; then
    printf "  %-12s  %7ss  ERROR: %s\n" "$label" "${real_s:-?}" "$first_line"
  else
    printf "  %-12s  %7ss  %s\n" "$label" "${real_s:-?}" "$first_line"
  fi
}

# ---------------------------------------------------------------------------
# Suite: sort — merge sort on N random integers
# ---------------------------------------------------------------------------

generate_sort() {
  local n="$1"
  cat > "$TMPDIR_BASE/sort_${n}.gicel" <<GICEL
import Prelude
import Console
splitHalf :: \\a. List a -> (List a, List a)
splitHalf := fix \$ \\self xs. case xs {
  Nil => (Nil, Nil);
  Cons x rest => case rest {
    Nil => (Cons x Nil, Nil);
    Cons y rest2 => { (as, bs) := self rest2; (Cons x as, Cons y bs) }
  }
}
merge :: List Int -> List Int -> List Int
merge := fix \$ \\self xs ys. case xs {
  Nil => ys;
  Cons x xr => case ys {
    Nil => xs;
    Cons y yr => if x <= y then Cons x \$ self xr ys else Cons y \$ self xs yr
  }
}
msort :: List Int -> List Int
msort := fix \$ \\self xs. case xs {
  Nil => Nil;
  Cons x rest => case rest {
    Nil => [x];
    _   => { (l, r) := splitHalf xs; merge (self l) (self r) }
  }
}
lcg :: Int -> Int -> List Int
lcg := fix \$ \\self seed n.
  if n == 0 then Nil
  else { next := mod (seed * 1103515245 + 12345) 2147483648; Cons (mod next 10000) \$ self next (n - 1) }
main := do { sorted := msort (lcg 42 $n); putLine \$ "n=$n sorted=" <> show (length sorted) }
GICEL
}

suite_sort() {
  echo "=== Sort (merge sort) ==="
  for n in 50 100 200 500 1000 2000; do
    generate_sort "$n"
    run_timed "N=$n" "$TMPDIR_BASE/sort_${n}.gicel" \
      --recursion --max-steps 100000000 --max-depth 50000 --max-nesting 50000 --timeout 30s
  done
  echo
}

# ---------------------------------------------------------------------------
# Suite: map — immutable Map insert + lookup
# ---------------------------------------------------------------------------

generate_map() {
  local n="$1"
  cat > "$TMPDIR_BASE/map_${n}.gicel" <<GICEL
import Prelude
import Data.Map as Map
import Console
buildMap :: Int -> Int -> Map Int Int
buildMap := fix \$ \\self seed n.
  if n == 0 then (Map.empty :: Map Int Int)
  else { next := mod (seed * 1103515245 + 12345) 2147483648; Map.insert (mod next 100000) n \$ self next (n - 1) }
lookupAll :: Map Int Int -> Int -> Int -> Int
lookupAll := fix \$ \\self m seed n.
  if n == 0 then 0
  else { next := mod (seed * 1103515245 + 12345) 2147483648; found := maybe 0 id (Map.lookup (mod next 100000) m); found + self m next (n - 1) }
main := do {
  m := buildMap 42 $n;
  total := lookupAll m 42 $n;
  putLine \$ "n=$n size=" <> show (Map.size m) <> " sum=" <> show total
}
GICEL
}

suite_map() {
  echo "=== Map (AVL insert + lookup) ==="
  for n in 50 100 200 500 1000 2000; do
    generate_map "$n"
    run_timed "N=$n" "$TMPDIR_BASE/map_${n}.gicel" \
      --recursion --max-steps 100000000 --max-depth 50000 --max-nesting 50000 --timeout 30s
  done
  echo
}

# ---------------------------------------------------------------------------
# Suite: sieve — Eratosthenes via Effect.Array
# ---------------------------------------------------------------------------

generate_sieve() {
  local n="$1"
  cat > "$TMPDIR_BASE/sieve_${n}.gicel" <<GICEL
import Prelude
import Effect.Array
import Console
sieve := \\limit. do {
  is <- new (limit + 1) True;
  writeAt 0 False is; writeAt 1 False is;
  flip fix 2 \$ \\outer i.
    if i * i > limit then pure ()
    else do {
      v <- readAt i is;
      case v { Just True => flip fix (i * i) \$ \\inner j. if j > limit then pure () else do { writeAt j False is; inner \$ j + i }; _ => pure () };
      outer \$ i + 1
    };
  flip fix (2, 0) \$ \\count (i, acc).
    if i > limit then pure acc
    else do { v <- readAt i is; count (i + 1, if maybe False id v then acc + 1 else acc) }
}
main := do { n <- sieve $n; putLine \$ "primes up to $n: " <> show n }
GICEL
}

suite_sieve() {
  echo "=== Sieve (Eratosthenes, Effect.Array) ==="
  for n in 100 500 1000 5000 10000 50000; do
    generate_sieve "$n"
    run_timed "N=$n" "$TMPDIR_BASE/sieve_${n}.gicel" \
      --recursion --max-steps 500000000 --max-depth 100000 --max-nesting 100000 --max-alloc 500000000 --timeout 60s
  done
  echo
}

# ---------------------------------------------------------------------------
# Suite: set — immutable Set operations (union, intersection, difference)
# ---------------------------------------------------------------------------

generate_set() {
  local n="$1"
  cat > "$TMPDIR_BASE/set_${n}.gicel" <<GICEL
import Prelude
import Data.Set as Set
import Console
buildSet :: Int -> Int -> Set Int
buildSet := fix \$ \\self seed n.
  if n == 0 then (Set.empty :: Set Int)
  else { next := mod (seed * 1103515245 + 12345) 2147483648; Set.insert (mod next 100000) \$ self next (n - 1) }
main := do {
  s1 := buildSet 42 $n;
  s2 := buildSet 7 $n;
  u := Set.union s1 s2;
  i := Set.intersection s1 s2;
  putLine \$ "n=$n s1=" <> show (Set.size s1) <> " union=" <> show (Set.size u) <> " inter=" <> show (Set.size i)
}
GICEL
}

suite_set() {
  echo "=== Set (union/intersection) ==="
  for n in 50 100 200 500 1000; do
    generate_set "$n"
    run_timed "N=$n" "$TMPDIR_BASE/set_${n}.gicel" \
      --recursion --max-steps 100000000 --max-depth 50000 --max-nesting 50000 --timeout 30s
  done
  echo
}

# ---------------------------------------------------------------------------
# Suite: doblock — compile cost of deep do-block bind chains
# ---------------------------------------------------------------------------

generate_doblock() {
  local n="$1"
  local src="import Prelude\nimport Effect.State\nimport Console\ncompute := thunk do {\n  put 0;\n"
  for ((i = 0; i < n; i++)); do
    src+="  n${i} <- get; put (n${i} + 1);\n"
  done
  src+="  result <- get;\n  pure result\n}\nmain := do { r <- force compute; putLine \$ \"n=$n result=\" <> show r }\n"
  printf '%b' "$src" > "$TMPDIR_BASE/doblock_${n}.gicel"
}

suite_doblock() {
  echo "=== Do-block (state bind chain, compile+eval) ==="
  for n in 5 10 20 30 50; do
    generate_doblock "$n"
    run_timed "binds=$n" "$TMPDIR_BASE/doblock_${n}.gicel" \
      --recursion --max-steps 1000000 --max-depth 10000 --timeout 30s
  done
  echo
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

echo "GICEL Scaling Test"
echo "Binary: $GICEL"
echo "Temp:   $TMPDIR_BASE"
echo

ALL_SUITES=(sort map sieve set doblock)

if [ ${#SUITES[@]} -eq 0 ]; then
  SUITES=("${ALL_SUITES[@]}")
fi

for suite in "${SUITES[@]}"; do
  case "$suite" in
    sort)    suite_sort ;;
    map)     suite_map ;;
    sieve)   suite_sieve ;;
    set)     suite_set ;;
    doblock) suite_doblock ;;
    *) echo "Unknown suite: $suite (available: ${ALL_SUITES[*]})" ;;
  esac
done
