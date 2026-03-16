package stdlib

import (
	"context"
	"fmt"
	"testing"

	"github.com/cwd-k2/gicel/internal/eval"
)

// Helper constructors.
func intVal(n int64) eval.Value          { return &eval.HostVal{Inner: n} }
func strVal(s string) eval.Value         { return &eval.HostVal{Inner: s} }
func runeVal(r rune) eval.Value          { return &eval.HostVal{Inner: r} }
func args(vs ...eval.Value) []eval.Value { return vs }

var (
	ctx = context.Background()
	ce  = eval.NewCapEnv(nil)
)

func assertInt(t *testing.T, v eval.Value, want int64) {
	t.Helper()
	hv, ok := v.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", v)
	}
	if hv.Inner != want {
		t.Fatalf("expected %d, got %v", want, hv.Inner)
	}
}

func assertCon(t *testing.T, v eval.Value, con string) {
	t.Helper()
	cv, ok := v.(*eval.ConVal)
	if !ok {
		t.Fatalf("expected ConVal, got %T", v)
	}
	if cv.Con != con {
		t.Fatalf("expected %s, got %s", con, cv.Con)
	}
}

func assertStr(t *testing.T, v eval.Value, want string) {
	t.Helper()
	hv, ok := v.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", v)
	}
	if hv.Inner != want {
		t.Fatalf("expected %q, got %v", want, hv.Inner)
	}
}

// --- Num ---

