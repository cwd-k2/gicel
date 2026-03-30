package unify

import (
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// unifyLevels solves the constraint l1 = l2 at the universe level.
func (u *Unifier) UnifyLevels(l1, l2 types.LevelExpr) error {
	l1 = u.zonkLevel(l1)
	l2 = u.zonkLevel(l2)

	// Meta solving.
	if m, ok := l1.(*types.LevelMeta); ok {
		return u.solveLevelMeta(m, l2)
	}
	if m, ok := l2.(*types.LevelMeta); ok {
		return u.solveLevelMeta(m, l1)
	}

	// Structural comparison.
	switch a := l1.(type) {
	case *types.LevelLit:
		if b, ok := l2.(*types.LevelLit); ok && a.N == b.N {
			return nil
		}
	case *types.LevelVar:
		if b, ok := l2.(*types.LevelVar); ok && a.Name == b.Name {
			return nil
		}
	case *types.LevelMax:
		if b, ok := l2.(*types.LevelMax); ok {
			if err := u.UnifyLevels(a.A, b.A); err != nil {
				return err
			}
			return u.UnifyLevels(a.B, b.B)
		}
	case *types.LevelSucc:
		if b, ok := l2.(*types.LevelSucc); ok {
			return u.UnifyLevels(a.E, b.E)
		}
	}

	return &UnifyError{
		Kind:  UnifyMismatch,
		Label: l1.LevelString() + " vs " + l2.LevelString(),
	}
}

// solveLevelMeta binds a level metavariable to a level expression.
func (u *Unifier) solveLevelMeta(m *types.LevelMeta, l types.LevelExpr) error {
	if m2, ok := l.(*types.LevelMeta); ok && m2.ID == m.ID {
		return nil
	}
	if levelOccursIn(m.ID, l) {
		return &UnifyError{
			Kind:   UnifyOccursCheck,
			MetaID: m.ID,
		}
	}
	u.trailLevelWrite(m.ID)
	u.levelSoln[m.ID] = l
	return nil
}

// levelOccursIn checks whether a level metavariable ID appears in a level expression.
// No budget check: level expressions are structurally small (bounded by the
// number of level operations in the source program).
func levelOccursIn(id int, l types.LevelExpr) bool {
	switch ll := l.(type) {
	case *types.LevelMeta:
		return ll.ID == id
	case *types.LevelMax:
		return levelOccursIn(id, ll.A) || levelOccursIn(id, ll.B)
	case *types.LevelSucc:
		return levelOccursIn(id, ll.E)
	default:
		return false
	}
}

// ZonkLevelDefault replaces solved level metas with their solutions and
// unsolved level metas with L0. Use after type checking is complete.
func (u *Unifier) ZonkLevelDefault(l types.LevelExpr) types.LevelExpr {
	if l == nil {
		return types.L0
	}
	switch ll := l.(type) {
	case *types.LevelMeta:
		soln, ok := u.levelSoln[ll.ID]
		if !ok {
			return types.L0 // default unsolved to L0
		}
		return u.ZonkLevelDefault(soln)
	case *types.LevelMax:
		zA := u.ZonkLevelDefault(ll.A)
		zB := u.ZonkLevelDefault(ll.B)
		if zA == ll.A && zB == ll.B {
			return ll
		}
		return &types.LevelMax{A: zA, B: zB}
	case *types.LevelSucc:
		zE := u.ZonkLevelDefault(ll.E)
		if zE == ll.E {
			return ll
		}
		return &types.LevelSucc{E: zE}
	default:
		return l
	}
}

// zonkLevel replaces solved level metavariables with their solutions.
func (u *Unifier) zonkLevel(l types.LevelExpr) types.LevelExpr {
	if l == nil {
		return types.L0
	}
	switch ll := l.(type) {
	case *types.LevelMeta:
		soln, ok := u.levelSoln[ll.ID]
		if !ok {
			return ll
		}
		result := u.zonkLevel(soln)
		if result != soln {
			if u.snapshotDepth > 0 {
				u.trailLevelWrite(ll.ID)
			}
			u.levelSoln[ll.ID] = result // path compression
		}
		return result
	case *types.LevelMax:
		zA := u.zonkLevel(ll.A)
		zB := u.zonkLevel(ll.B)
		if zA == ll.A && zB == ll.B {
			return ll
		}
		return &types.LevelMax{A: zA, B: zB}
	case *types.LevelSucc:
		zE := u.zonkLevel(ll.E)
		if zE == ll.E {
			return ll
		}
		return &types.LevelSucc{E: zE}
	default:
		return l
	}
}
