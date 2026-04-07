package unify

import (
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// UnifyLevels solves the constraint l1 = l2 at the universe level.
func (u *Unifier) UnifyLevels(l1, l2 types.LevelExpr) error {
	l1 = u.zonkLevel(l1)
	l2 = u.zonkLevel(l2)

	// Meta solving (before normalization to preserve meta structure).
	if m, ok := l1.(*types.LevelMeta); ok {
		return u.solveLevelMeta(m, l2)
	}
	if m, ok := l2.(*types.LevelMeta); ok {
		return u.solveLevelMeta(m, l1)
	}

	// Normalize both sides only when neither contains a meta.
	// If either side has a meta, preserve structure for structural matching
	// (e.g., max(?l, L1) = max(L0, L1) should match structurally first).
	hasMeta := levelContainsMeta(l1) || levelContainsMeta(l2)
	if !hasMeta {
		l1 = normalizeLevel(l1)
		l2 = normalizeLevel(l2)
	}

	// Post-normalization meta check.
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

	return &LevelMismatchError{
		A: l1.LevelString(),
		B: l2.LevelString(),
	}
}

// solveLevelMeta binds a level metavariable to a level expression.
func (u *Unifier) solveLevelMeta(m *types.LevelMeta, l types.LevelExpr) error {
	if m2, ok := l.(*types.LevelMeta); ok && m2.ID == m.ID {
		return nil
	}
	if levelOccursIn(m.ID, l) {
		return &OccursError{
			MetaID:  m.ID,
			IsLevel: true,
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

// levelContainsMeta reports whether a level expression contains any LevelMeta.
func levelContainsMeta(l types.LevelExpr) bool {
	switch ll := l.(type) {
	case *types.LevelMeta:
		return true
	case *types.LevelMax:
		return levelContainsMeta(ll.A) || levelContainsMeta(ll.B)
	case *types.LevelSucc:
		return levelContainsMeta(ll.E)
	default:
		return false
	}
}

// normalizeLevel simplifies concrete LevelMax and LevelSucc expressions.
//
//	max(LevelLit(a), LevelLit(b)) → LevelLit(max(a, b))
//	max(l, l) → l
//	succ(LevelLit(n)) → LevelLit(n+1)
func normalizeLevel(l types.LevelExpr) types.LevelExpr {
	switch ll := l.(type) {
	case *types.LevelMax:
		a := normalizeLevel(ll.A)
		b := normalizeLevel(ll.B)
		la, okA := a.(*types.LevelLit)
		lb, okB := b.(*types.LevelLit)
		if okA && okB {
			if la.N >= lb.N {
				return la
			}
			return lb
		}
		if types.LevelEqual(a, b) {
			return a
		}
		if a != ll.A || b != ll.B {
			return &types.LevelMax{A: a, B: b}
		}
		return l
	case *types.LevelSucc:
		e := normalizeLevel(ll.E)
		if le, ok := e.(*types.LevelLit); ok {
			return &types.LevelLit{N: le.N + 1}
		}
		if e != ll.E {
			return &types.LevelSucc{E: e}
		}
		return l
	default:
		return l
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
		// Normalize: max(n, m) → max(n, m) as concrete, max(l, l) → l.
		return normalizeLevel(&types.LevelMax{A: zA, B: zB})
	case *types.LevelSucc:
		zE := u.ZonkLevelDefault(ll.E)
		return normalizeLevel(&types.LevelSucc{E: zE})
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
