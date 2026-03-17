package stdlib

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/cwd-k2/gicel/internal/eval"
)

// Num provides arithmetic: Num class with Int and Double instances, and operators.
var Num Pack = func(e Registrar) error {
	// Int primitives
	e.RegisterPrim("_addInt", addIntImpl)
	e.RegisterPrim("_subInt", subIntImpl)
	e.RegisterPrim("_mulInt", mulIntImpl)
	e.RegisterPrim("_divInt", divIntImpl)
	e.RegisterPrim("_modInt", modIntImpl)
	e.RegisterPrim("_negInt", negIntImpl)
	e.RegisterPrim("_eqInt", eqIntImpl)
	e.RegisterPrim("_cmpInt", cmpIntImpl)
	e.RegisterPrim("_showInt", numShowIntImpl)
	// Double primitives
	e.RegisterPrim("_addDouble", addDoubleImpl)
	e.RegisterPrim("_subDouble", subDoubleImpl)
	e.RegisterPrim("_mulDouble", mulDoubleImpl)
	e.RegisterPrim("_negDouble", negDoubleImpl)
	e.RegisterPrim("_eqDouble", eqDoubleImpl)
	e.RegisterPrim("_cmpDouble", cmpDoubleImpl)
	e.RegisterPrim("_showDouble", showDoubleImpl)
	e.RegisterPrim("_divDouble", divDoubleImpl)
	// Conversion primitives
	e.RegisterPrim("_toDouble", toDoubleImpl)
	e.RegisterPrim("_round", roundImpl)
	e.RegisterPrim("_floor", floorImpl)
	e.RegisterPrim("_ceiling", ceilingImpl)
	e.RegisterPrim("_truncate", truncateImpl)
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

func numShowIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: strconv.FormatInt(n, 10)}, ce, nil
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

// --- Double primitives ---

func asFloat64(v eval.Value) (float64, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return 0, fmt.Errorf("stdlib/num: expected HostVal, got %T", v)
	}
	f, ok := hv.Inner.(float64)
	if !ok {
		return 0, fmt.Errorf("stdlib/num: expected float64, got %T", hv.Inner)
	}
	return f, nil
}

func floatResult(f float64, ce eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	return &eval.HostVal{Inner: f}, ce, nil
}

func addDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64(args[1])
	if err != nil {
		return nil, ce, err
	}
	return floatResult(a+b, ce)
}

func subDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64(args[1])
	if err != nil {
		return nil, ce, err
	}
	return floatResult(a-b, ce)
}

func mulDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64(args[1])
	if err != nil {
		return nil, ce, err
	}
	return floatResult(a*b, ce)
}

func divDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64(args[1])
	if err != nil {
		return nil, ce, err
	}
	return floatResult(a/b, ce) // IEEE 754: division by zero yields ±Inf
}

func negDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	return floatResult(-a, ce)
}

func eqDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64(args[1])
	if err != nil {
		return nil, ce, err
	}
	if a == b {
		return &eval.ConVal{Con: "True"}, ce, nil
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func cmpDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64(args[1])
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

func showDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: strconv.FormatFloat(f, 'g', -1, 64)}, ce, nil
}

// --- Conversion primitives ---

func toDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return floatResult(float64(n), ce)
}

func roundImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	return intResult(int64(math.RoundToEven(f)), ce)
}

func floorImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	return intResult(int64(math.Floor(f)), ce)
}

func ceilingImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	return intResult(int64(math.Ceil(f)), ce)
}

func truncateImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64(args[0])
	if err != nil {
		return nil, ce, err
	}
	return intResult(int64(math.Trunc(f)), ce)
}
