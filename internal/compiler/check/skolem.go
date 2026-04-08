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

// checkSkolemSetEscapeSince checks that no soln write that happened at
// or after trailPos with metaID ≤ preID contains any skolem in the
// given set. The map is keyed by skolem ID and stores the source name
// for diagnostic messages.
//
// This is the *belt-and-suspenders* safety net for skolem escape. When
// level-based touchability is enabled, the guard in solveMeta prevents
// outer metas from being solved with inner-scope values. However, trial
// scopes (withTrial / withProbe) deliberately disable touchability so
// that speculative unifications can probe without commitment. A trial
// that succeeds and commits its solutions can in principle write a
// pre-preID meta with a value containing the active skolem — at which
// point this check is the only thing standing between the type checker
// and silently broken types.
//
// The previous full-map iteration walked u.Solutions() per polymorphic
// binding (once per TyForall introduction), which was O(total solns)
// per call. The cold-start profile showed this consuming 9.43% cum CPU
// on a Prelude compile, making it the largest single CPU consumer in
// the type checker after substDepth. The trail-incremental version
// walks only the writes that actually happened during the body, which
// for a typical forall body is a tiny fraction of the full soln map.
//
// Algorithm: VisitSolnWritesSince enumerates each unique meta written
// since trailPos. For each ID ≤ preID we read the current value via
// Solve and run the same Zonk + containsSkolem check as before. The
// set form lets a chain of nested forall binders (e.g. forall a. forall
// b. forall c. T) share a single trail walk instead of N separate ones.
func (ch *Checker) checkSkolemSetEscapeSince(skolemIDs map[int]string, preID int, trailPos int, s span.Span) {
	if len(skolemIDs) == 0 {
		return
	}
	var foundID int
	var found bool
	ch.unifier.VisitSolnWritesSince(trailPos, func(metaID int) {
		if found || metaID > preID {
			return
		}
		soln := ch.unifier.Solve(metaID)
		if soln == nil || !types.HasMeta(soln) {
			return
		}
		zonked := ch.unifier.Zonk(soln)
		if id, ok := ch.containsSkolem(zonked, skolemIDs); ok {
			foundID = id
			found = true
		}
	})
	if found {
		ch.addCodedError(diagnostic.ErrSkolemEscape, s,
			"type variable '"+skolemIDs[foundID]+"' would escape its scope")
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
