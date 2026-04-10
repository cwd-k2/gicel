package ir

import "github.com/cwd-k2/gicel/internal/lang/types"

// TupleLabel delegates to types.TupleLabel, which is the canonical
// encoding of tuple position labels. Retained here for backward
// compatibility with existing callers in host/stdlib and runtime/eval.
func TupleLabel(pos int) string {
	return types.TupleLabel(pos)
}
