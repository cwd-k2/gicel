// Grade boundary enforcement — verifies that capability fields respect grade
// annotations across computation boundaries.
// Does NOT cover: grade_axiom.go (axiom verification), grade.go (algebra resolution).
package check

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Grade boundary check ---
//
// Grade verification operates at two levels with distinct responsibilities:
//
// 1. Structural (unifier): row_unify.go ensures both sides of a unification
//    agree on the same grade value. This is pure shape matching — it resolves
//    grade metavariables but does not interpret them as algebraic elements.
//
// 2. Algebraic (this file): checkGradeBoundary verifies that resolved grades
//    satisfy GradeAlgebra laws (e.g., Join(Drop, g) = g for preservation).
//    This layer runs after unification and uses type family reduction.
//
// The two layers are complementary: (1) determines *what* the grade is,
// (2) determines whether it *permits* the operation.

// checkGradeBoundary verifies that capability fields with grade annotations
// respect their grades across the computation boundary.
//
// For each graded field in pre: if the field appears in post with the same type
// (i.e., was preserved unchanged), the grade must permit preservation.
// A field that was consumed (absent from post) or transitioned (type changed)
// is always valid regardless of grade.
//
// Two enforcement paths:
//   - Concrete grade (no metas): fast path via gradeCanPreserveDynamic with immediate error.
//   - Grade containing metas: emit CtFunEq constraint "GradeJoin(Drop, grade) ~ grade"
//     so the solver can re-check once the meta is solved.
//
// If no GradeAlgebra instance is found for the grade kind, grade enforcement
// is skipped — the field is treated as unrestricted.
func (ch *Checker) checkGradeBoundary(comp *types.TyCBPV, s span.Span) {
	preFields := extractCapFields(ch, comp.Pre)
	if len(preFields) == 0 {
		return
	}

	postFields := extractCapFields(ch, comp.Post)

	for _, f := range preFields {
		if !f.IsGraded() {
			continue // no grade constraints (unrestricted)
		}

		postTy := types.RowFieldType(postFields, f.Label)
		if postTy == nil {
			continue // consumed: field not in post → OK
		}

		preTy := ch.unifier.Zonk(f.Type)
		postTy = ch.unifier.Zonk(postTy)

		if !types.Equal(preTy, postTy) {
			continue // transitioned: type changed → OK
		}

		// Field preserved unchanged. Check each grade allows preservation.
		for _, grade := range f.Grades {
			grade = ch.unifier.Zonk(grade)
			gk := ch.kindOfType(grade)
			if gk == nil {
				gk = gradeAlgebraKind(ch)
			}

			// Verify that GradeAlgebra exists for the grade kind.
			// Without an algebra, Join/Compose/Drop are undefined and
			// grade enforcement cannot operate — the annotation is
			// semantically vacuous. Report this explicitly rather than
			// silently treating the field as unrestricted.
			algebra := ch.resolveGradeAlgebra(gk)
			if !algebra.valid {
				ch.addDiag(diagnostic.ErrMultiplicity, s,
					diagFmt{Format: "grade @%s on capability %q requires impl %s %s",
						Args: []any{types.Pretty(grade), f.Label, gradeAlgebraClassName, types.Pretty(gk)}})
				continue
			}

			if gradeContainsMeta(grade) {
				// Deferred path: emit CtFunEq for Join(Drop, grade) ~ grade.
				// When the meta is solved, the solver re-processes the constraint
				// and the family reduces to a concrete result for unification.
				ch.emitGradePreserveConstraint(grade, gk, s)
				continue
			}

			// Fast path: concrete grade, check immediately.
			if !ch.gradeCanPreserveDynamic(grade, gk) {
				ch.addDiag(diagnostic.ErrMultiplicity, s,
					diagFmt{Format: "@%s capability %q must be consumed (type unchanged across computation boundary)",
						Args: []any{types.Pretty(grade), f.Label}})
			}
		}
	}
}

// gradeCanPreserveDynamic checks whether a field with the given grade can be
// preserved unchanged across a computation boundary, using the resolved grade algebra.
func (ch *Checker) gradeCanPreserveDynamic(grade types.Type, gradeKind types.Type) bool {
	algebra := ch.resolveGradeAlgebra(gradeKind)
	if !algebra.valid {
		return true // no grade algebra → treat as unrestricted
	}
	joined, ok := ch.reduceTyFamily(algebra.joinFamily, []types.Type{algebra.dropValue, grade}, span.Span{})
	if !ok {
		// Family reduction stuck (e.g., unsolved meta in args).
		// Assume OK; will be checked when the meta solves.
		return true
	}
	return types.Equal(joined, grade)
}

