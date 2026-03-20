package stdlib

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

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
	return boolVal(a == b), ce, nil
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
		return ordVal(-1), ce, nil
	case a > b:
		return ordVal(1), ce, nil
	default:
		return ordVal(0), ce, nil
	}
}

// --- Double primitives ---

func asFloat64Num(v eval.Value) (float64, error) { return asFloat64(v, "num") }

func floatResult(f float64, ce eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	return &eval.HostVal{Inner: f}, ce, nil
}

func addDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	return floatResult(a+b, ce)
}

func subDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	return floatResult(a-b, ce)
}

func mulDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	return floatResult(a*b, ce)
}

func divDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	return floatResult(a/b, ce) // IEEE 754: division by zero yields ±Inf
}

func negDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return floatResult(-a, ce)
}

func eqDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	return boolVal(a == b), ce, nil
}

func cmpDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asFloat64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	switch {
	case a < b:
		return ordVal(-1), ce, nil
	case a > b:
		return ordVal(1), ce, nil
	default:
		return ordVal(0), ce, nil
	}
}

func showDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64Num(args[0])
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
	f, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return intResult(int64(math.RoundToEven(f)), ce)
}

func floorImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return intResult(int64(math.Floor(f)), ce)
}

func ceilingImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return intResult(int64(math.Ceil(f)), ce)
}

func truncateImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return intResult(int64(math.Trunc(f)), ce)
}
