package types

import "fmt"

// isTupleLabels reports whether labels form a valid tuple encoding:
// ["_1", "_2", ..., "_n"] in order, with no gaps.
func isTupleLabels(labels []string) bool {
	for i, l := range labels {
		if l != fmt.Sprintf("_%d", i+1) {
			return false
		}
	}
	return true
}
