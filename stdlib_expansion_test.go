package gicel_test

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// Stdlib expansion tests — TDD for Prelude (Str, List) and Effect.IO additions.
// ---------------------------------------------------------------------------

// runWithPacks compiles source with given packs and evaluates "main".
func runWithPacks(t *testing.T, source string, packs ...gicel.Pack) gicel.Value {
	t.Helper()
	eng := gicel.NewEngine()
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
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

func assertHostString(t *testing.T, v gicel.Value, expected string) {
	t.Helper()
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal(%q), got %T: %v", expected, v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", hv.Inner, hv.Inner)
	}
	if s != expected {
		t.Fatalf("expected %q, got %q", expected, s)
	}
}

func assertHostBool(t *testing.T, v gicel.Value, expected bool) {
	t.Helper()
	con, ok := v.(*gicel.ConVal)
	if !ok {
		t.Fatalf("expected ConVal(True/False), got %T: %v", v, v)
	}
	if expected && con.Con != "True" {
		t.Fatalf("expected True, got %s", con.Con)
	}
	if !expected && con.Con != "False" {
		t.Fatalf("expected False, got %s", con.Con)
	}
}

// ===========================================================================
// Prelude — Str expansion
// ===========================================================================

func TestStrCharAt(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := charAt 0 \"hello\"", gicel.Prelude)
	// charAt returns Maybe Rune; 'h' = Just 'h'
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
}

func TestStrCharAtOutOfBounds(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := charAt 99 \"hi\"", gicel.Prelude)
	assertConVal(t, v, "Nothing")
}

func TestStrSubstring(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := substring 1 2 \"hello\"", gicel.Prelude)
	assertHostString(t, v, "el")
}

func TestStrToUpper(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := toUpper \"hello\"", gicel.Prelude)
	assertHostString(t, v, "HELLO")
}

func TestStrToLower(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := toLower \"HELLO\"", gicel.Prelude)
	assertHostString(t, v, "hello")
}

func TestStrTrim(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := trim \"  hello  \"", gicel.Prelude)
	assertHostString(t, v, "hello")
}

func TestStrContains(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := contains \"ell\" \"hello\"", gicel.Prelude)
	assertHostBool(t, v, true)
}

func TestStrContainsFalse(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := contains \"xyz\" \"hello\"", gicel.Prelude)
	assertHostBool(t, v, false)
}

func TestStrSplit(t *testing.T) {
	// split "," "a,b,c" => List String: Cons "a" (Cons "b" (Cons "c" Nil))
	v := runWithPacks(t, "import Prelude\nmain := split \",\" \"a,b,c\"", gicel.Prelude)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	assertHostString(t, con.Args[0], "a")
}

func TestStrJoin(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := join \",\" (Cons \"a\" (Cons \"b\" (Cons \"c\" Nil)))", gicel.Prelude)
	assertHostString(t, v, "a,b,c")
}

func TestStrShowInt(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := showInt 42", gicel.Prelude)
	assertHostString(t, v, "42")
}

func TestStrShowBool(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := showBool True", gicel.Prelude)
	assertHostString(t, v, "True")
}

func TestStrReadInt(t *testing.T) {
	// readInt returns Maybe Int
	v := runWithPacks(t, "import Prelude\nmain := readInt \"42\"", gicel.Prelude)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	hv, ok := con.Args[0].(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", con.Args[0])
	}
	if hv.Inner.(int64) != 42 {
		t.Fatalf("expected 42, got %v", hv.Inner)
	}
}

func TestStrReadIntFail(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := readInt \"abc\"", gicel.Prelude)
	assertConVal(t, v, "Nothing")
}

func TestStrFromRunes(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := fromRunes (Cons 'h' (Cons 'i' Nil))", gicel.Prelude)
	assertHostString(t, v, "hi")
}

// ===========================================================================
// Packed type class
// ===========================================================================

func TestPackedStringRoundtrip(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main :: String
main := pack (unpack "hello")
`, gicel.Prelude)
	assertHostString(t, v, "hello")
}

func TestPackedListIdentity(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := pack (Cons True Nil)
`, gicel.Prelude)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	assertConVal(t, con.Args[0], "True")
	assertConVal(t, con.Args[1], "Nil")
}

// ===========================================================================
// Prelude — List expansion
// ===========================================================================

func TestListZip(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := zip (Cons True (Cons False Nil)) (Cons () Nil)
`, gicel.Prelude)
	// zip [True,False] [()] = [(True, ())]
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	pair, ok := con.Args[0].(*gicel.RecordVal)
	if !ok || len(pair.Fields) != 2 {
		t.Fatalf("expected tuple, got %v", con.Args[0])
	}
	assertConVal(t, con.Args[1], "Nil")
}

func TestListUnzip(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := unzip (Cons (True, ()) Nil)
`, gicel.Prelude)
	// unzip [(True, ())] = ([True], [()])
	rv, ok := v.(*gicel.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Fatalf("expected tuple, got %v", v)
	}
}

