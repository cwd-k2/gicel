package gicel_test

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// Prelude expansion tests — TDD: these tests define the expected behavior
// for new Prelude functions. Written before the implementation.
// ---------------------------------------------------------------------------

// runPure compiles a source fragment (with Prelude + Std.Num) and evaluates "main".
func runPure(t *testing.T, source string) gicel.Value {
	t.Helper()
	eng := gicel.NewEngine()
	if err := gicel.Num(eng); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(source)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	return result.Value
}

func assertConVal(t *testing.T, v gicel.Value, name string) {
	t.Helper()
	con, ok := v.(*gicel.ConVal)
	if !ok {
		t.Fatalf("expected ConVal(%s), got %T: %v", name, v, v)
	}
	if con.Con != name {
		t.Fatalf("expected ConVal(%s), got ConVal(%s)", name, con.Con)
	}
}

func assertHostInt(t *testing.T, v gicel.Value, expected int64) {
	t.Helper()
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal(%d), got %T: %v", expected, v, v)
	}
	n, ok := hv.Inner.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T: %v", hv.Inner, hv.Inner)
	}
	if n != expected {
		t.Fatalf("expected %d, got %d", expected, n)
	}
}

// --- id ---

func TestPreludeId(t *testing.T) {
	v := runPure(t, `main := id True`)
	assertConVal(t, v, "True")
}

func TestPreludeIdInt(t *testing.T) {
	v := runPure(t, `
import Std.Num
main := id 42
`)
	assertHostInt(t, v, 42)
}

// --- const ---

func TestPreludeConst(t *testing.T) {
	v := runPure(t, `main := const True False`)
	assertConVal(t, v, "True")
}

// --- flip ---

func TestPreludeFlip(t *testing.T) {
	v := runPure(t, `
import Std.Num
main := flip const 1 2
`)
	assertHostInt(t, v, 2)
}

// --- compose (.) ---

func TestPreludeCompose(t *testing.T) {
	v := runPure(t, `main := (not . not) True`)
	assertConVal(t, v, "True")
}

func TestPreludeComposeInfix(t *testing.T) {
	v := runPure(t, `
f := not . not . not
main := f True
`)
	assertConVal(t, v, "False")
}

// --- not ---

func TestPreludeNot(t *testing.T) {
	v := runPure(t, `main := not True`)
	assertConVal(t, v, "False")
}

func TestPreludeNotFalse(t *testing.T) {
	v := runPure(t, `main := not False`)
	assertConVal(t, v, "True")
}

// --- (&&) ---

func TestPreludeAnd(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{`main := True && True`, "True"},
		{`main := True && False`, "False"},
		{`main := False && True`, "False"},
		{`main := False && False`, "False"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			v := runPure(t, tt.src)
			assertConVal(t, v, tt.want)
		})
	}
}

// --- (||) ---

func TestPreludeOr(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{`main := True || True`, "True"},
		{`main := True || False`, "True"},
		{`main := False || True`, "True"},
		{`main := False || False`, "False"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			v := runPure(t, tt.src)
			assertConVal(t, v, tt.want)
		})
	}
}

// --- maybe ---

func TestPreludeMaybeNothing(t *testing.T) {
	v := runPure(t, `main := maybe False not Nothing`)
	assertConVal(t, v, "False")
}

func TestPreludeMaybeJust(t *testing.T) {
	v := runPure(t, `main := maybe False not (Just True)`)
	assertConVal(t, v, "False")
}

// --- result ---

func TestPreludeResultOk(t *testing.T) {
	v := runPure(t, `main := result (\_ -> False) (\x -> x) (Ok True)`)
	assertConVal(t, v, "True")
}

func TestPreludeResultErr(t *testing.T) {
	v := runPure(t, `main := result (\_ -> False) (\x -> x) (Err ())`)
	assertConVal(t, v, "False")
}

// --- fst / snd ---

func TestPreludeFst(t *testing.T) {
	v := runPure(t, `main := fst (True, False)`)
	assertConVal(t, v, "True")
}

func TestPreludeSnd(t *testing.T) {
	v := runPure(t, `main := snd (True, False)`)
	assertConVal(t, v, "False")
}

// --- head / tail ---

func TestPreludeHead(t *testing.T) {
	v := runPure(t, `main := head (Cons True Nil)`)
	assertConVal(t, v, "Just")
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "True")
}

func TestPreludeHeadNil(t *testing.T) {
	v := runPure(t, `main := head Nil`)
	assertConVal(t, v, "Nothing")
}

func TestPreludeTail(t *testing.T) {
	v := runPure(t, `main := tail (Cons True (Cons False Nil))`)
	assertConVal(t, v, "Just")
}

