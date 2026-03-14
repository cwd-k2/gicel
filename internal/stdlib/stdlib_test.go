package stdlib

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/eval"
)

// Helper constructors.
func intVal(n int64) eval.Value         { return &eval.HostVal{Inner: n} }
func strVal(s string) eval.Value        { return &eval.HostVal{Inner: s} }
func runeVal(r rune) eval.Value         { return &eval.HostVal{Inner: r} }
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
	_, err := asInt64(strVal("oops"))
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