func TestListTake(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := take 2 (Cons True (Cons False (Cons True Nil)))", gicel.Prelude)
	// take 2 [True,False,True] = [True,False]
	con := v.(*gicel.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	second := con.Args[1].(*gicel.ConVal)
	if second.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", second.Con)
	}
	assertConVal(t, second.Args[1], "Nil")
}

func TestListDrop(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := drop 1 (Cons True (Cons False Nil))", gicel.Prelude)
	// drop 1 [True,False] = [False]
	con := v.(*gicel.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "False")
	assertConVal(t, con.Args[1], "Nil")
}

func TestListReverse(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := reverse (Cons True (Cons False Nil))
`, gicel.Prelude)
	// reverse [True,False] = [False,True]
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "False")
}

func TestListIndex(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := index 1 (Cons True (Cons False Nil))", gicel.Prelude)
	// index 1 [True,False] = Just False
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestListIndexOutOfBounds(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := index 5 (Cons True Nil)", gicel.Prelude)
	assertConVal(t, v, "Nothing")
}

func TestListReplicate(t *testing.T) {
	v := runWithPacks(t, "import Prelude\nmain := replicate 3 True", gicel.Prelude)
	// replicate 3 True = [True, True, True]
	con := v.(*gicel.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestListFoldl(t *testing.T) {
	// foldl (\acc x -> case x { True -> acc + 1; False -> acc }) 0 [True, False, True]
	// = ((0 + 1) + 0) + 1 = 2
	v := runWithPacks(t, `
import Prelude
main := foldl (\acc x. case x { True -> acc + 1; False -> acc }) 0 (Cons True (Cons False (Cons True Nil)))
`, gicel.Prelude)
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	if hv.Inner.(int64) != 2 {
		t.Fatalf("expected 2, got %v", hv.Inner)
	}
}

// ===========================================================================
// Data.Slice
// ===========================================================================

func TestSliceSingletonLength(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Slice
main := length (pack (Cons True Nil) :: Slice Bool)
`, gicel.Prelude, gicel.DataSlice)
	assertHostInt(t, v, 1)
}

func TestSliceIndex(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Slice
main :: Maybe Bool
main := index 0 (pack (Cons True Nil))
`, gicel.Prelude, gicel.DataSlice)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestSlicePackedRoundtrip(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Slice
main := unpack (pack (Cons True (Cons False Nil)) :: Slice Bool)
`, gicel.Prelude, gicel.DataSlice)
	con := v.(*gicel.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
	second := con.Args[1].(*gicel.ConVal)
	assertConVal(t, second.Args[0], "False")
	assertConVal(t, second.Args[1], "Nil")
}

func TestSliceFmap(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Slice
main := length (fmap not (pack (Cons True Nil) :: Slice Bool))
`, gicel.Prelude, gicel.DataSlice)
	assertHostInt(t, v, 1)
}

func TestSliceMonoid(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Slice
s1 := pack (Cons True Nil) :: Slice Bool
s2 := pack (Cons False Nil) :: Slice Bool
main := length (append s1 s2)
`, gicel.Prelude, gicel.DataSlice)
	assertHostInt(t, v, 2)
}

// ===========================================================================
// Data.Stream
// ===========================================================================

func TestStreamHead(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Stream
main := headS (LCons True (\_. LNil))
`, gicel.Prelude, gicel.DataStream)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestStreamHeadSEmpty(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Stream
main :: Maybe Bool
main := headS LNil
`, gicel.Prelude, gicel.DataStream)
	assertConVal(t, v, "Nothing")
}

func TestStreamToListFromList(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Stream
main := toList (fromList (Cons True (Cons False Nil)) :: Stream Bool)
`, gicel.Prelude, gicel.DataStream)
	con := v.(*gicel.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
	second := con.Args[1].(*gicel.ConVal)
	assertConVal(t, second.Args[0], "False")
	assertConVal(t, second.Args[1], "Nil")
}

func TestStreamTake(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Stream
main := take 2 (LCons True (\_. LCons False (\_. LCons True (\_. LNil))))
`, gicel.Prelude, gicel.DataStream)
	con := v.(*gicel.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
	second := con.Args[1].(*gicel.ConVal)
	assertConVal(t, second.Args[0], "False")
	assertConVal(t, second.Args[1], "Nil")
}

func TestStreamFmap(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Stream
main := headS (fmap not (LCons True (\_. LNil)))
`, gicel.Prelude, gicel.DataStream)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestStreamDrop(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Stream
main := headS (drop 1 (LCons True (\_. LCons False (\_. LNil))))
`, gicel.Prelude, gicel.DataStream)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "False")
}

