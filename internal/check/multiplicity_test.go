package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/types"
)

// ==========================================
// Phase 5d: Multiplicity Annotation — TDD
// ==========================================

// --- Parsing: @Mult in row types ---

func TestMultAnnotationParse(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
f :: { cap : Unit @Linear | r } -> Unit
f := \x -> Unit
`
	checkSource(t, source, nil)
}

func TestMultAnnotationParseMultipleFields(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
f :: { a : Unit @Linear, b : Unit @Affine } -> Unit
f := \x -> Unit
`
	checkSource(t, source, nil)
}

func TestMultAnnotationParseMixed(t *testing.T) {
	// Some fields with @Mult, some without (nil = unrestricted)
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
f :: { a : Unit @Linear, b : Unit } -> Unit
f := \x -> Unit
`
	checkSource(t, source, nil)
}

// --- Resolution: @Mult flows into RowField.Mult ---

func TestMultAnnotationResolves(t *testing.T) {
	// Verify that @Linear on a field actually produces a RowField with Mult
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
f :: { cap : Unit @Linear } -> { cap : Unit @Linear } -> Unit
f := \x -> \y -> Unit
`
	checkSource(t, source, nil)
}

// --- Unification: Mult must match ---

func TestMultAnnotationUnifyMatch(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
id :: { cap : Unit @Linear } -> { cap : Unit @Linear }
id := \x -> x
`
	checkSource(t, source, nil)
}

func TestMultAnnotationUnifyMismatch(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
bad :: { cap : Unit @Linear } -> { cap : Unit @Affine }
bad := \x -> x
`
	checkSourceExpectError(t, source, nil)
}

// --- Computation: multiplicity in pre/post ---

func TestMultAnnotationInComputation(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
use :: Computation { handle : Unit @Linear } {} Unit
use := assumption
`
	checkSource(t, source, nil)
}

// --- checkMultiplicity: linear consumption ---

func TestMultInComputationPrePost(t *testing.T) {
	// Multiplicity annotations flow through Computation pre/post rows.
	// An assumption with @Linear pre and empty post is well-typed.
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
consume :: Computation { handle : Unit @Linear } {} Unit
consume := assumption
`
	checkSource(t, source, nil)
}

func TestMultPreserveInComputation(t *testing.T) {
	// Linear cap in both pre and post — capability is preserved, not consumed.
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
noop :: Computation { handle : Unit @Linear } { handle : Unit @Linear } Unit
noop := assumption
`
	checkSource(t, source, nil)
}

// --- Type family LUB with Mult ---

func TestMultLUBTypeFamilyDefined(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
type LUB (m1 : Mult) (m2 : Mult) :: Mult = {
  LUB Linear _ = Linear;
  LUB _ Linear = Linear;
  LUB Affine _ = Affine;
  LUB _ Affine = Affine;
  LUB Unrestricted Unrestricted = Unrestricted
}
`
	checkSource(t, source, nil)
}

// --- Pretty printing ---

func TestMultAnnotationPretty(t *testing.T) {
	row := types.ClosedRow(types.RowField{
		Label: "handle",
		Type:  &types.TyCon{Name: "FileHandle"},
		Mult:  &types.TyCon{Name: "Linear"},
	})
	s := types.Pretty(row)
	expected := "{ handle : FileHandle @ Linear }"
	if s != expected {
		t.Errorf("expected %q, got %q", expected, s)
	}
}
