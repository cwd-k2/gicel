// Package stdlib provides standard library packs for GICEL.
package stdlib

import (
	"fmt"

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
		return 0, fmt.Errorf("stdlib/%s: expected HostVal, got %T", pack, v)
	}
	n, ok := hv.Inner.(int64)
	if !ok {
		return 0, fmt.Errorf("stdlib/%s: expected int64, got %T", pack, hv.Inner)
	}
	return n, nil
}

// asFloat64 extracts a float64 from a HostVal.
func asFloat64(v eval.Value, pack string) (float64, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return 0, fmt.Errorf("stdlib/%s: expected HostVal, got %T", pack, v)
	}
	f, ok := hv.Inner.(float64)
	if !ok {
		return 0, fmt.Errorf("stdlib/%s: expected float64, got %T", pack, hv.Inner)
	}
	return f, nil
}

// boolVal constructs a Bool ConVal from a Go bool.
func boolVal(b bool) *eval.ConVal {
	if b {
		return &eval.ConVal{Con: "True"}
	}
	return &eval.ConVal{Con: "False"}
}

// ordVal constructs an Ordering ConVal from a comparison result (-1, 0, 1).
func ordVal(cmp int) *eval.ConVal {
	switch {
	case cmp < 0:
		return &eval.ConVal{Con: "LT"}
	case cmp > 0:
		return &eval.ConVal{Con: "GT"}
	default:
		return &eval.ConVal{Con: "EQ"}
	}
}

// unitVal is the shared unit value (empty record).
var unitVal = eval.UnitVal
