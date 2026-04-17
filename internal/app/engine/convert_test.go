// Value conversion tests — ToValue, FromHost, MustHost, FromRecord, FromCon, type helpers.
// Does NOT cover: runtime behavior (engine_host_api_test.go), boundary errors (engine_boundary_test.go).

package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/lang/types"
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
	if !ok || rv.Len() != 0 {
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
	rty := testOps.App(testOps.Con(types.TyConRecord), types.ClosedRow(
		RowField{Label: "x", Type: testOps.Con("Int")},
		RowField{Label: "y", Type: testOps.Con("Int")},
	))
	got := testOps.Pretty(rty)
	if !strings.Contains(got, "Record") {
		t.Errorf("expected Record type, got %s", got)
	}
}

func TestTupleTypeHelper(t *testing.T) {
	tt := testOps.App(testOps.Con(types.TyConRecord), types.ClosedRow(
		RowField{Label: types.TupleLabel(1), Type: testOps.Con("Int")},
		RowField{Label: types.TupleLabel(2), Type: testOps.Con("Bool")},
	))
	got := testOps.Pretty(tt)
	if got != "(Int, Bool)" {
		t.Errorf("expected (Int, Bool), got %s", got)
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
	// Con constructs a type constructor.
	intTy := testOps.Con("Int")
	if testOps.Pretty(intTy) != "Int" {
		t.Errorf("expected Int, got %s", testOps.Pretty(intTy))
	}

	// Arrow constructs a function type.
	arrTy := testOps.Arrow(testOps.Con("Bool"), testOps.Con("Bool"))
	if got := testOps.Pretty(arrTy); got != "Bool -> Bool" {
		t.Errorf("expected Bool -> Bool, got %s", got)
	}

	// EmptyRowType constructs an empty row.
	row := EmptyRowType()
	if testOps.Pretty(row) != "{}" {
		t.Errorf("expected {}, got %s", testOps.Pretty(row))
	}

	// ClosedRowType constructs a row with fields.
	row2 := ClosedRowType(RowField{Label: "x", Type: testOps.Con("Int")})
	if got := testOps.Pretty(row2); !strings.Contains(got, "x") {
		t.Errorf("expected row with field x, got %s", got)
	}

	// Forall constructs a quantified type.
	forallTy := testOps.Forall("a", types.TypeOfTypes, testOps.Arrow(testOps.Var("a"), testOps.Var("a")))
	if got := testOps.Pretty(forallTy); !strings.Contains(got, `\`) {
		t.Errorf(`expected \ in pretty, got %s`, got)
	}

	// Comp constructs a computation type.
	compTy := testOps.Comp(EmptyRowType(), EmptyRowType(), testOps.Con("Bool"), nil)
	if got := testOps.Pretty(compTy); !strings.Contains(got, "Bool") {
		t.Errorf("expected Bool in computation type, got %s", got)
	}

	// Var constructs a type variable.
	tv := testOps.Var("a")
	if testOps.Pretty(tv) != "a" {
		t.Errorf("expected a, got %s", testOps.Pretty(tv))
	}

	// Kind helpers.
	k := KindType()
	if !types.Equal(k, KindType()) {
		t.Errorf("Type should equal Type")
	}
	kr := KindRow()
	if types.Equal(kr, KindType()) {
		t.Errorf("Row should not equal Type")
	}
	ka := testOps.Arrow(KindType(), KindType())
	if !types.Equal(ka, testOps.Arrow(KindType(), KindType())) {
		t.Errorf("Arrow(Type,Type) should equal itself")
	}

	// Equal
	if !testOps.Equal(testOps.Con("Int"), testOps.Con("Int")) {
		t.Errorf("Int should equal Int")
	}
	if testOps.Equal(testOps.Con("Int"), testOps.Con("Bool")) {
		t.Errorf("Int should not equal Bool")
	}
}
