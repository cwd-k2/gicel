package stdlib

import (
	"context"
	"math"
	"math/bits"

	"github.com/cwd-k2/gicel/internal/lang/types"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Math provides mathematical functions: integer power, bitwise ops,
// and transcendental functions on Double.
var Math Pack = func(e Registrar) error {
	// Int
	e.RegisterPrim("_powInt", powIntImpl)
	e.RegisterPrim("_clampInt", clampIntImpl)
	e.RegisterPrim("_divMod", divModImpl)

	// Bitwise
	e.RegisterPrim("_bitAnd", bitAndImpl)
	e.RegisterPrim("_bitOr", bitOrImpl)
	e.RegisterPrim("_bitXor", bitXorImpl)
	e.RegisterPrim("_bitNot", bitNotImpl)
	e.RegisterPrim("_shiftL", shiftLImpl)
	e.RegisterPrim("_shiftR", shiftRImpl)
	e.RegisterPrim("_popCount", popCountImpl)

	// Double
	e.RegisterPrim("_sqrt", unaryFloatImpl(math.Sqrt))
	e.RegisterPrim("_cbrt", unaryFloatImpl(math.Cbrt))
	e.RegisterPrim("_sin", unaryFloatImpl(math.Sin))
	e.RegisterPrim("_cos", unaryFloatImpl(math.Cos))
	e.RegisterPrim("_tan", unaryFloatImpl(math.Tan))
	e.RegisterPrim("_asin", unaryFloatImpl(math.Asin))
	e.RegisterPrim("_acos", unaryFloatImpl(math.Acos))
	e.RegisterPrim("_atan", unaryFloatImpl(math.Atan))
	e.RegisterPrim("_atan2", binaryFloatImpl(math.Atan2))
	e.RegisterPrim("_exp", unaryFloatImpl(math.Exp))
	e.RegisterPrim("_log", unaryFloatImpl(math.Log))
	e.RegisterPrim("_log2", unaryFloatImpl(math.Log2))
	e.RegisterPrim("_log10", unaryFloatImpl(math.Log10))
	e.RegisterPrim("_powDouble", binaryFloatImpl(math.Pow))
	e.RegisterPrim("_floor", unaryFloatImpl(math.Floor))
	e.RegisterPrim("_ceil", unaryFloatImpl(math.Ceil))
	e.RegisterPrim("_round", unaryFloatImpl(math.Round))
	e.RegisterPrim("_isNaN", isNaNImpl)
	e.RegisterPrim("_isInfinite", isInfiniteImpl)
	e.RegisterPrim("_clampDouble", clampDoubleImpl)

	return e.RegisterModule("Data.Math", mathSource)
}

var mathSource = mustReadSource("math")

// --- Int operations ---

func powIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	base, err := asInt64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	exp, err := asInt64(args[1], "math")
	if err != nil {
		return nil, ce, err
	}
	if exp < 0 {
		return eval.IntVal(0), ce, nil
	}
	result := int64(1)
	b := base
	for e := exp; e > 0; e >>= 1 {
		if e&1 == 1 {
			result *= b
		}
		b *= b
	}
	return eval.IntVal(result), ce, nil
}

func clampIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	lo, err := asInt64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	hi, err := asInt64(args[1], "math")
	if err != nil {
		return nil, ce, err
	}
	x, err := asInt64(args[2], "math")
	if err != nil {
		return nil, ce, err
	}
	if x < lo {
		x = lo
	} else if x > hi {
		x = hi
	}
	return eval.IntVal(x), ce, nil
}

func divModImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64(args[1], "math")
	if err != nil {
		return nil, ce, err
	}
	if b == 0 {
		return nil, ce, &eval.RuntimeError{Message: "divMod: division by zero"}
	}
	d := a / b
	m := a % b
	// Euclidean: ensure non-negative remainder
	if m < 0 {
		if b > 0 {
			d--
			m += b
		} else {
			d++
			m -= b
		}
	}
	tuple := eval.NewRecordFromMap(map[string]eval.Value{
		types.TupleLabel(1): eval.IntVal(d),
		types.TupleLabel(2): eval.IntVal(m),
	})
	return tuple, ce, nil
}

// --- Bitwise operations ---

func bitAndImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64(args[1], "math")
	if err != nil {
		return nil, ce, err
	}
	return eval.IntVal(a & b), ce, nil
}

func bitOrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64(args[1], "math")
	if err != nil {
		return nil, ce, err
	}
	return eval.IntVal(a | b), ce, nil
}

func bitXorImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	b, err := asInt64(args[1], "math")
	if err != nil {
		return nil, ce, err
	}
	return eval.IntVal(a ^ b), ce, nil
}

func bitNotImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	return eval.IntVal(^a), ce, nil
}

func shiftLImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	n, err := asInt64(args[1], "math")
	if err != nil {
		return nil, ce, err
	}
	if n < 0 || n >= 64 {
		return eval.IntVal(0), ce, nil
	}
	return eval.IntVal(a << uint(n)), ce, nil
}

func shiftRImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	n, err := asInt64(args[1], "math")
	if err != nil {
		return nil, ce, err
	}
	if n < 0 || n >= 64 {
		if a < 0 {
			return eval.IntVal(-1), ce, nil
		}
		return eval.IntVal(0), ce, nil
	}
	return eval.IntVal(a >> uint(n)), ce, nil
}

func popCountImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asInt64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	return eval.IntVal(int64(bits.OnesCount64(uint64(a)))), ce, nil
}

// --- Double operations ---

func unaryFloatImpl(fn func(float64) float64) eval.PrimImpl {
	return func(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		a, err := asFloat64(args[0], "math")
		if err != nil {
			return nil, ce, err
		}
		return floatResult(fn(a), ce)
	}
}

func binaryFloatImpl(fn func(float64, float64) float64) eval.PrimImpl {
	return func(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		a, err := asFloat64(args[0], "math")
		if err != nil {
			return nil, ce, err
		}
		b, err := asFloat64(args[1], "math")
		if err != nil {
			return nil, ce, err
		}
		return floatResult(fn(a, b), ce)
	}
}

func isNaNImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	if math.IsNaN(a) {
		return &eval.ConVal{Con: "True"}, ce, nil
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func isInfiniteImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asFloat64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	if math.IsInf(a, 0) {
		return &eval.ConVal{Con: "True"}, ce, nil
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func clampDoubleImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	lo, err := asFloat64(args[0], "math")
	if err != nil {
		return nil, ce, err
	}
	hi, err := asFloat64(args[1], "math")
	if err != nil {
		return nil, ce, err
	}
	x, err := asFloat64(args[2], "math")
	if err != nil {
		return nil, ce, err
	}
	if x < lo {
		x = lo
	} else if x > hi {
		x = hi
	}
	return floatResult(x, ce)
}
