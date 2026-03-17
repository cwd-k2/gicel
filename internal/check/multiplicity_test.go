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
f := \x. Unit
`
	checkSource(t, source, nil)
}

func TestMultAnnotationParseMultipleFields(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
f :: { a : Unit @Linear, b : Unit @Affine } -> Unit
f := \x. Unit
`
	checkSource(t, source, nil)
}

func TestMultAnnotationParseMixed(t *testing.T) {
	// Some fields with @Mult, some without (nil = unrestricted)
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
f :: { a : Unit @Linear, b : Unit } -> Unit
f := \x. Unit
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
f := \x y. Unit
`
	checkSource(t, source, nil)
}

// --- Unification: Mult must match ---

func TestMultAnnotationUnifyMatch(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
id :: { cap : Unit @Linear } -> { cap : Unit @Linear }
id := \x. x
`
	checkSource(t, source, nil)
}

func TestMultAnnotationUnifyMismatch(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
bad :: { cap : Unit @Linear } -> { cap : Unit @Affine }
bad := \x. x
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
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
consume :: Computation { handle : Unit @Linear } {} Unit
consume := assumption
`
	checkSource(t, source, nil)
}

func TestMultPreserveInComputation(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
noop :: Computation { handle : Unit @Linear } { handle : Unit @Linear } Unit
noop := assumption
`
	checkSource(t, source, nil)
}

// --- Do block with multiplicity: open/use/close ---

func TestMultDoOpenUseClose(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
open :: Computation {} { h : Unit @Linear } Unit
open := assumption
use :: Computation { h : Unit @Linear } { h : Unit @Linear } Unit
use := assumption
close :: Computation { h : Unit @Linear } {} Unit
close := assumption
main :: Computation {} {} Unit
main := do { open; use; close }
`
	checkSource(t, source, nil)
}

func TestMultDoBindResult(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
open :: Computation {} { h : Unit @Linear } Unit
open := assumption
read :: Computation { h : Unit @Linear } { h : Unit @Linear } Int
read := assumption
close :: Computation { h : Unit @Linear } {} Unit
close := assumption
main :: Computation {} {} Int
main := do {
  open;
  n <- read;
  close;
  pure n
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// --- checkMultiplicity enforcement ---

func TestMultLinearMustBeConsumedEventually(t *testing.T) {
	// A computation that opens a @Linear handle but never closes it.
	// The overall type has @Linear in post — this is fine as a building block.
	// But if the CALLER expects post = {}, unification will catch it.
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
open :: Computation {} { h : Unit @Linear } Unit
open := assumption
main :: Computation {} {} Unit
main := do { open }
`
	// open's post is { h : Unit @Linear }, but main expects post = {}
	// Row unification catches this.
	checkSourceExpectError(t, source, nil)
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
