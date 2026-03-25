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
func gradeAlgebraKind(ch *Checker) types.Kind {
	if k, ok := ch.reg.LookupPromotedKind("Mult"); ok {
		return k
	}
	return types.KType{}
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

			if gradeContainsMeta(grade) {
				// Deferred path: emit CtFunEq for $GradeJoin(Zero, grade) ~ grade.
				// When the meta is solved, the solver re-processes the constraint
				// and the family reduces to a concrete result for unification.
				ch.emitGradePreserveConstraint(grade, s)
				continue
			}

			// Fast path: concrete grade, check immediately.
			if !gradeCanPreserve(grade) {
				ch.addCodedError(diagnostic.ErrMultiplicity, s,
					fmt.Sprintf("@%s capability %q must be consumed (type unchanged across computation boundary)",
						types.Pretty(grade), f.Label))
			}
		}
	}
}

// emitGradePreserveConstraint emits a CtFunEq constraint encoding the
// preservation check: $GradeJoin(Zero, grade) ~ grade.
//
// If the grade is e.g. a metavariable ?m, this constraint says:
// "when ?m is solved, Join(Zero, ?m) must equal ?m" — which is the
// algebraic definition of gradeCanPreserve.
func (ch *Checker) emitGradePreserveConstraint(grade types.Type, s span.Span) {
	multKind := gradeAlgebraKind(ch)
	zero := types.Con("Zero")
	args := []types.Type{zero, grade}

	resultMeta := ch.freshMeta(multKind)
	blocking := ch.unifier.CollectBlockingMetas(args)
	if len(blocking) == 0 {
		// Invariant: gradeContainsMeta was true, so CollectBlockingMetas should
		// find at least one meta. If not, the meta was zonked between the check
		// and here. Fall back to the concrete fast path.
		if !gradeCanPreserve(ch.unifier.Zonk(grade)) {
			ch.addCodedError(diagnostic.ErrMultiplicity, s,
				fmt.Sprintf("@%s capability must be consumed (type unchanged across computation boundary)",
					types.Pretty(grade)))
		}
		return
	}

	ct := &CtFunEq{
		FamilyName: gradeJoinFamily,
		Args:       args,
		ResultMeta: resultMeta,
		BlockingOn: blocking,
		S:          s,
	}
	ch.solver.RegisterStuckFunEq(ct)

	// When the family reduces, resultMeta will be unified with Join(Zero, grade).
	// Unify resultMeta ~ grade so that preservation is enforced: the result
	// of Join(Zero, grade) must equal grade itself.
	_ = ch.unifier.Unify(resultMeta, grade) //nolint:errcheck // advisory
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
