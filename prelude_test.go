package gicel_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// Prelude expansion tests — TDD: these tests define the expected behavior
// for new Prelude functions. Written before the implementation.
// ---------------------------------------------------------------------------

// runPure compiles a source fragment (with Prelude) and evaluates "main".
func runPure(t *testing.T, source string) gicel.Value {
	t.Helper()
	eng := gicel.NewEngine()
	if err := gicel.Prelude(eng); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "import Prelude") {
		source = "import Prelude\n" + source
	}
	rt, err := eng.NewRuntime(source)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
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
import Prelude
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
import Prelude
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
	v := runPure(t, `main := result (\_. False) (\x. x) (Ok True)`)
	assertConVal(t, v, "True")
}

func TestPreludeResultErr(t *testing.T) {
	v := runPure(t, `main := result (\_. False) (\x. x) (Err ())`)
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
always := \_. False
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
	v := runPure(t, "import Prelude\nmain := 1 < 2")
	assertConVal(t, v, "True")
}

func TestPreludeGtOp(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := 2 > 1")
	assertConVal(t, v, "True")
}

func TestPreludeLeOp(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := 1 <= 1")
	assertConVal(t, v, "True")
}

func TestPreludeGeOp(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := 2 >= 1")
	assertConVal(t, v, "True")
}

func TestPreludeMin(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := min 3 7")
	assertHostInt(t, v, 3)
}

func TestPreludeMax(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := max 3 7")
	assertHostInt(t, v, 7)
}

// --- abs / sign in Prelude (Num) ---

func TestNumAbs(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := abs (negate 5)")
	assertHostInt(t, v, 5)
}

func TestNumAbsPositive(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := abs 3")
	assertHostInt(t, v, 3)
}

