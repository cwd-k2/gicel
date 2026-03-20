package ir

import "testing"

// binding creates a Binding whose expression references the given names.
func binding(name string, refs ...string) Binding {
	// Build an expression that has exactly these free variables.
	if len(refs) == 0 {
		return Binding{Name: name, Expr: &Lit{Value: int64(0)}}
	}
	var expr Core = &Var{Name: refs[0]}
	for _, r := range refs[1:] {
		expr = &App{Fun: expr, Arg: &Var{Name: r}}
	}
	return Binding{Name: name, Expr: expr}
}

func bindingNames(bs []Binding) []string {
	names := make([]string, len(bs))
	for i, b := range bs {
		names[i] = b.Name
	}
	return names
}

func TestSortBindingsNoReorder(t *testing.T) {
	// a, b, c — no inter-dependencies. Order preserved.
	bs := []Binding{
		binding("a"),
		binding("b"),
		binding("c"),
	}
	sorted := SortBindings(bs)
	names := bindingNames(sorted)
	if len(names) != 3 || names[0] != "a" || names[1] != "b" || names[2] != "c" {
		t.Fatalf("expected [a b c], got %v", names)
	}
}

func TestSortBindingsForwardRef(t *testing.T) {
	// dict references myFn, which is defined later.
	// After sorting, myFn should come before dict.
	bs := []Binding{
		binding("dict", "myFn"),
		binding("myFn"),
	}
	sorted := SortBindings(bs)
	names := bindingNames(sorted)
	if names[0] != "myFn" || names[1] != "dict" {
		t.Fatalf("expected [myFn dict], got %v", names)
	}
}

func TestSortBindingsChain(t *testing.T) {
	// c depends on b, b depends on a.
	bs := []Binding{
		binding("c", "b"),
		binding("b", "a"),
		binding("a"),
	}
	sorted := SortBindings(bs)
	names := bindingNames(sorted)
	// a must come before b, b before c.
	idx := make(map[string]int)
	for i, n := range names {
		idx[n] = i
	}
	if idx["a"] >= idx["b"] || idx["b"] >= idx["c"] {
		t.Fatalf("expected a < b < c, got %v", names)
	}
}

func TestSortBindingsExternalRef(t *testing.T) {
	// References to names not in the binding group are ignored.
	bs := []Binding{
		binding("x", "externalFn"),
		binding("y"),
	}
	sorted := SortBindings(bs)
	names := bindingNames(sorted)
	if len(names) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(names))
	}
}

func TestSortBindingsCycle(t *testing.T) {
	// Mutual recursion: a references b, b references a.
	// Both should appear; order between them is unspecified but both must be present.
	bs := []Binding{
		binding("a", "b"),
		binding("b", "a"),
	}
	sorted := SortBindings(bs)
	names := bindingNames(sorted)
	if len(names) != 2 {
		t.Fatalf("expected 2 bindings, got %v", names)
	}
	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["a"] || !nameSet["b"] {
		t.Fatalf("missing binding in cycle, got %v", names)
	}
}

func TestSortBindingsInstanceDictPattern(t *testing.T) {
	// Simulates the real scenario:
	// _prim is an assumption (PrimOp, no refs)
	// helper references _prim
	// Eq$Bool (instance dict) references helper
	// user references Eq$Bool (via class method)
	bs := []Binding{
		binding("Eq$Bool", "helper"),
		binding("user", "Eq$Bool"),
		binding("helper", "_prim"),
		binding("_prim"),
	}
	sorted := SortBindings(bs)
	idx := make(map[string]int)
	for i, b := range sorted {
		idx[b.Name] = i
	}
	// _prim < helper < Eq$Bool < user
	if idx["_prim"] >= idx["helper"] {
		t.Fatalf("_prim should precede helper, got %v", bindingNames(sorted))
	}
	if idx["helper"] >= idx["Eq$Bool"] {
		t.Fatalf("helper should precede Eq$Bool, got %v", bindingNames(sorted))
	}
	if idx["Eq$Bool"] >= idx["user"] {
		t.Fatalf("Eq$Bool should precede user, got %v", bindingNames(sorted))
	}
}

func TestSortBindingsSelfReference(t *testing.T) {
	// A self-referencing binding (e.g., letrec f = ... f ...).
	// SortBindings should handle this gracefully — the binding has
	// in-degree from itself, but the j != i guard prevents self-edges.
	bs := []Binding{
		binding("f", "f", "x"), // f references itself and x
		binding("x"),           // x has no dependencies
	}
	sorted := SortBindings(bs)
	if len(sorted) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(sorted))
	}
	// x should come before f (f depends on x)
	idx := make(map[string]int)
	for i, b := range sorted {
		idx[b.Name] = i
	}
	if idx["x"] >= idx["f"] {
		t.Fatalf("x should precede f, got %v", bindingNames(sorted))
	}
}

func TestSortBindingsSingleton(t *testing.T) {
	bs := []Binding{binding("a")}
	sorted := SortBindings(bs)
	if len(sorted) != 1 || sorted[0].Name != "a" {
		t.Fatalf("expected [a], got %v", bindingNames(sorted))
	}
}
