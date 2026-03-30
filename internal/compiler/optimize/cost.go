package optimize

import "github.com/cwd-k2/gicel/internal/lang/ir"

// nodeSize counts the number of Core IR nodes in a tree.
// Stops early when the count exceeds limit (0 = no limit).
func nodeSize(c ir.Core, limit int) int {
	count := 0
	ir.Walk(c, func(_ ir.Core) bool {
		count++
		return limit == 0 || count <= limit
	})
	return count
}
