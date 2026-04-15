package ir

import "github.com/cwd-k2/gicel/internal/lang/types"

// maxTraversalDepth is the structural depth limit for recursive Core IR
// traversals, sourced from the shared types.MaxTraversalDepth constant
// to ensure consistent limits across type and IR layers.
const maxTraversalDepth = types.MaxTraversalDepth