func TestAddIntImpl(t *testing.T) {
	v, _, err := addIntImpl(ctx, ce, args(intVal(1), intVal(2)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 3)
}

func TestSubIntImpl(t *testing.T) {
	v, _, err := subIntImpl(ctx, ce, args(intVal(5), intVal(3)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 2)
}

func TestMulIntImpl(t *testing.T) {
	v, _, err := mulIntImpl(ctx, ce, args(intVal(3), intVal(4)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 12)
}

func TestDivIntImplZero(t *testing.T) {
	_, _, err := divIntImpl(ctx, ce, args(intVal(1), intVal(0)), nil)
	if err == nil {
		t.Fatal("expected error for division by zero")
	}
}

func TestModIntImplZero(t *testing.T) {
	_, _, err := modIntImpl(ctx, ce, args(intVal(1), intVal(0)), nil)
	if err == nil {
		t.Fatal("expected error for modulo by zero")
	}
}

func TestNegIntImpl(t *testing.T) {
	v, _, err := negIntImpl(ctx, ce, args(intVal(42)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, -42)
}

func TestEqIntImplTrue(t *testing.T) {
	v, _, err := eqIntImpl(ctx, ce, args(intVal(1), intVal(1)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "True")
}

func TestEqIntImplFalse(t *testing.T) {
	v, _, err := eqIntImpl(ctx, ce, args(intVal(1), intVal(2)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "False")
}

func TestCmpIntImplLT(t *testing.T) {
	v, _, err := cmpIntImpl(ctx, ce, args(intVal(1), intVal(2)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "LT")
}

func TestCmpIntImplGT(t *testing.T) {
	v, _, err := cmpIntImpl(ctx, ce, args(intVal(2), intVal(1)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "GT")
}

func TestCmpIntImplEQ(t *testing.T) {
	v, _, err := cmpIntImpl(ctx, ce, args(intVal(1), intVal(1)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "EQ")
}

// --- Str ---

func TestEqStrImpl(t *testing.T) {
	v, _, err := eqStrImpl(ctx, ce, args(strVal("a"), strVal("a")), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "True")
}

func TestCmpStrImpl(t *testing.T) {
	v, _, err := cmpStrImpl(ctx, ce, args(strVal("a"), strVal("b")), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "LT")
}

func TestAppendStrImpl(t *testing.T) {
	v, _, err := appendStrImpl(ctx, ce, args(strVal("a"), strVal("b")), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, v, "ab")
}

func TestEmptyStrImpl(t *testing.T) {
	v, _, err := emptyStrImpl(ctx, ce, args(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, v, "")
}

func TestLengthStrImpl(t *testing.T) {
	v, _, err := lengthStrImpl(ctx, ce, args(strVal("hello")), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 5)
}

func TestLengthStrImplUnicode(t *testing.T) {
	v, _, err := lengthStrImpl(ctx, ce, args(strVal("日本語")), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 3)
}

func TestEqRuneImpl(t *testing.T) {
	v, _, err := eqRuneImpl(ctx, ce, args(runeVal('a'), runeVal('a')), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "True")
}

func TestCmpRuneImpl(t *testing.T) {
	v, _, err := cmpRuneImpl(ctx, ce, args(runeVal('a'), runeVal('b')), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "LT")
}

func TestSubstringImplNegativeCount(t *testing.T) {
	v, _, err := substringImpl(ctx, ce, args(intVal(0), intVal(-1), strVal("hello")), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, v, "")
}

func TestToRunesImpl(t *testing.T) {
	v, _, err := toRunesImpl(ctx, ce, args(strVal("hi")), nil)
	if err != nil {
		t.Fatal(err)
	}
	c1, ok := v.(*eval.ConVal)
	if !ok || c1.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	r1, ok := c1.Args[0].(*eval.HostVal)
	if !ok || r1.Inner != 'h' {
		t.Fatalf("expected 'h', got %v", c1.Args[0])
	}
	c2 := c1.Args[1].(*eval.ConVal)
	r2 := c2.Args[0].(*eval.HostVal)
	if r2.Inner != 'i' {
		t.Fatalf("expected 'i', got %v", c2.Args[0])
	}
}

// --- Fail ---

func TestFailImpl(t *testing.T) {
	_, _, err := failImpl(ctx, ce, args(&eval.ConVal{Con: "Unit"}), nil)
	if err == nil {
		t.Fatal("expected error from fail")
	}
	if _, ok := err.(*eval.RuntimeError); !ok {
		t.Fatalf("expected RuntimeError, got %T", err)
	}
}

// --- State ---

func TestGetImplMissing(t *testing.T) {
	_, _, err := getImpl(ctx, ce, args(), nil)
	if err == nil {
		t.Fatal("expected error when state capability is missing")
	}
}

func TestPutImpl(t *testing.T) {
	stCe := eval.NewCapEnv(map[string]any{"state": intVal(1)})
	v, newCe, err := putImpl(ctx, stCe, args(intVal(42)), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := v.(*eval.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Fatalf("expected empty RecordVal (unit), got %v", v)
	}
	s, ok := newCe.Get("state")
	if !ok {
		t.Fatal("state missing after put")
	}
	assertInt(t, s.(eval.Value), 42)
}

func TestGetImplNonValue(t *testing.T) {
	stCe := eval.NewCapEnv(map[string]any{"state": "not-a-value"})
	_, _, err := getImpl(ctx, stCe, args(), nil)
	if err == nil {
		t.Fatal("expected error when state is not a Value")
	}
}

// --- asInt64 / asString error ---

func TestAsInt64Error(t *testing.T) {
	_, err := asInt64(strVal("oops"), "test")
	if err == nil {
		t.Fatal("expected error from asInt64 with string")
	}
}

func TestAsStringError(t *testing.T) {
	_, err := asString(intVal(42))
	if err == nil {
		t.Fatal("expected error from asString with int")
	}
}

// --- Stream (buildStream/streamConst removed — now pure GICEL recursion) ---

// --- Slice ---

func sliceOf(vals ...eval.Value) eval.Value {
	items := make([]eval.Value, len(vals))
	copy(items, vals)
	return &eval.HostVal{Inner: items}
}

func TestSliceEmptyImpl(t *testing.T) {
	v, _, err := sliceEmptyImpl(ctx, ce, args(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := asSlice(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 0 {
		t.Fatalf("expected empty slice, got %d elements", len(s))
	}
}

func TestSliceSingletonImpl(t *testing.T) {
	v, _, err := sliceSingletonImpl(ctx, ce, args(intVal(42)), nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := asSlice(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 {
		t.Fatalf("expected 1 element, got %d", len(s))
	}
	assertInt(t, s[0], 42)
}

func TestSliceLengthImpl(t *testing.T) {
	v, _, err := sliceLengthImpl(ctx, ce, args(sliceOf(intVal(1), intVal(2), intVal(3))), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 3)
}

func TestSliceIndexImpl(t *testing.T) {
	v, _, err := sliceIndexImpl(ctx, ce, args(intVal(1), sliceOf(intVal(10), intVal(20))), nil)
	if err != nil {
		t.Fatal(err)
	}
	con := v.(*eval.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	assertInt(t, con.Args[0], 20)
}

func TestSliceIndexOutOfBounds(t *testing.T) {
	v, _, err := sliceIndexImpl(ctx, ce, args(intVal(5), sliceOf(intVal(1))), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nothing")
}

func TestSliceConsImpl(t *testing.T) {
	v, _, err := sliceConsImpl(ctx, ce, args(intVal(0), sliceOf(intVal(1), intVal(2))), nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := asSlice(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(s))
	}
	assertInt(t, s[0], 0)
}

func TestSliceSnocImpl(t *testing.T) {
	v, _, err := sliceSnocImpl(ctx, ce, args(sliceOf(intVal(1)), intVal(2)), nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := asSlice(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(s))
	}
	assertInt(t, s[1], 2)
}

func TestSliceIndexNegative(t *testing.T) {
	v, _, err := sliceIndexImpl(ctx, ce, args(intVal(-1), sliceOf(intVal(10))), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nothing")
}

func TestSliceAppendOrder(t *testing.T) {
	// [1] ++ [2,3] = [1,2,3] — element order must be preserved (M21).
	v, _, err := sliceAppendImpl(ctx, ce, args(sliceOf(intVal(1)), sliceOf(intVal(2), intVal(3))), nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := asSlice(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(s))
	}
	assertInt(t, s[0], 1)
	assertInt(t, s[1], 2)
	assertInt(t, s[2], 3)
}

func TestSliceMapImpl(t *testing.T) {
	fn := &eval.Closure{Param: "x", Body: nil}
	applier := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		n := arg.(*eval.HostVal).Inner.(int64)
		return intVal(n * 2), capEnv, nil
	}
	v, _, err := sliceMapImpl(ctx, ce, args(fn, sliceOf(intVal(1), intVal(2))), applier)
	if err != nil {
		t.Fatal(err)
	}
	s, err := asSlice(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(s))
	}
	assertInt(t, s[0], 2)
	assertInt(t, s[1], 4)
}

func TestSliceMapImplEmpty(t *testing.T) {
	fn := &eval.Closure{Param: "x", Body: nil}
	applier := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		t.Fatal("applier should not be called on empty slice")
		return nil, capEnv, nil
	}
	v, _, err := sliceMapImpl(ctx, ce, args(fn, sliceOf()), applier)
	if err != nil {
		t.Fatal(err)
	}
	s, err := asSlice(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 0 {
		t.Fatalf("expected empty slice, got %d elements", len(s))
	}
}

func TestSliceFoldrDirection(t *testing.T) {
	// foldr (\x acc -> x : acc) [] [1,2,3] with non-commutative op
	// Right fold: 1 : (2 : (3 : [])) = [1,2,3]
	// Left fold would give: 3 : (2 : (1 : [])) = [3,2,1]
	fn := &eval.Closure{Param: "x", Body: nil}
	applier := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		if _, ok := fn.(*eval.Closure); ok {
			return &eval.HostVal{Inner: arg}, capEnv, nil // partial: capture element
		}
		elem := fn.(*eval.HostVal).Inner.(eval.Value)
		acc := arg.(*eval.HostVal).Inner.([]eval.Value)
		result := make([]eval.Value, 0, len(acc)+1)
		result = append(result, elem)
		result = append(result, acc...)
		return &eval.HostVal{Inner: result}, capEnv, nil
	}
	emptySlice := &eval.HostVal{Inner: []eval.Value{}}
	v, _, err := sliceFoldrImpl(ctx, ce, args(fn, emptySlice, sliceOf(intVal(1), intVal(2), intVal(3))), applier)
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*eval.HostVal).Inner.([]eval.Value)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	// Right fold preserves order: [1,2,3]
	assertInt(t, result[0], 1)
	assertInt(t, result[1], 2)
	assertInt(t, result[2], 3)
}

func TestSliceFoldrImplEmpty(t *testing.T) {
	fn := &eval.Closure{Param: "x", Body: nil}
	applier := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		t.Fatal("applier should not be called on empty slice")
		return nil, capEnv, nil
	}
	v, _, err := sliceFoldrImpl(ctx, ce, args(fn, intVal(42), sliceOf()), applier)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 42)
}

func TestSliceFoldlDirection(t *testing.T) {
	// foldl (\acc x -> acc ++ [x]) [] [1,2,3]
	// Left fold: (([] ++ [1]) ++ [2]) ++ [3] = [1,2,3]
	// Right fold would give: [3,2,1]
	fn := &eval.Closure{Param: "acc", Body: nil}
	applier := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		if _, ok := fn.(*eval.Closure); ok {
			return &eval.HostVal{Inner: arg}, capEnv, nil // partial: capture acc
		}
		acc := fn.(*eval.HostVal).Inner.(eval.Value).(*eval.HostVal).Inner.([]eval.Value)
		elem := arg
		result := make([]eval.Value, len(acc)+1)
		copy(result, acc)
		result[len(acc)] = elem
		return &eval.HostVal{Inner: result}, capEnv, nil
	}
	emptySlice := &eval.HostVal{Inner: []eval.Value{}}
	v, _, err := sliceFoldlImpl(ctx, ce, args(fn, emptySlice, sliceOf(intVal(1), intVal(2), intVal(3))), applier)
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*eval.HostVal).Inner.([]eval.Value)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	// Left fold preserves order: [1,2,3]
	assertInt(t, result[0], 1)
	assertInt(t, result[1], 2)
	assertInt(t, result[2], 3)
}

func TestSliceFromListImpl(t *testing.T) {
	list := conList(intVal(10), intVal(20), intVal(30))
	v, _, err := sliceFromListImpl(ctx, ce, args(list), nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := asSlice(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(s))
	}
	assertInt(t, s[0], 10)
	assertInt(t, s[1], 20)
	assertInt(t, s[2], 30)
}

func TestSliceToListImpl(t *testing.T) {
	v, _, err := sliceToListImpl(ctx, ce, args(sliceOf(intVal(1), intVal(2))), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	assertInt(t, items[0], 1)
	assertInt(t, items[1], 2)
}

// Stream unit tests removed — recursive operations now expressed in GICEL.

// --- fromRunes ---

func TestFromRunesImplEmpty(t *testing.T) {
	v, _, err := fromRunesImpl(ctx, ce, args(&eval.ConVal{Con: "Nil"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, v, "")
}

func TestFromRunesImpl(t *testing.T) {
	list := conList(runeVal('h'), runeVal('i'))
	v, _, err := fromRunesImpl(ctx, ce, args(list), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, v, "hi")
}

// --- List ---

// conList builds a Cons/Nil chain; delegates to the production buildList.
func conList(vals ...eval.Value) eval.Value { return buildList(vals) }

func TestFromSliceImpl(t *testing.T) {
	input := &eval.HostVal{Inner: []any{intVal(1), intVal(2), intVal(3)}}
	v, _, err := fromSliceImpl(ctx, ce, args(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestFromSliceImplPassthrough(t *testing.T) {
	list := conList(intVal(1), intVal(2))
	v, _, err := fromSliceImpl(ctx, ce, args(list), nil)
	if err != nil {
		t.Fatal(err)
	}
	if v != list {
		t.Fatal("expected passthrough for ConVal list")
	}
}

func TestToSliceImpl(t *testing.T) {
	list := conList(intVal(1), intVal(2), intVal(3))
	v, _, err := toSliceImpl(ctx, ce, args(list), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := v.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", v)
	}
	items := hv.Inner.([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestLengthImplEmpty(t *testing.T) {
	v, _, err := lengthImpl(ctx, ce, args(&eval.ConVal{Con: "Nil"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 0)
}

func TestLengthImpl(t *testing.T) {
	list := conList(intVal(1), intVal(2), intVal(3))
	v, _, err := lengthImpl(ctx, ce, args(list), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 3)
}

func TestConcatImpl(t *testing.T) {
	xs := conList(intVal(1), intVal(2))
	ys := conList(intVal(3))
	v, _, err := concatImpl(ctx, ce, args(xs, ys), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

// --- Str: charAt, split, join ---

func TestCharAtImpl(t *testing.T) {
	v, _, err := charAtImpl(ctx, ce, args(intVal(1), strVal("abc")), nil)
	if err != nil {
		t.Fatal(err)
	}
	con := v.(*eval.ConVal)
	if con.Con != "Just" {
		t.Fatalf("expected Just, got %s", con.Con)
	}
	r, err := asRune(con.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if r != 'b' {
		t.Fatalf("expected 'b', got '%c'", r)
	}
}

func TestCharAtImplOutOfBounds(t *testing.T) {
	v, _, err := charAtImpl(ctx, ce, args(intVal(3), strVal("abc")), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nothing")
}

func TestCharAtImplExactBoundary(t *testing.T) {
	// Index exactly at len(runes) should return Nothing, not panic.
	v, _, err := charAtImpl(ctx, ce, args(intVal(3), strVal("abc")), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nothing")
}

func TestSplitImpl(t *testing.T) {
	v, _, err := splitImpl(ctx, ce, args(strVal(","), strVal("a,b,c")), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	assertStr(t, items[0], "a")
	assertStr(t, items[1], "b")
	assertStr(t, items[2], "c")
}

func TestJoinImpl(t *testing.T) {
	list := conList(strVal("a"), strVal("b"), strVal("c"))
	v, _, err := joinImpl(ctx, ce, args(strVal("-"), list), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, v, "a-b-c")
}

// --- Slice: exact boundary index ---

func TestSliceIndexExactBoundary(t *testing.T) {
	// Index exactly at len(slice) should return Nothing, not panic.
	v, _, err := sliceIndexImpl(ctx, ce, args(intVal(2), sliceOf(intVal(10), intVal(20))), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nothing")
}

func TestRoundTrip(t *testing.T) {
	original := []any{intVal(1), intVal(2), intVal(3)}
	list := sliceToList(original)
	items, ok := listToSlice(list)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != len(original) {
		t.Fatalf("round-trip: expected %d items, got %d", len(original), len(items))
	}
}

// --- List new primitives ---

// boolApplier creates an Applier that applies a Bool-returning predicate.
func boolApplier(pred func(eval.Value) bool) eval.Applier {
	return func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		if pred(arg) {
			return &eval.ConVal{Con: "True"}, capEnv, nil
		}
		return &eval.ConVal{Con: "False"}, capEnv, nil
	}
}

func TestDropWhileImpl(t *testing.T) {
	// dropWhile (>2) [3,4,1,5] = [1,5]
	list := conList(intVal(3), intVal(4), intVal(1), intVal(5))
	pred := &eval.Closure{Param: "x", Body: nil}
	applier := boolApplier(func(v eval.Value) bool {
		return v.(*eval.HostVal).Inner.(int64) > 2
	})
	v, _, err := dropWhileImpl(ctx, ce, args(pred, list), applier)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	assertInt(t, items[0], 1)
	assertInt(t, items[1], 5)
}

func TestDropWhileImplAll(t *testing.T) {
	list := conList(intVal(1), intVal(2))
	pred := &eval.Closure{Param: "x", Body: nil}
	applier := boolApplier(func(eval.Value) bool { return true })
	v, _, err := dropWhileImpl(ctx, ce, args(pred, list), applier)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

func TestSpanImpl(t *testing.T) {
	// span (<3) [1,2,4,1] = ([1,2], [4,1])
	list := conList(intVal(1), intVal(2), intVal(4), intVal(1))
	pred := &eval.Closure{Param: "x", Body: nil}
	applier := boolApplier(func(v eval.Value) bool {
		return v.(*eval.HostVal).Inner.(int64) < 3
	})
	v, _, err := spanImpl(ctx, ce, args(pred, list), applier)
	if err != nil {
		t.Fatal(err)
	}
	rv := v.(*eval.RecordVal)
	prefix, ok := listToSlice(rv.Fields["_1"])
	if !ok {
		t.Fatal("expected list for _1")
	}
	suffix, ok := listToSlice(rv.Fields["_2"])
	if !ok {
		t.Fatal("expected list for _2")
	}
	if len(prefix) != 2 {
		t.Fatalf("expected 2 prefix items, got %d", len(prefix))
	}
	if len(suffix) != 2 {
		t.Fatalf("expected 2 suffix items, got %d", len(suffix))
	}
	assertInt(t, prefix[0], 1)
	assertInt(t, prefix[1], 2)
	assertInt(t, suffix[0], 4)
	assertInt(t, suffix[1], 1)
}

func TestSortByImpl(t *testing.T) {
	list := conList(intVal(3), intVal(1), intVal(2))
	cmpFn := &eval.Closure{Param: "a", Body: nil}
	applier := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		if _, ok := fn.(*eval.Closure); ok {
			// partial application: capture first arg
			return &eval.HostVal{Inner: arg}, capEnv, nil
		}
		// full application: compare
		a := fn.(*eval.HostVal).Inner.(eval.Value).(*eval.HostVal).Inner.(int64)
		b := arg.(*eval.HostVal).Inner.(int64)
		if a < b {
			return &eval.ConVal{Con: "LT"}, capEnv, nil
		}
		if a > b {
			return &eval.ConVal{Con: "GT"}, capEnv, nil
		}
		return &eval.ConVal{Con: "EQ"}, capEnv, nil
	}
	v, _, err := sortByImpl(ctx, ce, args(cmpFn, list), applier)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	assertInt(t, items[0], 1)
	assertInt(t, items[1], 2)
	assertInt(t, items[2], 3)
}

func TestSortByImplStable(t *testing.T) {
	// sortBy on single element
	list := conList(intVal(42))
	v, _, err := sortByImpl(ctx, ce, args(&eval.Closure{Param: "a"}, list), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertInt(t, items[0], 42)
}

func TestScanlImpl(t *testing.T) {
	// scanl (+) 0 [1,2,3] = [0,1,3,6]
	f := &eval.Closure{Param: "acc", Body: nil}
	list := conList(intVal(1), intVal(2), intVal(3))
	applier := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		if _, ok := fn.(*eval.Closure); ok {
			return &eval.HostVal{Inner: arg}, capEnv, nil
		}
		a := fn.(*eval.HostVal).Inner.(eval.Value).(*eval.HostVal).Inner.(int64)
		b := arg.(*eval.HostVal).Inner.(int64)
		return intVal(a + b), capEnv, nil
	}
	v, _, err := scanlImpl(ctx, ce, args(f, intVal(0), list), applier)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}
	assertInt(t, items[0], 0)
	assertInt(t, items[1], 1)
	assertInt(t, items[2], 3)
	assertInt(t, items[3], 6)
}

func TestScanlImplEmpty(t *testing.T) {
	f := &eval.Closure{Param: "acc", Body: nil}
	v, _, err := scanlImpl(ctx, ce, args(f, intVal(42), &eval.ConVal{Con: "Nil"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertInt(t, items[0], 42)
}

func TestUnfoldrImpl(t *testing.T) {
	// unfoldr (\n -> if n==0 then Nothing else Just (n, n-1)) 3 = [3,2,1]
	f := &eval.Closure{Param: "n", Body: nil}
	applier := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		n := arg.(*eval.HostVal).Inner.(int64)
		if n == 0 {
			return &eval.ConVal{Con: "Nothing"}, capEnv, nil
		}
		pair := &eval.RecordVal{Fields: map[string]eval.Value{
			"_1": intVal(n),
			"_2": intVal(n - 1),
		}}
		return &eval.ConVal{Con: "Just", Args: []eval.Value{pair}}, capEnv, nil
	}
	v, _, err := unfoldrImpl(ctx, ce, args(f, intVal(3)), applier)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	assertInt(t, items[0], 3)
	assertInt(t, items[1], 2)
	assertInt(t, items[2], 1)
}

func TestIterateNImpl(t *testing.T) {
	// iterateN 4 (*2) 1 = [1,2,4,8]
	f := &eval.Closure{Param: "x", Body: nil}
	applier := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		n := arg.(*eval.HostVal).Inner.(int64)
		return intVal(n * 2), capEnv, nil
	}
	v, _, err := iterateNImpl(ctx, ce, args(intVal(4), f, intVal(1)), applier)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}
	assertInt(t, items[0], 1)
	assertInt(t, items[1], 2)
	assertInt(t, items[2], 4)
	assertInt(t, items[3], 8)
}

func TestIterateNImplZero(t *testing.T) {
	v, _, err := iterateNImpl(ctx, ce, args(intVal(0), &eval.Closure{Param: "x"}, intVal(1)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

// --- Map primitives ---

// intCmpApplier creates an Applier that compares int64 values via Ordering.
// Handles curried style: apply(cmpFn, a) → partial, apply(partial, b) → Ordering.
// Uses HostVal with a marker struct to distinguish partials from regular values.
type intCmpPartialInner struct{ val int64 }

func intCmpApplier() eval.Applier {
	return func(fn eval.Value, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		// Second application: partial(b) → Ordering
		if hv, ok := fn.(*eval.HostVal); ok {
			if p, ok := hv.Inner.(*intCmpPartialInner); ok {
				b := arg.(*eval.HostVal).Inner.(int64)
				switch {
				case p.val < b:
					return &eval.ConVal{Con: "LT"}, capEnv, nil
				case p.val > b:
					return &eval.ConVal{Con: "GT"}, capEnv, nil
				default:
					return &eval.ConVal{Con: "EQ"}, capEnv, nil
				}
			}
		}
		// First application: cmpFn(a) → partial capturing a
		a := arg.(*eval.HostVal).Inner.(int64)
		return &eval.HostVal{Inner: &intCmpPartialInner{val: a}}, capEnv, nil
	}
}

func TestMapInsertLookup(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"} // dummy, Applier handles comparison directly

	// Create empty map.
	emptyV, _, err := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Insert (1, "a").
	m1, _, err := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), emptyV), apply)
	if err != nil {
		t.Fatal(err)
	}

	// Insert (2, "b").
	m2, _, err := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), strVal("b"), m1), apply)
	if err != nil {
		t.Fatal(err)
	}

	// Lookup 1 → Just "a".
	v, _, err := mapLookupImpl(ctx, ce, args(cmpFn, intVal(1), m2), apply)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Just")
	assertStr(t, v.(*eval.ConVal).Args[0], "a")

	// Lookup 3 → Nothing.
	v, _, err = mapLookupImpl(ctx, ce, args(cmpFn, intVal(3), m2), apply)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nothing")
}

func TestMapDeleteSize(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), emptyV), apply)
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), strVal("b"), m1), apply)

	// Size = 2.
	sv, _, _ := mapSizeImpl(ctx, ce, args(m2), nil)
	assertInt(t, sv, 2)

	// Delete key 1.
	m3, _, _ := mapDeleteImpl(ctx, ce, args(cmpFn, intVal(1), m2), apply)
	sv2, _, _ := mapSizeImpl(ctx, ce, args(m3), nil)
	assertInt(t, sv2, 1)

	// Lookup 1 → Nothing (deleted).
	v, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(1), m3), apply)
	assertCon(t, v, "Nothing")

	// Lookup 2 → still present.
	v2, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(2), m3), apply)
	assertCon(t, v2, "Just")
}

func TestMapToListFromList(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	// Build list [(2, "b"), (1, "a"), (3, "c")]
	pairs := []eval.Value{
		&eval.RecordVal{Fields: map[string]eval.Value{"_1": intVal(2), "_2": strVal("b")}},
		&eval.RecordVal{Fields: map[string]eval.Value{"_1": intVal(1), "_2": strVal("a")}},
		&eval.RecordVal{Fields: map[string]eval.Value{"_1": intVal(3), "_2": strVal("c")}},
	}
	list := buildList(pairs)

	// fromList then toList: should be sorted.
	m, _, err := mapFromListImpl(ctx, ce, args(cmpFn, list), apply)
	if err != nil {
		t.Fatal(err)
	}
	sorted, _, err := mapToListImpl(ctx, ce, args(m), nil)
	if err != nil {
		t.Fatal(err)
	}

	items, _ := listToSlice(sorted)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// In-order traversal should give sorted keys: 1, 2, 3.
	for i, want := range []int64{1, 2, 3} {
		pair := items[i].(*eval.RecordVal)
		assertInt(t, pair.Fields["_1"], want)
	}
}

// --- Map: additional coverage ---

func TestMapEmptySize(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, err := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, err := mapSizeImpl(ctx, ce, args(emptyV), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, sv, 0)
}

func TestMapLookupEmpty(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)

	v, _, err := mapLookupImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nothing")
}

func TestMapInsertOverwrite(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)

	// Insert (1, "a"), then overwrite with (1, "b").
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), emptyV), apply)
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("b"), m1), apply)

	// Lookup 1 should return "b" (overwritten).
	v, _, err := mapLookupImpl(ctx, ce, args(cmpFn, intVal(1), m2), apply)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Just")
	assertStr(t, v.(*eval.ConVal).Args[0], "b")

	// Size should remain 1 (overwrite, not insert).
	sv, _, _ := mapSizeImpl(ctx, ce, args(m2), nil)
	assertInt(t, sv, 1)
}

func TestMapDeleteNonexistent(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), emptyV), apply)

	// Delete key 99 (not present): size should remain 1.
	m2, _, _ := mapDeleteImpl(ctx, ce, args(cmpFn, intVal(99), m1), apply)
	sv, _, _ := mapSizeImpl(ctx, ce, args(m2), nil)
	assertInt(t, sv, 1)
}

func TestMapDeleteEmpty(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)

	// Delete from empty map should not error.
	m2, _, err := mapDeleteImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ := mapSizeImpl(ctx, ce, args(m2), nil)
	assertInt(t, sv, 0)
}

func TestMapMemberImpl(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), emptyV), apply)

	// Member check: key 1 → True.
	v, _, err := mapMemberImpl(ctx, ce, args(cmpFn, intVal(1), m1), apply)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "True")

	// Member check: key 99 → False.
	v2, _, err := mapMemberImpl(ctx, ce, args(cmpFn, intVal(99), m1), apply)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v2, "False")
}

