package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Internal type family names for grade algebra. The '$' prefix is not
// valid in user identifiers, guaranteeing no collision.
const (
	gradeJoinFamily = "$GradeJoin"
	gradeDropFamily = "$GradeDrop"
)

// --- Grade algebra: Join and Drop ---

// gradeJoin computes Join(a, b) for two grade values from the same algebra.
// Dispatches on the head type constructor of the grades.
func gradeJoin(a, b types.Type) types.Type {
	ac, aOk := a.(*types.TyCon)
	bc, bOk := b.(*types.TyCon)
	if !aOk || !bOk {
		// Non-TyCon grade (e.g., unsolved meta). Return first argument as
		// conservative fallback. The CtFunEq path in checkGradeBoundary
		// handles the deferred case when metas are involved.
		return a
	}
	return usageJoin(ac, bc)
}

// gradeDrop returns the Drop element for the usage grade algebra.
// Always Zero. When multiple grade algebras are supported, dispatch
// on the algebra tag here.
func gradeDrop(_ types.Type) types.Type {
	return types.Con("Zero")
}

// gradeCanPreserve checks whether a field with the given grade can be preserved
// unchanged across a computation boundary: Join(Drop, grade) ~ grade.
func gradeCanPreserve(grade types.Type) bool {
	drop := gradeDrop(grade)
	joined := gradeJoin(drop, grade)
	return types.Equal(joined, grade)
}

// usageJoin implements the Join operation for the usage/multiplicity grade algebra.
//
// Lattice:
//
//	  Unrestricted
//	      |
//	    Affine
//	   /      \
//	Zero    Linear
//
// Zero and Linear are incomparable. Join(Zero, Linear) = Affine.
func usageJoin(a, b *types.TyCon) *types.TyCon {
	if a.Name == b.Name {
		return a
	}

	// Canonical ordering for lookup: sort by name to reduce cases.
	x, y := a.Name, b.Name
	if x > y {
		x, y = y, x
	}

	// After sort: x <= y (string order: Affine < Linear < Unrestricted < Zero).
	// This is NOT the lattice order — it is only used to reduce case count.
	switch {
	case x == "Unrestricted" || y == "Unrestricted":
		return types.Con("Unrestricted")
	case x == "Affine" || y == "Affine":
		return types.Con("Affine")
	case x == "Linear" && y == "Zero":
		// After sort x <= y: "Linear" < "Zero", so this is the only reachable ordering.
		return types.Con("Affine")
	default:
		// Unknown grade constructor name. The usage algebra is closed
		// ({Zero, Linear, Affine, Unrestricted}); reaching here means a
		// grade value was promoted from a user-defined type that does not
		// belong to the algebra. Return first argument conservatively.
		return a
	}
}

// --- Grade algebra type families ---

// registerGradeAlgebraFamilies registers internal type families $GradeJoin
// and $GradeDrop that encode the usage/multiplicity lattice as type-level
// equations. These families enable constraint-based grade enforcement via
// CtFunEq when grades contain unsolved metavariables.
//
// The equations mirror the usageJoin lattice and gradeDrop exactly.
func (ch *Checker) registerGradeAlgebraFamilies() {
	multKind := gradeAlgebraKind(ch)

	zero := types.Con("Zero")
	linear := types.Con("Linear")
	affine := types.Con("Affine")
	unrestricted := types.Con("Unrestricted")
	wildcard := &types.TyVar{Name: "_"}

	// $GradeJoin :: Mult -> Mult -> Mult
	//
	// Equations encode the full usage lattice join. Order matters:
	// specific patterns before wildcards (closed family semantics).
	joinInfo := &TypeFamilyInfo{
		Name: gradeJoinFamily,
		Params: []TFParam{
			{Name: "a", Kind: multKind},
			{Name: "b", Kind: multKind},
		},
		ResultKind: multKind,
		Equations: []tfEquation{
			// Identity cases for non-absorbing elements.
			// Unrestricted identity is subsumed by the wildcard cases below.
			{Patterns: []types.Type{zero, zero}, RHS: zero},
			{Patterns: []types.Type{linear, linear}, RHS: linear},
			{Patterns: []types.Type{affine, affine}, RHS: affine},
			// Unrestricted absorbs everything (including itself)
			{Patterns: []types.Type{unrestricted, wildcard}, RHS: unrestricted},
			{Patterns: []types.Type{wildcard, unrestricted}, RHS: unrestricted},
			// Zero ⊔ Linear = Affine (incomparable elements)
			{Patterns: []types.Type{zero, linear}, RHS: affine},
			{Patterns: []types.Type{linear, zero}, RHS: affine},
			// Affine absorbs Zero and Linear
			{Patterns: []types.Type{affine, wildcard}, RHS: affine},
			{Patterns: []types.Type{wildcard, affine}, RHS: affine},
		},
	}
	ch.reg.RegisterFamily(gradeJoinFamily, joinInfo)

	// $GradeDrop :: Mult
	//
	// Zero-parameter family. Drop for the usage algebra is always Zero.
	dropInfo := &TypeFamilyInfo{
		Name:       gradeDropFamily,
		ResultKind: multKind,
		Equations: []tfEquation{
			{Patterns: nil, RHS: zero},
		},
	}
	ch.reg.RegisterFamily(gradeDropFamily, dropInfo)
}

