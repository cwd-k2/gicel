package types

// maxTraversalDepth bounds recursive type-structure traversals to prevent
// Go stack overflow on pathologically deep or cyclic type trees.
const maxTraversalDepth = 512

// DepthExceededError is the typed panic raised when a type traversal exceeds
// maxTraversalDepth. Callers that want to recover from this specific
// condition (e.g. family/reduce.safeSubstMany) MUST type-assert on
// `*DepthExceededError` rather than catching arbitrary panics, so that real
// programming bugs (nil dereferences, index out of range) continue to
// propagate as crashes instead of being misclassified as "depth exceeded".
type DepthExceededError struct{}

func (*DepthExceededError) Error() string {
	return "types: traversal depth exceeded (possible cyclic or pathologically deep type)"
}

// depthExceeded panics with a *DepthExceededError. Silent truncation would
// produce incorrect results (missed variables, partial substitution), so
// callers of public traversal functions (Subst, OccursIn, FreeVars, MapType,
// etc.) must either tolerate the panic or recover it at an explicit
// boundary by type-asserting on *DepthExceededError.
func depthExceeded() {
	panic(&DepthExceededError{})
}
