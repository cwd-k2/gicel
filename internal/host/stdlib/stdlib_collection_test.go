// Stdlib collection tests — Map (AVL), Set, MutMap, MutSet, Array, appendIO.
// Does NOT cover: stdlib_test.go, stdlib_slice_test.go, stdlib_string_test.go.
package stdlib

import (
	"fmt"
	"testing"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// --- Map primitives ---

// intCmpApplier creates an Applier that compares int64 values via Ordering.
// Handles curried style: apply(cmpFn, a) → partial, apply(partial, b) → Ordering.
// Uses HostVal with a marker struct to distinguish partials from regular values.
type intCmpPartialInner struct{ val int64 }

func intCmpApplier() eval.Applier {
	return eval.ApplierFrom(func(fn eval.Value, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
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
	})
}

func TestMapInsertLookup(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"} // dummy, Applier handles comparison directly

	// Create empty map.
	emptyV, _, err := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
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

	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), emptyV), apply)
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), strVal("b"), m1), apply)

	// Size = 2.
	sv, _, _ := mapSizeImpl(ctx, ce, args(m2), eval.Applier{})
	assertInt(t, sv, 2)

	// Delete key 1.
	m3, _, _ := mapDeleteImpl(ctx, ce, args(cmpFn, intVal(1), m2), apply)
	sv2, _, _ := mapSizeImpl(ctx, ce, args(m3), eval.Applier{})
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
		eval.NewRecordFromMap(map[string]eval.Value{"_1": intVal(2), "_2": strVal("b")}),
		eval.NewRecordFromMap(map[string]eval.Value{"_1": intVal(1), "_2": strVal("a")}),
		eval.NewRecordFromMap(map[string]eval.Value{"_1": intVal(3), "_2": strVal("c")}),
	}
	list := buildList(pairs)

	// fromList then toList: should be sorted.
	m, _, err := mapFromListImpl(ctx, ce, args(cmpFn, list), apply)
	if err != nil {
		t.Fatal(err)
	}
	sorted, _, err := mapToListImpl(ctx, ce, args(m), eval.Applier{})
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
		assertInt(t, pair.MustGet("_1"), want)
	}
}

// --- Map: additional coverage ---

func TestMapEmptySize(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, err := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	if err != nil {
		t.Fatal(err)
	}
	sv, _, err := mapSizeImpl(ctx, ce, args(emptyV), eval.Applier{})
	if err != nil {
		t.Fatal(err)
	}
	assertInt(t, sv, 0)
}

func TestMapLookupEmpty(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})

	v, _, err := mapLookupImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nothing")
}

func TestMapInsertOverwrite(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})

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
	sv, _, _ := mapSizeImpl(ctx, ce, args(m2), eval.Applier{})
	assertInt(t, sv, 1)
}

func TestMapDeleteNonexistent(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), emptyV), apply)

	// Delete key 99 (not present): size should remain 1.
	m2, _, _ := mapDeleteImpl(ctx, ce, args(cmpFn, intVal(99), m1), apply)
	sv, _, _ := mapSizeImpl(ctx, ce, args(m2), eval.Applier{})
	assertInt(t, sv, 1)
}

func TestMapDeleteEmpty(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})

	// Delete from empty map should not error.
	m2, _, err := mapDeleteImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ := mapSizeImpl(ctx, ce, args(m2), eval.Applier{})
	assertInt(t, sv, 0)
}

