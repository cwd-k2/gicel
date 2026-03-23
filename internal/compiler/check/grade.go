package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Grade algebra: Join and Drop ---

// gradeJoin computes Join(a, b) for two grade values from the same algebra.
// Dispatches on the head type constructor of the grades.
func gradeJoin(a, b types.Type) types.Type {
	ac, aOk := a.(*types.TyCon)
	bc, bOk := b.(*types.TyCon)
	if !aOk || !bOk {
		return a // fallback: keep first (conservative)
	}
	return usageJoin(ac, bc)
}

// gradeDrop returns the Drop element for the grade algebra of the given grade.
func gradeDrop(grade types.Type) types.Type {
	if con, ok := grade.(*types.TyCon); ok {
		switch con.Name {
		case "Linear", "Affine", "Unrestricted", "Zero":
			return &types.TyCon{Name: "Zero"}
		}
	}
	return &types.TyCon{Name: "Zero"} // default for unknown algebras
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

	// Sorted pairs: (Affine,Linear), (Affine,Unrestricted), (Affine,Zero),
	//               (Linear,Unrestricted), (Linear,Zero), (Unrestricted,Zero)
	switch {
	case x == "Unrestricted" || y == "Unrestricted":
		return &types.TyCon{Name: "Unrestricted"}
	case x == "Affine" || y == "Affine":
		// Affine joined with anything except Unrestricted = Affine (handled above)
		return &types.TyCon{Name: "Affine"}
	case (x == "Linear" && y == "Zero") || (x == "Zero" && y == "Linear"):
		return &types.TyCon{Name: "Affine"}
	default:
		return a // fallback
	}
}

// --- Grade boundary check ---

// checkGradeBoundary verifies that capability fields with grade annotations
// respect their grades across the computation boundary.
//
// Phase 2: structural enforcement. The grade algebra (Join, Drop) is defined
// and boundary violations are detected via gradeCanPreserve. A linear or
// zero-graded field that appears unchanged in post triggers a multiplicity error.
// Grade constraint emission via CtFunEq (associated type families) is deferred
// to a future phase for full algebraic grade propagation.
//
// For each graded field in pre: if the field appears in post with the same type
// (i.e., was preserved unchanged), the grade must permit preservation.
// A field that was consumed (absent from post) or transitioned (type changed)
// is always valid regardless of grade.
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

		postTy := capFieldTypeByLabel(postFields, f.Label)
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
			if !gradeCanPreserve(grade) {
				ch.addCodedError(diagnostic.ErrMultiplicity, s,
					fmt.Sprintf("@%s capability %q must be consumed (type unchanged across computation boundary)",
						types.Pretty(grade), f.Label))
			}
		}
	}
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

// capFieldTypeByLabel returns the type of a field with the given label, or nil.
func capFieldTypeByLabel(fields []types.RowField, label string) types.Type {
	for _, f := range fields {
		if f.Label == label {
			return f.Type
		}
	}
	return nil
}
