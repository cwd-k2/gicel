// Stdlib slice tests — SliceEmpty, SliceSingleton, SliceLength, SliceIndex, SliceMap,
// SliceFoldr, SliceFoldl, SliceFromList, SliceToList, FromRunes, SliceIndexExactBoundary.
// Does NOT cover: stdlib_test.go, stdlib_string_test.go, stdlib_collection_test.go.
package stdlib

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

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

func TestSliceIndexNegative(t *testing.T) {
	v, _, err := sliceIndexImpl(ctx, ce, args(intVal(-1), sliceOf(intVal(10))), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nothing")
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
	// foldr (\x acc -> x: acc) [] [1,2,3] with non-commutative op
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