func TestMapMemberImpl(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
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
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), intVal(10), emptyV), apply)
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), intVal(20), m1), apply)

	// foldlWithKey (\acc k v -> acc + v) 0 {1:10, 2:20} = 30
	foldF := &eval.Closure{Param: "acc"}
	sumApplier := eval.ApplierFrom(func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
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
	})

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
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})

	// m1 = {1: 10, 2: 20}
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), intVal(10), emptyV), cmpApply)
	m1, _, _ = mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), intVal(20), m1), cmpApply)

	// m2 = {2: 5, 3: 30}
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), intVal(5), emptyV), cmpApply)
	m2, _, _ = mapInsertImpl(ctx, ce, args(cmpFn, intVal(3), intVal(30), m2), cmpApply)

	// unionWith (\a b -> a + b) m1 m2
	// On collision (key 2): 20 + 5 = 25
	addF := &eval.Closure{Param: "a"}

	// Combined applier: handles both int comparison (for AVL lookup/insert)
	// and addition (for the merge function). Distinguishes by fn type.
	type addPartial struct{ val int64 }
	unionApply := eval.ApplierFrom(func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
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
		return cmpApply.Apply(fn, arg, capEnv)
	})

	result, _, err := mapUnionWithImpl(ctx, ce, args(addF, m1, m2), unionApply)
	if err != nil {
		t.Fatal(err)
	}

	sv, _, _ := mapSizeImpl(ctx, ce, args(result), eval.Applier{})
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
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	v, _, err := mapToListImpl(ctx, ce, args(emptyV), eval.Applier{})
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
	sv, _, _ := mapSizeImpl(ctx, ce, args(m), eval.Applier{})
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
	m, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	for i := range int64(20) {
		var err error
		m, _, err = mapInsertImpl(ctx, ce, args(cmpFn, intVal(i), intVal(i*100), m), apply)
		if err != nil {
			t.Fatal(err)
		}
	}
	sv, _, _ := mapSizeImpl(ctx, ce, args(m), eval.Applier{})
	assertInt(t, sv, 20)

	// Verify all keys are found.
	for i := range int64(20) {
		v, _, err := mapLookupImpl(ctx, ce, args(cmpFn, intVal(i), m), apply)
		if err != nil {
			t.Fatal(err)
		}
		assertCon(t, v, "Just")
		assertInt(t, v.(*eval.ConVal).Args[0], i*100)
	}

	// toList should return sorted keys.
	sorted, _, _ := mapToListImpl(ctx, ce, args(m), eval.Applier{})
	items, _ := listToSlice(sorted)
	if len(items) != 20 {
		t.Fatalf("expected 20 items, got %d", len(items))
	}
	for i, want := range items {
		pair := want.(*eval.RecordVal)
		assertInt(t, pair.MustGet("_1"), int64(i))
	}
}

func TestMapUnionWithLeftBias(t *testing.T) {
	// union = unionWith (\a _. a) — first map's value must win.
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})

	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), intVal(100), emptyV), apply)
	m1, _, _ = mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), intVal(200), m1), apply)

	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), intVal(999), emptyV), apply)
	m2, _, _ = mapInsertImpl(ctx, ce, args(cmpFn, intVal(3), intVal(300), m2), apply)

	// Left-preference combiner: \a _. a
	leftF := &eval.Closure{Param: "left"}
	type leftPartial struct{ val int64 }
	leftApply := eval.ApplierFrom(func(fn eval.Value, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		if hv, ok := fn.(*eval.HostVal); ok {
			if p, ok := hv.Inner.(*leftPartial); ok {
				return intVal(p.val), capEnv, nil // return first, ignore second
			}
		}
		if _, ok := fn.(*eval.Closure); ok {
			a := arg.(*eval.HostVal).Inner.(int64)
			return &eval.HostVal{Inner: &leftPartial{val: a}}, capEnv, nil
		}
		return apply.Apply(fn, arg, capEnv)
	})

	result, _, err := mapUnionWithImpl(ctx, ce, args(leftF, m1, m2), leftApply)
	if err != nil {
		t.Fatal(err)
	}

	// Key 1: m1 has 100, m2 has 999. Left-biased → 100.
	v1, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(1), result), apply)
	assertCon(t, v1, "Just")
	assertInt(t, v1.(*eval.ConVal).Args[0], 100)

	// Key 2: only in m1 → 200.
	v2, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(2), result), apply)
	assertCon(t, v2, "Just")
	assertInt(t, v2.(*eval.ConVal).Args[0], 200)

	// Key 3: only in m2 → 300.
	v3, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(3), result), apply)
	assertCon(t, v3, "Just")
	assertInt(t, v3.(*eval.ConVal).Args[0], 300)

	sv, _, _ := mapSizeImpl(ctx, ce, args(result), eval.Applier{})
	assertInt(t, sv, 3)
}

