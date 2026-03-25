package ir

import "strconv"

// TupleLabel returns the canonical field label for a tuple position (0-based).
func TupleLabel(pos int) string {
	return "_" + strconv.Itoa(pos)
}