func TestPreludeTailNil(t *testing.T) {
	v := runPure(t, `main := tail Nil`)
	assertConVal(t, v, "Nothing")
}

// --- null ---

func TestPreludeNull(t *testing.T) {
	v := runPure(t, `main := null Nil`)
	assertConVal(t, v, "True")
}

func TestPreludeNullNonEmpty(t *testing.T) {
	v := runPure(t, `main := null (Cons True Nil)`)
	assertConVal(t, v, "False")
}

// --- map (alias for fmap on List) ---

func TestPreludeMap(t *testing.T) {
	v := runPure(t, `main := map not (Cons True (Cons False Nil))`)
	assertConVal(t, v, "Cons")
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "False") // not True
}

// --- filter ---

func TestPreludeFilter(t *testing.T) {
	// filter not [True, False, True] = [False]... wait, filter keeps elements where predicate is True.
	// filter not [True, False, True] → not True = False (drop), not False = True (keep), not True = False (drop) → [False]
	v := runPure(t, `main := filter not (Cons True (Cons False (Cons True Nil)))`)
	assertConVal(t, v, "Cons")
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "False")
	assertConVal(t, con.Args[1], "Nil")
}

func TestPreludeFilterAllDrop(t *testing.T) {
	v := runPure(t, `
always := \_ -> False
main := filter always (Cons True (Cons False Nil))
`)
	assertConVal(t, v, "Nil")
}

// --- singleton ---

func TestPreludeSingleton(t *testing.T) {
	v := runPure(t, `main := singleton True`)
	assertConVal(t, v, "Cons")
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "True")
	assertConVal(t, con.Args[1], "Nil")
}

// --- Comparison operators ---

func TestPreludeEqOp(t *testing.T) {
	v := runPure(t, `main := True == True`)
	assertConVal(t, v, "True")
}

func TestPreludeNeqOp(t *testing.T) {
	v := runPure(t, `main := True /= False`)
	assertConVal(t, v, "True")
}

func TestPreludeLtOp(t *testing.T) {
	v := runPure(t, "import Std.Num\nmain := 1 < 2")
	assertConVal(t, v, "True")
}

func TestPreludeGtOp(t *testing.T) {
	v := runPure(t, "import Std.Num\nmain := 2 > 1")
	assertConVal(t, v, "True")
}

func TestPreludeLeOp(t *testing.T) {
	v := runPure(t, "import Std.Num\nmain := 1 <= 1")
	assertConVal(t, v, "True")
}

func TestPreludeGeOp(t *testing.T) {
	v := runPure(t, "import Std.Num\nmain := 2 >= 1")
	assertConVal(t, v, "True")
}

func TestPreludeMin(t *testing.T) {
	v := runPure(t, "import Std.Num\nmain := min 3 7")
	assertHostInt(t, v, 3)
}

func TestPreludeMax(t *testing.T) {
	v := runPure(t, "import Std.Num\nmain := max 3 7")
	assertHostInt(t, v, 7)
}

// --- abs / sign in Std.Num ---

func TestNumAbs(t *testing.T) {
	v := runPure(t, "import Std.Num\nmain := abs (negate 5)")
	assertHostInt(t, v, 5)
}

func TestNumAbsPositive(t *testing.T) {
	v := runPure(t, "import Std.Num\nmain := abs 3")
	assertHostInt(t, v, 3)
}

func TestNumSign(t *testing.T) {
	tests := []struct {
		src  string
		want int64
	}{
		{"import Std.Num\nmain := sign 42", 1},
		{"import Std.Num\nmain := sign (negate 3)", -1},
		{"import Std.Num\nmain := sign 0", 0},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			v := runPure(t, tt.src)
			assertHostInt(t, v, tt.want)
		})
	}
}

// --- Operator precedence ---

func TestPreludePrecedence(t *testing.T) {
	// (==) is infix 4, (&&) is infixr 3, (||) is infixr 2
	// True == True && False should parse as (True == True) && False
	v := runPure(t, `main := True == True && False`)
	assertConVal(t, v, "False")
}

func TestPreludeOrAndPrecedence(t *testing.T) {
	// True || False && False should parse as True || (False && False) = True
	v := runPure(t, `main := True || False && False`)
	assertConVal(t, v, "True")
}

func TestPreludeComposePrecedence(t *testing.T) {
	// (.) has infixr 9 which is highest among user-defined operators
	// not . not $ True — but we don't have $, so test with explicit app
	v := runPure(t, `
f := not . not
main := f True
`)
	assertConVal(t, v, "True")
}
