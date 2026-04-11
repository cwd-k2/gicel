package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Grade tests — parsing, resolution, unification, do-block integration,
// grade boundary behavior, gradeAlgebraKind, gradeContainsMeta.
// Does NOT cover: GradeAlgebra enforcement (engine/engine_grade_test.go).

// --- Parsing: @Grade in row types ---

func TestGradeAnnotationParse(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
f :: { cap: Unit @Linear | r } -> Unit
f := \x. Unit
`
	checkSource(t, source, nil)
}

func TestGradeAnnotationParseMultipleFields(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
f :: { a: Unit @Linear, b: Unit @Affine } -> Unit
f := \x. Unit
`
	checkSource(t, source, nil)
}

func TestGradeAnnotationParseMixed(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
f :: { a: Unit @Linear, b: Unit } -> Unit
f := \x. Unit
`
	checkSource(t, source, nil)
}

// --- Resolution: @Grade flows into RowField.Grades ---

func TestGradeAnnotationResolves(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
f :: { cap: Unit @Linear } -> { cap: Unit @Linear } -> Unit
f := \x y. Unit
`
	checkSource(t, source, nil)
}

// --- Unification: grade must match ---

func TestGradeAnnotationUnifyMatch(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
id :: { cap: Unit @Linear } -> { cap: Unit @Linear }
id := \x. x
`
	checkSource(t, source, nil)
}

func TestGradeAnnotationUnifyMismatch(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
bad :: { cap: Unit @Linear } -> { cap: Unit @Affine }
bad := \x. x
`
	checkSourceExpectError(t, source, nil)
}

// --- Computation: grade in pre/post ---

func TestGradeInComputation(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
use :: Computation { handle: Unit @Linear } {} Unit
use := assumption
`
	checkSource(t, source, nil)
}

func TestGradePreserveInComputation(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
noop :: Computation { handle: Unit @Linear } { handle: Unit @Linear } Unit
noop := assumption
`
	checkSource(t, source, nil)
}

// --- Do block with grade: open/use/close ---

func TestGradeDoOpenUseClose(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
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
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
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
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

// --- Linear consumption via row unification ---

func TestGradeLinearMustBeConsumedEventually(t *testing.T) {
	// open's post is { h: Unit @Linear }, but main expects post = {}
	// Row unification catches this.
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
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
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
type LUB :: Mult := \(m1: Mult) (m2: Mult). case (m1, m2) {
  (Linear, _) => Linear;
  (_, Linear) => Linear;
  (Affine, _) => Affine;
  (_, Affine) => Affine;
  (Unrestricted, Unrestricted) => Unrestricted
}
`
	checkSource(t, source, nil)
}

// --- Grade behavior: protocol transitions and no-annotation ---

func TestGradeLinearSingleUseClose(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
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
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
close :: Computation { h: Unit @Linear } {} Unit
close := assumption
good :: Computation { h: Unit @Linear } {} Unit
good := do { close }
`
	checkSource(t, source, nil)
}

func TestGradeUnrestrictedAllowsMultiple(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
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
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
form S := { A: S; B: S; C: S; }
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
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Unit := { Unit: Unit; }
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

// --- Grade boundary: no GradeAlgebra → error ---

func TestGradeBoundaryNoAlgebra_LinearRejected(t *testing.T) {
	// Without a GradeAlgebra instance, grade annotations cannot be
	// enforced. The checker reports an error requiring the user to
	// define the algebra before using grade annotations.
	// Grade enforcement with Prelude (which defines GradeAlgebra Mult)
	// is tested in engine/engine_grade_test.go.
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; Zero: Mult; }
form Unit := { Unit: Unit; }
noop :: Computation { h: Unit @Linear } { h: Unit @Linear } Unit
noop := assumption
main :: Computation { h: Unit @Linear } { h: Unit @Linear } Unit
main := do { noop }
`
	errText := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errText, "GradeAlgebra") {
		t.Errorf("expected error mentioning GradeAlgebra, got: %s", errText)
	}
}

// --- Unit: gradeContainsMeta ---

func TestGradeContainsMeta_Concrete(t *testing.T) {
	ty := &types.TyCon{Name: "Linear"}
	if gradeContainsMeta(ty) {
		t.Error("concrete type should not contain meta")
	}
}

func TestGradeContainsMeta_Meta(t *testing.T) {
	ty := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	if !gradeContainsMeta(ty) {
		t.Error("TyMeta should be detected")
	}
}

func TestGradeContainsMeta_NestedMeta(t *testing.T) {
	ty := &types.TyApp{
		Fun: &types.TyCon{Name: "F"},
		Arg: &types.TyMeta{ID: 2, Kind: types.TypeOfTypes},
	}
	if !gradeContainsMeta(ty) {
		t.Error("nested TyMeta should be detected")
	}
}

// --- Unit: resolveGradeAlgebra without GradeAlgebra class ---

func TestResolveGradeAlgebra_NoClass(t *testing.T) {
	ch := newTestChecker()
	algebra := ch.resolveGradeAlgebra(types.TypeOfTypes)
	if algebra.valid {
		t.Error("expected valid=false when no GradeAlgebra class is registered")
	}
}

// --- Verify gradeAlgebraKind uses promoted kind when available ---

func TestGradeAlgebraKind_FallbackToKType(t *testing.T) {
	ch := newTestChecker()
	k := gradeAlgebraKind(ch)
	if !types.Equal(k, types.TypeOfTypes) {
		t.Errorf("expected TypeOfTypes fallback, got %s", types.PrettyTypeAsKind(k))
	}
}

func TestGradeAlgebraKind_UsesPromotedKind(t *testing.T) {
	ch := newTestChecker()
	ch.reg.RegisterPromotedKind("Mult", types.PromotedDataKind("Mult"))
	k := gradeAlgebraKind(ch)
	tc, ok := k.(*types.TyCon)
	if !ok {
		t.Fatalf("expected *TyCon, got %T", k)
	}
	if tc.Name != "Mult" {
		t.Errorf("expected TyCon{Mult}, got TyCon{%s}", tc.Name)
	}
	if !types.IsKindLevel(tc.Level) {
		t.Errorf("expected level 1 (kind level), got %v", tc.Level)
	}
}
