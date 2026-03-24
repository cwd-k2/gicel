// Stdlib string and list tests — list construction, fromSlice/toSlice, length, concat,
// charAt, split, join, dropWhile, span, sortBy, scanl, unfoldr, iterateN, reverse,
// take, drop, index, replicate, zip, unzip, foldl, and associated error/edge cases.
// Does NOT cover: stdlib_test.go, stdlib_slice_test.go, stdlib_collection_test.go.
package stdlib

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

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
	// unfoldr (\n. if n==0 then Nothing else Just (n, n-1)) 3 = [3,2,1]
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
	// span (\_. False) [1,2] = ([], [1,2])
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

