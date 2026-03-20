package types

// maxTraversalDepth bounds recursive type-structure traversals to prevent
// Go stack overflow on pathologically deep or cyclic type trees.
const maxTraversalDepth = 512