// ===========================================================================
// Fusion integration (RegisterRewriteRule pipeline)
// ===========================================================================

func TestSliceFusionMapMapIntegration(t *testing.T) {
	// Verify that the production sliceMapMapFusion rule fires through the engine pipeline.
	// Two consecutive fmap calls should be fused into one _sliceMap with a composed function.
	v := runWithPacks(t, `
import Prelude
import Data.Slice
main := length (fmap (\x. x + 1) (fmap (\x. x + 2) (pack (Cons 0 Nil) :: Slice Int)))
`, gicel.Prelude, gicel.DataSlice)
	assertHostInt(t, v, 1)
}

func TestStringPackedRoundtripIntegration(t *testing.T) {
	// Verify production strPackedRoundtrip fires: fromRunes (toRunes x) → x
	v := runWithPacks(t, `
import Prelude
main := fromRunes (toRunes "abc")
`, gicel.Prelude)
	assertHostString(t, v, "abc")
}

// ===========================================================================
// Effect.IO
// ===========================================================================

func TestIOPrint(t *testing.T) {
	eng := gicel.NewEngine()
	if err := gicel.Prelude(eng); err != nil {
		t.Fatal(err)
	}
	if err := gicel.EffectIO(eng); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import Effect.IO
main := do { print "hello" }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Fatalf("expected (), got %T: %v", result.Value, result.Value)
	}
	// Check that io buffer captured the output.
	buf, ok := result.CapEnv.Get("io")
	if !ok {
		t.Fatal("expected io capability in result")
	}
	msgs, ok := buf.([]string)
	if !ok {
		t.Fatalf("expected []string io buffer, got %T", buf)
	}
	if len(msgs) != 1 || msgs[0] != "hello" {
		t.Fatalf("expected [\"hello\"], got %v", msgs)
	}
}

// ===========================================================================
// Prelude — List expansion — new primitives and GICEL functions
// ===========================================================================

func TestListDropWhile(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := dropWhile not (Cons False (Cons False (Cons True (Cons False Nil))))
`, gicel.Prelude)
	// dropWhile not [F,F,T,F] — not F = True (drop), not F = True (drop), not T = False (stop)
	// result = [T,F]
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "True")
	con2 := con.Args[1].(*gicel.ConVal)
	assertConVal(t, con2.Args[0], "False")
	assertConVal(t, con2.Args[1], "Nil")
}

func TestListSpan(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := fst (span not (Cons False (Cons True Nil)))
`, gicel.Prelude)
	// span not [F,T] — not F = True (take), not T = False (stop)
	// fst = [F]
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "False")
	assertConVal(t, con.Args[1], "Nil")
}

func TestListBreak(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := fst (break not (Cons True (Cons False Nil)))
`, gicel.Prelude)
	// break not [T,F] = span (not . not) [T,F] = span id [T,F]
	// id T = True (take), id F = False (stop) → fst = [T]
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "True")
	assertConVal(t, con.Args[1], "Nil")
}

func TestListSortBy(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := sortBy compare (Cons 3 (Cons 1 (Cons 2 Nil)))
`, gicel.Prelude)
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 1 {
		t.Fatalf("expected 1 first, got %v", hv.Inner)
	}
}

func TestListSort(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := sort (Cons 3 (Cons 1 (Cons 2 Nil)))
`, gicel.Prelude)
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 1 {
		t.Fatalf("expected 1 first, got %v", hv.Inner)
	}
	con2 := con.Args[1].(*gicel.ConVal)
	hv2 := con2.Args[0].(*gicel.HostVal)
	if hv2.Inner.(int64) != 2 {
		t.Fatalf("expected 2 second, got %v", hv2.Inner)
	}
	con3 := con2.Args[1].(*gicel.ConVal)
	hv3 := con3.Args[0].(*gicel.HostVal)
	if hv3.Inner.(int64) != 3 {
		t.Fatalf("expected 3 third, got %v", hv3.Inner)
	}
}

func TestListSortEmpty(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := sort Nil :: List Int
`, gicel.Prelude)
	assertConVal(t, v, "Nil")
}

func TestListScanl(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := scanl (\x y. x + y) 0 (Cons 1 (Cons 2 (Cons 3 Nil)))
`, gicel.Prelude)
	// scanl (+) 0 [1,2,3] = [0,1,3,6]
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 0 {
		t.Fatalf("expected 0 first, got %v", hv.Inner)
	}
}

func TestListUnfoldr(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := unfoldr (\n. case n == 0 { True -> Nothing; False -> Just (n, n - 1) }) 3
`, gicel.Prelude)
	// unfoldr from 3: Just (3,2), Just (2,1), Just (1,0), Nothing → [3,2,1]
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 3 {
		t.Fatalf("expected 3 first, got %v", hv.Inner)
	}
}

