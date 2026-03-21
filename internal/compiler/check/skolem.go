package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// containsSkolem checks whether the given type contains any TySkolem
// with an ID in the provided set, after zonking.
// Returns the ID and true if found.
func (ch *Checker) containsSkolem(ty types.Type, skolemIDs map[int]string) (int, bool) {
	ty = ch.unifier.Zonk(ty)
	if sk, ok := ty.(*types.TySkolem); ok {
		if _, found := skolemIDs[sk.ID]; found {
			return sk.ID, true
		}
		return 0, false
	}
	for _, child := range ty.Children() {
		if id, found := ch.containsSkolem(child, skolemIDs); found {
			return id, true
		}
	}
	return 0, false
}

// checkSkolemEscape verifies that no skolem from the given set appears
// in the type. Adds a diagnostic error if escape is detected.
func (ch *Checker) checkSkolemEscape(ty types.Type, skolemIDs map[int]string, s span.Span) {
	if len(skolemIDs) == 0 {
		return
	}
	if id, found := ch.containsSkolem(ty, skolemIDs); found {
		name := skolemIDs[id]
		ch.addCodedError(diagnostic.ErrSkolemEscape, s,
			fmt.Sprintf("existential type variable '#%s' escapes its scope", name))
	}
}

// removeSkolemIDsFrom removes any skolem IDs found in ty from the ids map.
// Used to exclude GADT-refined skolems from escape checking: constructor
// skolems that appear in GivenEqs values are part of the refinement and
// may legitimately appear in the result type.
func removeSkolemIDsFrom(ids map[int]string, ty types.Type) {
	if sk, ok := ty.(*types.TySkolem); ok {
		delete(ids, sk.ID)
		return
	}
	for _, child := range ty.Children() {
		removeSkolemIDsFrom(ids, child)
	}
}

// checkSkolemEscapeInSolutions checks that a skolem does not appear in
// solutions for metas created before the given scope boundary (preID).
// Metas created after the skolem are in its scope and may reference it.
func (ch *Checker) checkSkolemEscapeInSolutions(skolem *types.TySkolem, preID int, s span.Span) {
	ids := map[int]string{skolem.ID: skolem.Name}
	for metaID, soln := range ch.unifier.Solutions() {
		if metaID > preID {
			continue // meta is within the skolem's scope
		}
		zonked := ch.unifier.Zonk(soln)
		if _, found := ch.containsSkolem(zonked, ids); found {
			ch.addCodedError(diagnostic.ErrSkolemEscape, s,
				fmt.Sprintf("type variable '%s' would escape its scope", skolem.Name))
			return
		}
	}
}
