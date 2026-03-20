// Value conversion tests — ToValue, FromHost, MustHost, FromRecord, FromCon, type helpers.
// Does NOT cover: runtime behavior (engine_host_api_test.go), boundary errors (engine_boundary_test.go).

package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

func TestValueConversion(t *testing.T) {
	// HostVal wraps arbitrary Go values.
	hv := &eval.HostVal{Inner: "hello"}
	if hv.Inner != "hello" {
		t.Errorf("expected HostVal.Inner = hello, got %v", hv.Inner)
	}
	if hv.String() != "HostVal(hello)" {
		t.Errorf("unexpected HostVal.String(): %s", hv.String())
	}

	// ConVal represents constructors.
	cv := &eval.ConVal{Con: "True"}
	if cv.String() != "True" {
		t.Errorf("expected True, got %s", cv.String())
	}

	// ConVal with arguments.
	cv2 := &eval.ConVal{Con: "Just", Args: []eval.Value{&eval.ConVal{Con: "True"}}}
	s := cv2.String()
	if !strings.Contains(s, "Just") || !strings.Contains(s, "True") {
		t.Errorf("expected (Just True), got %s", s)
	}
}

// ToValue and FromBool round-trip.
func TestToValueFromBool(t *testing.T) {
	v := ToValue(true)
	b, ok := FromBool(v)
	if !ok || b != true {
		t.Errorf("ToValue(true) -> FromBool failed")
	}
	v2 := ToValue(false)
	b2, ok := FromBool(v2)
	if !ok || b2 != false {
		t.Errorf("ToValue(false) -> FromBool failed")
	}
}

// ToValue(nil) produces ().
func TestToValueNil(t *testing.T) {
	v := ToValue(nil)
	rv, ok := v.(*eval.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("ToValue(nil) should produce (), got %s", v)
	}
}

// FromHost on non-HostVal returns ok=false.
func TestFromHostNonHost(t *testing.T) {
	v := ToValue(true) // ConVal, not HostVal
	_, ok := FromHost(v)
	if ok {
		t.Error("FromHost on ConVal should return ok=false")
	}
}

// MustHost panics on wrong type.
func TestMustHostPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from MustHost on ConVal")
		}
	}()
	MustHost[int](ToValue(true))
}

func TestRuntimeErrorType(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.Fail)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.Fail
main := do { _ <- fail; pure True }
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected runtime error from fail")
	}
	var rtErr *eval.RuntimeError
	if !errors.As(err, &rtErr) {
		t.Errorf("expected RuntimeError, got %T: %v", err, err)
	}
}

func TestFromRecord(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := { x: 1, y: 2 }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	fields, ok := FromRecord(result.Value)
	if !ok {
		t.Fatal("expected record value")
	}
	if len(fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(fields))
	}
	if _, ok := fields["x"]; !ok {
		t.Error("missing field x")
	}
}

func TestFromRecordNonRecord(t *testing.T) {
	_, ok := FromRecord(ToValue(42))
	if ok {
		t.Error("FromRecord on HostVal should return ok=false")
	}
}

func TestRecordTypeHelper(t *testing.T) {
	rty := RecordType(
		RowField{Label: "x", Type: ConType("Int")},
		RowField{Label: "y", Type: ConType("Int")},
	)
	got := TypePretty(rty)
	if !strings.Contains(got, "Record") {
		t.Errorf("expected Record type, got %s", got)
	}
}

func TestTupleTypeHelper(t *testing.T) {
	tt := TupleType(ConType("Int"), ConType("Bool"))
	got := TypePretty(tt)
	if !strings.Contains(got, "Record") || !strings.Contains(got, "_1") {
		t.Errorf("expected Record{_1, _2} type, got %s", got)
	}
}

func TestNewCapEnvExported(t *testing.T) {
	ce := eval.NewCapEnv(map[string]any{"key": "val"})
	v, ok := ce.Get("key")
	if !ok {
		t.Fatal("expected key in CapEnv")
	}
	if v != "val" {
		t.Errorf("expected val, got %v", v)
	}
}

func TestFromConDefensiveCopy(t *testing.T) {
	v := &eval.ConVal{Con: "Pair", Args: []eval.Value{ToValue(1), ToValue(2)}}
	_, args, ok := FromCon(v)
	if !ok {
		t.Fatal("expected ConVal")
	}
	// Mutate the returned slice.
	args[0] = ToValue(999)
	// Original must be unchanged.
	_, orig, _ := FromCon(v)
	if MustHost[int](orig[0]) != 1 {
		t.Error("FromCon returned slice aliases internal Args — mutation leaked")
	}
}

func TestFromRecordDefensiveCopy(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), "import Prelude\nmain := { a: 1, b: 2 }")
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	fields, ok := FromRecord(result.Value)
	if !ok {
		t.Fatal("expected record value")
	}
	// Mutate the returned map.
	fields["injected"] = ToValue(999)
	// Re-extract — original must not contain the injected key.
	fields2, _ := FromRecord(result.Value)
	if _, found := fields2["injected"]; found {
		t.Error("FromRecord returned map aliases internal Fields — mutation leaked")
	}
}

func TestTypeHelpers(t *testing.T) {
	// ConType constructs a type constructor.
	intTy := ConType("Int")
	if TypePretty(intTy) != "Int" {
		t.Errorf("expected Int, got %s", TypePretty(intTy))
	}

	// ArrowType constructs a function type.
	arrTy := ArrowType(ConType("Bool"), ConType("Bool"))
	if got := TypePretty(arrTy); got != "Bool -> Bool" {
		t.Errorf("expected Bool -> Bool, got %s", got)
	}

	// EmptyRowType constructs an empty row.
	row := EmptyRowType()
	if TypePretty(row) != "{}" {
		t.Errorf("expected {}, got %s", TypePretty(row))
	}

	// ClosedRowType constructs a row with fields.
	row2 := ClosedRowType(RowField{Label: "x", Type: ConType("Int")})
	if got := TypePretty(row2); !strings.Contains(got, "x") {
		t.Errorf("expected row with field x, got %s", got)
	}

	// ForallType constructs a quantified type.
	forallTy := ForallType("a", ArrowType(VarType("a"), VarType("a")))
	if got := TypePretty(forallTy); !strings.Contains(got, `\`) {
		t.Errorf(`expected \ in pretty, got %s`, got)
	}

	// CompType constructs a computation type.
	compTy := CompType(EmptyRowType(), EmptyRowType(), ConType("Bool"))
	if got := TypePretty(compTy); !strings.Contains(got, "Bool") {
		t.Errorf("expected Bool in computation type, got %s", got)
	}

	// VarType constructs a type variable.
	tv := VarType("a")
	if TypePretty(tv) != "a" {
		t.Errorf("expected a, got %s", TypePretty(tv))
	}

	// Kind helpers.
	k := KindType()
	if !k.Equal(KindType()) {
		t.Errorf("KType should equal KType")
	}
	kr := KindRow()
	if kr.Equal(KindType()) {
		t.Errorf("KRow should not equal KType")
	}
	ka := KindArrow(KindType(), KindType())
	if !ka.Equal(KindArrow(KindType(), KindType())) {
		t.Errorf("KArrow(Type,Type) should equal itself")
	}

	// TypeEqual
	if !TypeEqual(ConType("Int"), ConType("Int")) {
		t.Errorf("Int should equal Int")
	}
	if TypeEqual(ConType("Int"), ConType("Bool")) {
		t.Errorf("Int should not equal Bool")
	}
}
