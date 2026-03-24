//go:build probe

// Stdlib collection probe tests — Show instances, Data.Map/Set operations,
// Effect.Map/Set/Array mutable collection operations.
// Does NOT cover: stdlib_test.go (basic edge cases), effect_test.go (state/fail interactions).
package probe_test

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe E: Show instances for collection types
// ===================================================================

func TestProbeE_ShowMap(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Map
main := show (insert 1 "a" (insert 2 "b" (empty :: Map Int String)))
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T", hv.Inner)
	}
	if !strings.Contains(s, "Map.fromList") {
		t.Fatalf("expected Show Map to contain 'Map.fromList', got %q", s)
	}
}

func TestProbeE_ShowSet(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Set
main := show (insert 3 (insert 1 (insert 2 (empty :: Set Int))))
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T", hv.Inner)
	}
	if !strings.Contains(s, "Set.fromList") {
		t.Fatalf("expected Show Set to contain 'Set.fromList', got %q", s)
	}
}

func TestProbeE_ShowSlice(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Slice
main := show (fromList (Cons 1 (Cons 2 (Cons 3 Nil))) :: Slice Int)
`, gicel.Prelude, gicel.DataSlice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T", hv.Inner)
	}
	if !strings.Contains(s, "Slice.fromList") {
		t.Fatalf("expected Show Slice to contain 'Slice.fromList', got %q", s)
	}
}

// ===================================================================
// Probe E: Data.Set methods — fold, filter, map
// ===================================================================

func TestProbeE_SetFold(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Set
s := insert 1 (insert 2 (insert 3 (empty :: Set Int)))
main := fold (+) 0 s
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 6)
}

func TestProbeE_SetFilter(t *testing.T) {
	// Filter keeps only even elements.
	v, err := pdRun(t, `
import Prelude
import Data.Set
s := insert 1 (insert 2 (insert 3 (insert 4 (empty :: Set Int))))
main := size (filter (\x. mod x 2 == 0) s)
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 2)
}

