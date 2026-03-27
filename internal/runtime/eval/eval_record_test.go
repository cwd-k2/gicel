package eval

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/ir"
)

func TestEvalRecordLitEmpty(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordLit{}
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := r.Value.(*RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T", r.Value)
	}
	if rv.Len() != 0 {
		t.Errorf("expected empty record, got %d fields", rv.Len())
	}
}

func TestEvalRecordLit(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordLit{
		Fields: []ir.RecordField{
			{Label: "x", Value: &ir.Lit{Value: int64(42)}},
			{Label: "y", Value: &ir.Lit{Value: int64(7)}},
		},
	}
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := r.Value.(*RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T", r.Value)
	}
	xv, ok := rv.Get("x")
	if !ok {
		t.Fatal("missing field x")
	}
	if hv, ok := xv.(*HostVal); !ok || hv.Inner != int64(42) {
		t.Errorf("expected x = 42, got %v", xv)
	}
}

func TestEvalRecordProj(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordProj{
		Record: &ir.RecordLit{
			Fields: []ir.RecordField{
				{Label: "x", Value: &ir.Lit{Value: int64(42)}},
				{Label: "y", Value: &ir.Lit{Value: int64(7)}},
			},
		},
		Label: "y",
	}
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(7) {
		t.Errorf("expected HostVal(7), got %v", r.Value)
	}
}

func TestEvalRecordProjMissing(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordProj{
		Record: &ir.RecordLit{
			Fields: []ir.RecordField{
				{Label: "x", Value: &ir.Lit{Value: int64(42)}},
			},
		},
		Label: "z",
	}
	_, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for missing field")
	}
}

func TestEvalRecordUpdate(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordUpdate{
		Record: &ir.RecordLit{
			Fields: []ir.RecordField{
				{Label: "x", Value: &ir.Lit{Value: int64(42)}},
				{Label: "y", Value: &ir.Lit{Value: int64(7)}},
			},
		},
		Updates: []ir.RecordField{
			{Label: "x", Value: &ir.Lit{Value: int64(100)}},
		},
	}
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := r.Value.(*RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T", r.Value)
	}
	xv := rv.MustGet("x").(*HostVal)
	if xv.Inner != int64(100) {
		t.Errorf("expected x = 100, got %v", xv.Inner)
	}
	yv := rv.MustGet("y").(*HostVal)
	if yv.Inner != int64(7) {
		t.Errorf("expected y = 7 (unchanged), got %v", yv.Inner)
	}
}

func TestEvalRecordPattern(t *testing.T) {
	ev := newTestEval()
	// case { x: 42, y: 7 } of { x: a } -> a
	term := &ir.Case{
		Scrutinee: &ir.RecordLit{
			Fields: []ir.RecordField{
				{Label: "x", Value: &ir.Lit{Value: int64(42)}},
				{Label: "y", Value: &ir.Lit{Value: int64(7)}},
			},
		},
		Alts: []ir.Alt{
			{
				Pattern: &ir.PRecord{
					Fields: []ir.PRecordField{
						{Label: "x", Pattern: &ir.PVar{Name: "a"}},
					},
				},
				Body: &ir.Var{Name: "a"},
			},
		},
	}
	ir.AnnotateFreeVars(term)
	ir.AssignIndices(term)
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected HostVal(42), got %v", r.Value)
	}
}

func TestEvalRecordPatternWild(t *testing.T) {
	ev := newTestEval()
	// case { x: 42 } of { x: _ } -> 0
	term := &ir.Case{
		Scrutinee: &ir.RecordLit{
			Fields: []ir.RecordField{
				{Label: "x", Value: &ir.Lit{Value: int64(42)}},
			},
		},
		Alts: []ir.Alt{
			{
				Pattern: &ir.PRecord{
					Fields: []ir.PRecordField{
						{Label: "x", Pattern: &ir.PWild{}},
					},
				},
				Body: &ir.Lit{Value: int64(0)},
			},
		},
	}
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(0) {
		t.Errorf("expected HostVal(0), got %v", r.Value)
	}
}

// =============================================
// RecordVal invariant tests — NewRecord sorting, Get binary search, Update merge
// =============================================

func TestNewRecordSortsUnsortedFields(t *testing.T) {
	// NewRecord with unsorted fields should produce sorted output.
	fields := []RecordField{
		{Label: "z", Value: &HostVal{Inner: int64(3)}},
		{Label: "a", Value: &HostVal{Inner: int64(1)}},
		{Label: "m", Value: &HostVal{Inner: int64(2)}},
	}
	rv := NewRecord(fields)
	raw := rv.RawFields()
	if len(raw) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(raw))
	}
	if raw[0].Label != "a" || raw[1].Label != "m" || raw[2].Label != "z" {
		t.Errorf("expected sorted [a, m, z], got [%s, %s, %s]",
			raw[0].Label, raw[1].Label, raw[2].Label)
	}
}

