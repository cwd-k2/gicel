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
	return eval.IntVal(n), ce, nil
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
		return nil, ce, errDivisionByZero
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
		return nil, ce, errModuloByZero
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

// floatToInt safely converts a float64 to int64, rejecting NaN, Inf, and
// values outside the representable int64 range.
func floatToInt(f float64, op string, ce eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	if math.IsNaN(f) {
		return nil, ce, fmt.Errorf("%s: NaN cannot be converted to Int", op)
	}
	if math.IsInf(f, 0) {
		return nil, ce, fmt.Errorf("%s: Inf cannot be converted to Int", op)
	}
	if f > float64(math.MaxInt64) || f < float64(math.MinInt64) {
		return nil, ce, fmt.Errorf("%s: value %g out of Int range", op, f)
	}
	return intResult(int64(f), ce)
}

func roundImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return floatToInt(math.RoundToEven(f), "round", ce)
}

func floorImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return floatToInt(math.Floor(f), "floor", ce)
}

func ceilingImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return floatToInt(math.Ceil(f), "ceiling", ce)
}

func truncateImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	f, err := asFloat64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	return floatToInt(math.Trunc(f), "truncate", ce)
}

func gcdImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64Num(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64Num(args[1])
	if err != nil {
		return nil, ce, err
	}
	// Absolute values
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return intResult(a, ce)
}
