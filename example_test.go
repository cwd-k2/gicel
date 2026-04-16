// Examples embed tests — verify the examples catalogue and the path
// resolution protects against traversal.

package gicel_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

func TestExamples_NonEmpty(t *testing.T) {
	names := gicel.Examples()
	if len(names) == 0 {
		t.Fatal("no examples embedded")
	}
	// Spot-check a few stable entries.
	for _, want := range []string{"basics.hello", "basics.arithmetic"} {
		if !slices.Contains(names, want) {
			t.Errorf("expected example %q in catalogue", want)
		}
	}
}

func TestExamples_SortedUnique(t *testing.T) {
	names := gicel.Examples()
	seen := make(map[string]bool, len(names))
	for i, n := range names {
		if seen[n] {
			t.Errorf("duplicate example %q", n)
		}
		seen[n] = true
		if i > 0 && names[i-1] > n {
			t.Errorf("Examples not sorted: %q before %q", names[i-1], n)
		}
	}
}

func TestExamples_NamesHaveNoSlashes(t *testing.T) {
	for _, n := range gicel.Examples() {
		if strings.Contains(n, "/") {
			t.Errorf("example name %q contains raw slash (should be dotted)", n)
		}
	}
}

func TestExample_ValidName(t *testing.T) {
	src := gicel.Example("basics.hello")
	if src == "" {
		t.Fatal("expected non-empty source for basics.hello")
	}
	// Hello example is expected to import Prelude.
	if !strings.Contains(src, "import Prelude") {
		preview := src
		if len(preview) > 80 {
			preview = preview[:80]
		}
		t.Fatalf("source missing Prelude import: %q", preview)
	}
}

func TestExample_Missing(t *testing.T) {
	if src := gicel.Example("nonexistent.thing"); src != "" {
		t.Fatalf("expected empty for unknown name, got %d bytes", len(src))
	}
}

// TestExample_RejectsTraversal guards the ".." filter in example.go:45.
// Without this filter a caller could request ../../etc/... from the
// embedded FS; the filter is the security boundary.
func TestExample_RejectsTraversal(t *testing.T) {
	cases := []string{
		"..",
		"basics...hello",  // ".." collapses into parent reference
		"basics.../hello", // explicit traversal attempt
	}
	for _, c := range cases {
		if src := gicel.Example(c); src != "" {
			t.Errorf("Example(%q) returned content — traversal not rejected", c)
		}
	}
}

func TestExample_AllNamesResolve(t *testing.T) {
	for _, n := range gicel.Examples() {
		if gicel.Example(n) == "" {
			t.Errorf("catalogued name %q does not resolve", n)
		}
	}
}
