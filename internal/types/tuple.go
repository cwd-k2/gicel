package types

import "strconv"

// TupleLabel returns the canonical field label for a 1-based tuple position.
// Position 1 → "_1", position 2 → "_2", etc.
func TupleLabel(pos int) string {
	return "_" + strconv.Itoa(pos)
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