func TestNewRecordAlreadySorted(t *testing.T) {
	// NewRecord with already sorted fields should produce the same order.
	fields := []RecordField{
		{Label: "a", Value: &HostVal{Inner: int64(1)}},
		{Label: "b", Value: &HostVal{Inner: int64(2)}},
		{Label: "c", Value: &HostVal{Inner: int64(3)}},
	}
	rv := NewRecord(fields)
	raw := rv.RawFields()
	if raw[0].Label != "a" || raw[1].Label != "b" || raw[2].Label != "c" {
		t.Errorf("expected [a, b, c], got [%s, %s, %s]",
			raw[0].Label, raw[1].Label, raw[2].Label)
	}
}

func TestGetBinarySearchSingleField(t *testing.T) {
	rv := NewRecord([]RecordField{
		{Label: "x", Value: &HostVal{Inner: int64(42)}},
	})
	v, ok := rv.Get("x")
	if !ok {
		t.Fatal("expected to find field x")
	}
	if hv := v.(*HostVal); hv.Inner != int64(42) {
		t.Errorf("expected 42, got %v", hv.Inner)
	}
	_, ok = rv.Get("y")
	if ok {
		t.Error("should not find non-existent field y")
	}
}

func TestGetBinarySearchTwoFields(t *testing.T) {
	rv := NewRecord([]RecordField{
		{Label: "b", Value: &HostVal{Inner: int64(2)}},
		{Label: "a", Value: &HostVal{Inner: int64(1)}},
	})
	v, ok := rv.Get("a")
	if !ok {
		t.Fatal("expected to find field a")
	}
	if hv := v.(*HostVal); hv.Inner != int64(1) {
		t.Errorf("expected 1, got %v", hv.Inner)
	}
	v, ok = rv.Get("b")
	if !ok {
		t.Fatal("expected to find field b")
	}
	if hv := v.(*HostVal); hv.Inner != int64(2) {
		t.Errorf("expected 2, got %v", hv.Inner)
	}
}

func TestGetBinarySearchFiveFields(t *testing.T) {
	rv := NewRecord([]RecordField{
		{Label: "e", Value: &HostVal{Inner: int64(5)}},
		{Label: "c", Value: &HostVal{Inner: int64(3)}},
		{Label: "a", Value: &HostVal{Inner: int64(1)}},
		{Label: "d", Value: &HostVal{Inner: int64(4)}},
		{Label: "b", Value: &HostVal{Inner: int64(2)}},
	})
	for _, tc := range []struct {
		label string
		want  int64
	}{
		{"a", 1}, {"b", 2}, {"c", 3}, {"d", 4}, {"e", 5},
	} {
		v, ok := rv.Get(tc.label)
		if !ok {
			t.Errorf("expected to find field %s", tc.label)
			continue
		}
		if hv := v.(*HostVal); hv.Inner != tc.want {
			t.Errorf("field %s: expected %d, got %v", tc.label, tc.want, hv.Inner)
		}
	}
	// Non-existent fields
	for _, label := range []string{"f", "z", "0", "aa"} {
		_, ok := rv.Get(label)
		if ok {
			t.Errorf("should not find non-existent field %s", label)
		}
	}
}

func TestGetBinarySearchTenFields(t *testing.T) {
	fields := []RecordField{
		{Label: "j", Value: &HostVal{Inner: int64(10)}},
		{Label: "f", Value: &HostVal{Inner: int64(6)}},
		{Label: "b", Value: &HostVal{Inner: int64(2)}},
		{Label: "h", Value: &HostVal{Inner: int64(8)}},
		{Label: "d", Value: &HostVal{Inner: int64(4)}},
		{Label: "i", Value: &HostVal{Inner: int64(9)}},
		{Label: "a", Value: &HostVal{Inner: int64(1)}},
		{Label: "g", Value: &HostVal{Inner: int64(7)}},
		{Label: "c", Value: &HostVal{Inner: int64(3)}},
		{Label: "e", Value: &HostVal{Inner: int64(5)}},
	}
	rv := NewRecord(fields)
	if rv.Len() != 10 {
		t.Fatalf("expected 10 fields, got %d", rv.Len())
	}
	// Verify sorted order.
	raw := rv.RawFields()
	for i := 1; i < len(raw); i++ {
		if raw[i].Label <= raw[i-1].Label {
			t.Errorf("fields not sorted: %s <= %s at index %d", raw[i].Label, raw[i-1].Label, i)
		}
	}
	// Verify all fields accessible.
	for _, label := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		_, ok := rv.Get(label)
		if !ok {
			t.Errorf("expected to find field %s", label)
		}
	}
}

