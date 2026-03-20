package ir

// maxTraversalDepth is the structural depth limit for recursive Core IR
// traversals. Exceeding this limit causes the traversal to bail out
// gracefully, preventing Go stack overflow on pathologically deep trees.
const maxTraversalDepth = 512
