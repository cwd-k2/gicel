//go:build probe

package probe_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe E (Parser): Integration-level parser/syntax probes through
// the Engine API. Verifies the full front-end pipeline: lexer, parser,
// type checker, Core lowering, and evaluation for syntax edge cases.
// ===================================================================

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func peParserCompile(t *testing.T, source string, packs ...gicel.Pack) (*gicel.Runtime, error) {
	t.Helper()
	eng := gicel.NewEngine()
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	return eng.NewRuntime(source)
}

func peParserRun(t *testing.T, source string, packs ...gicel.Pack) (gicel.Value, error) {
	t.Helper()
	rt, err := peParserCompile(t, source, packs...)
	if err != nil {
		return nil, err
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

func peParserCompileMustFail(t *testing.T, source string, packs ...gicel.Pack) string {
	t.Helper()
	_, err := peParserCompile(t, source, packs...)
	if err == nil {
		t.Fatal("expected compilation error, got none")
	}
	return err.Error()
}

// ----- 1. Lexer/Parser Errors Through Engine API -----

func TestProbeE_Parser_UnterminatedString(t *testing.T) {
	errMsg := peParserCompileMustFail(t, `main := "hello`)
	if !strings.Contains(errMsg, "unterminated") {
		t.Errorf("expected 'unterminated' in error, got: %s", errMsg)
	}
}

func TestProbeE_Parser_NullByteInSource(t *testing.T) {
	source := "main\x00 := 42"
	_, err := peParserCompile(t, source)
	// Null byte is an "unexpected character" — just verify no panic.
	_ = err
}

func TestProbeE_Parser_BinaryJunk(t *testing.T) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	_, err := peParserCompile(t, string(data))
	if err == nil {
		t.Error("expected error for binary data")
	}
}

func TestProbeE_Parser_EmptySource(t *testing.T) {
	_, err := peParserCompile(t, "")
	// Empty source may or may not error (might produce a default main).
	// The important thing is no panic.
	_ = err
}

func TestProbeE_Parser_OnlyComments(t *testing.T) {
	_, err := peParserCompile(t, "-- just a comment\n{- block -}")
	// Comments-only source may or may not error. No panic is the criterion.
	_ = err
}

// ----- 2. Operator Parsing -----

func TestProbeE_Parser_InfixNonAssocChainRuntime(t *testing.T) {
	source := `import Prelude
main := 1 == 2 == 3`
	errMsg := peParserCompileMustFail(t, source, gicel.Prelude)
	if !strings.Contains(errMsg, "non-associative") {
		t.Errorf("expected 'non-associative' in error, got: %s", errMsg)
	}
}

func TestProbeE_Parser_OperatorSection(t *testing.T) {
	source := `import Prelude
inc := (+ 1)
main := inc 41`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

func TestProbeE_Parser_LeftSection(t *testing.T) {
	source := `import Prelude
dec := (100 -)
main := dec 58`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

func TestProbeE_Parser_OperatorAsValue(t *testing.T) {
	source := `import Prelude
myAdd := (+)
main := myAdd 20 22`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

func TestProbeE_Parser_PrecedenceCorrectness(t *testing.T) {
	source := `import Prelude
main := 1 + 2 * 3`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 7 {
		t.Errorf("expected 7, got %d", hv)
	}
}

func TestProbeE_Parser_RightAssocDollar(t *testing.T) {
	source := `import Prelude
double := \x. x + x
inc := \x. x + 1
main := double $ inc $ 10`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 22 {
		t.Errorf("expected 22, got %d", hv)
	}
}

func TestProbeE_Parser_DotComposition(t *testing.T) {
	source := `import Prelude
double := \x. x + x
inc := \x. x + 1
main := (double . inc) 10`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 22 {
		t.Errorf("expected 22, got %d", hv)
	}
}

// ----- 3. Pattern Matching -----

func TestProbeE_Parser_NestedPatternMatch(t *testing.T) {
	source := `import Prelude
data Pair a b := MkPair a b
main := case MkPair (Just 42) True {
  MkPair Nothing _ -> 0;
  MkPair (Just n) True -> n;
  _ -> 0
}`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

func TestProbeE_Parser_LiteralPattern(t *testing.T) {
	source := `import Prelude
f := \n. case n { 0 -> "zero"; 1 -> "one"; _ -> "other" }
main := f 1`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[string](v)
	if hv != "one" {
		t.Errorf("expected 'one', got %s", hv)
	}
}

func TestProbeE_Parser_TuplePatternMatch(t *testing.T) {
	source := `import Prelude
main := case (1, 2) { (a, b) -> a + b }`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}
}

// ----- 4. Do-Notation -----

func TestProbeE_Parser_DoBlockRuntime(t *testing.T) {
	source := `import Prelude
main := do {
  x := 1;
  y := 2;
  pure (x + y)
}`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}
}

func TestProbeE_Parser_DoBlockWithBind(t *testing.T) {
	source := `import Prelude
main := do {
  x <- pure 1;
  y <- pure 2;
  pure (x + y)
}`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}
}

// ----- 5. Import Errors -----

func TestProbeE_Parser_ImportNonexistentModule(t *testing.T) {
	source := `import Nonexistent
main := 1`
	errMsg := peParserCompileMustFail(t, source)
	if !strings.Contains(errMsg, "Nonexistent") {
		t.Errorf("expected 'Nonexistent' in error, got: %s", errMsg)
	}
}

// ----- 6. Record/Projection -----

func TestProbeE_Parser_RecordLiteralRuntime(t *testing.T) {
	source := `main := { x: 1, y: 2 }.#x`
	v, err := peParserRun(t, source)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 1 {
		t.Errorf("expected 1, got %d", hv)
	}
}

func TestProbeE_Parser_TupleProjection(t *testing.T) {
	source := `main := (10, 20).#_2`
	v, err := peParserRun(t, source)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 20 {
		t.Errorf("expected 20, got %d", hv)
	}
}

// ----- 7. Lambda -----

func TestProbeE_Parser_LambdaMultipleParams(t *testing.T) {
	source := `import Prelude
main := (\x y. x + y) 20 22`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

// ----- 8. Type Application -----

func TestProbeE_Parser_TypeApplicationRuntime(t *testing.T) {
	source := `import Prelude
id :: \a. a -> a
id := \x. x
main := id @Int 42`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

// ----- 9. Deep Nesting Crash Resistance -----

func TestProbeE_Parser_DeepNestedParensRuntime(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 300; i++ {
		b.WriteString("(")
	}
	b.WriteString("42")
	for i := 0; i < 300; i++ {
		b.WriteString(")")
	}
	_, err := peParserCompile(t, b.String())
	_ = err // No panic is the success criterion.
}

func TestProbeE_Parser_DeepNestedLambdasRuntime(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 100; i++ {
		b.WriteString("\\x. ")
	}
	b.WriteString("42")
	_, err := peParserCompile(t, b.String())
	_ = err
}

// ----- 10. GADT End-to-End -----

func TestProbeE_Parser_GADTEndToEnd(t *testing.T) {
	source := `import Prelude
data Expr a := {
  LitBool :: Bool -> Expr Bool;
  LitInt  :: Int -> Expr Int
}

getInt :: Expr Int -> Maybe Int
getInt := \e. case e {
  LitInt n -> Just n
}

main := getInt (LitInt 42)`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %T %v", v, v)
	}
}

// ----- 11. Block Expression -----

func TestProbeE_Parser_BlockExprEndToEnd(t *testing.T) {
	source := `import Prelude
main := { x := 10; y := 32; x + y }`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

// ----- 12. List Literal -----

func TestProbeE_Parser_ListLiteralEndToEnd(t *testing.T) {
	source := `import Prelude
main := [1, 2, 3]`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	_ = v
}

// ----- 13. Qualified Name -----

func TestProbeE_Parser_QualifiedNameRuntime(t *testing.T) {
	source := `import Prelude as P
main := P.add 20 22`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

// ----- 14. Error Message Quality -----

func TestProbeE_Parser_ErrorMsgQuality(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		keyword string
	}{
		{"missing_body", "main :=", "expression"},
		{"bad_data_decl", "data := True", "uppercase"},
		{"bad_import", "import\nmain := 1", "uppercase"},
		{"unterminated_string", `main := "hello`, "unterminated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errMsg := peParserCompileMustFail(t, tc.source)
			if !strings.Contains(strings.ToLower(errMsg), strings.ToLower(tc.keyword)) {
				t.Errorf("expected %q in error, got: %s", tc.keyword, errMsg)
			}
		})
	}
}

// ----- 15. Class/Instance End-to-End -----

func TestProbeE_Parser_ClassInstanceEndToEnd(t *testing.T) {
	source := `import Prelude
data Color := Red | Green | Blue

class Named a {
  name :: a -> String
}

instance Named Color {
  name := \c. case c {
    Red -> "red";
    Green -> "green";
    Blue -> "blue"
  }
}

main := name Red`
	v, err := peParserRun(t, source, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[string](v)
	if hv != "red" {
		t.Errorf("expected 'red', got %s", hv)
	}
}
