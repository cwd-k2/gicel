// Package stdlib provides standard library packs for GICEL.
package stdlib

import (
	"context"
	"math"

	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Registrar is the registration interface that Engine implements.
type Registrar = registry.Registrar

// Pack configures a Registrar with a coherent set of types, primitives, and modules.
type Pack = registry.Pack

// Allocation cost estimates for ChargeAlloc in stdlib primitives.
// These parallel the evaluator's cost model (eval.go costClosure, costConBase, etc.)
// and target the same order-of-magnitude accuracy.
const (
	costSlotSize  = 16 // one pointer in []Value / []any / []string
	costConsNode  = 64 // ConVal{"Cons", [elem, tail]}: 32 base + 2×16 args
	costTupleNode = 80 // RecordVal{_1, _2}: 32 base + 2×24 fields (sorted slice)
	costAVLNode   = 64 // avlNode struct (key, value, left, right, height)
	costPerByte   = 1  // string/[]rune allocation per byte
)

// checkedMulCost multiplies n by costPerUnit, returning the result.
// If the multiplication would overflow int64, returns math.MaxInt64
// so that subsequent budget.ChargeAlloc will trigger the allocation
// limit rather than wrapping to a negative value.
func checkedMulCost(n int64, costPerUnit int64) int64 {
	if costPerUnit > 0 && n > math.MaxInt64/costPerUnit {
		return math.MaxInt64
	}
	return n * costPerUnit
}

// freeIn checks if name appears free in a Core expression.
// Used by fusion rules to guard against variable capture.
func freeIn(name string, c ir.Core) bool {
	_, ok := ir.FreeVars(c)[name]
	return ok
}

// asInt64 extracts an int64 from a HostVal. Shared by Num and Str packs.
func asInt64(v eval.Value, pack string) (int64, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return 0, errExpected("stdlib/"+pack, "HostVal", v)
	}
	n, ok := hv.Inner.(int64)
	if !ok {
		return 0, errExpected("stdlib/"+pack, "int64", hv.Inner)
	}
	return n, nil
}

// asFloat64 extracts a float64 from a HostVal.
func asFloat64(v eval.Value, pack string) (float64, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return 0, errExpected("stdlib/"+pack, "HostVal", v)
	}
	f, ok := hv.Inner.(float64)
	if !ok {
		return 0, errExpected("stdlib/"+pack, "float64", hv.Inner)
	}
	return f, nil
}

// boolVal returns the interned Bool value for b.
func boolVal(b bool) eval.Value {
	return eval.BoolVal(b)
}

// ordVal returns the interned Ordering value for a comparison result.
func ordVal(cmp int) eval.Value {
	switch {
	case cmp < 0:
		return eval.LTVal
	case cmp > 0:
		return eval.GTVal
	default:
		return eval.EQVal
	}
}

// unitVal is the shared unit value (empty record).
var unitVal = eval.UnitVal

// driveEffectful forces a suspended computation (thunk) and then drives
// any resulting deferred effectful value to completion. This is the
// Go-side equivalent of GICEL's `bind comp pure` wrapping pattern.
// Used by handler primitives (tryAt, runStateAt, etc.) that receive a
// raw Suspended argument without the GICEL wrapper's bind+pure layer.
//
// The thunk body may contain OpBind chains that the VM's execute loop
// drives internally; only the final expression returns as a deferred
// effectful PrimVal. ForceEffectful executes that final step without
// adding spurious arguments.
func driveEffectful(thunk eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	val, newCe, err := apply.Apply(thunk, unitVal, ce)
	if err != nil {
		return nil, ce, err
	}
	if apply.ForceEffectful != nil {
		return apply.ForceEffectful(val, newCe)
	}
	return val, newCe, nil
}

// withLabel wraps a PrimImpl to skip the first argument (label literal from
// label erasure). Used to create named capability variants: the label is a
// type-level parameter that flows through as a runtime string argument via
// the label erasure pass. The named prim ignores it and delegates to the
// original implementation with the remaining arguments.
func withLabel(fn eval.PrimImpl) eval.PrimImpl {
	return func(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
		if err := validateLabelArg(args); err != nil {
			return nil, ce, err
		}
		return fn(ctx, ce, args[1:], apply)
	}
}

// validateLabelArg checks that args[0] is a label string (HostVal wrapping string).
// Named capability operations require an explicit @#label argument; without it,
// the label erasure pass doesn't inject the string, and args[0] is a misaligned
// value argument. This guard converts the resulting crash into a clear error.
func validateLabelArg(args []eval.Value) error {
	if len(args) == 0 {
		return &eval.RuntimeError{Message: "named capability operation requires @#label argument"}
	}
	if hv, ok := args[0].(*eval.HostVal); ok {
		if _, ok := hv.Inner.(string); ok {
			return nil
		}
	}
	return &eval.RuntimeError{Message: "named capability operation requires @#label argument; got a non-label value (missing @#label?)"}
}

// withLabelNoCompare wraps a PrimImpl that normally takes [compare, arg1, arg2, ...]
// to accept [label, arg1, arg2, ...] instead. The label is dropped, and a nil
// placeholder is inserted at position 0 so the original impl's arg indices work.
// Used for named capability variants where the compare function is stored in the
// data structure handle and args[0] (compare) is never accessed at runtime.
//
// INVARIANT: the wrapped fn must not read args[0]. All current consumers
// (mmapInsertImpl, mmapLookupImpl, etc.) use m.cmp from the handle instead.
func withLabelNoCompare(fn eval.PrimImpl) eval.PrimImpl {
	return func(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
		if err := validateLabelArg(args); err != nil {
			return nil, ce, err
		}
		padded := make([]eval.Value, len(args))
		padded[0] = nil // dummy compare placeholder (never accessed — see INVARIANT)
		copy(padded[1:], args[1:])
		return fn(ctx, ce, padded, apply)
	}
}
