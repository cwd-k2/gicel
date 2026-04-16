// Hover doc-lookup tests — module-keyed doc resolution for variable refs.
// Does NOT cover: token coverage (engine_hover_coverage_test.go),
// example smoke (engine_hover_example_test.go).

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// TestHoverDoc_QualifiedRefsKeepProvenance verifies that qualified
// references (A.foo and B.foo) get distinct doc comments instead of
// being collapsed by name. The bug surfaced by the stage2 review:
// docs were keyed only by name in the adapter, so registration order
// silently overwrote earlier docs.
func TestHoverDoc_QualifiedRefsKeepProvenance(t *testing.T) {
	eng := NewEngine()
	eng.EnableHoverIndex()
	if err := eng.RegisterModule("A", `
-- doc from A
foo := 1
`); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("B", `
-- doc from B
foo := 2
`); err != nil {
		t.Fatal(err)
	}

	src := `
import A as A
import B as B
first := A.foo
second := B.foo
main := first
`
	ar := eng.Analyze(context.Background(), src)
	if !ar.Complete {
		t.Fatalf("analyze should succeed, errors=%v", ar.Errors)
	}
	if ar.HoverIndex == nil {
		t.Fatal("expected hover index")
	}
	posA := strings.Index(src, "A.foo") + 2 // inside `foo`
	posB := strings.LastIndex(src, "B.foo") + 2
	hoverA := ar.HoverIndex.HoverAt(span.Pos(posA))
	hoverB := ar.HoverIndex.HoverAt(span.Pos(posB))
	if !strings.Contains(hoverA, "doc from A") {
		t.Errorf("hoverA missing A's doc: %q", hoverA)
	}
	if !strings.Contains(hoverB, "doc from B") {
		t.Errorf("hoverB missing B's doc: %q", hoverB)
	}
	if strings.Contains(hoverA, "doc from B") {
		t.Errorf("hoverA leaked B's doc: %q", hoverA)
	}
}

// TestHoverDoc_AliasResolvesToActualModule verifies that aliased imports
// (`import Long as L`) still surface the correct doc — the recorder
// must use the resolved module name, not the surface alias.
func TestHoverDoc_AliasResolvesToActualModule(t *testing.T) {
	eng := NewEngine()
	eng.EnableHoverIndex()
	if err := eng.RegisterModule("Long", `
-- doc from Long
greet := 1
`); err != nil {
		t.Fatal(err)
	}

	src := `
import Long as L
main := L.greet
`
	ar := eng.Analyze(context.Background(), src)
	if !ar.Complete {
		t.Fatalf("analyze should succeed, errors=%v", ar.Errors)
	}
	if ar.HoverIndex == nil {
		t.Fatal("expected hover index")
	}
	pos := strings.Index(src, "L.greet") + 2 // inside `greet`
	hover := ar.HoverIndex.HoverAt(span.Pos(pos))
	if !strings.Contains(hover, "doc from Long") {
		t.Errorf("alias lookup did not resolve to actual module doc: %q", hover)
	}
}

// TestHoverDoc_LocalDefinitionDoc verifies that a local definition's doc
// is found when the variable is referenced by name (no qualifier). This
// guards the empty-module slot in the keyed map.
func TestHoverDoc_LocalDefinitionDoc(t *testing.T) {
	eng := NewEngine()
	eng.EnableHoverIndex()

	src := `
-- local doc here
helper := 1
main := helper
`
	ar := eng.Analyze(context.Background(), src)
	if !ar.Complete {
		t.Fatalf("analyze should succeed, errors=%v", ar.Errors)
	}
	if ar.HoverIndex == nil {
		t.Fatal("expected hover index")
	}
	pos := strings.LastIndex(src, "helper") + 2
	hover := ar.HoverIndex.HoverAt(span.Pos(pos))
	if !strings.Contains(hover, "local doc here") {
		t.Errorf("local doc not found at reference site: %q", hover)
	}
}