// gradeAlgebraKind returns the kind to use for grade algebra parameters.
// If "Mult" is registered as a promoted kind (via DataKinds), returns
// KData{"Mult"}; otherwise falls back to KType{}.
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
}

// resolveGradeAlgebra looks up a GradeAlgebra instance for the given grade kind
// and extracts the associated type family names by reducing the associated types.
// Falls back to the internal $GradeJoin/$GradeDrop families if no user-defined
// GradeAlgebra instance is found.
func (ch *Checker) resolveGradeAlgebra(gradeKind types.Type) resolvedGradeAlgebra {
	classInfo, hasClass := ch.reg.LookupClass(gradeAlgebraClassName)
	if hasClass {
		// Match grade kind against instance type args.
		// GradeAlgebra takes a Kind-kinded parameter (g: Kind).
		// Instance: impl GradeAlgebra Mult := ...
		// Instance TypeArgs[0] = TyCon("Mult"), which is a type constructor (kind Type).
		// Grade kind = KData{"Mult"} (promoted kind from DataKinds).
		// Match by comparing the type arg name with the KData name.
		instances := ch.reg.InstancesForClass(gradeAlgebraClassName)
		for _, inst := range instances {
			if len(inst.TypeArgs) == 0 {
				continue
			}
			if con, ok := inst.TypeArgs[0].(*types.TyCon); ok {
				if dk, ok := gradeKind.(*types.TyCon); ok && types.IsKindLevel(dk.Level) && dk.Name == con.Name {
					return ch.extractGradeAlgebra(classInfo, inst)
				}
			}
		}
	}
	// Fallback: use internal families.
	return resolvedGradeAlgebra{
		joinFamily: gradeJoinFamily,
		dropValue:  types.Con("Zero"),
	}
}

// extractGradeAlgebra extracts GradeJoin and GradeDrop from a matched instance
// by reducing the associated type families with the instance's type args.
func (ch *Checker) extractGradeAlgebra(classInfo *ClassInfo, inst *InstanceInfo) resolvedGradeAlgebra {
	result := resolvedGradeAlgebra{
		joinFamily: gradeJoinFamily, // default fallback
		dropValue:  types.Con("Zero"),
	}
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
			// The reduced result should be a type family name (TyCon).
			if con, ok := reduced.(*types.TyCon); ok {
				result.joinFamily = con.Name
			} else {
				result.joinFamily = assocName
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
//   - Concrete grade (no metas): fast path via gradeCanPreserve with immediate error.
//   - Grade containing metas: emit CtFunEq constraint "$GradeJoin(Zero, grade) ~ grade"
//     so the solver can re-check once the meta is solved.
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
	drop := algebra.dropValue
	joined, ok := ch.reduceTyFamily(algebra.joinFamily, []types.Type{drop, grade}, span.Span{})
	if !ok {
		// Family reduction failed (stuck or no match).
		// Fallback to the hardcoded check for backward compatibility.
		return gradeCanPreserve(grade)
	}
	return types.Equal(joined, grade)
}

// emitGradePreserveConstraint emits a CtFunEq constraint encoding the
// preservation check: Join(Drop, grade) ~ grade.
//
// If the grade is e.g. a metavariable ?m, this constraint says:
// "when ?m is solved, Join(Drop, ?m) must equal ?m" — which is the
// algebraic definition of gradeCanPreserve.
func (ch *Checker) emitGradePreserveConstraint(grade types.Type, gradeKind types.Type, s span.Span) {
	algebra := ch.resolveGradeAlgebra(gradeKind)
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
	ch.solver.RegisterStuckFunEq(ct)

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
