package optimize

import "github.com/cwd-k2/gicel/internal/lang/ir"

// nodeSize counts the number of Core IR nodes in a tree.
// Stops early when the count exceeds limit (0 = no limit).
// If the traversal is truncated by the depth limit, returns limit+1
// so that size-guarded optimizations are conservatively skipped.
func nodeSize(c ir.Core, limit int) int {
	count := 0
	complete := ir.Walk(c, func(_ ir.Core) bool {
		count++
		return limit == 0 || count <= limit
	})
	if !complete && limit > 0 && count <= limit {
		return limit + 1
	}
	return count
}
