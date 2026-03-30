package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// MergeImpl implements the merge primitive for parallel composition.
// merge : Computation pre₁ post₁ a → Computation pre₂ post₂ b
//       → Computation (Merge pre₁ pre₂) (Merge post₁ post₂) (a, b)
//
// At runtime, both computations are executed sequentially with the same
// capability environment. Disjointness of pre₁ and pre₂ is guaranteed
// by the type checker's Merge row family. The results are paired.
var MergeImpl eval.PrimImpl = func(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	// args[0] = first effectful computation (PrimVal or closure)
	// args[1] = second effectful computation (PrimVal or closure)
	// Both are suspended computations — force by applying to Unit.
	val1, ce, err := apply(args[0], unitVal, ce)
	if err != nil {
		return nil, ce, err
	}
	val2, ce, err := apply(args[1], unitVal, ce)
	if err != nil {
		return nil, ce, err
	}
	// Return (a, b) as Record { _1: a, _2: b }.
	return eval.NewRecordFromMap(map[string]eval.Value{
		"_1": val1,
		"_2": val2,
	}), ce, nil
}

// DagImpl implements the dag primitive for capability row inversion.
// dag : Gate pre post → Gate post pre
//
// At runtime, dag is identity — pre/post swap is a type-level operation.
// The capability environment interpretation is determined by the caller's
// Bind chain, not by the Gate itself.
var DagImpl eval.PrimImpl = func(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return args[0], ce, nil
}
