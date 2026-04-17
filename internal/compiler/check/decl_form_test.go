// Form declaration tests — duplicate constructors, nullary constructors, pipe shorthand, universe enforcement.
// Does NOT cover: DataKinds, type families.

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Universe enforcement tests ---

func TestFormKindAnnotationRejectsSort(t *testing.T) {
	// form Foo :: Kind := ... should be rejected: form result kind must be Type.
	source := `form Foo :: Kind := { Bar: Foo; }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrKindMismatch)
}

func TestFormKindAnnotationAllowsType(t *testing.T) {
	// form Foo :: Type := ... should be accepted.
	source := `form Foo :: Type := { Bar: Foo; }`
	checkSource(t, source, nil)
}

func TestFormNoKindAnnotationDefault(t *testing.T) {
	// form Foo := ... (no annotation) should default to Type and succeed.
	source := `form Foo := { Bar: Foo; }`
	checkSource(t, source, nil)
}

func TestFormDuplicateConstructor(t *testing.T) {
	source := `form Bad := { X: Bad; X: Bad; }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrDuplicateDecl)
}

func TestFormDuplicateConstructorDistant(t *testing.T) {
	source := `form Bad := { A: Bad; B: Int -> Bad; C: Bad; A: Bad; }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrDuplicateDecl)
}

func TestFormNullaryConstructor(t *testing.T) {
	source := `form Unit := { MkUnit: Unit; }`
	prog := checkSource(t, source, nil)
	if len(prog.DataDecls) != 1 {
		t.Fatalf("expected 1 data decl, got %d", len(prog.DataDecls))
	}
	dd := prog.DataDecls[0]
	if dd.Name != "Unit" {
		t.Errorf("expected Unit, got %s", dd.Name)
	}
	if len(dd.Cons) != 1 {
		t.Fatalf("expected 1 constructor, got %d", len(dd.Cons))
	}
	con := dd.Cons[0]
	if con.Name != "MkUnit" {
		t.Errorf("expected MkUnit, got %s", con.Name)
	}
	if len(con.Fields) != 0 {
		t.Errorf("expected 0 fields for nullary constructor, got %d", len(con.Fields))
	}
}

func TestFormManyConstructors(t *testing.T) {
	source := `form Octet := {
  A: Octet;
  B: Octet;
  C: Octet;
  D: Octet;
  E: Octet;
  F: Octet;
  G: Octet;
  H: Octet;
}`
	prog := checkSource(t, source, nil)
	if len(prog.DataDecls) != 1 {
		t.Fatalf("expected 1 data decl, got %d", len(prog.DataDecls))
	}
	dd := prog.DataDecls[0]
	if len(dd.Cons) != 8 {
		t.Fatalf("expected 8 constructors, got %d", len(dd.Cons))
	}
	expected := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	for i, name := range expected {
		if dd.Cons[i].Name != name {
			t.Errorf("constructor %d: expected %s, got %s", i, name, dd.Cons[i].Name)
		}
	}
}

// TestFormPipeShorthandConstructorType verifies that pipe shorthand syntax
// registers constructor types correctly. Succ in `form Nat := Zero | Succ Nat`
// must have arity 1 and its type must include Nat -> Nat (not unit return).
func TestFormPipeShorthandConstructorType(t *testing.T) {
	source := `form Nat := Zero | Succ Nat`
	prog := checkSource(t, source, nil)
	if len(prog.DataDecls) != 1 {
		t.Fatalf("expected 1 data decl, got %d", len(prog.DataDecls))
	}
	dd := prog.DataDecls[0]
	if dd.Name != "Nat" {
		t.Errorf("expected Nat, got %s", dd.Name)
	}
	if len(dd.Cons) != 2 {
		t.Fatalf("expected 2 constructors, got %d", len(dd.Cons))
	}

	// Zero: nullary constructor (arity 0).
	zero := dd.Cons[0]
	if zero.Name != "Zero" {
		t.Errorf("expected Zero, got %s", zero.Name)
	}
	if len(zero.Fields) != 0 {
		t.Errorf("Zero: expected 0 fields, got %d", len(zero.Fields))
	}

	// Succ: unary constructor (arity 1) with field type Nat.
	succ := dd.Cons[1]
	if succ.Name != "Succ" {
		t.Errorf("expected Succ, got %s", succ.Name)
	}
	if len(succ.Fields) != 1 {
		t.Fatalf("Succ: expected 1 field, got %d", len(succ.Fields))
	}

	// The field type should be Nat (TyCon "Nat").
	fieldTy := succ.Fields[0]
	con, ok := fieldTy.(*types.TyCon)
	if !ok {
		t.Fatalf("Succ field: expected *types.TyCon, got %T (%s)", fieldTy, testOps.Pretty(fieldTy))
	}
	if con.Name != "Nat" {
		t.Errorf("Succ field: expected TyCon Nat, got %s", con.Name)
	}
}
