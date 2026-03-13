package stdlib

import (
	"context"

	"github.com/cwd-k2/gomputation/internal/eval"
)

// Fail provides the fail effect capability.
var Fail Pack = func(e Registrar) error {
	e.RegisterPrim("failWith", failImpl)
	return e.RegisterModule("Std.Fail", failSource)
}

const failSource = `
import Prelude

failWith :: forall e r a. e -> Computation { fail : e | r } { fail : e | r } a
failWith := assumption

fail :: forall r a. Computation { fail : Unit | r } { fail : Unit | r } a
fail := failWith Unit

fromMaybe :: forall a r. Maybe a -> Computation { fail : Unit | r } { fail : Unit | r } a
fromMaybe := \m -> case m {
  Nothing -> fail;
  Just x  -> pure x
}

fromResult :: forall e a r. Result e a -> Computation { fail : e | r } { fail : e | r } a
fromResult := \r -> case r {
  Err e -> failWith e;
  Ok x  -> pure x
}
`

func failImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return nil, ce, &eval.RuntimeError{Message: "fail"}
}
