package types

import "github.com/cwd-k2/gicel/internal/lang/syntax"

// TupleLabel delegates to syntax.TupleLabel, the single authoritative
// encoding of tuple position labels. Re-exported here for convenience
// since most callers already import types.
func TupleLabel(pos int) string {
	return syntax.TupleLabel(pos)
}

// isTupleLabels reports whether labels form a valid tuple encoding:
// ["_1", "_2", ..., "_n"] in order, with no gaps.
func isTupleLabels(labels []string) bool {
	for i, l := range labels {
		if l != TupleLabel(i+1) {
			return false
		}
	}
	return true
}