func TestMapFoldlWithKeyImpl(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), intVal(10), emptyV), apply)
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), intVal(20), m1), apply)

	// foldlWithKey (\acc k v -> acc + v) 0 {1:10, 2:20} = 30
	foldF := &eval.Closure{Param: "acc", Body: nil}
	sumApplier := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		switch f := fn.(type) {
		case *eval.Closure:
			// First application: capture acc
			return &eval.HostVal{Inner: &sumState{step: 0, acc: arg}}, capEnv, nil
		case *eval.HostVal:
			st := f.Inner.(*sumState)
			if st.step == 0 {
				// Second application: key (ignore)
				return &eval.HostVal{Inner: &sumState{step: 1, acc: st.acc}}, capEnv, nil
			}
			// Third application: value (add to acc)
			accN := st.acc.(*eval.HostVal).Inner.(int64)
			valN := arg.(*eval.HostVal).Inner.(int64)
			return intVal(accN + valN), capEnv, nil
		}
		return nil, capEnv, fmt.Errorf("unexpected fn type: %T", fn)
	}

	v, _, err := mapFoldlWithKeyImpl(ctx, ce, args(foldF, intVal(0), m2), sumApplier)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 30)
}

type sumState struct {
	step int
	acc  eval.Value
}

func TestMapUnionWithImpl(t *testing.T) {
	cmpApply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)

	// m1 = {1: 10, 2: 20}
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), intVal(10), emptyV), cmpApply)
	m1, _, _ = mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), intVal(20), m1), cmpApply)

	// m2 = {2: 5, 3: 30}
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), intVal(5), emptyV), cmpApply)
	m2, _, _ = mapInsertImpl(ctx, ce, args(cmpFn, intVal(3), intVal(30), m2), cmpApply)

	// unionWith (\a b -> a + b) m1 m2
	// On collision (key 2): 20 + 5 = 25
	addF := &eval.Closure{Param: "a", Body: nil}

	// Combined applier: handles both int comparison (for AVL lookup/insert)
	// and addition (for the merge function). Distinguishes by fn type.
	type addPartial struct{ val int64 }
	unionApply := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		// Merge function: Closure → partial, addPartial → sum
		if _, ok := fn.(*eval.Closure); ok {
			return &eval.HostVal{Inner: &addPartial{val: arg.(*eval.HostVal).Inner.(int64)}}, capEnv, nil
		}
		if hv, ok := fn.(*eval.HostVal); ok {
			if p, ok := hv.Inner.(*addPartial); ok {
				b := arg.(*eval.HostVal).Inner.(int64)
				return intVal(p.val + b), capEnv, nil
			}
		}
		// Comparison: delegate to intCmpApplier
		return cmpApply(fn, arg, capEnv)
	}

	result, _, err := mapUnionWithImpl(ctx, ce, args(addF, m1, m2), unionApply)
	if err != nil {
		t.Fatal(err)
	}

	sv, _, _ := mapSizeImpl(ctx, ce, args(result), nil)
	assertInt(t, sv, 3)

	// Check key 2 has value 25 (combined).
	v2, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(2), result), cmpApply)
	assertCon(t, v2, "Just")
	assertInt(t, v2.(*eval.ConVal).Args[0], 25)

	// Check key 3 has value 30 (from m2 only).
	v3, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(3), result), cmpApply)
	assertCon(t, v3, "Just")
	assertInt(t, v3.(*eval.ConVal).Args[0], 30)
}

