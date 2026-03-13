package types

import "testing"

// --- Group 1A: TyEvidenceRow type construction and interface ---

func TestEvEmptyRow(t *testing.T) {
	r := EvEmptyRow()
	if !r.IsCapabilityRow() {
		t.Fatal("expected capability fiber")
	}
	if r.Entries.EntryCount() != 0 {
		t.Fatalf("expected 0 entries, got %d", r.Entries.EntryCount())
	}
	if r.Tail != nil {
		t.Fatal("expected closed row")
	}
	if len(r.Children()) != 0 {
		t.Fatalf("expected 0 children, got %d", len(r.Children()))
	}
}

func TestEvClosedRow(t *testing.T) {
	r := EvClosedRow(
		RowField{Label: "x", Type: Con("Int")},
		RowField{Label: "y", Type: Con("Bool")},
	)
	if r.Entries.EntryCount() != 2 {
		t.Fatalf("expected 2 entries, got %d", r.Entries.EntryCount())
	}
	// Fields should be sorted by label.
	fields := r.CapFields()
	if fields[0].Label != "x" || fields[1].Label != "y" {
		t.Errorf("expected sorted labels [x, y], got [%s, %s]", fields[0].Label, fields[1].Label)
	}
	if len(r.Children()) != 2 {
		t.Fatalf("expected 2 children, got %d", len(r.Children()))
	}
}

func TestEvOpenRow(t *testing.T) {
	tail := Var("r")
	r := EvOpenRow(
		[]RowField{{Label: "db", Type: Con("DB")}},
		tail,
	)
	if r.Tail == nil {
		t.Fatal("expected open row")
	}
	if r.Entries.EntryCount() != 1 {
		t.Fatalf("expected 1 entry, got %d", r.Entries.EntryCount())
	}
	// Children: field type + tail
	if len(r.Children()) != 2 {
		t.Fatalf("expected 2 children (field + tail), got %d", len(r.Children()))
	}
}

func TestEvEmptyConstraintRow(t *testing.T) {
	r := EvEmptyConstraintRow()
	if !r.IsConstraintRow() {
		t.Fatal("expected constraint fiber")
	}
	if r.Entries.EntryCount() != 0 {
		t.Fatalf("expected 0 entries, got %d", r.Entries.EntryCount())
	}
}

func TestEvSingleConstraint(t *testing.T) {
	r := EvSingleConstraint("Eq", []Type{Con("Int")})
	if r.Entries.EntryCount() != 1 {
		t.Fatalf("expected 1 entry, got %d", r.Entries.EntryCount())
	}
	entries := r.ConEntries()
	if entries[0].ClassName != "Eq" {
		t.Errorf("expected Eq, got %s", entries[0].ClassName)
	}
	// Children: one arg (Int)
	if len(r.Children()) != 1 {
		t.Fatalf("expected 1 child, got %d", len(r.Children()))
	}
}

func TestEvFiberKind(t *testing.T) {
	cap := EvEmptyRow()
	if !cap.Entries.FiberKind().Equal(KRow{}) {
		t.Error("capability fiber should have Row kind")
	}
	con := EvEmptyConstraintRow()
	if !con.Entries.FiberKind().Equal(KConstraint{}) {
		t.Error("constraint fiber should have Constraint kind")
	}
}

func TestEvCapabilityMapChildren(t *testing.T) {
	r := EvClosedRow(
		RowField{Label: "x", Type: Con("Int")},
		RowField{Label: "y", Type: Con("Bool")},
	)
	mapped := r.Entries.MapChildren(func(ty Type) Type {
		if c, ok := ty.(*TyCon); ok {
			return Con("Mapped" + c.Name)
		}
		return ty
	})
	cap := mapped.(*CapabilityEntries)
	if cap.Fields[0].Type.(*TyCon).Name != "MappedInt" {
		t.Errorf("expected MappedInt, got %s", cap.Fields[0].Type.(*TyCon).Name)
	}
	if cap.Fields[1].Type.(*TyCon).Name != "MappedBool" {
		t.Errorf("expected MappedBool, got %s", cap.Fields[1].Type.(*TyCon).Name)
	}
}

func TestEvConstraintMapChildren(t *testing.T) {
	r := EvSingleConstraint("Eq", []Type{Con("Int")})
	mapped := r.Entries.MapChildren(func(ty Type) Type {
		if c, ok := ty.(*TyCon); ok {
			return Con("Mapped" + c.Name)
		}
		return ty
	})
	con := mapped.(*ConstraintEntries)
	if con.Entries[0].Args[0].(*TyCon).Name != "MappedInt" {
		t.Errorf("expected MappedInt, got %s", con.Entries[0].Args[0].(*TyCon).Name)
	}
}

func TestEvRowFieldSorting(t *testing.T) {
	// Fields should be sorted after normalization.
	r := EvClosedRow(
		RowField{Label: "z", Type: Con("String")},
		RowField{Label: "a", Type: Con("Int")},
		RowField{Label: "m", Type: Con("Bool")},
	)
	fields := r.CapFields()
	if fields[0].Label != "a" || fields[1].Label != "m" || fields[2].Label != "z" {
		t.Errorf("expected sorted [a, m, z], got [%s, %s, %s]",
			fields[0].Label, fields[1].Label, fields[2].Label)
	}
}

func TestEvFiberMismatchPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on CapFields() for constraint row")
		}
	}()
	r := EvEmptyConstraintRow()
	_ = r.CapFields() // should panic
}

func TestEvTypeInterface(t *testing.T) {
	// TyEvidenceRow satisfies the Type interface.
	var ty Type = EvEmptyRow()
	_ = ty.Span()
	_ = ty.Children()
}
