package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/eval"
)

// Num provides integer arithmetic: Num class, Eq/Ord Int instances, and operators.
var Num Pack = func(e Registrar) error {
	e.RegisterPrim("_addInt", addIntImpl)
	e.RegisterPrim("_subInt", subIntImpl)
	e.RegisterPrim("_mulInt", mulIntImpl)
	e.RegisterPrim("_divInt", divIntImpl)
	e.RegisterPrim("_modInt", modIntImpl)
	e.RegisterPrim("_negInt", negIntImpl)
	e.RegisterPrim("_eqInt", eqIntImpl)
	e.RegisterPrim("_cmpInt", cmpIntImpl)
	return e.RegisterModule("Std.Num", numSource)
}

var numSource = mustReadSource("num")

func asInt64Num(v eval.Value) (int64, error) { return asInt64(v, "num") }

func intResult(n int64, ce eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	return &eval.HostVal{Inner: n}, ce, nil
}

func addIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	return intResult(a+b, ce)
}

func subIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	return intResult(a-b, ce)
}

func mulIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	return intResult(a*b, ce)
}

func divIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	if b == 0 {
		return nil, ce, fmt.Errorf("division by zero")
	}
	return intResult(a/b, ce)
}

func modIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	if b == 0 {
		return nil, ce, fmt.Errorf("modulo by zero")
	}
	return intResult(a%b, ce)
}

func negIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return intResult(-a, ce)
}

func eqIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	if a == b {
		return &eval.ConVal{Con: "True"}, ce, nil
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func cmpIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	switch {
	case a < b:
		return &eval.ConVal{Con: "LT"}, ce, nil
	case a > b:
		return &eval.ConVal{Con: "GT"}, ce, nil
	default:
		return &eval.ConVal{Con: "EQ"}, ce, nil
	}
}
