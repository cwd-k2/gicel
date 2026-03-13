package gomputation_test

import (
	"context"
	"testing"

	gmp "github.com/cwd-k2/gomputation"
)

// ---------------------------------------------------------------------------
// Stdlib expansion tests — TDD for Std.Str, Std.List, and Std.IO additions.
// ---------------------------------------------------------------------------

// runWithPacks compiles source with given packs and evaluates "main".
func runWithPacks(t *testing.T, source string, packs ...gmp.Pack) gmp.Value {
	t.Helper()
	eng := gmp.NewEngine()
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

func assertHostString(t *testing.T, v gmp.Value, expected string) {
	t.Helper()
	hv, ok := v.(*gmp.HostVal)
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

func assertHostBool(t *testing.T, v gmp.Value, expected bool) {
	t.Helper()
	con, ok := v.(*gmp.ConVal)
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
	v := runWithPacks(t, "import Std.Num\nimport Std.Str\nmain := charAt 0 \"hello\"", gmp.Num, gmp.Str)
	// charAt returns Maybe Rune; 'h' = Just 'h'
	con, ok := v.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
}

func TestStrCharAtOutOfBounds(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.Str\nmain := charAt 99 \"hi\"", gmp.Num, gmp.Str)
	assertConVal(t, v, "Nothing")
}

func TestStrSubstring(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.Str\nmain := substring 1 2 \"hello\"", gmp.Num, gmp.Str)
	assertHostString(t, v, "el")
}

func TestStrToUpper(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := toUpper \"hello\"", gmp.Str)
	assertHostString(t, v, "HELLO")
}

func TestStrToLower(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := toLower \"HELLO\"", gmp.Str)
	assertHostString(t, v, "hello")
}

func TestStrTrim(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := trim \"  hello  \"", gmp.Str)
	assertHostString(t, v, "hello")
}

func TestStrContains(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := contains \"ell\" \"hello\"", gmp.Str)
	assertHostBool(t, v, true)
}

func TestStrContainsFalse(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := contains \"xyz\" \"hello\"", gmp.Str)
	assertHostBool(t, v, false)
}

func TestStrSplit(t *testing.T) {
	// split "," "a,b,c" => List String: Cons "a" (Cons "b" (Cons "c" Nil))
	v := runWithPacks(t, "import Std.Str\nmain := split \",\" \"a,b,c\"", gmp.Str)
	con, ok := v.(*gmp.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	assertHostString(t, con.Args[0], "a")
}

func TestStrJoin(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := join \",\" (Cons \"a\" (Cons \"b\" (Cons \"c\" Nil)))", gmp.Str)
	assertHostString(t, v, "a,b,c")
}

func TestStrShowInt(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.Str\nmain := showInt 42", gmp.Num, gmp.Str)
	assertHostString(t, v, "42")
}

func TestStrShowBool(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := showBool True", gmp.Str)
	assertHostString(t, v, "True")
}

func TestStrReadInt(t *testing.T) {
	// readInt returns Maybe Int
	v := runWithPacks(t, "import Std.Num\nimport Std.Str\nmain := readInt \"42\"", gmp.Num, gmp.Str)
	con, ok := v.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	hv, ok := con.Args[0].(*gmp.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", con.Args[0])
	}
	if hv.Inner.(int64) != 42 {
		t.Fatalf("expected 42, got %v", hv.Inner)
	}
}

func TestStrReadIntFail(t *testing.T) {
	v := runWithPacks(t, "import Std.Str\nmain := readInt \"abc\"", gmp.Str)
	assertConVal(t, v, "Nothing")
}

// ===========================================================================
// Std.List expansion
// ===========================================================================

func TestListZip(t *testing.T) {
	v := runWithPacks(t, `
import Std.List
main := zip (Cons True (Cons False Nil)) (Cons Unit Nil)
`, gmp.List)
	// zip [True,False] [Unit] = [Pair True Unit]
	con, ok := v.(*gmp.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	pair, ok := con.Args[0].(*gmp.ConVal)
	if !ok || pair.Con != "Pair" {
		t.Fatalf("expected Pair, got %v", con.Args[0])
	}
	assertConVal(t, con.Args[1], "Nil")
}

func TestListUnzip(t *testing.T) {
	v := runWithPacks(t, `
import Std.List
main := unzip (Cons (Pair True Unit) Nil)
`, gmp.List)
	// unzip [Pair True Unit] = Pair [True] [Unit]
	con, ok := v.(*gmp.ConVal)
	if !ok || con.Con != "Pair" {
		t.Fatalf("expected Pair, got %v", v)
	}
}

func TestListTake(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.List\nmain := take 2 (Cons True (Cons False (Cons True Nil)))", gmp.Num, gmp.List)
	// take 2 [True,False,True] = [True,False]
	con := v.(*gmp.ConVal)
	if con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", con.Con)
	}
	second := con.Args[1].(*gmp.ConVal)
	if second.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", second.Con)
	}
	assertConVal(t, second.Args[1], "Nil")
}

func TestListDrop(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.List\nmain := drop 1 (Cons True (Cons False Nil))", gmp.Num, gmp.List)
	// drop 1 [True,False] = [False]
	con := v.(*gmp.ConVal)
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
`, gmp.List)
	// reverse [True,False] = [False,True]
	con := v.(*gmp.ConVal)
	assertConVal(t, con.Args[0], "False")
}

func TestListIndex(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.List\nmain := index 1 (Cons True (Cons False Nil))", gmp.Num, gmp.List)
	// index 1 [True,False] = Just False
	con, ok := v.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	assertConVal(t, con.Args[0], "False")
}

func TestListIndexOutOfBounds(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.List\nmain := index 5 (Cons True Nil)", gmp.Num, gmp.List)
	assertConVal(t, v, "Nothing")
}

func TestListReplicate(t *testing.T) {
	v := runWithPacks(t, "import Std.Num\nimport Std.List\nmain := replicate 3 True", gmp.Num, gmp.List)
	// replicate 3 True = [True, True, True]
	con := v.(*gmp.ConVal)
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
`, gmp.Num, gmp.List)
	hv, ok := v.(*gmp.HostVal)
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
	eng := gmp.NewEngine()
	if err := gmp.Str(eng); err != nil {
		t.Fatal(err)
	}
	if err := gmp.IO(eng); err != nil {
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
	assertConVal(t, result.Value, "Unit")
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
	eng := gmp.NewEngine()
	if err := gmp.Num(eng); err != nil {
		t.Fatal(err)
	}
	if err := gmp.IO(eng); err != nil {
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
	assertConVal(t, result.Value, "Unit")
	buf, ok := result.CapEnv.Get("io")
	if !ok {
		t.Fatal("expected io capability")
	}
	msgs := buf.([]string)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}
