package gicel_test

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// Stdlib expansion tests — TDD for Std.Str, Std.List, and Std.IO additions.
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
// Std.Str expansion
// ===========================================================================

func TestStrCharAt(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.Str\nmain := charAt 0 \"hello\"", gicel.Num, gicel.Str)
	// charAt returns Maybe Rune; 'h' = Just 'h'
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
}

func TestStrCharAtOutOfBounds(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.Str\nmain := charAt 99 \"hi\"", gicel.Num, gicel.Str)
	assertConVal(t, v, "Nothing")
}

func TestStrSubstring(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.Str\nmain := substring 1 2 \"hello\"", gicel.Num, gicel.Str)
	assertHostString(t, v, "el")
}

func TestStrToUpper(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := toUpper \"hello\"", gicel.Str)
	assertHostString(t, v, "HELLO")
}

func TestStrToLower(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := toLower \"HELLO\"", gicel.Str)
	assertHostString(t, v, "hello")
}

func TestStrTrim(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := trim \"  hello  \"", gicel.Str)
	assertHostString(t, v, "hello")
}

func TestStrContains(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := contains \"ell\" \"hello\"", gicel.Str)
	assertHostBool(t, v, true)
}

func TestStrContainsFalse(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := contains \"xyz\" \"hello\"", gicel.Str)
	assertHostBool(t, v, false)
}

func TestStrSplit(t *testing.T) {
	// split "," "a,b,c" => List String: Cons "a" (Cons "b" (Cons "c" Nil))
	v := runWithPacks(t, "import Std.Str\nmain := split \",\" \"a,b,c\"", gicel.Str)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	assertHostString(t, con.Args[0], "a")
}

func TestStrJoin(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := join \",\" (Cons \"a\" (Cons \"b\" (Cons \"c\" Nil)))", gicel.Str)
	assertHostString(t, v, "a,b,c")
}

func TestStrShowInt(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.Str\nmain := showInt 42", gicel.Num, gicel.Str)
	assertHostString(t, v, "42")
}

func TestStrShowBool(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := showBool True", gicel.Str)
	assertHostString(t, v, "True")
}

func TestStrReadInt(t *testing.T) {
	// readInt returns Maybe Int
	v := runWithPacks(t, "import Std.Num\nimport Std.Str\nmain := readInt \"42\"", gicel.Num, gicel.Str)
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
	v := runWithPacks(t, "import Std.Str\nmain := readInt \"abc\"", gicel.Str)
	assertConVal(t, v, "Nothing")
}

func TestStrFromRunes(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := fromRunes (Cons 'h' (Cons 'i' Nil))", gicel.Str)
	assertHostString(t, v, "hi")
}

// ===========================================================================
// Packed type class
// ===========================================================================

func TestPackedStringRoundtrip(t *testing.T) {
	v := runWithPacks(t, `
import Std.Str
main :: String
main := pack (unpack "hello")
`, gicel.Str)
	assertHostString(t, v, "hello")
}

func TestPackedListIdentity(t *testing.T) {
	v := runWithPacks(t, `
main := pack (Cons True Nil)
`)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	assertConVal(t, con.Args[0], "True")
	assertConVal(t, con.Args[1], "Nil")
}

// ===========================================================================
// Std.List expansion
// ===========================================================================

func TestListZip(t *testing.T) {
	v := runWithPacks(t, `
import Std.List
main := zip (Cons True (Cons False Nil)) (Cons () Nil)
`, gicel.List)
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
import Std.List
main := unzip (Cons (True, ()) Nil)
`, gicel.List)
	// unzip [(True, ())] = ([True], [()])
	rv, ok := v.(*gicel.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Fatalf("expected tuple, got %v", v)
	}
}

func TestListTake(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.List\nmain := take 2 (Cons True (Cons False (Cons True Nil)))", gicel.Num, gicel.List)
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
	v := runWithPacks(t, "import Std.Num\nimport Std.List\nmain := drop 1 (Cons True (Cons False Nil))", gicel.Num, gicel.List)
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
import Std.List
main := reverse (Cons True (Cons False Nil))
`, gicel.List)
	// reverse [True,False] = [False,True]
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "False")
}

func TestListIndex(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.List\nmain := index 1 (Cons True (Cons False Nil))", gicel.Num, gicel.List)
	// index 1 [True,False] = Just False
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestListIndexOutOfBounds(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.List\nmain := index 5 (Cons True Nil)", gicel.Num, gicel.List)
	assertConVal(t, v, "Nothing")
}

func TestListReplicate(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.List\nmain := replicate 3 True", gicel.Num, gicel.List)
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
import Std.Num
import Std.List
main := foldl (\acc x. case x { True -> acc + 1; False -> acc }) 0 (Cons True (Cons False (Cons True Nil)))
`, gicel.Num, gicel.List)
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	if hv.Inner.(int64) != 2 {
		t.Fatalf("expected 2, got %v", hv.Inner)
	}
}

// ===========================================================================
// Std.Slice
// ===========================================================================

func TestSliceSingletonLength(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Slice
main := sliceLength (sliceSingleton True)
`, gicel.Num, gicel.Slice)
	assertHostInt(t, v, 1)
}

