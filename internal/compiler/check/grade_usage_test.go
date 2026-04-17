// Grade usage tests — inferDo grade boundary check, buildAccumulatedGrade.
// Does NOT cover: GradeAlgebra axiom verification (grade_test.go),
// engine-level grade integration (engine/engine_grade_test.go).
package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Gap 2: inferDo now fires checkGradeBoundary ---

func TestGradeUsage_InferDoLinearPreservation(t *testing.T) {
	// Without explicit type annotation, the do-block is inferred.
	// @Linear capability preserved (not consumed) should be caught.
	// This is a unit-level test without GradeAlgebra, so the error
	// is about missing GradeAlgebra (same behavior as checkDo path).
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; Zero: Mult; }
form Unit := { Unit: Unit; }
noop :: Computation { h: Unit @Linear } { h: Unit @Linear } Unit
noop := assumption
main := do { noop }
`
	errText := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errText, "GradeAlgebra") {
		t.Errorf("expected GradeAlgebra error from inferDo boundary check, got: %s", errText)
	}
}

func TestGradeUsage_InferDoLinearConsumed(t *testing.T) {
	// @Linear consumed (type changes or disappears) → no error.
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; Zero: Mult; }
form Unit := { Unit: Unit; }
close :: Computation { h: Unit @Linear } {} Unit
close := assumption
good := do { close }
`
	checkSource(t, source, nil)
}

// --- Unit: buildAccumulatedGrade ---

func TestBuildAccumulatedGrade(t *testing.T) {
	ch := newTestChecker()
	drop := &types.TyCon{Name: "Zero"}
	unit := &types.TyCon{Name: "Linear"}
	algebra := resolvedGradeAlgebra{
		dropValue: drop,
		unitValue: unit,
		valid:     true,
	}
	// 0 usages → Drop
	result := ch.buildAccumulatedGrade(algebra, 0)
	if !testOps.Equal(result, drop) {
		t.Errorf("0 usages: expected Drop, got %s", testOps.Pretty(result))
	}
	// 1 usage → Unit
	result = ch.buildAccumulatedGrade(algebra, 1)
	if !testOps.Equal(result, unit) {
		t.Errorf("1 usage: expected Unit (Linear), got %s", testOps.Pretty(result))
	}
	// No GradeUnit → nil
	noUnit := resolvedGradeAlgebra{dropValue: drop, valid: true}
	result = ch.buildAccumulatedGrade(noUnit, 1)
	if result != nil {
		t.Errorf("no GradeUnit: expected nil, got %s", testOps.Pretty(result))
	}
}

func TestResolveGradeUnit_NoClass(t *testing.T) {
	ch := newTestChecker()
	unit := ch.resolveGradeUnit()
	if unit != nil {
		t.Error("expected nil when no GradeAlgebra class is registered")
	}
}
