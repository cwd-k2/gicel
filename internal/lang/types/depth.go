package types

// maxTraversalDepth bounds recursive type-structure traversals to prevent
// Go stack overflow on pathologically deep or cyclic type trees.
const maxTraversalDepth = 512

// depthExceeded panics to signal that a type traversal has exceeded the
// maximum depth. Silent truncation would produce incorrect results
// (missed variables, partial substitution). Callers of public traversal
// functions (Subst, OccursIn, FreeVars, MapType, etc.) may recover this
// panic at appropriate boundaries.
func depthExceeded() {
	panic("types: traversal depth exceeded (possible cyclic or pathologically deep type)")
}
