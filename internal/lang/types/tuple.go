package types

import "strconv"

// TupleLabel returns the canonical field label for a 1-based tuple position.
// Position 1 → "_1", position 2 → "_2", etc.
// This is the single authoritative encoding of tuple position labels,
// shared by syntax, IR, runtime, and host code.
func TupleLabel(pos int) string {
	return "_" + strconv.Itoa(pos)
}

// IsTupleLabel reports whether label is a canonical tuple position label ("_1", "_2", ...).
func IsTupleLabel(label string) bool {
	if len(label) < 2 || label[0] != '_' {
		return false
	}
	for i := 1; i < len(label); i++ {
		if label[i] < '0' || label[i] > '9' {
			return false
		}
	}
	return true
}