func TestNumSign(t *testing.T) {
	tests := []struct {
		src  string
		want int64
	}{
		{"import Prelude\nmain := sign 42", 1},
		{"import Prelude\nmain := sign (negate 3)", -1},
		{"import Prelude\nmain := sign 0", 0},
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
	v := runPure(t, `
f := not . not
main := f True
`)
	assertConVal(t, v, "True")
}

// ===========================================================================
// Group 0: Missing Instances
// ===========================================================================

func TestApplicativeList(t *testing.T) {
	// wrap 42 = [42]
	v := runPure(t, "import Prelude\nmain := wrap 42 :: List Int")
	con := v.(*gicel.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	assertHostInt(t, con.Args[0], 42)
	assertConVal(t, con.Args[1], "Nil")
}

func TestApplicativeListAp(t *testing.T) {
	// ap [not] [True, False] = [False, True]
	v := runPure(t, `main := ap (Cons not Nil) (Cons True (Cons False Nil))`)
	con := v.(*gicel.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "False")
	con2 := con.Args[1].(*gicel.ConVal)
	assertConVal(t, con2.Args[0], "True")
	assertConVal(t, con2.Args[1], "Nil")
}

func TestEqResultOk(t *testing.T) {
	v := runPure(t, `main := eq (Ok True) (Ok True)`)
	assertConVal(t, v, "True")
}

func TestEqResultErr(t *testing.T) {
	v := runPure(t, `main := eq (Err False) (Err True)`)
	assertConVal(t, v, "False")
}

func TestEqResultMixed(t *testing.T) {
	v := runPure(t, `main := eq (Ok True) (Err False)`)
	assertConVal(t, v, "False")
}

func TestOrdResultErrLtOk(t *testing.T) {
	v := runPure(t, `main := compare (Err True) (Ok True)`)
	assertConVal(t, v, "LT")
}

func TestFunctorResult(t *testing.T) {
	v := runPure(t, `main := fmap not (Ok True)`)
	con := v.(*gicel.ConVal)
	if con.Con != "Ok" {
		t.Fatalf("expected Ok, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestFunctorResultErr(t *testing.T) {
	v := runPure(t, `main := fmap not (Err ())`)
	assertConVal(t, v, "Err")
}

func TestFoldableResult(t *testing.T) {
	v := runPure(t, `main := foldr (\x _. Just x) Nothing (Ok True)`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestFoldableResultErr(t *testing.T) {
	v := runPure(t, `main := foldr (\x _. Just x) Nothing (Err ())`)
	assertConVal(t, v, "Nothing")
}

func TestOrdListLex(t *testing.T) {
	// [1,2] < [1,3]
	v := runPure(t, "import Prelude\nmain := compare (Cons 1 (Cons 2 Nil)) (Cons 1 (Cons 3 Nil))")
	assertConVal(t, v, "LT")
}

func TestOrdListPrefix(t *testing.T) {
	// [1] < [1,2]
	v := runPure(t, "import Prelude\nmain := compare (Cons 1 Nil) (Cons 1 (Cons 2 Nil))")
	assertConVal(t, v, "LT")
}

// ===========================================================================
// Group 1: $ and &
// ===========================================================================

func TestDollarOp(t *testing.T) {
	v := runPure(t, `main := not $ True`)
	assertConVal(t, v, "False")
}

func TestDollarPrecedence(t *testing.T) {
	// not $ not $ True = not (not True) = True
	v := runPure(t, `main := not $ not $ True`)
	assertConVal(t, v, "True")
}

func TestPipeOp(t *testing.T) {
	v := runPure(t, `main := True & not`)
	assertConVal(t, v, "False")
}

func TestPipeChain(t *testing.T) {
	v := runPure(t, `main := True & not & not`)
	assertConVal(t, v, "True")
}

// ===========================================================================
// Group 2: Functor / Applicative / Monad operators
// ===========================================================================

func TestFmapOp(t *testing.T) {
	v := runPure(t, `main := not <$> Just True`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestFlippedFmapOp(t *testing.T) {
	v := runPure(t, `main := Just True <&> not`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestApOp(t *testing.T) {
	v := runPure(t, `main := Just not <*> Just True`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestSequenceRight(t *testing.T) {
	v := runPure(t, `main := Just True *> Just False`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestSequenceLeft(t *testing.T) {
	v := runPure(t, `main := Just True <* Just False`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestReverseBind(t *testing.T) {
	v := runPure(t, `main := (\x. Just (not x)) =<< Just True`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestKleisliLR(t *testing.T) {
	v := runPure(t, `
f := (\x. Just (not x)) >=> (\y. Just (not y))
main := f True
`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
}

// ===========================================================================
// Group 3: Utility Functions
// ===========================================================================

func TestOn(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := on (\\x y. x + y) (\\_. 1) True False")
	assertHostInt(t, v, 2)
}

func TestComparing(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := comparing fst (1, True) (2, False)")
	assertConVal(t, v, "LT")
}

func TestEquating(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := equating fst (1, True) (1, False)")
	assertConVal(t, v, "True")
}

func TestIsJust(t *testing.T) {
	v := runPure(t, `main := isJust (Just True)`)
	assertConVal(t, v, "True")
}

func TestIsNothing(t *testing.T) {
	v := runPure(t, `main := isNothing Nothing`)
	assertConVal(t, v, "True")
}

func TestIsOk(t *testing.T) {
	v := runPure(t, `main := isOk (Ok True)`)
	assertConVal(t, v, "True")
}

func TestIsErr(t *testing.T) {
	v := runPure(t, `main := isErr (Err ())`)
	assertConVal(t, v, "True")
}

func TestFromOk(t *testing.T) {
	v := runPure(t, `main := fromOk False (Ok True)`)
	assertConVal(t, v, "True")
}

func TestFromOkDefault(t *testing.T) {
	v := runPure(t, `main := fromOk False (Err ())`)
	assertConVal(t, v, "False")
}

func TestFromErr(t *testing.T) {
	v := runPure(t, `main := fromErr False (Err True)`)
	assertConVal(t, v, "True")
}

func TestBool(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := bool 0 1 True")
	assertHostInt(t, v, 1)
}

func TestBoolFalse(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := bool 0 1 False")
	assertHostInt(t, v, 0)
}

func TestSwap(t *testing.T) {
	v := runPure(t, `main := fst (swap (True, False))`)
	assertConVal(t, v, "False")
}

func TestCurry(t *testing.T) {
	v := runPure(t, `main := curry fst True False`)
	assertConVal(t, v, "True")
}

func TestUncurry(t *testing.T) {
	v := runPure(t, `main := uncurry const (True, False)`)
	assertConVal(t, v, "True")
}

func TestMjoin(t *testing.T) {
	v := runPure(t, `main := mjoin (Just (Just True))`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestMjoinNothing(t *testing.T) {
	v := runPure(t, `main := mjoin Nothing :: Maybe Bool`)
	assertConVal(t, v, "Nothing")
}

func TestVoid(t *testing.T) {
	v := runPure(t, `main := void (Just True)`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
}

func TestLiftA2(t *testing.T) {
	v := runPure(t, `main := liftA2 const (Just True) (Just False)`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestFoldMap(t *testing.T) {
	// foldMap singleton [1,2,3] = [1,2,3] (Monoid for List is append)
	v := runPure(t, "import Prelude\nmain := foldMap singleton (Cons 1 (Cons 2 Nil))")
	con := v.(*gicel.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	assertHostInt(t, con.Args[0], 1)
}

func TestToList(t *testing.T) {
	// toList (Just True) = [True] via Foldable Maybe
	v := runPure(t, `main := toList (Just True)`)
	con := v.(*gicel.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
	assertConVal(t, con.Args[1], "Nil")
}

func TestToListNothing(t *testing.T) {
	v := runPure(t, `main := toList Nothing :: List Bool`)
	assertConVal(t, v, "Nil")
}

// ===========================================================================
// Group 4: List Functions via foldr
// ===========================================================================

func TestFind(t *testing.T) {
	v := runPure(t, `main := find not (Cons True (Cons False Nil))`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestFindNone(t *testing.T) {
	v := runPure(t, `main := find not (Cons True Nil)`)
	assertConVal(t, v, "Nothing")
}

func TestElem(t *testing.T) {
	v := runPure(t, `main := elem True (Cons False (Cons True Nil))`)
	assertConVal(t, v, "True")
}

func TestElemMissing(t *testing.T) {
	v := runPure(t, `main := elem True (Cons False Nil)`)
	assertConVal(t, v, "False")
}

func TestNotElem(t *testing.T) {
	v := runPure(t, `main := notElem True (Cons False Nil)`)
	assertConVal(t, v, "True")
}

func TestAny(t *testing.T) {
	v := runPure(t, `main := any not (Cons True (Cons False Nil))`)
	assertConVal(t, v, "True")
}

func TestAll(t *testing.T) {
	v := runPure(t, `main := all not (Cons False Nil)`)
	assertConVal(t, v, "True")
}

func TestAllFalse(t *testing.T) {
	v := runPure(t, `main := all not (Cons True Nil)`)
	assertConVal(t, v, "False")
}

func TestAnd(t *testing.T) {
	v := runPure(t, `main := and (Cons True (Cons True Nil))`)
	assertConVal(t, v, "True")
}

func TestAndFalse(t *testing.T) {
	v := runPure(t, `main := and (Cons True (Cons False Nil))`)
	assertConVal(t, v, "False")
}

func TestOr(t *testing.T) {
	v := runPure(t, `main := or (Cons False (Cons True Nil))`)
	assertConVal(t, v, "True")
}

func TestOrFalse(t *testing.T) {
	v := runPure(t, `main := or (Cons False Nil)`)
	assertConVal(t, v, "False")
}

func TestLookup(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := lookup 2 (Cons (1, True) (Cons (2, False) Nil))")
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestLookupMissing(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := lookup 3 (Cons (1, True) Nil)")
	assertConVal(t, v, "Nothing")
}

func TestConcatMap(t *testing.T) {
	// concatMap (\x. Cons x (Cons x Nil)) [True] = [True, True]
	v := runPure(t, `main := concatMap (\x. Cons x (Cons x Nil)) (Cons True Nil)`)
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "True")
	con2 := con.Args[1].(*gicel.ConVal)
	assertConVal(t, con2.Args[0], "True")
	assertConVal(t, con2.Args[1], "Nil")
}

func TestFlatten(t *testing.T) {
	// flatten [[1],[2,3]] = [1,2,3]
	v := runPure(t, "import Prelude\nmain := flatten (Cons (Cons 1 Nil) (Cons (Cons 2 (Cons 3 Nil)) Nil))")
	items := listToSliceTest(t, v)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestCatMaybes(t *testing.T) {
	v := runPure(t, `main := catMaybes (Cons (Just True) (Cons Nothing (Cons (Just False) Nil)))`)
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "True")
	con2 := con.Args[1].(*gicel.ConVal)
	assertConVal(t, con2.Args[0], "False")
	assertConVal(t, con2.Args[1], "Nil")
}

func TestMapMaybe(t *testing.T) {
	// mapMaybe (\x. case x { True -> Just False; False -> Nothing }) [True, False, True]
	v := runPure(t, `main := mapMaybe (\x. case x { True -> Just False; False -> Nothing }) (Cons True (Cons False (Cons True Nil)))`)
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "False")
	con2 := con.Args[1].(*gicel.ConVal)
	assertConVal(t, con2.Args[0], "False")
	assertConVal(t, con2.Args[1], "Nil")
}

func TestPartition(t *testing.T) {
	v := runPure(t, `main := fst (partition not (Cons True (Cons False (Cons True Nil))))`)
	// partition not [T,F,T] → falses=[T,T], trues=[F]... wait, not True = False, not False = True
	// partition predicate splits: True-branch (predicate holds) = [False], False-branch = [True, True]
	// fst = True-branch = [False]
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "False")
	assertConVal(t, con.Args[1], "Nil")
}

func TestTakeWhile(t *testing.T) {
	v := runPure(t, `main := takeWhile not (Cons False (Cons False (Cons True Nil)))`)
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "False")
	con2 := con.Args[1].(*gicel.ConVal)
	assertConVal(t, con2.Args[0], "False")
	assertConVal(t, con2.Args[1], "Nil")
}

func TestIntersperse(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := intersperse 0 (Cons 1 (Cons 2 (Cons 3 Nil)))")
	// [1, 0, 2, 0, 3]
	items := listToSliceTest(t, v)
	if len(items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(items))
	}
	assertHostInt(t, items[0], 1)
	assertHostInt(t, items[1], 0)
	assertHostInt(t, items[2], 2)
	assertHostInt(t, items[3], 0)
	assertHostInt(t, items[4], 3)
}

func TestIntersperseEmpty(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := intersperse 0 Nil :: List Int")
	assertConVal(t, v, "Nil")
}

func TestIntersperseSingleton(t *testing.T) {
	v := runPure(t, "import Prelude\nmain := intersperse 0 (Cons 1 Nil)")
	items := listToSliceTest(t, v)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestNub(t *testing.T) {
	v := runPure(t, `main := nub (Cons True (Cons False (Cons True Nil)))`)
	items := listToSliceTest(t, v)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

// --- operator section tests ---

func TestOperatorSectionFoldr(t *testing.T) {
	v := runPure(t, `import Prelude
main := foldr (+) 0 (Cons 1 (Cons 2 (Cons 3 Nil)))`)
	assertHostInt(t, v, 6)
}

func TestOperatorSectionCompose(t *testing.T) {
	v := runPure(t, `main := (.) not not True`)
	assertConVal(t, v, "True")
}

// --- Result + class constraint tests (instance dedup) ---

func TestFunctorResultWithConstraint(t *testing.T) {
	// This failed before: fmap with a constrained function over (Result e)
	// produced a Closure due to instance duplication from transitive imports.
	v := runPure(t, `import Prelude
main := fmap (\x. x + 1) (Ok 5)`)
	con := v.(*gicel.ConVal)
	if con.Con != "Ok" {
		t.Fatalf("expected Ok, got %s", con.Con)
	}
	assertHostInt(t, con.Args[0], 6)
}

func TestFoldableResultWithConstraint(t *testing.T) {
	v := runPure(t, `import Prelude
main := foldr (+) 0 (Ok 10)`)
	assertHostInt(t, v, 10)
}

// ===========================================================================
// Show typeclass
// ===========================================================================

func TestShowBoolTrue(t *testing.T) {
	v := runPure(t, `main := show True`)
	assertHostString(t, v, "True")
}

func TestShowBoolFalse(t *testing.T) {
	v := runPure(t, `main := show False`)
	assertHostString(t, v, "False")
}

func TestShowUnit(t *testing.T) {
	v := runPure(t, `main := show ()`)
	assertHostString(t, v, "()")
}

func TestShowOrdering(t *testing.T) {
	v := runPure(t, `main := show LT`)
	assertHostString(t, v, "LT")
}

func TestShowInt(t *testing.T) {
	v := runPure(t, `import Prelude
main := show 42`)
	assertHostString(t, v, "42")
}

// ===========================================================================
// Alternative typeclass
// ===========================================================================

func TestAlternativeMaybeNone(t *testing.T) {
	v := runPure(t, `main := (none :: Maybe Bool)`)
	assertConVal(t, v, "Nothing")
}

func TestAlternativeMaybeAltJust(t *testing.T) {
	v := runPure(t, `main := alt (Just True) (Just False)`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestAlternativeMaybeAltNothing(t *testing.T) {
	v := runPure(t, `main := alt Nothing (Just False)`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestAlternativeListNone(t *testing.T) {
	v := runPure(t, `main := (none :: List Bool)`)
	assertConVal(t, v, "Nil")
}

func TestAlternativeOperator(t *testing.T) {
	v := runPure(t, `main := Nothing <|> Just True`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
}

func TestGuardTrue(t *testing.T) {
	v := runPure(t, `main := guard True :: Maybe ()`)
	con := v.(*gicel.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
}

func TestGuardFalse(t *testing.T) {
	v := runPure(t, `main := guard False :: Maybe ()`)
	assertConVal(t, v, "Nothing")
}

// --- helpers ---

func listToSliceTest(t *testing.T, v gicel.Value) []gicel.Value {
	t.Helper()
	var result []gicel.Value
	for {
		con, ok := v.(*gicel.ConVal)
		if !ok {
			t.Fatalf("expected ConVal, got %T", v)
		}
		if con.Con == "Nil" {
			return result
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			t.Fatalf("malformed list node: %s", con.Con)
		}
		result = append(result, con.Args[0])
		v = con.Args[1]
	}
}
