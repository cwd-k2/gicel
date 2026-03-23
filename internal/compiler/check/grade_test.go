package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Grade tests — parsing, resolution, unification, do-block integration.
// Does NOT cover: grade boundary enforcement (Phase 2, via CtFunEq).

// --- Parsing: @Grade in row types ---

func TestGradeAnnotationParse(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
f :: { cap: Unit @Linear | r } -> Unit
f := \x. Unit
`
	checkSource(t, source, nil)
}

func TestGradeAnnotationParseMultipleFields(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
f :: { a: Unit @Linear, b: Unit @Affine } -> Unit
f := \x. Unit
`
	checkSource(t, source, nil)
}

func TestGradeAnnotationParseMixed(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
f :: { a: Unit @Linear, b: Unit } -> Unit
f := \x. Unit
`
	checkSource(t, source, nil)
}

// --- Resolution: @Grade flows into RowField.Grades ---

func TestGradeAnnotationResolves(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
f :: { cap: Unit @Linear } -> { cap: Unit @Linear } -> Unit
f := \x y. Unit
`
	checkSource(t, source, nil)
}

// --- Unification: grade must match ---

func TestGradeAnnotationUnifyMatch(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
id :: { cap: Unit @Linear } -> { cap: Unit @Linear }
id := \x. x
`
	checkSource(t, source, nil)
}

func TestGradeAnnotationUnifyMismatch(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
bad :: { cap: Unit @Linear } -> { cap: Unit @Affine }
bad := \x. x
`
	checkSourceExpectError(t, source, nil)
}

// --- Computation: grade in pre/post ---

func TestGradeInComputation(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
use :: Computation { handle: Unit @Linear } {} Unit
use := assumption
`
	checkSource(t, source, nil)
}

func TestGradePreserveInComputation(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
noop :: Computation { handle: Unit @Linear } { handle: Unit @Linear } Unit
noop := assumption
`
	checkSource(t, source, nil)
}

// --- Do block with grade: open/use/close ---

func TestGradeDoOpenUseClose(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
open :: Computation {} { h: Unit @Linear } Unit
open := assumption
use :: Computation { h: Unit @Linear } { h: Unit @Linear } Unit
use := assumption
close :: Computation { h: Unit @Linear } {} Unit
close := assumption
main :: Computation {} {} Unit
main := do { open; use; close }
`
	checkSource(t, source, nil)
}

func TestGradeDoBindResult(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
open :: Computation {} { h: Unit @Linear } Unit
open := assumption
read :: Computation { h: Unit @Linear } { h: Unit @Linear } Int
read := assumption
close :: Computation { h: Unit @Linear } {} Unit
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

// --- Linear consumption via row unification ---

func TestGradeLinearMustBeConsumedEventually(t *testing.T) {
	// open's post is { h: Unit @Linear }, but main expects post = {}
	// Row unification catches this.
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
open :: Computation {} { h: Unit @Linear } Unit
open := assumption
main :: Computation {} {} Unit
main := do { open }
`
	checkSourceExpectError(t, source, nil)
}

// --- Type family LUB with grade ---

func TestGradeLUBTypeFamilyDefined(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
type LUB :: Mult := \(m1: Mult) (m2: Mult). case m1 {
  Linear _ => Linear;
  _ Linear => Linear;
  Affine _ => Affine;
  _ Affine => Affine;
  Unrestricted Unrestricted => Unrestricted
}
`
	checkSource(t, source, nil)
}

// --- Grade behavior: protocol transitions and no-annotation ---

func TestGradeLinearSingleUseClose(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
use :: Computation { h: Unit @Linear } { h: Unit @Linear } Unit
use := assumption
close :: Computation { h: Unit @Linear } {} Unit
close := assumption
good :: Computation { h: Unit @Linear } {} Unit
good := do { use; close }
`
	checkSource(t, source, nil)
}

func TestGradeLinearConsumeOnly(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
close :: Computation { h: Unit @Linear } {} Unit
close := assumption
good :: Computation { h: Unit @Linear } {} Unit
good := do { close }
`
	checkSource(t, source, nil)
}

func TestGradeUnrestrictedAllowsMultiple(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
use :: Computation { h: Unit @Unrestricted } { h: Unit @Unrestricted } Unit
use := assumption
close :: Computation { h: Unit @Unrestricted } {} Unit
close := assumption
f :: Computation { h: Unit @Unrestricted } {} Unit
f := do { use; use; use; close }
`
	checkSource(t, source, nil)
}

func TestGradeTypeChangingPreservation(t *testing.T) {
	// Protocol state transition — type changes at each step.
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
data S := { A: S; B: S; C: S; }
step1 :: Computation { ch: A @Linear } { ch: B @Linear } Unit
step1 := assumption
step2 :: Computation { ch: B @Linear } { ch: C @Linear } Unit
step2 := assumption
close :: Computation { ch: C @Linear } {} Unit
close := assumption
f :: Computation { ch: A @Linear } {} Unit
f := do { step1; step2; close }
`
	checkSource(t, source, nil)
}

func TestGradeNoAnnotationNoRestriction(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
use :: Computation { h: Unit } { h: Unit } Unit
use := assumption
close :: Computation { h: Unit } {} Unit
close := assumption
f :: Computation { h: Unit } {} Unit
f := do { use; use; use; close }
`
	checkSource(t, source, nil)
}

// --- Pretty printing ---

func TestGradeAnnotationPretty(t *testing.T) {
	row := types.ClosedRow(types.RowField{
		Label:  "handle",
		Type:   &types.TyCon{Name: "FileHandle"},
		Grades: []types.Type{&types.TyCon{Name: "Linear"}},
	})
	s := types.Pretty(row)
	expected := "{ handle: FileHandle @ Linear }"
	if s != expected {
		t.Errorf("expected %q, got %q", expected, s)
	}
}
