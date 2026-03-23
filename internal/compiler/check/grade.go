package check

import (
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
	return multJoin(ac, bc)
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

// multJoin implements the Join operation for the Mult grade algebra.
//
// Lattice:
//
//	      Unrestricted
//	          |
//	        Affine
//	       /      \
//	    Zero    Linear
//
// Zero and Linear are incomparable. Join(Zero, Linear) = Affine.
func multJoin(a, b *types.TyCon) *types.TyCon {
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
// Phase 1: structural placeholder. The grade algebra (Join, Drop) is defined
// and the infrastructure is in place, but enforcement is deferred to Phase 2
// when grade constraints will be emitted as CtFunEq through associated type
// families. Row unification already enforces type-level protocol compliance;
// the boundary check will add grade-specific verification on top.
//
// For each graded field in pre: if the field appears in post with the same type
// (i.e., was preserved unchanged), the grade must permit preservation.
// A field that was consumed (absent from post) or transitioned (type changed)
// is always valid regardless of grade.
func (ch *Checker) checkGradeBoundary(comp *types.TyCBPV, s span.Span) {
	// Phase 1: no enforcement. Grade checking will be activated in Phase 2
	// when GradeAlgebra is expressible as data + impl with associated type
	// families, and grade constraints flow through the solver as CtFunEq.
	_ = comp
	_ = s
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