func TestProbeE_SetMap(t *testing.T) {
	// Map doubles each element.
	v, err := pdRun(t, `
import Prelude
import Data.Set
s := insert 1 (insert 2 (insert 3 (empty :: Set Int)))
main := fold (+) 0 (map (* 2) s)
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 12) // 2 + 4 + 6 = 12
}

// ===================================================================
// Probe E: Effect.Map — mutable ordered map operations
// ===================================================================

func TestProbeE_MMapBasicOps(t *testing.T) {
	// Insert two entries, lookup both, verify size.
	v, err := pdRun(t, `
import Prelude
import Effect.Map as MMap
main := do {
  m <- MMap.new;
  MMap.insert 1 "one" m;
  MMap.insert 2 "two" m;
  v1 <- MMap.lookup 1 m;
  v2 <- MMap.lookup 2 m;
  s := MMap.size m;
  pure (s, v1, v2)
}
`, gicel.Prelude, gicel.EffectMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	pdAssertInt(t, rec.Fields["_1"], 2)
	pdAssertCon(t, rec.Fields["_2"], "Just")
	pdAssertCon(t, rec.Fields["_3"], "Just")
}

func TestProbeE_MMapDeleteAndMember(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Effect.Map as MMap
main := do {
  m <- MMap.new;
  MMap.insert 1 "one" m;
  MMap.insert 2 "two" m;
  before <- MMap.member 1 m;
  MMap.delete 1 m;
  after <- MMap.member 1 m;
  s := MMap.size m;
  pure (before, after, s)
}
`, gicel.Prelude, gicel.EffectMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	pdAssertCon(t, rec.Fields["_1"], "True")
	pdAssertCon(t, rec.Fields["_2"], "False")
	pdAssertInt(t, rec.Fields["_3"], 1)
}

func TestProbeE_MMapFromListToList(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Effect.Map as MMap
main := do {
  m <- MMap.fromList (Cons (1, "a") (Cons (2, "b") (Cons (3, "c") Nil)));
  s := MMap.size m;
  entries <- MMap.toList m;
  pure (s, length entries)
}
`, gicel.Prelude, gicel.EffectMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	pdAssertInt(t, rec.Fields["_1"], 3)
	pdAssertInt(t, rec.Fields["_2"], 3)
}

func TestProbeE_MMapAdjust(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Effect.Map as MMap
main := do {
  m <- MMap.new;
  MMap.insert 1 10 m;
  MMap.adjust 1 (+ 5) m;
  MMap.lookup 1 m
}
`, gicel.Prelude, gicel.EffectMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	pdAssertInt(t, con.Args[0], 15)
}

// ===================================================================
// Probe E: Effect.Set — mutable ordered set operations
// ===================================================================

func TestProbeE_MSetBasicOps(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Effect.Set as MSet
main := do {
  s <- MSet.new;
  MSet.insert 1 s;
  MSet.insert 2 s;
  MSet.insert 3 s;
  MSet.insert 1 s;
  has2 <- MSet.member 2 s;
  has9 <- MSet.member 9 s;
  n := MSet.size s;
  pure (n, has2, has9)
}
`, gicel.Prelude, gicel.EffectSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	pdAssertInt(t, rec.Fields["_1"], 3) // duplicate insert collapsed
	pdAssertCon(t, rec.Fields["_2"], "True")
	pdAssertCon(t, rec.Fields["_3"], "False")
}

func TestProbeE_MSetDeleteAndSize(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Effect.Set as MSet
main := do {
  s <- MSet.new;
  MSet.insert 10 s;
  MSet.insert 20 s;
  MSet.insert 30 s;
  MSet.delete 20 s;
  after <- MSet.member 20 s;
  n := MSet.size s;
  pure (after, n)
}
`, gicel.Prelude, gicel.EffectSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	pdAssertCon(t, rec.Fields["_1"], "False")
	pdAssertInt(t, rec.Fields["_2"], 2)
}

func TestProbeE_MSetFromListToList(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Effect.Set as MSet
main := do {
  s <- MSet.fromList (Cons 5 (Cons 3 (Cons 1 (Cons 3 Nil))));
  n := MSet.size s;
  xs <- MSet.toList s;
  pure (n, length xs)
}
`, gicel.Prelude, gicel.EffectSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	pdAssertInt(t, rec.Fields["_1"], 3) // duplicate 3 collapsed
	pdAssertInt(t, rec.Fields["_2"], 3)
}

// ===================================================================
// Probe E: Effect.Array — mutable fixed-size array operations
// ===================================================================

func TestProbeE_ArrayNewAndSize(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Effect.Array
main := do {
  arr <- new 5 0;
  s := size arr;
  pure s
}
`, gicel.Prelude, gicel.EffectArray)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 5)
}

func TestProbeE_ArrayReadWriteAt(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Effect.Array
main := do {
  arr <- new 3 0;
  writeAt 0 10 arr;
  writeAt 1 20 arr;
  writeAt 2 30 arr;
  v0 <- readAt 0 arr;
  v1 <- readAt 1 arr;
  v2 <- readAt 2 arr;
  pure (v0, v1, v2)
}
`, gicel.Prelude, gicel.EffectArray)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	// readAt returns Maybe a — each should be Just n.
	for _, field := range []struct {
		name string
		val  int64
	}{
		{"_1", 10},
		{"_2", 20},
		{"_3", 30},
	} {
		con, ok := rec.Fields[field.name].(*gicel.ConVal)
		if !ok || con.Con != "Just" {
			t.Fatalf("expected Just for %s, got %v", field.name, rec.Fields[field.name])
		}
		pdAssertInt(t, con.Args[0], field.val)
	}
}

func TestProbeE_ArrayFromSliceToSlice(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Slice as S
import Effect.Array
sl := fromList (Cons 10 (Cons 20 (Cons 30 Nil))) :: Slice Int
main := do {
  arr <- fromSlice sl;
  s := size arr;
  sl2 <- toSlice arr;
  n := S.length sl2;
  pure (s, n)
}
`, gicel.Prelude, gicel.DataSlice, gicel.EffectArray)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	pdAssertInt(t, rec.Fields["_1"], 3)
	pdAssertInt(t, rec.Fields["_2"], 3)
}

