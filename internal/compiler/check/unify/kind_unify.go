package unify

import "github.com/cwd-k2/gicel/internal/lang/types"

// isGroundKind returns true if k is a concrete kind that inhabits Sort₀.
// These are TyCon at level 1: Type, Row, Constraint, and promoted data kinds.
func isGroundKind(k types.Type) bool {
	if tc, ok := k.(*types.TyCon); ok {
		return types.IsKindLevel(tc.Level)
	}
	return false
}

// isSortLevel returns true if k is a TyCon at the given universe level.
func isSortLevel(k types.Type, level int) bool {
	if tc, ok := k.(*types.TyCon); ok {
		if lit, ok := tc.Level.(*types.LevelLit); ok {
			return lit.N == level
		}
	}
	return false
}
