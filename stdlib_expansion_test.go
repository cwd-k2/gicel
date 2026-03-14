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
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
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
main := foldl (\acc -> \x -> case x { True -> acc + 1; False -> acc }) 0 (Cons True (Cons False (Cons True Nil)))
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
	result, err := rt.RunContextFull(context.Background(), nil, nil, "main")
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
	result, err := rt.RunContextFull(context.Background(), nil, nil, "main")
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
