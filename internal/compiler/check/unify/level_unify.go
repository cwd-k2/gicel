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
			TypeOps: u.TypeOps,
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

// normalizeLevel simplifies level expressions using the free semilattice laws:
//
//	max(LevelLit(a), LevelLit(b)) → LevelLit(max(a, b))   concrete reduction
//	max(l, 0) → l                                          absorption (identity)
//	max(l, l) → l                                          idempotence
//	max(l2, l1) → max(l1, l2) when l1 < l2                 canonical ordering (commutativity)
//	succ(LevelLit(n)) → LevelLit(n+1)                      concrete reduction
func normalizeLevel(l types.LevelExpr) types.LevelExpr {
	switch ll := l.(type) {
	case *types.LevelMax:
		a := normalizeLevel(ll.A)
		b := normalizeLevel(ll.B)
		la, okA := a.(*types.LevelLit)
		lb, okB := b.(*types.LevelLit)
		// Both concrete: take max.
		if okA && okB {
			if la.N >= lb.N {
				return la
			}
			return lb
		}
		// Absorption: max(l, 0) = l (identity element of the semilattice).
		if okA && la.N == 0 {
			return b
		}
		if okB && lb.N == 0 {
			return a
		}
		// Idempotence: max(l, l) = l.
		if types.LevelEqual(a, b) {
			return a
		}
		// Canonical ordering: sort operands so max(l2, l1) and max(l1, l2)
		// normalize to the same form.
		if levelCompare(a, b) > 0 {
			a, b = b, a
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

// levelCompare defines a total order on LevelExpr for canonical normalization.
// Returns negative if a < b, zero if equal, positive if a > b.
// Ordering: LevelLit < LevelVar < LevelMeta < LevelSucc < LevelMax.
// Within the same constructor: compare by N, Name, ID, or recursively.
func levelCompare(a, b types.LevelExpr) int {
	ta, tb := levelTag(a), levelTag(b)
	if ta != tb {
		return ta - tb
	}
	switch aa := a.(type) {
	case *types.LevelLit:
		return aa.N - b.(*types.LevelLit).N
	case *types.LevelVar:
		bb := b.(*types.LevelVar)
		if aa.Name < bb.Name {
			return -1
		}
		if aa.Name > bb.Name {
			return 1
		}
		return 0
	case *types.LevelMeta:
		return aa.ID - b.(*types.LevelMeta).ID
	case *types.LevelSucc:
		return levelCompare(aa.E, b.(*types.LevelSucc).E)
	case *types.LevelMax:
		bb := b.(*types.LevelMax)
		if c := levelCompare(aa.A, bb.A); c != 0 {
			return c
		}
		return levelCompare(aa.B, bb.B)
	default:
		return 0
	}
}

// levelTag returns a numeric tag for ordering LevelExpr constructors.
func levelTag(l types.LevelExpr) int {
	switch l.(type) {
	case *types.LevelLit:
		return 0
	case *types.LevelVar:
		return 1
	case *types.LevelMeta:
		return 2
	case *types.LevelSucc:
		return 3
	case *types.LevelMax:
		return 4
	default:
		return 5
	}
}