// emitGradePreserveConstraint emits a CtFunEq constraint encoding the
// preservation check: Join(Drop, grade) ~ grade.
//
// If the grade is e.g. a metavariable ?m, this constraint says:
// "when ?m is solved, Join(Drop, ?m) must equal ?m" — which is the
// algebraic definition of grade preservation.
func (ch *Checker) emitGradePreserveConstraint(grade types.Type, gradeKind types.Type, s span.Span) {
	algebra := ch.resolveGradeAlgebra(gradeKind)
	if !algebra.valid {
		return // no grade algebra → skip constraint emission
	}
	args := []types.Type{algebra.dropValue, grade}

	resultMeta := ch.freshMeta(gradeKind)
	blocking := ch.unifier.CollectBlockingMetas(args)
	if len(blocking) == 0 {
		// Invariant: gradeContainsMeta was true, so CollectBlockingMetas should
		// find at least one meta. If not, the meta was zonked between the check
		// and here. Fall back to the concrete fast path.
		if !ch.gradeCanPreserveDynamic(ch.unifier.Zonk(grade), gradeKind) {
			ch.addDiag(diagnostic.ErrMultiplicity, s,
				diagFmt{Format: "@%s capability must be consumed (type unchanged across computation boundary)", Args: []any{types.Pretty(grade)}})
		}
		return
	}

	ct := &CtFunEq{
		FamilyName: algebra.joinFamily,
		Args:       args,
		ResultMeta: resultMeta,
		BlockingOn: blocking,
		OnFailure: func(errSpan span.Span, expected, actual types.Type) {
			ch.addDiag(diagnostic.ErrMultiplicity, errSpan,
				diagFmt{Format: "@%s capability must be consumed (grade preservation violation: expected %s, got %s)", Args: []any{types.Pretty(grade), types.Pretty(expected), types.Pretty(actual)}})
		},
		S: s,
	}
	ch.registerStuckFunEq(ct)

	// When the family reduces, resultMeta will be unified with Join(Zero, grade).
	// Unify resultMeta ~ grade so that preservation is enforced: the result
	// of Join(Zero, grade) must equal grade itself.
	ch.emitEq(resultMeta, grade, s, nil)
}

// resolveGradeDrop returns the GradeDrop value for the default grade algebra,
// or nil if no grade algebra is available.
func (ch *Checker) resolveGradeDrop() types.Type {
	gk := gradeAlgebraKind(ch)
	algebra := ch.resolveGradeAlgebra(gk)
	if !algebra.valid {
		return nil
	}
	return algebra.dropValue
}

// extractCompGrade extracts the Grade from a TyCBPV, or nil if ungraded.
func (ch *Checker) extractCompGrade(ty types.Type) types.Type {
	ty = ch.unifier.Zonk(ty)
	if comp, ok := ty.(*types.TyCBPV); ok {
		return comp.Grade
	}
	return nil
}

// composeGrades computes GradeCompose(g1, g2), or nil if either is nil.
func (ch *Checker) composeGrades(g1, g2 types.Type) types.Type {
	if g1 == nil || g2 == nil {
		return nil
	}
	gk := gradeAlgebraKind(ch)
	algebra := ch.resolveGradeAlgebra(gk)
	if !algebra.valid || algebra.composeFamily == "" {
		return nil
	}
	composed, ok := ch.reduceTyFamily(algebra.composeFamily, []types.Type{g1, g2}, span.Span{})
	if !ok {
		// Family reduction stuck — return a TyFamilyApp to defer.
		return &types.TyFamilyApp{Name: algebra.composeFamily, Args: []types.Type{g1, g2}}
	}
	return composed
}

// extractCapFields returns the capability fields from a zonked row type, or nil.
func extractCapFields(ch *Checker, ty types.Type) []types.RowField {
	ty = ch.unifier.Zonk(ty)
	ev, ok := ty.(*types.TyEvidenceRow)
	if !ok {
		return nil
	}
	cap, ok := ev.Entries.(*types.CapabilityEntries)
	if !ok {
		return nil
	}
	return cap.Fields
}

// joinGrades computes the grade join of two annotated capability fields.
// Uses the GradeJoin associated type family from GradeAlgebra when available;
// falls back to unification.
func (ch *Checker) joinGrades(result *types.RowField, other []types.Type, s span.Span) {
	if !result.IsGraded() && len(other) == 0 {
		return
	}

	// One side annotated, other unrestricted → take the annotation (more restrictive).
	if !result.IsGraded() && len(other) > 0 {
		result.Grades = other
		return
	}
	if result.IsGraded() && len(other) == 0 {
		return // keep result grades
	}

	// Both annotated: grade counts must match.
	if len(result.Grades) != len(other) {
		ch.addDiag(diagnostic.ErrTypeMismatch, s,
			diagFmt{Format: "grade count mismatch for %s: %d vs %d",
				Args: []any{result.Label, len(result.Grades), len(other)}})
		return
	}

	// Resolve the grade algebra to get the join family name.
	gk := gradeAlgebraKind(ch)
	algebra := ch.resolveGradeAlgebra(gk)

	for i := range result.Grades {
		a := ch.unifier.Zonk(result.Grades[i])
		b := ch.unifier.Zonk(other[i])

		// Try GradeJoin family reduction.
		if algebra.valid && algebra.joinFamily != "" {
			joinResult, ok := ch.reduceTyFamily(algebra.joinFamily, []types.Type{a, b}, s)
			if ok {
				result.Grades[i] = joinResult
				continue
			}
			// Stuck: emit CtFunEq for deferred join reduction.
			args := []types.Type{a, b}
			blocking := ch.unifier.CollectBlockingMetas(args)
			if len(blocking) > 0 {
				resultMeta := ch.freshMeta(gk)
				ct := &CtFunEq{
					FamilyName: algebra.joinFamily,
					Args:       args,
					ResultMeta: resultMeta,
					BlockingOn: blocking,
					S:          s,
				}
				ch.registerStuckFunEq(ct)
				result.Grades[i] = resultMeta
				continue
			}
		}
		// No GradeAlgebra or no blocking metas: fall back to equality constraint.
		ch.emitEq(a, b, s, nil)
	}
}
