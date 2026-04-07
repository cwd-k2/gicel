package check

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// containsSkolem checks whether the given type contains any TySkolem
// with an ID in the provided set. The type must already be zonked —
// this walker does NOT call Zonk and assumes meta substitution has
// been resolved by the caller. Use Zonk first when in doubt.
//
// The walk is short-circuited via FlagMetaFree (FlagMetaFree implies no
// metas AND no skolems), so meta-free subtrees are pruned in O(1).
// Returns the matched skolem ID and true on first match; (0, false)
// otherwise.
func (ch *Checker) containsSkolem(ty types.Type, skolemIDs map[int]string) (int, bool) {
	if !types.HasMeta(ty) {
		// FlagMetaFree → no metas and no skolems anywhere in this subtree.
		return 0, false
	}
	if sk, ok := ty.(*types.TySkolem); ok {
		if _, found := skolemIDs[sk.ID]; found {
			return sk.ID, true
		}
		return 0, false
	}
	var foundID int
	var found bool
	types.ForEachChild(ty, func(child types.Type) bool {
		if id, ok := ch.containsSkolem(child, skolemIDs); ok {
			foundID, found = id, true
			return false
		}
		return true
	})
	if found {
		return foundID, true
	}
	return 0, false
}

// checkSkolemEscape verifies that no skolem from the given set appears
// in the type. Adds a diagnostic error if escape is detected.
//
// The type is zonked once before the walk so that containsSkolem can
// rely on the no-internal-Zonk invariant.
func (ch *Checker) checkSkolemEscape(ty types.Type, skolemIDs map[int]string, s span.Span) {
	if len(skolemIDs) == 0 {
		return
	}
	zonked := ch.unifier.Zonk(ty)
	if id, found := ch.containsSkolem(zonked, skolemIDs); found {
		name := skolemIDs[id]
		ch.addCodedError(diagnostic.ErrSkolemEscape, s,
			"existential type variable '#"+name+"' escapes its scope")
	}
}

// checkSkolemEscapeInSolutions checks that a skolem does not appear in
// solutions for metas created before the given scope boundary (preID).
// Metas created after the skolem are in its scope and may reference it.
// Belt-and-suspenders check: when level-based touchability is enabled,
// this should never fire (the touchability guard prevents the escape).
//
// Cost note: this is called once per polymorphic binding via
// `bidir.go:checkAgainst`'s TyForall arm, which makes it run O(N) times
// per Prelude compile where N = number of forall introductions. Hot
// path optimization is therefore valuable even though the loop body
// rarely matches.
//
// Filtering order: HasMeta(soln) is checked before Zonk so that ground
// solutions are pruned in O(1) (FlagMetaFree fast path on composites).
// containsSkolem itself does not re-Zonk; the caller's single Zonk
// covers the whole subtree.
func (ch *Checker) checkSkolemEscapeInSolutions(skolem *types.TySkolem, preID int, s span.Span) {
	ids := map[int]string{skolem.ID: skolem.Name}
	for metaID, soln := range ch.unifier.Solutions() {
		if metaID > preID {
			continue // meta is within the skolem's scope
		}
		// Ground solutions (no TyMeta or TySkolem) cannot contain the
		// target skolem after Zonk — skip via the FlagMetaFree fast path.
		if !types.HasMeta(soln) {
			continue
		}
		zonked := ch.unifier.Zonk(soln)
		if _, found := ch.containsSkolem(zonked, ids); found {
			ch.addCodedError(diagnostic.ErrSkolemEscape, s,
				"type variable '"+skolem.Name+"' would escape its scope")
			return
		}
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
	types.ForEachChild(ty, func(child types.Type) bool {
		removeSkolemIDsFrom(ids, child)
		return true
	})
}