// --- Set primitives ---

func TestSetInsertMemberSize(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	emptyV, _, err := setEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	if err != nil {
		t.Fatal(err)
	}

	// Size 0.
	sv, _, _ := setSizeImpl(ctx, ce, args(emptyV), eval.Applier{})
	assertInt(t, sv, 0)

	// Insert 1.
	s1, _, err := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ = setSizeImpl(ctx, ce, args(s1), eval.Applier{})
	assertInt(t, sv, 1)

	// Insert 2.
	s2, _, err := setInsertImpl(ctx, ce, args(cmpFn, intVal(2), s1), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ = setSizeImpl(ctx, ce, args(s2), eval.Applier{})
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
	emptyV, _, _ := setEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	s1, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)
	s2, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(2), s1), apply)

	// Delete key 1.
	s3, _, err := setDeleteImpl(ctx, ce, args(cmpFn, intVal(1), s2), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ := setSizeImpl(ctx, ce, args(s3), eval.Applier{})
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
	emptyV, _, _ := setEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	s1, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)

	// Insert duplicate: size should remain 1.
	s2, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), s1), apply)
	sv, _, _ := setSizeImpl(ctx, ce, args(s2), eval.Applier{})
	assertInt(t, sv, 1)
}

func TestSetToListSorted(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := setEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	s1, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(3), emptyV), apply)
	s2, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), s1), apply)
	s3, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(2), s2), apply)

	v, _, err := setToListImpl(ctx, ce, args(s3), eval.Applier{})
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
	sv, _, _ := setSizeImpl(ctx, ce, args(s), eval.Applier{})
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
	sv, _, _ := setSizeImpl(ctx, ce, args(s), eval.Applier{})
	assertInt(t, sv, 0)
}

func TestSetDeleteNonexistent(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := setEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	s1, _, _ := setInsertImpl(ctx, ce, args(cmpFn, intVal(1), emptyV), apply)

	// Delete non-existent key: size unchanged.
	s2, _, err := setDeleteImpl(ctx, ce, args(cmpFn, intVal(99), s1), apply)
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ := setSizeImpl(ctx, ce, args(s2), eval.Applier{})
	assertInt(t, sv, 1)
}

func TestSetToListEmpty(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := setEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	v, _, err := setToListImpl(ctx, ce, args(emptyV), eval.Applier{})
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, v, "Nil")
}

// --- AVL tree balance / rotation tests ---

func TestAVLRotationViaSequentialInserts(t *testing.T) {
	// Inserting in ascending order forces left rotations (right-heavy).
	// Inserting in descending order forces right rotations (left-heavy).
	// This exercises both single and double rotations.
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	// Ascending: 1,2,3,...,15 forces left rotations.
	m, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	for i := int64(1); i <= 15; i++ {
		var err error
		m, _, err = mapInsertImpl(ctx, ce, args(cmpFn, intVal(i), intVal(i*10), m), apply)
		if err != nil {
			t.Fatal(err)
		}
	}
	sv, _, _ := mapSizeImpl(ctx, ce, args(m), eval.Applier{})
	assertInt(t, sv, 15)

	// Verify in-order traversal (sorted).
	sorted, _, _ := mapToListImpl(ctx, ce, args(m), eval.Applier{})
	items, _ := listToSlice(sorted)
	for i, item := range items {
		pair := item.(*eval.RecordVal)
		assertInt(t, pair.MustGet("_1"), int64(i+1))
	}

	// Descending: 15,14,...,1 forces right rotations.
	m2, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	for i := int64(15); i >= 1; i-- {
		var err error
		m2, _, err = mapInsertImpl(ctx, ce, args(cmpFn, intVal(i), intVal(i*10), m2), apply)
		if err != nil {
			t.Fatal(err)
		}
	}
	sv2, _, _ := mapSizeImpl(ctx, ce, args(m2), eval.Applier{})
	assertInt(t, sv2, 15)
}

