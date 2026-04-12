package check

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Grade usage utilities — GradeUnit-based accumulated grade computation.
//
// GradeUnit is the identity element of GradeCompose, representing "one
// unit of resource usage." For Mult: GradeUnit = Linear; for Trivial:
// GradeUnit = Triv. This enables semiring-style grade accumulation.
//
// Note: TyCBPV.Grade describes COMPUTATION resource consumption (how
// much resource the computation uses when run), not binding usage
// multiplicity (how many times a value can be used). Row-level grade
// enforcement (checkGradeBoundary in grade.go) handles capability
// consumption soundness. This file provides utilities for constructing
// accumulated grade values from GradeCompose and GradeUnit.

// buildAccumulatedGrade computes GradeCompose^n(GradeUnit) for n usages.
// Returns nil if the algebra lacks GradeUnit or reduction fails.
//   - 0 usages → GradeDrop
//   - 1 usage  → GradeUnit
//   - n usages → GradeCompose(GradeCompose(..., GradeUnit), GradeUnit)
func (ch *Checker) buildAccumulatedGrade(algebra resolvedGradeAlgebra, count int) types.Type {
	if algebra.unitValue == nil {
		return nil
	}
	if count == 0 {
		return algebra.dropValue
	}
	accumulated := algebra.unitValue
	for i := 1; i < count; i++ {
		composed, ok := ch.reduceTyFamily(algebra.composeFamily, []types.Type{accumulated, algebra.unitValue}, span.Span{})
		if !ok {
			return nil
		}
		accumulated = composed
	}
	return accumulated
}

// resolveGradeUnit returns the GradeUnit value for the default grade algebra,
// or nil if no grade algebra is available.
func (ch *Checker) resolveGradeUnit() types.Type {
	gk := gradeAlgebraKind(ch)
	algebra := ch.resolveGradeAlgebra(gk)
	if !algebra.valid {
		return nil
	}
	return algebra.unitValue
}
