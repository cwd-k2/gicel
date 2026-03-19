// Class elaboration tests — DataDecl generation, selectors, method scope, superclass dict, instance elaboration.
// Does NOT cover: instance resolution (instance_test.go), evidence (evidence_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/core"
)

// --- Type class elaboration tests ---

func TestClassElaboratesDataDecl(t *testing.T) {
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }`
	prog := checkSource(t, source, nil)
	// Should have generated Eq$Dict data declaration.
	found := false
	for _, d := range prog.DataDecls {
		if d.Name == "Eq$Dict" {
			found = true
			if len(d.Cons) != 1 || d.Cons[0].Name != "Eq$Dict" {
				t.Errorf("expected single constructor Eq$Dict")
			}
			if len(d.TyParams) != 1 {
				t.Errorf("expected 1 type param, got %d", len(d.TyParams))
			}
		}
	}
	if !found {
		t.Error("expected Eq$Dict data declaration")
	}
}

func TestClassElaboratesSelectors(t *testing.T) {
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }`
	prog := checkSource(t, source, nil)
	// Should have generated eq binding (selector).
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "eq" {
			found = true
			// Verify the type is a forall with a dict arrow.
			if b.Type == nil {
				t.Error("eq selector should have a type")
			}
			// Verify it's a TyLam wrapping a Lam (selector body).
			if tl, ok := b.Expr.(*core.TyLam); !ok {
				t.Errorf("eq selector should be a TyLam, got %T", b.Expr)
			} else if _, ok := tl.Body.(*core.Lam); !ok {
				t.Errorf("eq selector TyLam body should be a Lam, got %T", tl.Body)
			}
		}
	}
	if !found {
		t.Error("expected 'eq' selector binding")
	}
}

func TestClassMethodInScope(t *testing.T) {
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
f :: Eq a => a -> a -> Bool
f := \x y. eq x y`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "f" {
			found = true
			if b.Type == nil {
				t.Error("binding 'f' should have a type")
			}
		}
	}
	if !found {
		t.Error("expected binding 'f'")
	}
}

func TestSuperclassDictField(t *testing.T) {
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }`
	prog := checkSource(t, source, nil)
	found := false
	for _, d := range prog.DataDecls {
		if d.Name == "Ord$Dict" {
			found = true
			// First field should be Eq$Dict a (superclass dict)
			if len(d.Cons) != 1 {
				t.Fatalf("expected 1 constructor")
			}
			con := d.Cons[0]
			if len(con.Fields) != 2 { // Eq$Dict a, then a -> a -> Bool
				t.Errorf("expected 2 fields (super dict + method), got %d", len(con.Fields))
			}
		}
	}
	if !found {
		t.Error("expected Ord$Dict data declaration")
	}
}

func TestInstanceElaboratesBinding(t *testing.T) {
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "Eq$Bool" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Eq$Bool' dictionary binding")
	}
}

func TestInstanceWithContextElaborates(t *testing.T) {
	// instance Eq a => Eq (Maybe a) → dictionary function
	source := `data Bool := True | False
data Maybe a := Just a | Nothing
class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq (Maybe a) { eq := \x y. True }`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "Eq$(Maybe 'a)" {
			found = true
			// Should be a lambda (dict function) since it has context.
			if _, ok := b.Expr.(*core.Lam); !ok {
				t.Errorf("expected Lam for contextual instance, got %T", b.Expr)
			}
		}
	}
	if !found {
		t.Error("expected 'Eq$(Maybe 'a)' dictionary function binding")
	}
}
