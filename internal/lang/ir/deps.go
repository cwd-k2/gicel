package ir

// SortBindings reorders bindings so that each binding appears after
// the bindings it depends on, where possible. Dependencies are
// determined by free-variable analysis: if binding A's expression
// contains a free reference to binding B (both in the same group),
// then A depends on B.
//
// Cycles (mutual recursion) are preserved — the relative order of
// bindings within a cycle is left unchanged from the input.
//
// This is a Kahn's algorithm topological sort with stable fallback
// for cycle members.
func SortBindings(bs []Binding) []Binding {
	if len(bs) <= 1 {
		return bs
	}

	// Build index of names in this binding group.
	nameIdx := make(map[string]int, len(bs))
	for i, b := range bs {
		nameIdx[b.Name] = i
	}

	// Compute dependency edges: deps[i] = set of indices that binding i depends on.
	deps := make([]map[int]bool, len(bs))
	rdeps := make([][]int, len(bs)) // reverse edges for Kahn's
	inDeg := make([]int, len(bs))
	for i, b := range bs {
		fv := FreeVars(b.Expr)
		deps[i] = make(map[int]bool)
		for name := range fv {
			// Extract the unqualified component for dependency resolution.
			_, target := SplitQualifiedKey(VarKey(name))
			if j, ok := nameIdx[target]; ok && j != i {
				deps[i][j] = true
			}
		}
		inDeg[i] = len(deps[i])
		for j := range deps[i] {
			rdeps[j] = append(rdeps[j], i)
		}
	}

	// Kahn's algorithm: process nodes with in-degree 0.
	// Use a queue seeded in original order for stability.
	queue := make([]int, 0, len(bs))
	for i := range bs {
		if inDeg[i] == 0 {
			queue = append(queue, i)
		}
	}

	result := make([]Binding, 0, len(bs))
	emitted := make([]bool, len(bs))
	for len(queue) > 0 {
		idx := queue[0]
		queue = queue[1:]
		result = append(result, bs[idx])
		emitted[idx] = true
		for _, j := range rdeps[idx] {
			inDeg[j]--
			if inDeg[j] == 0 {
				queue = append(queue, j)
			}
		}
	}

	// Remaining nodes are in cycles — append in original order.
	for i, b := range bs {
		if !emitted[i] {
			result = append(result, b)
		}
	}

	return result
}
