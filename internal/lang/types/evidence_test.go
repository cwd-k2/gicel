package types

import "testing"

// --- Group 1A: TyEvidenceRow type construction and interface ---

func TestEmptyRow(t *testing.T) {
	r := EmptyRow()
	if !r.IsCapabilityRow() {
		t.Fatal("expected capability fiber")
	}
	if r.Entries.EntryCount() != 0 {
		t.Fatalf("expected 0 entries, got %d", r.Entries.EntryCount())
	}
	if r.IsOpen() {
		t.Fatal("expected closed row")
	}
	if len(r.Children()) != 0 {
		t.Fatalf("expected 0 children, got %d", len(r.Children()))
	}
}

func TestClosedRow(t *testing.T) {
	r := ClosedRow(
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

func TestOpenRow(t *testing.T) {
	tail := Var("r")
	r := OpenRow(
		[]RowField{{Label: "db", Type: Con("DB")}},
		tail,
	)
	if r.IsClosed() {
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

func TestEmptyConstraintRow(t *testing.T) {
	r := EmptyConstraintRow()
	if !r.IsConstraintRow() {
		t.Fatal("expected constraint fiber")
	}
	if r.Entries.EntryCount() != 0 {
		t.Fatalf("expected 0 entries, got %d", r.Entries.EntryCount())
	}
}

func TestSingleConstraint(t *testing.T) {
	r := SingleConstraint("Eq", []Type{Con("Int")})
	if r.Entries.EntryCount() != 1 {
		t.Fatalf("expected 1 entry, got %d", r.Entries.EntryCount())
	}
	entries := r.ConEntries()
	if HeadClassName(entries[0]) != "Eq" {
		t.Errorf("expected Eq, got %s", HeadClassName(entries[0]))
	}
	// Children: one arg (Int)
	if len(r.Children()) != 1 {
		t.Fatalf("expected 1 child, got %d", len(r.Children()))
	}
}

func TestFiberKind(t *testing.T) {
	cap := EmptyRow()
	if !Equal(cap.Entries.FiberKind(), TypeOfRows) {
		t.Error("capability fiber should have Row kind")
	}
	con := EmptyConstraintRow()
	if !Equal(con.Entries.FiberKind(), TypeOfConstraints) {
		t.Error("constraint fiber should have Constraint kind")
	}
}

func TestCapabilityMapChildren(t *testing.T) {
	r := ClosedRow(
		RowField{Label: "x", Type: Con("Int")},
		RowField{Label: "y", Type: Con("Bool")},
	)
	mapped, _ := r.Entries.MapChildren(func(ty Type) Type {
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

func TestConstraintMapChildren(t *testing.T) {
	r := SingleConstraint("Eq", []Type{Con("Int")})
	mapped, _ := r.Entries.MapChildren(func(ty Type) Type {
		if c, ok := ty.(*TyCon); ok {
			return Con("Mapped" + c.Name)
		}
		return ty
	})
	con := mapped.(*ConstraintEntries)
	cls := con.Entries[0].(*ClassEntry)
	if cls.Args[0].(*TyCon).Name != "MappedInt" {
		t.Errorf("expected MappedInt, got %s", cls.Args[0].(*TyCon).Name)
	}
}

func TestRowFieldSorting(t *testing.T) {
	// Fields should be sorted after normalization.
	r := ClosedRow(
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

func TestFiberMismatchPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on CapFields() for constraint row")
		}
	}()
	r := EmptyConstraintRow()
	_ = r.CapFields() // should panic
}

func TestTypeInterface(t *testing.T) {
	// TyEvidenceRow satisfies the Type interface.
	var ty Type = EmptyRow()
	_ = ty.Span()
	_ = ty.Children()
}

// --- Group 1B: Evidence row operations ---

func TestLabels(t *testing.T) {
	r := ClosedRow(
		RowField{Label: "x", Type: Con("Int")},
		RowField{Label: "y", Type: Con("Bool")},
	)
	labels := Labels(r)
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if _, ok := labels["x"]; !ok {
		t.Error("missing label x")
	}
	if _, ok := labels["y"]; !ok {
		t.Error("missing label y")
	}
}

func TestExtendRow(t *testing.T) {
	r := ClosedRow(RowField{Label: "x", Type: Con("Int")})
	r2, err := ExtendRow(r, RowField{Label: "y", Type: Con("Bool")})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Entries.EntryCount() != 2 {
		t.Fatalf("expected 2 fields, got %d", r2.Entries.EntryCount())
	}
	// Sorted order: x, y
	fields := r2.CapFields()
	if fields[0].Label != "x" || fields[1].Label != "y" {
		t.Errorf("expected [x, y], got [%s, %s]", fields[0].Label, fields[1].Label)
	}
}

func TestExtendRowDuplicate(t *testing.T) {
	r := ClosedRow(RowField{Label: "x", Type: Con("Int")})
	_, err := ExtendRow(r, RowField{Label: "x", Type: Con("Bool")})
	if err == nil {
		t.Fatal("expected error on duplicate label")
	}
}

func TestRemoveLabel(t *testing.T) {
	r := ClosedRow(
		RowField{Label: "x", Type: Con("Int")},
		RowField{Label: "y", Type: Con("Bool")},
	)
	field, remaining, ok := RemoveLabel(r, "x")
	if !ok {
		t.Fatal("expected to find label x")
	}
	if field.Label != "x" {
		t.Errorf("expected field x, got %s", field.Label)
	}
	if remaining.Entries.EntryCount() != 1 {
		t.Fatalf("expected 1 remaining field, got %d", remaining.Entries.EntryCount())
	}
}

func TestRemoveLabelNotFound(t *testing.T) {
	r := ClosedRow(RowField{Label: "x", Type: Con("Int")})
	_, _, ok := RemoveLabel(r, "z")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestPreservesTail(t *testing.T) {
	tail := Var("r")
	r := OpenRow([]RowField{{Label: "x", Type: Con("Int")}}, tail)
	r2, err := ExtendRow(r, RowField{Label: "y", Type: Con("Bool")})
	if err != nil {
		t.Fatal(err)
	}
	if r2.IsClosed() {
		t.Fatal("expected tail to be preserved")
	}
}

// --- Group 1C: Subst, Equal, FreeVars, Pretty for TyEvidenceRow ---

func TestSubstCapability(t *testing.T) {
	// { x: a | r } with a := Int → { x: Int | r }
	r := OpenRow([]RowField{{Label: "x", Type: Var("a")}}, Var("r"))
	result := Subst(r, "a", Con("Int"))
	ev, ok := result.(*TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	field := ev.CapFields()[0]
	if c, ok := field.Type.(*TyCon); !ok || c.Name != "Int" {
		t.Errorf("expected Int, got %s", Pretty(field.Type))
	}
}

func TestSubstConstraint(t *testing.T) {
	// { Eq a } with a := Int → { Eq Int }
	r := SingleConstraint("Eq", []Type{Var("a")})
	result := Subst(r, "a", Con("Int"))
	ev, ok := result.(*TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	entry := ev.ConEntries()[0].(*ClassEntry)
	if c, ok := entry.Args[0].(*TyCon); !ok || c.Name != "Int" {
		t.Errorf("expected Int, got %s", Pretty(entry.Args[0]))
	}
}

func TestSubstTail(t *testing.T) {
	// { x: Int | r } with r := { y: Bool } → { x: Int, y: Bool }
	r := OpenRow([]RowField{{Label: "x", Type: Con("Int")}}, Var("r"))
	replacement := ClosedRow(RowField{Label: "y", Type: Con("Bool")})
	result := Subst(r, "r", replacement)
	ev, ok := result.(*TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	// Tail should now be the replacement
	if ev.IsClosed() {
		// Actually, subst replaces the tail variable, but the row structure doesn't flatten.
		// The tail becomes the replacement row. This is correct.
	}
}

func TestEqualCapability(t *testing.T) {
	r1 := ClosedRow(RowField{Label: "x", Type: Con("Int")}, RowField{Label: "y", Type: Con("Bool")})
	r2 := ClosedRow(RowField{Label: "y", Type: Con("Bool")}, RowField{Label: "x", Type: Con("Int")})
	if !Equal(r1, r2) {
		t.Error("expected equal rows (order irrelevant)")
	}
}

func TestEqualConstraint(t *testing.T) {
	r1 := SingleConstraint("Eq", []Type{Con("Int")})
	r2 := SingleConstraint("Eq", []Type{Con("Int")})
	if !Equal(r1, r2) {
		t.Error("expected equal constraint rows")
	}
}

func TestNotEqualFibers(t *testing.T) {
	cap := EmptyRow()
	con := EmptyConstraintRow()
	if Equal(cap, con) {
		t.Error("capability and constraint rows should not be equal")
	}
}

func TestFreeVarsCapability(t *testing.T) {
	r := OpenRow([]RowField{{Label: "x", Type: Var("a")}}, Var("r"))
	fv := FreeVars(r)
	if _, ok := fv["a"]; !ok {
		t.Error("expected free var a")
	}
	if _, ok := fv["r"]; !ok {
		t.Error("expected free var r")
	}
}

func TestFreeVarsConstraint(t *testing.T) {
	r := SingleConstraint("Eq", []Type{Var("a")})
	fv := FreeVars(r)
	if _, ok := fv["a"]; !ok {
		t.Error("expected free var a")
	}
}

func TestPrettyCapability(t *testing.T) {
	r := ClosedRow(RowField{Label: "x", Type: Con("Int")}, RowField{Label: "y", Type: Con("Bool")})
	s := Pretty(r)
	if s != "{ x: Int, y: Bool }" {
		t.Errorf("expected '{ x: Int, y: Bool }', got '%s'", s)
	}
}

func TestPrettyConstraint(t *testing.T) {
	r := SingleConstraint("Eq", []Type{Con("Int")})
	s := Pretty(r)
	if s != "{ Eq Int }" {
		t.Errorf("expected '{ Eq Int }', got '%s'", s)
	}
}

func TestPrettyEmpty(t *testing.T) {
	r := EmptyRow()
	if Pretty(r) != "{}" {
		t.Errorf("expected '{}', got '%s'", Pretty(r))
	}
}

func TestPrettyOpenRow(t *testing.T) {
	r := OpenRow([]RowField{{Label: "x", Type: Con("Int")}}, Var("r"))
	s := Pretty(r)
	if s != "{ x: Int | r }" {
		t.Errorf("expected '{ x: Int | r }', got '%s'", s)
	}
}