func TestMapToListEmpty(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	v, _, err := mapToListImpl(ctx, ce, args(emptyV), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

func TestMapFromListEmpty(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	nilList := &eval.ConVal{Con: "Nil"}
	m, _, err := mapFromListImpl(ctx, ce, args(cmpFn, nilList), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ := mapSizeImpl(ctx, ce, args(m), nil)
	assertInt(t, sv, 0)
}

func TestAsMapValError(t *testing.T) {
	_, err := asMapVal(intVal(42))
	if err == nil {
		t.Fatal("expected error from asMapVal with non-map value")
	}
}

func TestMapManyInserts(t *testing.T) {
	// Insert 20 elements to exercise AVL rotations.
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	m, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	for i := int64(0); i < 20; i++ {
		var err error
		m, _, err = mapInsertImpl(ctx, ce, args(cmpFn, intVal(i), intVal(i*100), m), apply)
		if err != nil {
			t.Fatal(err)
		}
	}
	sv, _, _ := mapSizeImpl(ctx, ce, args(m), nil)
	assertInt(t, sv, 20)

	// Verify all keys are found.
	for i := int64(0); i < 20; i++ {
		v, _, err := mapLookupImpl(ctx, ce, args(cmpFn, intVal(i), m), apply)
		if err != nil {
			t.Fatal(err)
		}
		assertCon(t, v, "Just")
		assertInt(t, v.(*eval.ConVal).Args[0], i*100)
	}

	// toList should return sorted keys.
	sorted, _, _ := mapToListImpl(ctx, ce, args(m), nil)
	items, _ := listToSlice(sorted)
	if len(items) != 20 {
		t.Fatalf("expected 20 items, got %d", len(items))
	}
	for i, want := range items {
		pair := want.(*eval.RecordVal)
		assertInt(t, pair.Fields["_1"], int64(i))
	}
}

// --- Set primitives ---

func TestSetInsertMemberSize(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	emptyV, _, err := setEmptyImpl(ctx, ce, args(cmpFn), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Size 0.
	sv, _, _ := setSizeImpl(ctx, ce, args(emptyV), nil)
	assertInt(t, sv, 0)

	// Insert 1.
	s1, _, err := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ = setSizeImpl(ctx, ce, args(s1), nil)
	assertInt(t, sv, 1)

	// Insert 2.
	s2, _, err := setInsertImpl(ctx, ce, args(cmpFn, intVal(2), s1), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ = setSizeImpl(ctx, ce, args(s2), nil)
	assertInt(t, sv, 2)

	// Member 1 → True.
	v, _, err := setMemberImpl(ctx, ce, args(cmpFn, intVal(1), s2), apply)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "True")

	// Member 99 → False.
	v2, _, err := setMemberImpl(ctx, ce, args(cmpFn, intVal(99), s2), apply)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v2, "False")
}

func TestSetDeleteSize(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := setEmptyImpl(ctx, ce, args(cmpFn), nil)
	s1, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)
	s2, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(2), s1), apply)

	// Delete key 1.
	s3, _, err := setDeleteImpl(ctx, ce, args(cmpFn, intVal(1), s2), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ := setSizeImpl(ctx, ce, args(s3), nil)
	assertInt(t, sv, 1)

	// Member 1 → False (deleted).
	v, _, _ := setMemberImpl(ctx, ce, args(cmpFn, intVal(1), s3), apply)
	assertCon(t, v, "False")

	// Member 2 → True (still present).
	v2, _, _ := setMemberImpl(ctx, ce, args(cmpFn, intVal(2), s3), apply)
	assertCon(t, v2, "True")
}

func TestSetInsertDuplicate(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := setEmptyImpl(ctx, ce, args(cmpFn), nil)
	s1, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)

	// Insert duplicate: size should remain 1.
	s2, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), s1), apply)
	sv, _, _ := setSizeImpl(ctx, ce, args(s2), nil)
	assertInt(t, sv, 1)
}