func TestAVLDoubleRotation(t *testing.T) {
	// Insert in zigzag order to force left-right and right-left rotations.
	// Order: 3, 1, 2 (forces left-right double rotation).
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	m, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	for _, k := range []int64{3, 1, 2} {
		var err error
		m, _, err = mapInsertImpl(ctx, ce, args(cmpFn, intVal(k), intVal(k), m), apply)
		if err != nil {
			t.Fatal(err)
		}
	}
	sorted, _, _ := mapToListImpl(ctx, ce, args(m), eval.Applier{})
	items, _ := listToSlice(sorted)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	assertInt(t, items[0].(*eval.RecordVal).MustGet("_1"), 1)
	assertInt(t, items[1].(*eval.RecordVal).MustGet("_1"), 2)
	assertInt(t, items[2].(*eval.RecordVal).MustGet("_1"), 3)
}

func TestAVLDeleteNodeWithTwoChildren(t *testing.T) {
	// Delete a node that has both left and right children.
	// This exercises avlMinNode and the successor replacement.
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	m, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
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
	sv, _, _ := mapSizeImpl(ctx, ce, args(m2), eval.Applier{})
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
	_, _, err := mapFromListImpl(ctx, ce, args(cmpFn, intVal(42)), eval.Applier{})
	if err == nil {
		t.Fatal("expected error for non-list input")
	}
}

func TestMapFromListMalformedCons(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	// Malformed Cons with wrong arg count.
	badList := &eval.ConVal{Con: "Cons", Args: []eval.Value{intVal(1)}} // missing tail
	_, _, err := mapFromListImpl(ctx, ce, args(cmpFn, badList), eval.Applier{})
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
	pair := eval.NewRecordFromMap(map[string]eval.Value{"_1": intVal(1)})
	badList := &eval.ConVal{Con: "Cons", Args: []eval.Value{pair, &eval.ConVal{Con: "Nil"}}}
	_, _, err := mapFromListImpl(ctx, ce, args(cmpFn, badList), apply)
	if err == nil {
		t.Fatal("expected error for incomplete tuple")
	}
}

func TestSetFromListMalformed(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	// Non-list input.
	_, _, err := setFromListImpl(ctx, ce, args(cmpFn, intVal(42)), eval.Applier{})
	if err == nil {
		t.Fatal("expected error for non-list input to setFromList")
	}
}

func TestSetFromListMalformedCons(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	badList := &eval.ConVal{Con: "Cons", Args: []eval.Value{intVal(1)}}
	_, _, err := setFromListImpl(ctx, ce, args(cmpFn, badList), eval.Applier{})
	if err == nil {
		t.Fatal("expected error for malformed Cons")
	}
}

func TestMapPersistence(t *testing.T) {
	// AVL tree is persistent: inserting into m should not mutate the original.
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), emptyV), apply)
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), strVal("b"), m1), apply)

	// m1 should still have size 1.
	sv1, _, _ := mapSizeImpl(ctx, ce, args(m1), eval.Applier{})
	assertInt(t, sv1, 1)

	// m2 should have size 2.
	sv2, _, _ := mapSizeImpl(ctx, ce, args(m2), eval.Applier{})
	assertInt(t, sv2, 2)

	// Key 2 should not exist in m1.
	v, _, _ := mapLookupImpl(ctx, ce, args(cmpFn, intVal(2), m1), apply)
	assertCon(t, v, "Nothing")
}

func TestMapDeletePersistence(t *testing.T) {
	// Deleting from m should not mutate the original.
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}

	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
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
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	foldF := &eval.Closure{Param: "acc"}

	v, _, err := mapFoldlWithKeyImpl(ctx, ce, args(foldF, intVal(99), emptyV), eval.Applier{})
	if err != nil {
		t.Fatal(err)
	}
	// Empty map fold should return initial accumulator.
	assertInt(t, v, 99)
}

func TestAvlKeysToConsListNil(t *testing.T) {
	result := avlKeysToConsList(nil)
	con, ok := result.(*eval.ConVal)
	if !ok || con.Con != "Nil" {
		t.Errorf("expected Nil from nil node, got %v", result)
	}
}