func TestUpdateMergesAndMaintainsSortedOrder(t *testing.T) {
	base := NewRecord([]RecordField{
		{Label: "c", Value: &HostVal{Inner: int64(3)}},
		{Label: "a", Value: &HostVal{Inner: int64(1)}},
	})
	updated := base.Update([]RecordField{
		{Label: "b", Value: &HostVal{Inner: int64(20)}},
		{Label: "a", Value: &HostVal{Inner: int64(10)}},
	})
	raw := updated.RawFields()
	if len(raw) != 3 {
		t.Fatalf("expected 3 fields after update, got %d", len(raw))
	}
	// Must be sorted: a, b, c
	if raw[0].Label != "a" || raw[1].Label != "b" || raw[2].Label != "c" {
		t.Errorf("expected [a, b, c], got [%s, %s, %s]",
			raw[0].Label, raw[1].Label, raw[2].Label)
	}
	// a should be overwritten to 10
	if hv := raw[0].Value.(*HostVal); hv.Inner != int64(10) {
		t.Errorf("expected a = 10, got %v", hv.Inner)
	}
	// b is new, should be 20
	if hv := raw[1].Value.(*HostVal); hv.Inner != int64(20) {
		t.Errorf("expected b = 20, got %v", hv.Inner)
	}
	// c is unchanged
	if hv := raw[2].Value.(*HostVal); hv.Inner != int64(3) {
		t.Errorf("expected c = 3, got %v", hv.Inner)
	}
}

func TestUpdateOverwriteOnly(t *testing.T) {
	// Update that only overwrites existing fields should not change field count.
	base := NewRecord([]RecordField{
		{Label: "x", Value: &HostVal{Inner: int64(1)}},
		{Label: "y", Value: &HostVal{Inner: int64(2)}},
	})
	updated := base.Update([]RecordField{
		{Label: "x", Value: &HostVal{Inner: int64(100)}},
	})
	if updated.Len() != 2 {
		t.Fatalf("expected 2 fields after overwrite, got %d", updated.Len())
	}
	v, ok := updated.Get("x")
	if !ok {
		t.Fatal("missing field x")
	}
	if hv := v.(*HostVal); hv.Inner != int64(100) {
		t.Errorf("expected x = 100, got %v", hv.Inner)
	}
	v, ok = updated.Get("y")
	if !ok {
		t.Fatal("missing field y")
	}
	if hv := v.(*HostVal); hv.Inner != int64(2) {
		t.Errorf("expected y = 2, got %v", hv.Inner)
	}
}

func TestUpdateAddOnly(t *testing.T) {
	// Update that only adds new fields.
	base := NewRecord([]RecordField{
		{Label: "b", Value: &HostVal{Inner: int64(2)}},
	})
	updated := base.Update([]RecordField{
		{Label: "a", Value: &HostVal{Inner: int64(1)}},
		{Label: "c", Value: &HostVal{Inner: int64(3)}},
	})
	if updated.Len() != 3 {
		t.Fatalf("expected 3 fields, got %d", updated.Len())
	}
	raw := updated.RawFields()
	if raw[0].Label != "a" || raw[1].Label != "b" || raw[2].Label != "c" {
		t.Errorf("expected sorted [a, b, c], got [%s, %s, %s]",
			raw[0].Label, raw[1].Label, raw[2].Label)
	}
}

func TestUpdateDoesNotMutateOriginal(t *testing.T) {
	// Update should not mutate the original record.
	base := NewRecord([]RecordField{
		{Label: "x", Value: &HostVal{Inner: int64(1)}},
	})
	_ = base.Update([]RecordField{
		{Label: "x", Value: &HostVal{Inner: int64(999)}},
	})
	// Original should be unchanged.
	v, ok := base.Get("x")
	if !ok {
		t.Fatal("missing field x in original")
	}
	if hv := v.(*HostVal); hv.Inner != int64(1) {
		t.Errorf("original should be unchanged, got x = %v", hv.Inner)
	}
}

func TestNewRecordFromMapSorts(t *testing.T) {
	rv := NewRecordFromMap(map[string]Value{
		"z": &HostVal{Inner: int64(3)},
		"a": &HostVal{Inner: int64(1)},
		"m": &HostVal{Inner: int64(2)},
	})
	raw := rv.RawFields()
	if len(raw) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(raw))
	}
	if raw[0].Label != "a" || raw[1].Label != "m" || raw[2].Label != "z" {
		t.Errorf("expected sorted [a, m, z], got [%s, %s, %s]",
			raw[0].Label, raw[1].Label, raw[2].Label)
	}
}

func TestGetOnEmptyRecord(t *testing.T) {
	rv := NewRecord(nil)
	_, ok := rv.Get("x")
	if ok {
		t.Error("Get on empty record should return false")
	}
}

// Ensure unused import is used
var _ = context.Background
