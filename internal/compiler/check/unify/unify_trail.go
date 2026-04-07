// Unifier — trail-based snapshot/restore machinery and skolem/label contexts.
// O(1) Snapshot + O(k) Restore via an undo log of per-map mutations.
// Does NOT cover: core unification (unify.go), normalization (unify_normalize.go).

package unify

import (
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// trailTag discriminates the three maps that a trail entry can target.
type trailTag byte

const (
	trailSoln       trailTag = iota // soln map
	trailLabel                      // labels map
	trailSkolemSoln                 // skolemSoln map
	trailLevelSoln                  // levelSoln map
)

// trailEntry records a single map mutation for undo-log rollback.
// On Restore, entries are replayed in reverse order, restoring the
// pre-mutation value (or deleting the key if it did not exist).

// trailEntry records a single map mutation for undo-log rollback.
// On Restore, entries are replayed in reverse order, restoring the
// pre-mutation value (or deleting the key if it did not exist).
type trailEntry struct {
	tag      trailTag
	id       int
	existed  bool
	oldType  types.Type          // valid when tag == trailSoln or trailSkolemSoln
	oldLbl   map[string]struct{} // valid when tag == trailLabel
	oldLevel types.LevelExpr     // valid when tag == trailLevelSoln
}

// Unifier manages type unification.

// Snapshot records the current trail position for later rollback.
// O(1) — no map copying.
type Snapshot struct {
	pos int
}

// Snapshot captures the current unifier state for later rollback.

// Snapshot captures the current unifier state for later rollback.
func (u *Unifier) Snapshot() Snapshot {
	u.snapshotDepth++
	return Snapshot{pos: len(u.trail)}
}

// Restore rolls back the unifier to a previously saved snapshot by replaying
// the trail in reverse. O(k) where k = number of mutations since snapshot.

// Restore rolls back the unifier to a previously saved snapshot by replaying
// the trail in reverse. O(k) where k = number of mutations since snapshot.
func (u *Unifier) Restore(snap Snapshot) {
	for i := len(u.trail) - 1; i >= snap.pos; i-- {
		e := &u.trail[i]
		switch e.tag {
		case trailSoln:
			if e.existed {
				u.soln[e.id] = e.oldType
			} else {
				delete(u.soln, e.id)
			}
		case trailLabel:
			if e.existed {
				u.labels[e.id] = e.oldLbl
			} else {
				delete(u.labels, e.id)
			}
		case trailSkolemSoln:
			if e.existed {
				u.skolemSoln[e.id] = e.oldType
			} else {
				delete(u.skolemSoln, e.id)
			}
		case trailLevelSoln:
			if e.existed {
				u.levelSoln[e.id] = e.oldLevel
			} else {
				delete(u.levelSoln, e.id)
			}
		}
	}
	u.trail = u.trail[:snap.pos]
	u.snapshotDepth--
}

// trailSolnWrite records the current soln[id] value before mutation.

// trailSolnWrite records the current soln[id] value before mutation.
func (u *Unifier) trailSolnWrite(id int) {
	old, existed := u.soln[id]
	u.trail = append(u.trail, trailEntry{
		tag: trailSoln, id: id, existed: existed, oldType: old,
	})
}

// trailLabelWrite records the current labels[id] value before mutation.

// trailLabelWrite records the current labels[id] value before mutation.
func (u *Unifier) trailLabelWrite(id int) {
	old, existed := u.labels[id]
	u.trail = append(u.trail, trailEntry{
		tag: trailLabel, id: id, existed: existed, oldLbl: old,
	})
}

// trailSkolemWrite records the current skolemSoln[id] value before mutation.

// trailSkolemWrite records the current skolemSoln[id] value before mutation.
func (u *Unifier) trailSkolemWrite(id int) {
	if u.skolemSoln == nil {
		u.skolemSoln = make(map[int]types.Type)
	}
	old, existed := u.skolemSoln[id]
	u.trail = append(u.trail, trailEntry{
		tag: trailSkolemSoln, id: id, existed: existed, oldType: old,
	})
}

// trailLevelWrite records the current levelSoln[id] value before mutation.

// trailLevelWrite records the current levelSoln[id] value before mutation.
func (u *Unifier) trailLevelWrite(id int) {
	old, existed := u.levelSoln[id]
	u.trail = append(u.trail, trailEntry{
		tag: trailLevelSoln, id: id, existed: existed, oldLevel: old,
	})
}

// InstallGivenEq records a GADT given equality: the skolem with the given ID
// is locally equal to ty within the current scope. Use Snapshot/Restore to
// limit the lifetime of given equalities to a single case branch.

// InstallGivenEq records a GADT given equality: the skolem with the given ID
// is locally equal to ty within the current scope. Use Snapshot/Restore to
// limit the lifetime of given equalities to a single case branch.
func (u *Unifier) InstallGivenEq(skolemID int, ty types.Type) {
	u.trailSkolemWrite(skolemID)
	if u.skolemSoln == nil {
		u.skolemSoln = make(map[int]types.Type)
	}
	u.skolemSoln[skolemID] = ty
}

// RemoveGivenEq removes a given equality for the specified skolem.
// Used to scope given equalities to individual GADT case branches.

// RemoveGivenEq removes a given equality for the specified skolem.
// Used to scope given equalities to individual GADT case branches.
func (u *Unifier) RemoveGivenEq(skolemID int) {
	if u.skolemSoln != nil {
		delete(u.skolemSoln, skolemID)
	}
}

// RegisterLabelContext records the surrounding labels for a row metavariable.

// RegisterLabelContext records the surrounding labels for a row metavariable.
func (u *Unifier) RegisterLabelContext(id int, labels map[string]struct{}) {
	u.trailLabelWrite(id)
	u.labels[id] = labels
}

// normalize applies alias expansion, type family reduction, and special
// type normalization. Type family reduction is eager here for compatibility:
// many inference paths depend on TyFamilyApp being reduced before unification.
// The solver's CtFunEq path (L2-b) handles deferred reduction for stuck
// applications whose args contain unsolved metas.