// Regression: appendIO must not alias the caller's backing array.
func TestAppendIONoAlias(t *testing.T) {
	// Pre-allocate with spare capacity so append could reuse the array.
	orig := make([]string, 1, 4)
	orig[0] = "before"
	ce := eval.NewCapEnv(map[string]any{"io": orig})

	ce2 := appendIO(ce, "after")
	got, _ := ce2.Get("io")
	buf := got.([]string)
	if len(buf) != 2 || buf[0] != "before" || buf[1] != "after" {
		t.Fatalf("unexpected io buffer: %v", buf)
	}

	// The original slice must be untouched.
	if len(orig) != 1 || orig[0] != "before" {
		t.Errorf("appendIO mutated caller's original slice: %v", orig)
	}
}

// --- Map expansion unit tests (v0.14.0) ---

func TestMapKeysValues(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(3), strVal("c"), emptyV), apply)
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), m1), apply)
	m3, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), strVal("b"), m2), apply)

	keys, _, err := mapKeysImpl(ctx, ce, args(m3), eval.Applier{})
	if err != nil {
		t.Fatal(err)
	}
	kList := collectConsList(keys)
	if len(kList) != 3 {
		t.Fatalf("keys: expected 3, got %d", len(kList))
	}
	assertInt(t, kList[0], 1)
	assertInt(t, kList[1], 2)
	assertInt(t, kList[2], 3)

	vals, _, err := mapValuesImpl(ctx, ce, args(m3), eval.Applier{})
	if err != nil {
		t.Fatal(err)
	}
	vList := collectConsList(vals)
	if len(vList) != 3 {
		t.Fatalf("values: expected 3, got %d", len(vList))
	}
	assertStr(t, vList[0], "a")
	assertStr(t, vList[1], "b")
	assertStr(t, vList[2], "c")
}

func TestMapMapValuesImpl(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), intVal(10), emptyV), apply)
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), intVal(20), m1), apply)

	// Double each value.
	doubler := &eval.HostVal{Inner: "doubler"}
	doubleApply := eval.ApplierFrom(func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		if _, ok := fn.(*eval.HostVal); ok {
			if hv, ok := arg.(*eval.HostVal); ok {
				n := hv.Inner.(int64)
				return intVal(n * 2), capEnv, nil
			}
		}
		return apply.Apply(fn, arg, capEnv)
	})
	result, _, err := mapMapValuesImpl(ctx, ce, args(doubler, m2), doubleApply)
	if err != nil {
		t.Fatal(err)
	}
	vals, _, _ := mapValuesImpl(ctx, ce, args(result), eval.Applier{})
	vList := collectConsList(vals)
	if len(vList) != 2 {
		t.Fatalf("mapValues: expected 2, got %d", len(vList))
	}
	assertInt(t, vList[0], 20)
	assertInt(t, vList[1], 40)
}

func TestMapFilterWithKeyImpl(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	m1, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(1), intVal(10), emptyV), apply)
	m2, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(2), intVal(20), m1), apply)
	m3, _, _ := mapInsertImpl(ctx, ce, args(cmpFn, intVal(3), intVal(30), m2), apply)

	// Keep only entries where key > 1.
	// The apply function must handle BOTH comparison (for AVL tree operations)
	// and the filter predicate application.
	type filterPartial struct{ key eval.Value }
	pred := &eval.HostVal{Inner: "pred"}
	baseApply := intCmpApplier()
	filterApply := eval.ApplierFrom(func(fn, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		// Handle predicate applications.
		if fp, ok := fn.(*eval.HostVal); ok {
			if fp.Inner == "pred" {
				// First application: key → partial.
				return &eval.HostVal{Inner: &filterPartial{key: arg}}, capEnv, nil
			}
			if p, ok := fp.Inner.(*filterPartial); ok {
				// Second application: value (ignored, decide by key).
				k := p.key.(*eval.HostVal).Inner.(int64)
				if k > 1 {
					return &eval.ConVal{Con: "True"}, capEnv, nil
				}
				return &eval.ConVal{Con: "False"}, capEnv, nil
			}
		}
		// Fall through to comparison applier for AVL operations.
		return baseApply.Apply(fn, arg, capEnv)
	})
	result, _, err := mapFilterWithKeyImpl(ctx, ce, args(pred, m3), filterApply)
	if err != nil {
		t.Fatal(err)
	}
	rm := result.(*eval.HostVal).Inner.(*mapVal)
	if rm.size != 2 {
		t.Errorf("filterWithKey: expected size 2, got %d", rm.size)
	}
}