func TestProbeE_ArrayResize(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Effect.Array
main := do {
  arr <- new 2 0;
  writeAt 0 42 arr;
  writeAt 1 99 arr;
  arr2 <- resize 4 0 arr;
  s := size arr2;
  v0 <- readAt 0 arr2;
  v3 <- readAt 3 arr2;
  pure (s, v0, v3)
}
`, gicel.Prelude, gicel.EffectArray)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	pdAssertInt(t, rec.Fields["_1"], 4)
	// Original element at index 0 preserved.
	con0, ok := rec.Fields["_2"].(*gicel.ConVal)
	if !ok || con0.Con != "Just" {
		t.Fatalf("expected Just for _2, got %v", rec.Fields["_2"])
	}
	pdAssertInt(t, con0.Args[0], 42)
	// New element at index 3 should be the fill value 0.
	con3, ok := rec.Fields["_3"].(*gicel.ConVal)
	if !ok || con3.Con != "Just" {
		t.Fatalf("expected Just for _3, got %v", rec.Fields["_3"])
	}
	pdAssertInt(t, con3.Args[0], 0)
}

func TestProbeE_ArrayBoundsError(t *testing.T) {
	// Reading out of bounds should return Nothing, not panic.
	v, err := pdRun(t, `
import Prelude
import Effect.Array
main := do {
  arr <- new 3 0;
  readAt 99 arr
}
`, gicel.Prelude, gicel.EffectArray)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "Nothing")
}

// ===================================================================
// Probe E: Data.Map expansion — keys, values, mapValues, filterWithKey
// ===================================================================

func TestProbeE_MapKeysValues(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Map
m := insert 3 "c" (insert 1 "a" (insert 2 "b" (empty :: Map Int String)))
main := (length (keys m), length (values m))
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	pdAssertInt(t, rec.Fields["_1"], 3)
	pdAssertInt(t, rec.Fields["_2"], 3)
}

func TestProbeE_MapMapValues(t *testing.T) {
	// mapValues doubles all values; verify via lookup.
	v, err := pdRun(t, `
import Prelude
import Data.Map
m := insert 1 10 (insert 2 20 (empty :: Map Int Int))
m2 := mapValues (* 2) m
main := (lookup 1 m2, lookup 2 m2)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	for _, field := range []struct {
		name string
		val  int64
	}{
		{"_1", 20},
		{"_2", 40},
	} {
		con, ok := rec.Fields[field.name].(*gicel.ConVal)
		if !ok || con.Con != "Just" {
			t.Fatalf("expected Just for %s, got %v", field.name, rec.Fields[field.name])
		}
		pdAssertInt(t, con.Args[0], field.val)
	}
}

func TestProbeE_MapFilterWithKey(t *testing.T) {
	// Keep only entries where key > 1.
	v, err := pdRun(t, `
import Prelude
import Data.Map
m := insert 1 "a" (insert 2 "b" (insert 3 "c" (empty :: Map Int String)))
main := size (filterWithKey (\k v. k > 1) m)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 2)
}

// ===================================================================
// Probe E: Data.Set expansion — union, intersection, difference
// ===================================================================

func TestProbeE_SetUnion(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Set
s1 := insert 1 (insert 2 (insert 3 (empty :: Set Int)))
s2 := insert 3 (insert 4 (insert 5 (empty :: Set Int)))
main := size (union s1 s2)
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 5) // {1, 2, 3, 4, 5}
}

func TestProbeE_SetIntersection(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Set
s1 := insert 1 (insert 2 (insert 3 (empty :: Set Int)))
s2 := insert 2 (insert 3 (insert 4 (empty :: Set Int)))
main := size (intersection s1 s2)
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 2) // {2, 3}
}

func TestProbeE_SetDifference(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Set
s1 := insert 1 (insert 2 (insert 3 (empty :: Set Int)))
s2 := insert 2 (insert 3 (insert 4 (empty :: Set Int)))
main := size (difference s1 s2)
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 1) // {1}
}