func TestSetToListSorted(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := setEmptyImpl(ctx, ce, args(cmpFn), nil)
	s1, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(3), emptyV), apply)
	s2, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), s1), apply)
	s3, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(2), s2), apply)

	v, _, err := setToListImpl(ctx, ce, args(s3), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	assertInt(t, items[0], 1)
	assertInt(t, items[1], 2)
	assertInt(t, items[2], 3)
}

func TestSetFromListImpl(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	list := buildList([]eval.Value{intVal(3), intVal(1), intVal(2), intVal(1)})

	s, _, err := setFromListImpl(ctx, ce, args(cmpFn, list), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ := setSizeImpl(ctx, ce, args(s), nil)
	assertInt(t, sv, 3) // duplicates removed
}

func TestSetFromListEmpty(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	nilList := &eval.ConVal{Con: "Nil"}

	s, _, err := setFromListImpl(ctx, ce, args(cmpFn, nilList), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ := setSizeImpl(ctx, ce, args(s), nil)
	assertInt(t, sv, 0)
}

func TestSetDeleteNonexistent(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := setEmptyImpl(ctx, ce, args(cmpFn), nil)
	s1, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)

	// Delete non-existent key: size unchanged.
	s2, _, err := setDeleteImpl(ctx, ce, args(cmpFn, intVal(99), s1), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ := setSizeImpl(ctx, ce, args(s2), nil)
	assertInt(t, sv, 1)
}

func TestSetToListEmpty(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := setEmptyImpl(ctx, ce, args(cmpFn), nil)
	v, _, err := setToListImpl(ctx, ce, args(emptyV), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

// --- List: additional edge case coverage ---

func TestReverseImplEmpty(t *testing.T) {
	nilList := &eval.ConVal{Con: "Nil"}
	v, _, err := reverseImpl(ctx, ce, args(nilList), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

func TestTakeImplNegative(t *testing.T) {
	list := conList(intVal(1), intVal(2))
	v, _, err := takeImpl(ctx, ce, args(intVal(-5), list), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

func TestTakeImplExceedsLength(t *testing.T) {
	list := conList(intVal(1))
	v, _, err := takeImpl(ctx, ce, args(intVal(100), list), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestDropImplNegative(t *testing.T) {
	list := conList(intVal(1), intVal(2))
	v, _, err := dropImpl(ctx, ce, args(intVal(-3), list), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (no drop for negative), got %d", len(items))
	}
}

func TestDropImplExceedsLength(t *testing.T) {
	list := conList(intVal(1))
	v, _, err := dropImpl(ctx, ce, args(intVal(100), list), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

func TestIndexImplNegative(t *testing.T) {
	list := conList(intVal(10))
	v, _, err := indexImpl(ctx, ce, args(intVal(-1), list), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Negative index: walks past the list, returns Nothing.
	assertCon(t, v, "Nothing")
}

func TestReplicateImplZero(t *testing.T) {
	v, _, err := replicateImpl(ctx, ce, args(intVal(0), intVal(42)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

func TestReplicateImplNegative(t *testing.T) {
	v, _, err := replicateImpl(ctx, ce, args(intVal(-5), intVal(42)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

func TestZipImplUnequalLengths(t *testing.T) {
	xs := conList(intVal(1), intVal(2), intVal(3))
	ys := conList(intVal(10))
	v, _, err := zipImpl(ctx, ce, args(xs, ys), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 pair (min of 3, 1), got %d", len(items))
	}
}

func TestZipImplBothEmpty(t *testing.T) {
	xs := &eval.ConVal{Con: "Nil"}
	ys := &eval.ConVal{Con: "Nil"}
	v, _, err := zipImpl(ctx, ce, args(xs, ys), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

func TestUnzipImplEmpty(t *testing.T) {
	nilList := &eval.ConVal{Con: "Nil"}
	v, _, err := unzipImpl(ctx, ce, args(nilList), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := v.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T", v)
	}
	assertCon(t, rv.Fields["_1"], "Nil")
	assertCon(t, rv.Fields["_2"], "Nil")
}

func TestFoldlImplDirect(t *testing.T) {
	// foldl (\acc x -> acc + x) 0 [1,2,3] = 6
	f := &eval.Closure{Param: "acc", Body: nil}
	list := conList(intVal(1), intVal(2), intVal(3))
	applier := func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		if _, ok := fn.(*eval.Closure); ok {
			return &eval.HostVal{Inner: arg}, capEnv, nil
		}
		a := fn.(*eval.HostVal).Inner.(eval.Value).(*eval.HostVal).Inner.(int64)
		b := arg.(*eval.HostVal).Inner.(int64)
		return intVal(a + b), capEnv, nil
	}
	v, _, err := foldlImpl(ctx, ce, args(f, intVal(0), list), applier)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 6)
}

func TestFoldlImplEmpty(t *testing.T) {
	f := &eval.Closure{Param: "acc", Body: nil}
	nilList := &eval.ConVal{Con: "Nil"}
	v, _, err := foldlImpl(ctx, ce, args(f, intVal(42), nilList), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, v, 42)
}

func TestDropWhileImplEmpty(t *testing.T) {
	pred := &eval.Closure{Param: "x", Body: nil}
	nilList := &eval.ConVal{Con: "Nil"}
	v, _, err := dropWhileImpl(ctx, ce, args(pred, nilList), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

func TestSpanImplEmpty(t *testing.T) {
	pred := &eval.Closure{Param: "x", Body: nil}
	nilList := &eval.ConVal{Con: "Nil"}
	v, _, err := spanImpl(ctx, ce, args(pred, nilList), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv := v.(*eval.RecordVal)
	assertCon(t, rv.Fields["_1"], "Nil")
	assertCon(t, rv.Fields["_2"], "Nil")
}

func TestSpanImplNoneDrop(t *testing.T) {
	// span (\_ -> False) [1,2] = ([], [1,2])
	pred := &eval.Closure{Param: "x", Body: nil}
	list := conList(intVal(1), intVal(2))
	applier := boolApplier(func(eval.Value) bool { return false })
	v, _, err := spanImpl(ctx, ce, args(pred, list), applier)
	if err != nil {
		t.Fatal(err)
	}
	rv := v.(*eval.RecordVal)
	assertCon(t, rv.Fields["_1"], "Nil")
	suffix, ok := listToSlice(rv.Fields["_2"])
	if !ok {
		t.Fatal("expected list for _2")
	}
	if len(suffix) != 2 {
		t.Fatalf("expected 2 items in suffix, got %d", len(suffix))
	}
}

func TestSortByImplEmpty(t *testing.T) {
	nilList := &eval.ConVal{Con: "Nil"}
	v, _, err := sortByImpl(ctx, ce, args(&eval.Closure{Param: "a"}, nilList), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

func TestIterateNImplNegative(t *testing.T) {
	v, _, err := iterateNImpl(ctx, ce, args(intVal(-1), &eval.Closure{Param: "x"}, intVal(1)), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

func TestConcatImplFirstEmpty(t *testing.T) {
	xs := &eval.ConVal{Con: "Nil"}
	ys := conList(intVal(1), intVal(2))
	v, _, err := concatImpl(ctx, ce, args(xs, ys), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestConcatImplSecondEmpty(t *testing.T) {
	xs := conList(intVal(1))
	ys := &eval.ConVal{Con: "Nil"}
	v, _, err := concatImpl(ctx, ce, args(xs, ys), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := listToSlice(v)
	if !ok {
		t.Fatal("expected list")
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestLengthImplMalformed(t *testing.T) {
	// Non-list value should produce error.
	_, _, err := lengthImpl(ctx, ce, args(intVal(42)), nil)
	if err == nil {
		t.Fatal("expected error for non-list value")
	}
}

// --- AVL tree balance / rotation tests ---

func TestAVLRotationViaSequentialInserts(t *testing.T) {
	// Inserting in ascending order forces left rotations (right-heavy).
	// Inserting in descending order forces right rotations (left-heavy).
	// This exercises both single and double rotations.
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	// Ascending: 1,2,3,...,15 forces left rotations.
	m, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	for i := int64(1); i <= 15; i++ {
		var err error
		m, _, err = mapInsertImpl(ctx, ce, args(cmpFn, intVal(i), intVal(i*10), m), apply)
		if err != nil {
			t.Fatal(err)
		}
	}
	sv, _, _ := mapSizeImpl(ctx, ce, args(m), nil)
	assertInt(t, sv, 15)

	// Verify in-order traversal (sorted).
	sorted, _, _ := mapToListImpl(ctx, ce, args(m), nil)
	items, _ := listToSlice(sorted)
	for i, item := range items {
		pair := item.(*eval.RecordVal)
		assertInt(t, pair.Fields["_1"], int64(i+1))
	}

	// Descending: 15,14,...,1 forces right rotations.
	m2, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	for i := int64(15); i >= 1; i-- {
		var err error
		m2, _, err = mapInsertImpl(ctx, ce, args(cmpFn, intVal(i), intVal(i*10), m2), apply)
		if err != nil {
			t.Fatal(err)
		}
	}
	sv2, _, _ := mapSizeImpl(ctx, ce, args(m2), nil)
	assertInt(t, sv2, 15)
}

func TestAVLDoubleRotation(t *testing.T) {
	// Insert in zigzag order to force left-right and right-left rotations.
	// Order: 3, 1, 2 (forces left-right double rotation).
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	m, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	for _, k := range []int64{3, 1, 2} {
		var err error
		m, _, err = mapInsertImpl(ctx, ce, args(cmpFn, intVal(k), intVal(k), m), apply)
		if err != nil {
			t.Fatal(err)
		}
	}
	sorted, _, _ := mapToListImpl(ctx, ce, args(m), nil)
	items, _ := listToSlice(sorted)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	assertInt(t, items[0].(*eval.RecordVal).Fields["_1"], 1)
	assertInt(t, items[1].(*eval.RecordVal).Fields["_1"], 2)
	assertInt(t, items[2].(*eval.RecordVal).Fields["_1"], 3)
}

func TestAVLDeleteNodeWithTwoChildren(t *testing.T) {
	// Delete a node that has both left and right children.
	// This exercises avlMinNode and the successor replacement.
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	m, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	// Insert in order that creates node with two children: 2, 1, 3.
	for _, k := range []int64{2, 1, 3} {
		var err error
		m, _, err = mapInsertImpl(ctx, ce, args(cmpFn, intVal(k), intVal(k*10), m), apply)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Delete the root (key 2) which has both children.
	m2, _, err := mapDeleteImpl(ctx, ce, args(cmpFn, intVal(2), m), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ := mapSizeImpl(ctx, ce, args(m2), nil)
	assertInt(t, sv, 2)

	// Verify remaining keys.
	v1, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(1), m2), apply)
	assertCon(t, v1, "Just")
	v3, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(3), m2), apply)
	assertCon(t, v3, "Just")
	v2, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(2), m2), apply)
	assertCon(t, v2, "Nothing")
}

func TestMapValString(t *testing.T) {
	mv := &mapVal{root: nil, cmp: nil, size: 0}
	if mv.String() != "Map(...)" {
		t.Errorf("expected 'Map(...)', got %q", mv.String())
	}
}

func TestAsMapValInnerNotMap(t *testing.T) {
	// HostVal wrapping non-mapVal should error with correct message.
	hv := &eval.HostVal{Inner: "not-a-map"}
	_, err := asMapVal(hv)
	if err == nil {
		t.Fatal("expected error from asMapVal with non-map inner")
	}
}

func TestMapFromListMalformedNotList(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	// Pass a non-ConVal as the list.
	_, _, err := mapFromListImpl(ctx, ce, args(cmpFn, intVal(42)), nil)
	if err == nil {
		t.Fatal("expected error for non-list input")
	}
}

func TestMapFromListMalformedCons(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	// Malformed Cons with wrong arg count.
	badList := &eval.ConVal{Con: "Cons", Args: []eval.Value{intVal(1)}} // missing tail
	_, _, err := mapFromListImpl(ctx, ce, args(cmpFn, badList), nil)
	if err == nil {
		t.Fatal("expected error for malformed Cons")
	}
}

func TestMapFromListNonTuplePair(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	// Cons with non-RecordVal pair.
	badList := &eval.ConVal{Con: "Cons", Args: []eval.Value{intVal(42), &eval.ConVal{Con: "Nil"}}}
	_, _, err := mapFromListImpl(ctx, ce, args(cmpFn, badList), apply)
	if err == nil {
		t.Fatal("expected error for non-tuple pair")
	}
}

func TestMapFromListIncompleteTuple(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	// RecordVal without _1 and _2.
	pair := &eval.RecordVal{Fields: map[string]eval.Value{"_1": intVal(1)}}
	badList := &eval.ConVal{Con: "Cons", Args: []eval.Value{pair, &eval.ConVal{Con: "Nil"}}}
	_, _, err := mapFromListImpl(ctx, ce, args(cmpFn, badList), apply)
	if err == nil {
		t.Fatal("expected error for incomplete tuple")
	}
}

// --- List error path coverage ---

func TestFromSliceImplNonHostValNonConVal(t *testing.T) {
	// A non-HostVal, non-ConVal input should error.
	_, _, err := fromSliceImpl(ctx, ce, args(&eval.RecordVal{Fields: map[string]eval.Value{}}), nil)
	if err == nil {
		t.Fatal("expected error for non-HostVal/non-ConVal input")
	}
}

func TestFromSliceImplHostValNonSlice(t *testing.T) {
	// HostVal wrapping non-[]any should error.
	_, _, err := fromSliceImpl(ctx, ce, args(&eval.HostVal{Inner: "not-a-slice"}), nil)
	if err == nil {
		t.Fatal("expected error for HostVal wrapping non-slice")
	}
}

func TestToSliceImplPassthrough(t *testing.T) {
	// HostVal wrapping []any should pass through unchanged.
	original := &eval.HostVal{Inner: []any{int64(1), int64(2)}}
	v, _, err := toSliceImpl(ctx, ce, args(original), nil)
	if err != nil {
		t.Fatal(err)
	}
	if v != original {
		t.Fatal("expected passthrough for HostVal([]any)")
	}
}

func TestToSliceImplNonList(t *testing.T) {
	_, _, err := toSliceImpl(ctx, ce, args(intVal(42)), nil)
	if err == nil {
		t.Fatal("expected error for non-list value")
	}
}

func TestListToSliceMalformedCons(t *testing.T) {
	// Cons with wrong arg count.
	badList := &eval.ConVal{Con: "Cons", Args: []eval.Value{intVal(1)}}
	result, ok := listToSlice(badList)
	if ok {
		t.Fatalf("expected failure for malformed Cons, got %v", result)
	}
}

func TestListToSliceUnknownCon(t *testing.T) {
	// Unknown constructor (not Cons/Nil).
	unknown := &eval.ConVal{Con: "Other", Args: []eval.Value{}}
	result, ok := listToSlice(unknown)
	if ok {
		t.Fatalf("expected failure for unknown constructor, got %v", result)
	}
}

func TestBuildListEmpty(t *testing.T) {
	v := buildList(nil)
	assertCon(t, v, "Nil")
}

func TestUnzipImplNonTuple(t *testing.T) {
	// List containing a non-RecordVal element.
	list := conList(intVal(42))
	_, _, err := unzipImpl(ctx, ce, args(list), nil)
	if err == nil {
		t.Fatal("expected error for non-tuple element in unzip")
	}
}

func TestUnzipImplIncompleteTuple(t *testing.T) {
	// RecordVal with _1 but not _2.
	bad := &eval.RecordVal{Fields: map[string]eval.Value{"_1": intVal(1)}}
	list := conList(bad)
	_, _, err := unzipImpl(ctx, ce, args(list), nil)
	if err == nil {
		t.Fatal("expected error for tuple without _2")
	}
}

func TestSetFromListMalformed(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	// Non-list input.
	_, _, err := setFromListImpl(ctx, ce, args(cmpFn, intVal(42)), nil)
	if err == nil {
		t.Fatal("expected error for non-list input to setFromList")
	}
}

func TestSetFromListMalformedCons(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	badList := &eval.ConVal{Con: "Cons", Args: []eval.Value{intVal(1)}}
	_, _, err := setFromListImpl(ctx, ce, args(cmpFn, badList), nil)
	if err == nil {
		t.Fatal("expected error for malformed Cons")
	}
}

func TestLengthImplMalformedCons(t *testing.T) {
	// Cons with wrong name should error.
	bad := &eval.ConVal{Con: "Other", Args: []eval.Value{intVal(1), intVal(2)}}
	_, _, err := lengthImpl(ctx, ce, args(bad), nil)
	if err == nil {
		t.Fatal("expected error for unknown list constructor")
	}
}

func TestSliceToListRoundTrip(t *testing.T) {
	// sliceToList -> listToSlice round trip preserving wrapped HostVal items.
	items := []any{int64(1), "hello"}
	list := sliceToList(items)
	vals, ok := listToSlice(list)
	if !ok {
		t.Fatal("expected list")
	}
	if len(vals) != 2 {
		t.Fatalf("expected 2 items, got %d", len(vals))
	}
}

func TestSliceToListNonValue(t *testing.T) {
	// Items that are not eval.Value should be wrapped in HostVal.
	items := []any{42} // int, not eval.Value
	list := sliceToList(items)
	vals, ok := listToSlice(list)
	if !ok {
		t.Fatal("expected list")
	}
	hv, ok := vals[0].(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", vals[0])
	}
	if hv.Inner != 42 {
		t.Errorf("expected 42, got %v", hv.Inner)
	}
}

func TestMapPersistence(t *testing.T) {
	// AVL tree is persistent: inserting into m should not mutate the original.
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), emptyV), apply)
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), strVal("b"), m1), apply)

	// m1 should still have size 1.
	sv1, _, _ := mapSizeImpl(ctx, ce, args(m1), nil)
	assertInt(t, sv1, 1)

	// m2 should have size 2.
	sv2, _, _ := mapSizeImpl(ctx, ce, args(m2), nil)
	assertInt(t, sv2, 2)

	// Key 2 should not exist in m1.
	v, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(2), m1), apply)
	assertCon(t, v, "Nothing")
}

func TestMapDeletePersistence(t *testing.T) {
	// Deleting from m should not mutate the original.
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), emptyV), apply)
	m1, _, _ = mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), strVal("b"), m1), apply)
	m2, _, _ := mapDeleteImpl(ctx, ce, args(cmpFn, intVal(1), m1), apply)

	// m1 should still have key 1.
	v, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(1), m1), apply)
	assertCon(t, v, "Just")

	// m2 should not have key 1.
	v2, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(1), m2), apply)
	assertCon(t, v2, "Nothing")
}

func TestFoldlWithKeyEmpty(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), nil)
	foldF := &eval.Closure{Param: "acc", Body: nil}

	v, _, err := mapFoldlWithKeyImpl(ctx, ce, args(foldF, intVal(99), emptyV), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Empty map fold should return initial accumulator.
	assertInt(t, v, 99)
}

func TestCollectKeysNil(t *testing.T) {
	var keys []eval.Value
	collectKeys(nil, &keys)
	if len(keys) != 0 {
		t.Errorf("expected no keys from nil node, got %d", len(keys))
	}
}
