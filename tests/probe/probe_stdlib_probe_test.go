//go:build probe

// Stdlib correctness tests: list, map, set, show, string, numeric edge cases.
package probe_test

import (
	"math"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe C: Stdlib correctness — list, map, set, show edge cases
// ===================================================================

func TestProbeC_Stdlib_HeadEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := head Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nothing")
}

func TestProbeC_Stdlib_TailEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := tail Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nothing")
}

func TestProbeC_Stdlib_NullEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := null Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "True")
}

func TestProbeC_Stdlib_NullSingletonList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := null (Cons 1 Nil)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "False")
}

func TestProbeC_Stdlib_MapEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := map (\x. x + 1) Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nil")
}

func TestProbeC_Stdlib_FilterEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := filter (\x. True) Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nil")
}

func TestProbeC_Stdlib_FoldlEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := foldl (\acc x. acc + x) 0 Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

func TestProbeC_Stdlib_FoldrEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := foldr (\x acc. x + acc) 0 Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

func TestProbeC_Stdlib_ReverseEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := reverse Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nil")
}

func TestProbeC_Stdlib_ReverseSingletonList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := reverse (Cons 42 Nil)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	if len(con.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(con.Args))
	}
	hv, ok := con.Args[0].(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", con.Args[0])
	}
	if hv.Inner != int64(42) {
		t.Fatalf("expected 42, got %v", hv.Inner)
	}
}

func TestProbeC_Stdlib_LengthEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := length Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

func TestProbeC_Stdlib_ShowInt(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show 0
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "0")
}

func TestProbeC_Stdlib_ShowNegativeInt(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := showInt (negate 42)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "-42")
}

func TestProbeC_Stdlib_ShowBoolTrue(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show True
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "True")
}

func TestProbeC_Stdlib_ShowUnit(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show ()
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "()")
}

func TestProbeC_Stdlib_ShowMaybeJust(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show (Just 42)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T", hv.Inner)
	}
	if !strings.Contains(s, "Just") || !strings.Contains(s, "42") {
		t.Fatalf("expected show (Just 42) to contain 'Just' and '42', got %q", s)
	}
}

func TestProbeC_Stdlib_ShowNothing(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show (Nothing :: Maybe Int)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T", hv.Inner)
	}
	if !strings.Contains(s, "Nothing") {
		t.Fatalf("expected 'Nothing' in show output, got %q", s)
	}
}

func TestProbeC_Stdlib_ShowListEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show (Nil :: List Int)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T", hv.Inner)
	}
	// Should represent an empty list somehow (e.g. "[]" or "Nil").
	if s == "" {
		t.Fatal("show of empty list produced empty string")
	}
}

func TestProbeC_Stdlib_DivisionByZero(t *testing.T) {
	// Division by zero should produce an error, not a panic.
	_, err := probeRun(t, `
import Prelude
main := div 1 0
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected error from division by zero")
	}
	// Verify no panic propagation — the error itself is sufficient.
}

func TestProbeC_Stdlib_ModByZero(t *testing.T) {
	_, err := probeRun(t, `
import Prelude
main := mod 1 0
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected error from mod by zero")
	}
}

func TestProbeC_Stdlib_StringAppendEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := append "" ""
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "")
}

func TestProbeC_Stdlib_StrLenEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := strlen ""
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

// ===================================================================
// Probe C: Data.Slice edge cases
// ===================================================================

func TestProbeC_Stdlib_SliceEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Slice
main := length (empty :: Slice Int)
`, gicel.Prelude, gicel.DataSlice)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

func TestProbeC_Stdlib_SliceSingleton(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Slice
main := length (singleton 42)
`, gicel.Prelude, gicel.DataSlice)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 1)
}

// ===================================================================
// Probe C: Data.Map edge cases
// ===================================================================

func TestProbeC_Stdlib_MapEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Map
main := size (empty :: Map Int Int)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

func TestProbeC_Stdlib_MapInsertLookup(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Map
main := lookup 1 (insert 1 42 (empty :: Map Int Int))
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
}

func TestProbeC_Stdlib_MapLookupMissing(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Map
main := lookup 99 (empty :: Map Int Int)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nothing")
}

// ===================================================================
// Probe C: Data.Set edge cases
// ===================================================================

func TestProbeC_Stdlib_SetEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Set
main := size (empty :: Set Int)
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

// ===================================================================
// Probe D: Numeric edge cases
// ===================================================================

func TestProbeD_Num_IntOverflowAdd(t *testing.T) {
	// Int arithmetic wraps on overflow (Go int64 two's-complement semantics).
	// This is specified behavior, not a bug.
	v, err := pdRun(t, `
import Prelude
main := 9223372036854775807 + 1
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Go wraps int64: max + 1 = min
	pdAssertInt(t, v, math.MinInt64)
}

func TestProbeD_Num_IntOverflowMul(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := 9223372036854775807 * 2
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Go wraps: MaxInt64 * 2 = -2
	pdAssertInt(t, v, -2)
}

func TestProbeD_Num_DivByZero(t *testing.T) {
	_, err := pdRun(t, `
import Prelude
main := 10 / 0
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected error from division by zero")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected 'division by zero' error, got: %v", err)
	}
}

func TestProbeD_Num_ModByZero(t *testing.T) {
	_, err := pdRun(t, `
import Prelude
main := mod 10 0
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected error from modulo by zero")
	}
	if !strings.Contains(err.Error(), "modulo by zero") {
		t.Fatalf("expected 'modulo by zero' error, got: %v", err)
	}
}

func TestProbeD_Num_NegativeModulo(t *testing.T) {
	// Go's % returns result with sign of dividend.
	v, err := pdRun(t, `
import Prelude
main := mod (negate 7) 3
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, -1) // Go: (-7) % 3 == -1
}

