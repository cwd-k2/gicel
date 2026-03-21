package syntax

import "strconv"

// TupleLabel returns the canonical field label for a 1-based tuple position.
// Position 1 → "_1", position 2 → "_2", etc.
// This is the single authoritative encoding of tuple position labels,
// used by the parser, type checker, evaluator, and pretty-printers.
func TupleLabel(pos int) string {
	return "_" + strconv.Itoa(pos)
}