func TestMapKeysEmpty(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	emptyV, _, _ := mapEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	keys, _, err := mapKeysImpl(ctx, ce, args(emptyV), eval.Applier{})
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, keys, "Nil")
}

// --- Set expansion unit tests (v0.14.0) ---

func buildSet(t *testing.T, vals ...int64) eval.Value {
	t.Helper()
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	s, _, _ := setEmptyImpl(ctx, ce, args(cmpFn), eval.Applier{})
	for _, v := range vals {
		var err error
		s, _, err = setInsertImpl(ctx, ce, args(cmpFn, intVal(v), s), apply)
		if err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func setSize(t *testing.T, s eval.Value) int {
	t.Helper()
	m := s.(*eval.HostVal).Inner.(*mapVal)
	return m.size
}

func TestSetUnionImpl(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	apply := intCmpApplier()
	s1 := buildSet(t, 1, 2, 3)
	s2 := buildSet(t, 2, 3, 4)
	result, _, err := setUnionImpl(ctx, ce, args(cmpFn, s1, s2), apply)
	if err != nil {
		t.Fatal(err)
	}
	if sz := setSize(t, result); sz != 4 {
		t.Errorf("union: expected size 4, got %d", sz)
	}
}

func TestSetIntersectionImpl(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	apply := intCmpApplier()
	s1 := buildSet(t, 1, 2, 3)
	s2 := buildSet(t, 2, 3, 4)
	result, _, err := setIntersectionImpl(ctx, ce, args(cmpFn, s1, s2), apply)
	if err != nil {
		t.Fatal(err)
	}
	if sz := setSize(t, result); sz != 2 {
		t.Errorf("intersection: expected size 2, got %d", sz)
	}
}

func TestSetIntersectionDisjoint(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	apply := intCmpApplier()
	s1 := buildSet(t, 1, 2)
	s2 := buildSet(t, 3, 4)
	result, _, err := setIntersectionImpl(ctx, ce, args(cmpFn, s1, s2), apply)
	if err != nil {
		t.Fatal(err)
	}
	if sz := setSize(t, result); sz != 0 {
		t.Errorf("disjoint intersection: expected size 0, got %d", sz)
	}
}

func TestSetDifferenceImpl(t *testing.T) {
	cmpFn := &eval.HostVal{Inner: "cmp"}
	apply := intCmpApplier()
	s1 := buildSet(t, 1, 2, 3)
	s2 := buildSet(t, 2)
	result, _, err := setDifferenceImpl(ctx, ce, args(cmpFn, s1, s2), apply)
	if err != nil {
		t.Fatal(err)
	}
	if sz := setSize(t, result); sz != 2 {
		t.Errorf("difference: expected size 2, got %d", sz)
	}
}

// --- MutMap unit tests (v0.17.0) ---

func TestMutMapInsertLookupSize(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	mV, _, _ := mmapNewImpl(ctx, ce, args(cmpFn), eval.Applier{})
	mmapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), mV), apply)
	mmapInsertImpl(ctx, ce, args(cmpFn, intVal(2), strVal("b"), mV), apply)

	sizeV, _, _ := mmapSizeImpl(ctx, ce, args(mV), eval.Applier{})
	assertInt(t, sizeV, 2)

	lookV, _, _ := mmapLookupImpl(ctx, ce, args(cmpFn, intVal(1), mV), apply)
	assertCon(t, lookV, "Just")
	assertStr(t, lookV.(*eval.ConVal).Args[0], "a")

	missV, _, _ := mmapLookupImpl(ctx, ce, args(cmpFn, intVal(99), mV), apply)
	assertCon(t, missV, "Nothing")
}