func TestSliceIndex(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Slice
main :: Maybe Bool
main := sliceIndex 0 (pack (Cons True Nil))
`, gicel.Num, gicel.Slice)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestSlicePackedRoundtrip(t *testing.T) {
	v := runWithPacks(t, `
import Std.Slice
main := unpack (pack (Cons True (Cons False Nil)) :: Slice Bool)
`, gicel.Slice)
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
import Std.Num
import Std.Slice
main := sliceLength (fmap not (sliceSingleton True))
`, gicel.Num, gicel.Slice)
	assertHostInt(t, v, 1)
}

func TestSliceMonoid(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Slice
main := sliceLength (append (sliceSingleton True) (sliceSingleton False))
`, gicel.Num, gicel.Slice)
	assertHostInt(t, v, 2)
}

// ===========================================================================
// Std.Stream
// ===========================================================================

func TestStreamHeadS(t *testing.T) {
	v := runWithPacks(t, `
import Std.Stream
main := headS (LCons True (\_. LNil))
`, gicel.Stream)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestStreamHeadSEmpty(t *testing.T) {
	v := runWithPacks(t, `
import Std.Stream
main :: Maybe Bool
main := headS LNil
`, gicel.Stream)
	assertConVal(t, v, "Nothing")
}

func TestStreamToListFromList(t *testing.T) {
	v := runWithPacks(t, `
import Std.Stream
main := toList (fromList (Cons True (Cons False Nil)))
`, gicel.Stream)
	con := v.(*gicel.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	assertConVal(t, con.Args[0], "True")
	second := con.Args[1].(*gicel.ConVal)
	assertConVal(t, second.Args[0], "False")
	assertConVal(t, second.Args[1], "Nil")
}

func TestStreamTakeS(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Stream
main := takeS 2 (LCons True (\_. LCons False (\_. LCons True (\_. LNil))))
`, gicel.Num, gicel.Stream)
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
import Std.Stream
main := headS (fmap not (LCons True (\_. LNil)))
`, gicel.Stream)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestStreamDropS(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Stream
main := headS (dropS 1 (LCons True (\_. LCons False (\_. LNil))))
`, gicel.Num, gicel.Stream)
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
import Std.Num
import Std.Slice
main := sliceLength (fmap (\x. x + 1) (fmap (\x. x + 2) (sliceSingleton 0)))
`, gicel.Num, gicel.Slice)
	assertHostInt(t, v, 1)
}

func TestStringPackedRoundtripIntegration(t *testing.T) {
	// Verify production strPackedRoundtrip fires: fromRunes (toRunes x) → x
	v := runWithPacks(t, `
import Std.Str
main := fromRunes (toRunes "abc")
`, gicel.Str)
	assertHostString(t, v, "abc")
}

// ===========================================================================
// Std.IO
// ===========================================================================

func TestIOPrint(t *testing.T) {
	eng := gicel.NewEngine()
	if err := gicel.Str(eng); err != nil {
		t.Fatal(err)
	}
	if err := gicel.IO(eng); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Str
import Std.IO
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
// Std.List expansion — new primitives and GICEL functions
// ===========================================================================

func TestListDropWhile(t *testing.T) {
	v := runWithPacks(t, `
import Std.List
main := dropWhile not (Cons False (Cons False (Cons True (Cons False Nil))))
`, gicel.List)
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
import Std.List
main := fst (span not (Cons False (Cons True Nil)))
`, gicel.List)
	// span not [F,T] — not F = True (take), not T = False (stop)
	// fst = [F]
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "False")
	assertConVal(t, con.Args[1], "Nil")
}

func TestListBreak(t *testing.T) {
	v := runWithPacks(t, `
import Std.List
main := fst (break not (Cons True (Cons False Nil)))
`, gicel.List)
	// break not [T,F] = span (not . not) [T,F] = span id [T,F]
	// id T = True (take), id F = False (stop) → fst = [T]
	con := v.(*gicel.ConVal)
	assertConVal(t, con.Args[0], "True")
	assertConVal(t, con.Args[1], "Nil")
}

func TestListSortBy(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.List
main := sortBy compare (Cons 3 (Cons 1 (Cons 2 Nil)))
`, gicel.Num, gicel.List)
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 1 {
		t.Fatalf("expected 1 first, got %v", hv.Inner)
	}
}

func TestListSort(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.List
main := sort (Cons 3 (Cons 1 (Cons 2 Nil)))
`, gicel.Num, gicel.List)
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
import Std.Num
import Std.List
main := sort Nil :: List Int
`, gicel.Num, gicel.List)
	assertConVal(t, v, "Nil")
}

func TestListScanl(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.List
main := scanl (\x y. x + y) 0 (Cons 1 (Cons 2 (Cons 3 Nil)))
`, gicel.Num, gicel.List)
	// scanl (+) 0 [1,2,3] = [0,1,3,6]
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 0 {
		t.Fatalf("expected 0 first, got %v", hv.Inner)
	}
}

