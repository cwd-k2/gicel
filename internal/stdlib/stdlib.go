// Package stdlib provides standard library packs for GICEL.
package stdlib

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/reg"
)

// Registrar is the registration interface that Engine implements.
type Registrar = reg.Registrar

// Pack configures a Registrar with a coherent set of types, primitives, and modules.
type Pack = reg.Pack

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