func TestMutMapDeleteSize(t *testing.T) {
	apply := intCmpApplier()
	cmpFn := &eval.HostVal{Inner: "cmp"}
	mV, _, _ := mmapNewImpl(ctx, ce, args(cmpFn), eval.Applier{})
	mmapInsertImpl(ctx, ce, args(cmpFn, intVal(1), strVal("a"), mV), apply)
	mmapInsertImpl(ctx, ce, args(cmpFn, intVal(2), strVal("b"), mV), apply)
	mmapDeleteImpl(ctx, ce, args(cmpFn, intVal(1), mV), apply)

	sizeV, _, _ := mmapSizeImpl(ctx, ce, args(mV), eval.Applier{})
	assertInt(t, sizeV, 1)

	delV, _, _ := mmapLookupImpl(ctx, ce, args(cmpFn, intVal(1), mV), apply)
	assertCon(t, delV, "Nothing")
}

// --- Array unit tests (v0.17.0) ---

func TestArrayNewReadWrite(t *testing.T) {
	arrV, _, err := arrayNewImpl(ctx, ce, args(intVal(3), intVal(0)), eval.Applier{})
	if err != nil {
		t.Fatal(err)
	}
	sizeV, _, _ := arraySizeImpl(ctx, ce, args(arrV), eval.Applier{})
	assertInt(t, sizeV, 3)

	// Write 42 at index 1.
	arrayWriteImpl(ctx, ce, args(intVal(1), intVal(42), arrV), eval.Applier{})

	readV, _, _ := arrayReadImpl(ctx, ce, args(intVal(1), arrV), eval.Applier{})
	assertCon(t, readV, "Just")
	assertInt(t, readV.(*eval.ConVal).Args[0], 42)

	// Out of bounds → Nothing.
	oobV, _, _ := arrayReadImpl(ctx, ce, args(intVal(10), arrV), eval.Applier{})
	assertCon(t, oobV, "Nothing")

	// Negative index → Nothing.
	negV, _, _ := arrayReadImpl(ctx, ce, args(intVal(-1), arrV), eval.Applier{})
	assertCon(t, negV, "Nothing")
}

func TestArrayResize(t *testing.T) {
	arrV, _, _ := arrayNewImpl(ctx, ce, args(intVal(2), intVal(0)), eval.Applier{})
	arrayWriteImpl(ctx, ce, args(intVal(0), intVal(10), arrV), eval.Applier{})
	arrayWriteImpl(ctx, ce, args(intVal(1), intVal(20), arrV), eval.Applier{})

	resized, _, err := arrayResizeImpl(ctx, ce, args(intVal(4), intVal(99), arrV), eval.Applier{})
	if err != nil {
		t.Fatal(err)
	}
	sizeV, _, _ := arraySizeImpl(ctx, ce, args(resized), eval.Applier{})
	assertInt(t, sizeV, 4)

	// Preserved values.
	r0, _, _ := arrayReadImpl(ctx, ce, args(intVal(0), resized), eval.Applier{})
	assertCon(t, r0, "Just")
	assertInt(t, r0.(*eval.ConVal).Args[0], 10)

	// Fill value.
	r3, _, _ := arrayReadImpl(ctx, ce, args(intVal(3), resized), eval.Applier{})
	assertCon(t, r3, "Just")
	assertInt(t, r3.(*eval.ConVal).Args[0], 99)
}

func TestArrayNegativeSize(t *testing.T) {
	_, _, err := arrayNewImpl(ctx, ce, args(intVal(-1), intVal(0)), eval.Applier{})
	if err == nil {
		t.Error("expected error for negative array size")
	}
}

// collectConsList walks a Cons/Nil list and collects elements.
func collectConsList(v eval.Value) []eval.Value {
	var out []eval.Value
	for {
		con, ok := v.(*eval.ConVal)
		if !ok || con.Con == "Nil" {
			return out
		}
		if con.Con != "Cons" || len(con.Args) < 2 {
			return out
		}
		out = append(out, con.Args[0])
		v = con.Args[1]
	}
}
