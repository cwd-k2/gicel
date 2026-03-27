// Specialized stress tests — nested patterns, type application, higher-rank record fields, packed class resolution, literal patterns.
// Does NOT cover: stress_parser_test.go, stress_checker_test.go, stress_evaluator_test.go, stress_correctness_test.go, stress_crosscutting_test.go, stress_limits_test.go.
package stress_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// NP1. Nested patterns: bare constructor in pattern argument position
// ---------------------------------------------------------------------------

func TestNestedPatternBareConstructor(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form Pair := \a b. { MkPair: a -> b -> Pair a b; }

match := \p. case p { MkPair (Just x) Nothing => x; _ => False }
main := match (MkPair (Just True) Nothing)
`)
	if err != nil {
		t.Fatal("bare constructor in pattern arg should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

func TestNestedPatternParenthesized(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
isJustTrue := \m. case m { Just True => True; _ => False }
main := (isJustTrue (Just True), isJustTrue (Just False), isJustTrue Nothing)
`)
	if err != nil {
		t.Fatal("nested constructor pattern (Just True) should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify: (True, False, False) — positive AND negative cases
	rv := result.Value.(*gicel.RecordVal)
	assertCon(t, rv.MustGet("_1"), "True")
	assertCon(t, rv.MustGet("_2"), "False")
	assertCon(t, rv.MustGet("_3"), "False")
}

func TestNestedPatternExhaustiveness(t *testing.T) {
	// Non-exhaustive nested pattern should produce clear error.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \x. case x { Just True => True }
main := f (Just True)
`)
	if err == nil {
		t.Fatal("expected non-exhaustive error for nested pattern")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "non-exhaustive") {
		t.Errorf("expected non-exhaustive message, got: %s", errStr)
	}
}

func TestNestedPatternDeepNesting(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \x. case x { Just (Just (Just True)) => True; _ => False }
main := (f (Just (Just (Just True))), f (Just (Just (Just False))), f (Just (Just Nothing)))
`)
	if err != nil {
		t.Fatal("deeply nested pattern should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv := result.Value.(*gicel.RecordVal)
	assertCon(t, rv.MustGet("_1"), "True")
	assertCon(t, rv.MustGet("_2"), "False")
	assertCon(t, rv.MustGet("_3"), "False")
}

// ---------------------------------------------------------------------------
// TA1. Type application: explicit @Type should work
// ---------------------------------------------------------------------------

func TestExplicitTypeApplication(t *testing.T) {
	// Verify @Type actually constrains: id @Bool applied to non-Bool must fail.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
id :: \a. a -> a
id := \x. x
main := id @Bool True
`)
	if err != nil {
		t.Fatal("explicit type application should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")

	// Negative: @Bool constrains, so Int argument must fail.
	eng2 := gicel.NewEngine()
	eng2.Use(gicel.Prelude)
	_, err = eng2.NewRuntime(context.Background(), `
import Prelude
id :: \a. a -> a
id := \x. x
main := id @Bool 42
`)
	if err == nil {
		t.Fatal("expected type error: @Bool applied but Int argument given")
	}
}

func TestExplicitTypeApplicationWrongType(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
id :: \a. a -> a
id := \x. x
main := id @Int True
`)
	if err == nil {
		t.Fatal("expected type error: @Int applied but Bool argument given")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "mismatch") && !strings.Contains(errStr, "Int") {
		t.Errorf("expected type mismatch error mentioning Int, got: %s", errStr)
	}
}

func TestExplicitTypeApplicationOnConstructor(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := Just @Bool True
`)
	if err != nil {
		t.Fatal("explicit type application on constructor should work:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con := result.Value.(*gicel.ConVal)
	if con.Con != "Just" || len(con.Args) != 1 {
		t.Fatalf("expected Just with 1 arg, got %s", result.Value)
	}
	assertCon(t, con.Args[0], "True")
}

// ---------------------------------------------------------------------------
// HR1. Higher-rank types in record fields
// ---------------------------------------------------------------------------

func TestHigherRankRecordField(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
r :: Record { apply : \a. a -> a }
r := { apply: \x. x }
main := ((r.#apply) True, (r.#apply) 42)
`)
	if err != nil {
		t.Fatal("higher-rank record field should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify polymorphic use: (True, 42) — field used at Bool AND Int.
	rv := result.Value.(*gicel.RecordVal)
	assertCon(t, rv.MustGet("_1"), "True")
	v := gicel.MustHost[int64](rv.MustGet("_2"))
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

// ---------------------------------------------------------------------------
// PC1. Packed class: Slice instance resolution with concrete types
// ---------------------------------------------------------------------------

func TestPackedSliceConcreteType(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.DataSlice)

	// Test 1: with annotation
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Data.Slice
xs :: Slice Int
xs := fromList [1, 2, 3]
main := toList xs
`)
	if err != nil {
		t.Fatal("Packed (Slice Int) Int should resolve:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Result should be a List (Cons 1 (Cons 2 (Cons 3 Nil)))
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", result.Value)
	}

	// Test 2: without annotation (inferred)
	eng2 := gicel.NewEngine()
	eng2.Use(gicel.Prelude)
	eng2.Use(gicel.DataSlice)
	rt2, err := eng2.NewRuntime(context.Background(), `
import Prelude
import Data.Slice
main := toList (fromList [1, 2, 3] :: Slice Int)
`)
	if err != nil {
		t.Fatal("Packed Slice without annotation should resolve:", err)
	}
	result2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con2, ok := result2.Value.(*gicel.ConVal)
	if !ok || con2.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", result2.Value)
	}
}

// ---------------------------------------------------------------------------
// LP1. Literal patterns in case expressions
// ---------------------------------------------------------------------------

func TestLiteralPatternInt(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
describe := \n. case n { 0 => "zero"; 1 => "one"; _ => "other" }
main := describe 1
`)
	if err != nil {
		t.Fatal("integer literal pattern should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := gicel.MustHost[string](result.Value)
	if s != "one" {
		t.Errorf("expected \"one\", got %q", s)
	}
}

func TestLiteralPatternString(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
greet := \name. case name { "Alice" => "Hello Alice"; _ => "Hello stranger" }
main := greet "Alice"
`)
	if err != nil {
		t.Fatal("string literal pattern should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := gicel.MustHost[string](result.Value)
	if s != "Hello Alice" {
		t.Errorf("expected \"Hello Alice\", got %q", s)
	}
}

func TestLiteralPatternFallthrough(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
fib := \n. case n { 0 => 0; 1 => 1; _ => 99 }
main := fib 5
`)
	if err != nil {
		t.Fatal("literal pattern with wildcard fallthrough should work:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	v := gicel.MustHost[int64](result.Value)
	if v != 99 {
		t.Errorf("expected 99, got %d", v)
	}
}

func TestLiteralPatternInNestedCase(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \m. case m { Just n => case n { 0 => True; _ => False }; Nothing => False }
main := f (Just 0)
`)
	if err != nil {
		t.Fatal("literal pattern in nested case should work:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

func TestLiteralPatternRune(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
classify := \c. case c { 'a' => "vowel"; 'e' => "vowel"; _ => "other" }
main := classify 'a'
`)
	if err != nil {
		t.Fatal("rune literal pattern should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := gicel.MustHost[string](result.Value)
	if s != "vowel" {
		t.Errorf("expected \"vowel\", got %q (rune pattern match failed)", s)
	}
}

// ---------------------------------------------------------------------------
// List pattern stress — large list, deeply nested
// ---------------------------------------------------------------------------

func TestStressLargeListPattern(t *testing.T) {
	// 50-element exact-match list pattern.
	var sb strings.Builder
	sb.WriteString("import Prelude\n")
	sb.WriteString("f := \\xs. case xs { [")
	for i := range 50 {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("x%d", i))
	}
	sb.WriteString(fmt.Sprintf("] => x0 + x%d; _ => 0 }\n", 49))
	sb.WriteString("main := f [")
	for i := range 50 {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%d", i))
	}
	sb.WriteString("]\n")

	result, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxSteps: 500_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	n := gicel.MustHost[int64](result.Value)
	if n != 49 { // x0=0, x49=49, 0+49=49
		t.Errorf("expected 49, got %d", n)
	}
}

func TestStressNestedListPattern(t *testing.T) {
	// 10 levels of list nesting: [[[[[[[[[[x]]]]]]]]]]
	depth := 10
	var patBuf strings.Builder
	for range depth {
		patBuf.WriteString("[")
	}
	patBuf.WriteString("x")
	for range depth {
		patBuf.WriteString("]")
	}

	var valBuf strings.Builder
	for range depth {
		valBuf.WriteString("[")
	}
	valBuf.WriteString("42")
	for range depth {
		valBuf.WriteString("]")
	}

	src := fmt.Sprintf("import Prelude\nf := \\xs. case xs { %s => x; _ => 0 }\nmain := f %s\n",
		patBuf.String(), valBuf.String())

	result, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxSteps: 500_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	n := gicel.MustHost[int64](result.Value)
	if n != 42 {
		t.Errorf("expected 42, got %d", n)
	}
}
