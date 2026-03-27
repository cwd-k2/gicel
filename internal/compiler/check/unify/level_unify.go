package unify

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// unifyLevels solves the constraint l1 = l2 at the universe level.
func (u *Unifier) unifyLevels(l1, l2 types.LevelExpr) error {
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
			if err := u.unifyLevels(a.A, b.A); err != nil {
				return err
			}
			return u.unifyLevels(a.B, b.B)
		}
	case *types.LevelSucc:
		if b, ok := l2.(*types.LevelSucc); ok {
			return u.unifyLevels(a.E, b.E)
		}
	}

	return &UnifyError{
		Kind:   UnifyMismatch,
		Detail: fmt.Sprintf("level mismatch: %s vs %s", l1.LevelString(), l2.LevelString()),
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
			Detail: fmt.Sprintf("infinite level: ?l%d", m.ID),
		}
	}
	u.trailLevelWrite(m.ID)
	u.levelSoln[m.ID] = l
	return nil
}

// levelOccursIn checks whether a level metavariable ID appears in a level expression.
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

// DefaultLevels defaults all unsolved level metavariables to L0 (value type level).
// Called after constraint solving is complete to ground any remaining level metas.
func (u *Unifier) DefaultLevels() {
	// Unsolved level metas remain in the map with no entry.
	// zonkLevel returns them as-is; ZonkLevelDefault replaces them with L0.
	// This is a future extension point — for now the method exists as API surface.
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