func TestListUnfoldr(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.List
main := unfoldr (\n. case n == 0 { True -> Nothing; False -> Just (n, n - 1) }) 3
`, gicel.Num, gicel.List)
	// unfoldr from 3: Just (3,2), Just (2,1), Just (1,0), Nothing → [3,2,1]
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 3 {
		t.Fatalf("expected 3 first, got %v", hv.Inner)
	}
}

func TestListIterateN(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.List
main := iterateN 4 (\x. x * 2) 1
`, gicel.Num, gicel.List)
	// iterateN 4 (*2) 1 = [1,2,4,8]
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 1 {
		t.Fatalf("expected 1 first, got %v", hv.Inner)
	}
}

func TestListZipWith(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.List
main := zipWith (\x y. x + y) (Cons 1 (Cons 2 Nil)) (Cons 10 (Cons 20 Nil))
`, gicel.Num, gicel.List)
	// zipWith (+) [1,2] [10,20] = [11,22]
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 11 {
		t.Fatalf("expected 11 first, got %v", hv.Inner)
	}
}

func TestListIntercalate(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.List
main := intercalate (Cons 0 Nil) (Cons (Cons 1 Nil) (Cons (Cons 2 Nil) (Cons (Cons 3 Nil) Nil)))
`, gicel.Num, gicel.List)
	// intercalate [0] [[1],[2],[3]] = [1,0,2,0,3]
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 1 {
		t.Fatalf("expected 1 first, got %v", hv.Inner)
	}
}

func TestListNubBy(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.List
main := nubBy eq (Cons 1 (Cons 2 (Cons 1 (Cons 3 Nil))))
`, gicel.Num, gicel.List)
	// nubBy eq [1,2,1,3] = [1,2,3] (preserves first occurrences via foldl)
	con := v.(*gicel.ConVal)
	hv := con.Args[0].(*gicel.HostVal)
	if hv.Inner.(int64) != 1 {
		t.Fatalf("expected 1 first, got %v", hv.Inner)
	}
}

func TestIODebug(t *testing.T) {
	eng := gicel.NewEngine()
	if err := gicel.Num(eng); err != nil {
		t.Fatal(err)
	}
	if err := gicel.IO(eng); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
import Std.IO
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
// Std.Map integration
// ===========================================================================

func TestMapInsertLookupIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Map
main := mapLookup 1 (insert 1 True (mapEmpty :: Map Int Bool))
`, gicel.Num, gicel.Map)
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "True")
}

func TestMapLookupMissingIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Map
main := mapLookup 99 (insert 1 True (mapEmpty :: Map Int Bool))
`, gicel.Num, gicel.Map)
	assertConVal(t, v, "Nothing")
}

func TestMapSizeIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Map
main := mapSize (insert 2 False (insert 1 True (mapEmpty :: Map Int Bool)))
`, gicel.Num, gicel.Map)
	assertHostInt(t, v, 2)
}

func TestMapMemberIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Map
main := member 1 (insert 1 True (mapEmpty :: Map Int Bool))
`, gicel.Num, gicel.Map)
	assertConVal(t, v, "True")
}

func TestMapDeleteIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Map
main := mapSize (delete 1 (insert 1 True (mapEmpty :: Map Int Bool)))
`, gicel.Num, gicel.Map)
	assertHostInt(t, v, 0)
}

func TestMapFromListToListIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Map
main := mapSize (fromList (Cons (1, True) (Cons (2, False) Nil)) :: Map Int Bool)
`, gicel.Num, gicel.Map)
	assertHostInt(t, v, 2)
}

// ===========================================================================
// Std.Set integration
// ===========================================================================

func TestSetInsertMemberIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Set
main := setMember 1 (setInsert 1 (setEmpty :: Set Int))
`, gicel.Num, gicel.Set)
	assertConVal(t, v, "True")
}

func TestSetMemberMissingIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Set
main := setMember 99 (setInsert 1 (setEmpty :: Set Int))
`, gicel.Num, gicel.Set)
	assertConVal(t, v, "False")
}

func TestSetSizeIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Set
main := setSize (setInsert 2 (setInsert 1 (setEmpty :: Set Int)))
`, gicel.Num, gicel.Set)
	assertHostInt(t, v, 2)
}

func TestSetDeleteIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Set
main := setSize (setDelete 1 (setInsert 2 (setInsert 1 (setEmpty :: Set Int))))
`, gicel.Num, gicel.Set)
	assertHostInt(t, v, 1)
}

func TestSetFromListIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Set
main := setSize (setFromList (Cons 1 (Cons 2 (Cons 1 Nil))) :: Set Int)
`, gicel.Num, gicel.Set)
	assertHostInt(t, v, 2) // duplicates removed
}

func TestSetToListIntegration(t *testing.T) {
	v := runWithPacks(t, `
import Std.Num
import Std.Set
main := setToList (setInsert 2 (setInsert 1 (setEmpty :: Set Int)))
`, gicel.Num, gicel.Set)
	// Should be sorted: [1, 2]
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	assertHostInt(t, con.Args[0], 1)
}
