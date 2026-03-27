package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// gradeAlgebraKind returns the kind to use for grade algebra parameters.
// If "Mult" is registered as a promoted kind (via DataKinds), returns
// PromotedDataKind("Mult"); otherwise falls back to TypeOfTypes.
func gradeAlgebraKind(ch *Checker) types.Type {
	if k, ok := ch.reg.LookupPromotedKind("Mult"); ok {
		return k
	}
	return types.TypeOfTypes
}

// gradeAlgebraClassName is the name of the user-facing grade algebra class.
const gradeAlgebraClassName = "GradeAlgebra"

// resolvedGradeAlgebra holds the resolved join family name and drop value
// for a grade kind.
type resolvedGradeAlgebra struct {
	joinFamily string     // name of the GradeJoin type family
	dropValue  types.Type // the Drop element (promoted constructor, e.g. Zero)
	valid      bool       // false if no GradeAlgebra instance found
}

// resolveGradeAlgebra looks up a GradeAlgebra instance for the given grade kind
// and extracts the associated type family names by reducing the associated types.
// Returns a result with valid=false if no GradeAlgebra instance is found;
// callers must check valid before using the algebra.
func (ch *Checker) resolveGradeAlgebra(gradeKind types.Type) resolvedGradeAlgebra {
	classInfo, hasClass := ch.reg.LookupClass(gradeAlgebraClassName)
	if hasClass {
		// Match grade kind against instance type args.
		// GradeAlgebra takes a Kind-kinded parameter (g: Kind).
		// Instance: impl GradeAlgebra Mult := ...
		// Instance TypeArgs[0] = TyCon("Mult"), which is a type constructor (kind Type).
		// Grade kind = PromotedDataKind("Mult") (promoted kind from DataKinds).
		// Match by comparing the type arg name with the promoted data kind name.
		instances := ch.reg.InstancesForClass(gradeAlgebraClassName)
		for _, inst := range instances {
			if len(inst.TypeArgs) == 0 {
				continue
			}
			if con, ok := inst.TypeArgs[0].(*types.TyCon); ok {
				if dk, ok := gradeKind.(*types.TyCon); ok && types.IsKindLevel(dk.Level) && dk.Name == con.Name {
					result := ch.extractGradeAlgebra(classInfo, inst)
					result.valid = true
					return result
				}
			}
		}
	}
	// No GradeAlgebra instance found. Grade enforcement not available.
	return resolvedGradeAlgebra{valid: false}
}

// extractGradeAlgebra extracts GradeJoin and GradeDrop from a matched instance
// by reducing the associated type families with the instance's type args.
func (ch *Checker) extractGradeAlgebra(classInfo *ClassInfo, inst *InstanceInfo) resolvedGradeAlgebra {
	var result resolvedGradeAlgebra
	for _, assocName := range classInfo.AssocTypes {
		if _, ok := ch.reg.LookupFamily(assocName); !ok {
			continue
		}
		// Reduce the associated type with the instance's type args.
		reduced, didReduce := ch.reduceTyFamily(assocName, inst.TypeArgs, inst.S)
		if !didReduce {
			continue
		}
		switch assocName {
		case "GradeJoin":
			// The reduced result must be a type family name (TyCon).
			// If it doesn't reduce to a TyCon, the algebra is unusable.
			if con, ok := reduced.(*types.TyCon); ok {
				result.joinFamily = con.Name
			} else {
				return resolvedGradeAlgebra{}
			}
		case "GradeDrop":
			result.dropValue = reduced
		}
	}
	return result
}

// gradeContainsMeta reports whether ty contains any unsolved metavariable.
func gradeContainsMeta(ty types.Type) bool {
	return types.AnyType(ty, func(t types.Type) bool {
		_, ok := t.(*types.TyMeta)
		return ok
	})
}

// --- Grade boundary check ---

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
		if len(f.Grades) == 0 {
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

			if gradeContainsMeta(grade) {
				// Deferred path: emit CtFunEq for Join(Drop, grade) ~ grade.
				// When the meta is solved, the solver re-processes the constraint
				// and the family reduces to a concrete result for unification.
				ch.emitGradePreserveConstraint(grade, gk, s)
				continue
			}

			// Fast path: concrete grade, check immediately.
			if !ch.gradeCanPreserveDynamic(grade, gk) {
				ch.addCodedError(diagnostic.ErrMultiplicity, s,
					fmt.Sprintf("@%s capability %q must be consumed (type unchanged across computation boundary)",
						types.Pretty(grade), f.Label))
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
			ch.addCodedError(diagnostic.ErrMultiplicity, s,
				fmt.Sprintf("@%s capability must be consumed (type unchanged across computation boundary)",
					types.Pretty(grade)))
		}
		return
	}

	ct := &CtFunEq{
		FamilyName: algebra.joinFamily,
		Args:       args,
		ResultMeta: resultMeta,
		BlockingOn: blocking,
		OnFailure: func(errSpan span.Span, expected, actual types.Type) {
			ch.addCodedError(diagnostic.ErrMultiplicity, errSpan,
				fmt.Sprintf("@%s capability must be consumed (grade preservation violation: expected %s, got %s)",
					types.Pretty(grade), types.Pretty(expected), types.Pretty(actual)))
		},
		S: s,
	}
	ch.registerStuckFunEq(ct)

	// When the family reduces, resultMeta will be unified with Join(Zero, grade).
	// Unify resultMeta ~ grade so that preservation is enforced: the result
	// of Join(Zero, grade) must equal grade itself.
	ch.emitEq(resultMeta, grade, s, nil)
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
