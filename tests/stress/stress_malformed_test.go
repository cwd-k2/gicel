package stress_test

// Malformed input stress tests — adversarial token sequences, boundary cases,
// and invalid syntax that must be rejected gracefully.
// Does NOT cover: stress_extended_test.go (valid-program stress).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// Token boundary adversarial inputs
// ---------------------------------------------------------------------------

func TestMalformedDigitIdentBoundary(t *testing.T) {
	cases := []string{
		"main := 1a1",
		"main := 123abc",
		"main := 0x10",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{})
			if err == nil {
				t.Error("expected error for digit-ident boundary")
			}
		})
	}
}

func TestMalformedStringIdentBoundary(t *testing.T) {
	cases := []string{
		`main := a"hoge"`,
		`import Prelude; main := "hello"world`,
		`main := 42"str"`,
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
				Packs: []gicel.Pack{gicel.Prelude},
			})
			if err == nil {
				t.Error("expected error for string-ident boundary")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Operator adversarial inputs
// ---------------------------------------------------------------------------

func TestMalformedOperators(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // substring in error
	}{
		{"dot-mixed +.+", "import Prelude; main := 1 +.+ 2", "expected expression"},
		{"dot-mixed .+.", "import Prelude; main := 1 .+. 2", "expected expression"},
		{"reserved =:=", "import Prelude; infixl 5 =:=; (=:=) :: Int -> Int -> Int; (=:=) := \\x y. x; main := 0", "reserved symbol"},
		{"reserved ->", "import Prelude; main := 1 -> 2", "expected declaration"},
		{"reserved <-", "import Prelude; main := 1 <- 2", "expected declaration"},
		{"reserved :=", "import Prelude; main := 1 := 2", "expected declaration"},
		{"reserved |", "import Prelude; main := 1 | 2", "expected declaration"},
		{":: as binary op", "import Prelude; main := 1 :: 2", "expected type"},
		{"unbound ++", "import Prelude; main := 1 ++ 2", "unbound operator"},
		{"unbound ===", "import Prelude; main := 1 === 2", "unbound operator"},
		{".. operator", "import Prelude; main := 1 .. 2", "expected expression"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gicel.RunSandbox(tc.src, &gicel.SandboxConfig{
				Packs: []gicel.Pack{gicel.Prelude},
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected %q in error, got: %v", tc.want, err)
			}
		})
	}
}

func TestValidUserOperators(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"<==>", `import Prelude; infixl 4 <==>; (<==>) :: Int -> Int -> Bool; (<==>) := \x y. x == y; main := 1 <==> 1`},
		{"|>", `import Prelude; infixl 1 |>; (|>) :: \a b. a -> (a -> b) -> b; (|>) := \x f. f x; main := 1 |> (\x. x + 1)`},
		{"->+", `import Prelude; infixl 5 ->+; (->+) :: Int -> Int -> Int; (->+) := \x y. x + y; main := 1 ->+ 2`},
		{"<-+", `import Prelude; infixl 5 <-+; (<-+) :: Int -> Int -> Int; (<-+) := \x y. x + y; main := 1 <-+ 2`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gicel.RunSandbox(tc.src, &gicel.SandboxConfig{
				Packs: []gicel.Pack{gicel.Prelude},
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Unterminated literals
// ---------------------------------------------------------------------------

func TestMalformedUnterminatedLiterals(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"unterminated string", `main := "hello`, "unterminated"},
		{"unterminated rune", "main := 'a", "unterminated"},
		{"huge integer", "main := 99999999999999999999999999999999999999999", "invalid integer literal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gicel.RunSandbox(tc.src, &gicel.SandboxConfig{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected %q in error, got: %v", tc.want, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Malformed list syntax
// ---------------------------------------------------------------------------

func TestMalformedListSyntax(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"unclosed list pattern", "import Prelude; f := \\xs. case xs { [x, y -> x }; main := 0", "expected ]"},
		{"trailing comma in pattern", "import Prelude; f := \\xs. case xs { [x,] -> x; _ -> 0 }; main := 0", "expected pattern"},
		{"double comma in literal", "import Prelude; main := [1,,2]", "expected expression"},
		{"semicolon in list", "import Prelude; main := [1; 2; 3]", "expected ]"},
		{"list type mismatch", `import Prelude; f :: List Int -> Int; f := \xs. case xs { ["hello"] -> 0; _ -> 1 }; main := 0`, "type mismatch"},
		{"list pattern on non-list", `import Prelude; f :: Int -> Int; f := \x. case x { [a] -> a; _ -> 0 }; main := 0`, "type mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gicel.RunSandbox(tc.src, &gicel.SandboxConfig{
				Packs: []gicel.Pack{gicel.Prelude},
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected %q in error, got: %v", tc.want, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Semicolon edge cases
// ---------------------------------------------------------------------------

func TestSemicolonEdgeCases(t *testing.T) {
	t.Run("1000 semicolons", func(t *testing.T) {
		src := strings.Repeat(";", 1000)
		_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{})
		// No main, but should not panic — just report no entry point.
		if err == nil {
			t.Fatal("expected error (no main)")
		}
	})

	t.Run("semicolons around decls", func(t *testing.T) {
		src := ";;;import Prelude;;;;main := 1 + 2;;;;"
		result, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
			Packs: []gicel.Pack{gicel.Prelude},
		})
		if err != nil {
			t.Fatal(err)
		}
		n := gicel.MustHost[int64](result.Value)
		if n != 3 {
			t.Errorf("expected 3, got %d", n)
		}
	})
}

// ---------------------------------------------------------------------------
// Garbage / special-character inputs
// ---------------------------------------------------------------------------

func TestMalformedGarbageInputs(t *testing.T) {
	cases := []string{
		"@#$%",
		"\x00\x01\x02",
		"",
		"   ",
		"\n\n\n",
	}
	for i, src := range cases {
		t.Run(fmt.Sprintf("garbage_%d", i), func(t *testing.T) {
			_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{})
			// Should either error or return no-main, never panic.
			_ = err
		})
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
	sb.WriteString(fmt.Sprintf("] -> x0 + x%d; _ -> 0 }\n", 49))
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

	src := fmt.Sprintf("import Prelude\nf := \\xs. case xs { %s -> x; _ -> 0 }\nmain := f %s\n",
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