func TestListIterateN(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := iterateN 4 (\x. x * 2) 1
`, gicel.Prelude)
	// iterateN 4 (*2) 1 = [1,2,4,8]
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 1 {
		t.Fatalf("expected 1 first, got %v", hv.Inner)
	}
}

func TestListZipWith(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := zipWith (\x y. x + y) (Cons 1 (Cons 2 Nil)) (Cons 10 (Cons 20 Nil))
`, gicel.Prelude)
	// zipWith (+) [1,2] [10,20] = [11,22]
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 11 {
		t.Fatalf("expected 11 first, got %v", hv.Inner)
	}
}

func TestListIntercalate(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := intercalate (Cons 0 Nil) (Cons (Cons 1 Nil) (Cons (Cons 2 Nil) (Cons (Cons 3 Nil) Nil)))
`, gicel.Prelude)
	// intercalate [0] [[1],[2],[3]] = [1,0,2,0,3]
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 1 {
		t.Fatalf("expected 1 first, got %v", hv.Inner)
	}
}

func TestListNubBy(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
main := nubBy eq (Cons 1 (Cons 2 (Cons 1 (Cons 3 Nil))))
`, gicel.Prelude)
	// nubBy eq [1,2,1,3] = [1,2,3] (preserves first occurrences via foldl)
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 1 {
		t.Fatalf("expected 1 first, got %v", hv.Inner)
	}
}

func TestIODebug(t *testing.T) {
	eng := gicel.NewEngine()
	if err := gicel.Prelude(eng); err != nil {
		t.Fatal(err)
	}
	if err := gicel.EffectIO(eng); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import Effect.IO
main := do { debug 42 }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv2, ok2 := result.Value.(*gicel.RecordVal)
	if !ok2 || len(rv2.Fields) != 0 {
		t.Fatalf("expected (), got %T: %v", result.Value, result.Value)
	}
	buf, ok := result.CapEnv.Get("io")
	if !ok {
		t.Fatal("expected io capability")
	}
	msgs := buf.([]string)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

// ===========================================================================
// Data.Map integration
// ===========================================================================

func TestMapInsertLookupIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Map
main := mapLookup 1 (insert 1 True (mapEmpty :: Map Int Bool))
`, gicel.Prelude, gicel.DataMap)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestMapLookupMissingIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Map
main := mapLookup 99 (insert 1 True (mapEmpty :: Map Int Bool))
`, gicel.Prelude, gicel.DataMap)
	assertConVal(t, v, "Nothing")
}

func TestMapSizeIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Map
main := size (insert 2 False (insert 1 True (mapEmpty :: Map Int Bool)))
`, gicel.Prelude, gicel.DataMap)
	assertHostInt(t, v, 2)
}

func TestMapMemberIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Map
main := member 1 (insert 1 True (mapEmpty :: Map Int Bool))
`, gicel.Prelude, gicel.DataMap)
	assertConVal(t, v, "True")
}

func TestMapDeleteIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Map
main := size (delete 1 (insert 1 True (mapEmpty :: Map Int Bool)))
`, gicel.Prelude, gicel.DataMap)
	assertHostInt(t, v, 0)
}

func TestMapFromListToListIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Map
main := size (fromList (Cons (1, True) (Cons (2, False) Nil)) :: Map Int Bool)
`, gicel.Prelude, gicel.DataMap)
	assertHostInt(t, v, 2)
}

// ===========================================================================
// Data.Set integration
// ===========================================================================

func TestSetInsertMemberIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Set
main := member 1 (insert 1 (setEmpty :: Set Int))
`, gicel.Prelude, gicel.DataSet)
	assertConVal(t, v, "True")
}

func TestSetMemberMissingIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Set
main := member 99 (insert 1 (setEmpty :: Set Int))
`, gicel.Prelude, gicel.DataSet)
	assertConVal(t, v, "False")
}

func TestSetSizeIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Set
main := size (insert 2 (insert 1 (setEmpty :: Set Int)))
`, gicel.Prelude, gicel.DataSet)
	assertHostInt(t, v, 2)
}

func TestSetDeleteIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Set
main := size (delete 1 (insert 2 (insert 1 (setEmpty :: Set Int))))
`, gicel.Prelude, gicel.DataSet)
	assertHostInt(t, v, 1)
}

func TestSetFromListIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Set
main := size (fromList (Cons 1 (Cons 2 (Cons 1 Nil))) :: Set Int)
`, gicel.Prelude, gicel.DataSet)
	assertHostInt(t, v, 2) // duplicates removed
}

func TestSetToListIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Prelude
import Data.Set
main := toList (insert 2 (insert 1 (setEmpty :: Set Int)))
`, gicel.Prelude, gicel.DataSet)
	// Should be sorted: [1, 2]
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	assertHostInt(t, con.Args[0], 1)
}
