package stdlib

import (
	"context"
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

func conList(vals ...eval.Value) eval.Value {
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := len(vals) - 1; i >= 0; i-- {
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{vals[i], result}}
	}
	return result
}

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
