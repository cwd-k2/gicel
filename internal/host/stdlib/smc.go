package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// MergeImpl implements the merge primitive for parallel composition.
// merge : Computation pre₁ post₁ a → Computation pre₂ post₂ b
//
//	→ Computation (Merge pre₁ pre₂) (Merge post₁ post₂) (a, b)
//
// Direct application (merge comp1 comp2) is intercepted at compile time
// as a special form and compiled to ir.Merge → OpMerge, which provides
// correct CapEnv isolation. This PrimImpl is the fallback for indirect
// usage (e.g. let f = merge; f comp1 comp2), where CBV evaluates the
// arguments before reaching merge. In this case, effects have already
// been executed sequentially and the values are used directly.
var MergeImpl eval.PrimImpl = func(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	val1, ce, err := forceOrUse(args[0], ce, apply)
	if err != nil {
		return nil, ce, err
	}
	val2, ce, err := forceOrUse(args[1], ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return eval.NewRecordFromMap(map[string]eval.Value{
		"_1": val1,
		"_2": val2,
	}), ce, nil
}

// forceOrUse forces a suspended computation by applying unit, or returns
// an already-evaluated value as-is. In CBV, Computation-typed arguments
// may arrive pre-evaluated when merge is used indirectly (not as special form).
func forceOrUse(v eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	switch v.(type) {
	case *eval.VMClosure, *eval.Closure, *eval.PrimVal, *eval.VMThunkVal, *eval.ThunkVal:
		return apply.Apply(v, unitVal, ce)
	default:
		return v, ce, nil
	}
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