func TestProbeD_Num_DoubleDivByZero(t *testing.T) {
	// IEEE 754: 1.0 / 0.0 = +Inf, no error.
	v, err := pdRun(t, `
import Prelude
main := 1.0 / 0.0
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", v)
	}
	f, ok := hv.Inner.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T", hv.Inner)
	}
	if !math.IsInf(f, 1) {
		t.Fatalf("expected +Inf, got %v", f)
	}
}

func TestProbeD_Num_DoubleNaN(t *testing.T) {
	// 0.0 / 0.0 = NaN
	v, err := pdRun(t, `
import Prelude
main := 0.0 / 0.0
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", v)
	}
	f, ok := hv.Inner.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T", hv.Inner)
	}
	if !math.IsNaN(f) {
		t.Fatalf("expected NaN, got %v", f)
	}
}

func TestProbeD_Num_NaNEquality(t *testing.T) {
	// NaN == NaN should be False (IEEE 754).
	v, err := pdRun(t, `
import Prelude
nan := 0.0 / 0.0
main := nan == nan
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "False")
}

// ===================================================================
// Probe D: String edge cases
// ===================================================================

func TestProbeD_Str_EmptyConcat(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := "" <> ""
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertString(t, v, "")
}

func TestProbeD_Str_EmptyLength(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := strlen ""
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_Str_EmptyEq(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := "" == ""
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "True")
}

func TestProbeD_Str_UnicodeLength(t *testing.T) {
	// strlen counts runes, not bytes.
	v, err := pdRun(t, `
import Prelude
main := strlen "hello"
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 5 runes (each 1 char in this case)
	pdAssertInt(t, v, 5)
}

func TestProbeD_Str_ConcatIdentity(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := "abc" <> ""
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertString(t, v, "abc")
}

func TestProbeD_Str_SplitEmpty(t *testing.T) {
	// Splitting empty string by comma.
	v, err := pdRun(t, `
import Prelude
main := length (split "," "")
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// strings.Split("", ",") returns [""], so length = 1
	pdAssertInt(t, v, 1)
}

func TestProbeD_Str_JoinEmptyList(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := join "," Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertString(t, v, "")
}

func TestProbeD_Str_CharAtBeyondEnd(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := charAt 100 "hi"
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "Nothing")
}

func TestProbeD_Str_SubstringNegativeStart(t *testing.T) {
	// substring with negative start should clamp to 0.
	v, err := pdRun(t, `
import Prelude
main := substring (negate 5) 3 "abcdef"
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertString(t, v, "abc")
}

// ===================================================================
// Probe D: List edge cases
// ===================================================================

func TestProbeD_List_FoldlEmpty(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := foldl (\acc x. acc + x) 0 Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_LengthEmpty(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (Nil :: List Int)
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_HeadEmpty(t *testing.T) {
	// index 0 of empty list should be Nothing.
	v, err := pdRun(t, `
import Prelude
main := index 0 (Nil :: List Int)
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "Nothing")
}

func TestProbeD_List_TakeZero(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (take 0 (Cons 1 (Cons 2 Nil)))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_TakeMoreThanLength(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (take 100 (Cons 1 (Cons 2 Nil)))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 2) // take clips to actual length
}

func TestProbeD_List_DropMoreThanLength(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (drop 100 (Cons 1 (Cons 2 Nil)))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_ReverseEmpty(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (reverse (Nil :: List Int))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_ReverseSingleton(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := reverse (Cons 42 Nil)
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	pdAssertInt(t, con.Args[0], 42)
}

func TestProbeD_List_ConcatEmpty(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (concat (Nil :: List Int) (Nil :: List Int))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_ZipDifferentLengths(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (zip (Cons 1 (Cons 2 (Cons 3 Nil))) (Cons 10 Nil))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 1) // zip truncates to shorter list
}

func TestProbeD_List_ReplicateZero(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (replicate 0 42)
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_ReplicateNegative(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (replicate (negate 5) 42)
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0) // negative count produces empty list
}

// ===================================================================
// Probe D: Map/Set edge cases
// ===================================================================

func TestProbeD_Map_EmptySize(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Map
main := size (empty :: Map Int Int)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_Map_LookupMissing(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Map
main := lookup 42 (empty :: Map Int String)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "Nothing")
}

func TestProbeD_Map_DuplicateKeys(t *testing.T) {
	// Inserting same key twice should update, not duplicate.
	v, err := pdRun(t, `
import Prelude
import Data.Map
m := insert 1 "b" (insert 1 "a" (empty :: Map Int String))
main := (size m, lookup 1 m)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	pdAssertInt(t, rec.MustGet("_1"), 1) // size = 1, not 2
	justV, ok := rec.MustGet("_2").(*gicel.ConVal)
	if !ok || justV.Con != "Just" {
		t.Fatalf("expected Just, got %v", rec.MustGet("_2"))
	}
	pdAssertString(t, justV.Args[0], "b") // latest value wins
}

func TestProbeD_Map_DeleteFromEmpty(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Map
main := size (delete 1 (empty :: Map Int Int))
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_Set_EmptySize(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Set
main := size (empty :: Set Int)
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_Set_DuplicateInsert(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Set
main := size (insert 1 (insert 1 (empty :: Set Int)))
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 1) // duplicates collapsed
}

func TestProbeD_Set_MemberMissing(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Set
main := member 99 (insert 1 (empty :: Set Int))
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "False")
}
