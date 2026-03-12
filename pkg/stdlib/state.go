package stdlib

import (
	"context"

	gmp "github.com/cwd-k2/gomputation"
	"github.com/cwd-k2/gomputation/internal/eval"
)

// State provides get/put state capabilities.
var State Pack = func(e *gmp.Engine) error {
	e.RegisterPrim("get", getImpl)
	e.RegisterPrim("put", putImpl)
	return e.RegisterModule("Std.State", stateSource)
}

const stateSource = `
import Prelude

get :: forall s r. Computation { state : s | r } { state : s | r } s
get := assumption

put :: forall s r. s -> Computation { state : s | r } { state : s | r } Unit
put := assumption

modify :: forall s r. (s -> s) -> Computation { state : s | r } { state : s | r } Unit
modify := \f -> do { s <- get; put (f s) }
`

func getImpl(_ context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
	v, ok := ce.Get("state")
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "get: no state capability"}
	}
	return v.(gmp.Value), ce, nil
}

func putImpl(_ context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
	newCe := ce.Set("state", args[0])
	return &gmp.ConVal{Con: "Unit"}, newCe, nil
}
