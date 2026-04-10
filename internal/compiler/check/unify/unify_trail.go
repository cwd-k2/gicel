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
type trailEntry struct {
	tag      trailTag
	id       int
	existed  bool
	oldType  types.Type          // valid when tag == trailSoln or trailSkolemSoln
	oldLbl   map[string]struct{} // valid when tag == trailLabel
	oldLevel types.LevelExpr     // valid when tag == trailLevelSoln
}

// Snapshot records the current trail position for later rollback.
// O(1) — no map copying.
type Snapshot struct {
	pos int
}

// Snapshot captures the current unifier state for later rollback.
func (u *Unifier) Snapshot() Snapshot {
	u.snapshotDepth++
	return Snapshot{pos: len(u.trail)}
}

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

// TrailLen returns the current trail length, suitable as a position
// marker passed to VisitSolnWritesSince. Use to record the start of a
// scope before running checker work; afterwards, walking from the saved
// position visits exactly the soln writes that happened during the
// scope.
//
// Cheap (one slice length read). No allocation. Safe outside snapshot
// scopes — the trail accumulates writes regardless of snapshot depth.
func (u *Unifier) TrailLen() int { return len(u.trail) }

// VisitSolnWritesSince calls fn for each soln write at or after the
// given trail position, in trail order. fn receives the meta ID; the
// caller can resolve the current value via Solve(id).
//
// NOT deduplicated: if the same meta was written multiple times in
// the range (e.g. via path compression), fn is called multiple times
// with the same id. Callers whose check is idempotent over the current
// value can ignore the duplicates; callers that need exactly one visit
// per id must dedupe themselves. The non-deduping API eliminates a
// dedup-map allocation per call.
//
// Used by the bidirectional checker's TyForall arm to walk only the
// soln writes that happened during the body, instead of iterating the
// entire soln map. The cold-start profile showed the full-map
// iteration consuming 9.43% cum CPU on Prelude before this method was
// introduced.
func (u *Unifier) VisitSolnWritesSince(pos int, fn func(metaID int)) {
	for i := pos; i < len(u.trail); i++ {
		e := &u.trail[i]
		if e.tag == trailSoln {
			fn(e.id)
		}
	}
}

// IsolationToken captures the state that BeginProbeIsolated saves
// and EndProbeIsolated restores. Use to wrap a trial unification
// scope that needs **state isolation** stronger than withTrial /
// withProbe alone.
//
// What this isolates beyond a plain Snapshot/Restore:
//
//   - SolverLevel is forced to -1 so touchability is suspended for
//     the duration; speculative unifications can solve metas at any
//     level (the same semantics as withProbe).
//   - OnSolve is nilled out so solveMeta cannot reach into the
//     solver's worklist. Snapshot/Restore can roll back trail-tracked
//     state (soln, labels, skolemSoln, levelSoln) but **cannot** undo
//     worklist mutations made by Reactivate; nilling OnSolve removes
//     the side channel entirely.
//   - FamilyReducer and AliasExpander are nilled out so the trial
//     sees only raw types — matching the semantics that the previous
//     "tmp := NewUnifier()" callers relied on (the fresh unifier did
//     not have these callbacks set).
//   - FlexSkolems is preserved across the scope so the caller may
//     temporarily flip it inside (e.g. canUnifyWith for GADT
//     accessibility) and have it auto-restored.
//
// Caller MUST pair every BeginProbeIsolated with EndProbeIsolated,
// passing back the returned token. Panics across the scope leak the
// state — but probe paths are not expected to panic; an internal
// invariant violation means the compiler is in an unrecoverable
// state regardless.
type IsolationToken struct {
	snap    Snapshot
	level   int
	onSolve func(int)
	family  FamilyReducer
	alias   AliasExpander
	flexSks bool
}

// BeginProbeIsolated enters an isolated probe scope on the unifier.
// All mutations during the scope are rolled back when EndProbeIsolated
// is called with the returned token; side-effect callbacks are
// suspended; touchability is disabled.
//
// Returns a stack-friendly token (small struct, no heap allocation).
func (u *Unifier) BeginProbeIsolated() IsolationToken {
	tok := IsolationToken{
		snap:    u.Snapshot(),
		level:   u.SolverLevel,
		onSolve: u.OnSolve,
		family:  u.FamilyReducer,
		alias:   u.AliasExpander,
		flexSks: u.FlexSkolems,
	}
	u.SolverLevel = -1
	u.OnSolve = nil
	u.FamilyReducer = nil
	u.AliasExpander = nil
	return tok
}

// EndProbeIsolated reverts everything that BeginProbeIsolated changed.
// MUST be called with the token returned by the matching Begin call.
func (u *Unifier) EndProbeIsolated(tok IsolationToken) {
	u.AliasExpander = tok.alias
	u.FamilyReducer = tok.family
	u.OnSolve = tok.onSolve
	u.FlexSkolems = tok.flexSks
	u.SolverLevel = tok.level
	u.Restore(tok.snap)
}

// trailSolnWrite records the current soln[id] value before mutation.
func (u *Unifier) trailSolnWrite(id int) {
	old, existed := u.soln[id]
	u.trail = append(u.trail, trailEntry{
		tag: trailSoln, id: id, existed: existed, oldType: old,
	})
}

// trailLabelWrite records the current labels[id] value before mutation.
func (u *Unifier) trailLabelWrite(id int) {
	old, existed := u.labels[id]
	u.trail = append(u.trail, trailEntry{
		tag: trailLabel, id: id, existed: existed, oldLbl: old,
	})
}

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
func (u *Unifier) trailLevelWrite(id int) {
	old, existed := u.levelSoln[id]
	u.trail = append(u.trail, trailEntry{
		tag: trailLevelSoln, id: id, existed: existed, oldLevel: old,
	})
}

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
func (u *Unifier) RemoveGivenEq(skolemID int) {
	if u.skolemSoln != nil {
		delete(u.skolemSoln, skolemID)
	}
}

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
