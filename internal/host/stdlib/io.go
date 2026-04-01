package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// capIO is the canonical capability key for the IO effect.
const capIO = "io"

// IO provides log/dbg capabilities using a CapEnv buffer.
// Output is accumulated in the capIO capability as []string.
var IO Pack = func(e Registrar) error {
	e.RegisterPrim("_ioLog", logImpl)
	e.RegisterPrim("_ioDbg", dbgImpl)
	return e.RegisterModule("Effect.IO", ioSource)
}

var ioSource = mustReadSource("io")

func logImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	newCe := appendIO(ce, s)
	return unitVal, newCe, nil
}

func dbgImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s := eval.PrettyValue(args[0])
	newCe := appendIO(ce, s)
	return unitVal, newCe, nil
}

// appendIO appends a message to the IO capability buffer.
// The three-index slice ensures append allocates a new backing array,
// preventing aliasing with the caller's original slice.
func appendIO(ce eval.CapEnv, msg string) eval.CapEnv {
	existing, _ := ce.Get(capIO)
	var buf []string
	if b, ok := existing.([]string); ok {
		buf = append(b[:len(b):len(b)], msg)
	} else {
		buf = []string{msg}
	}
	return ce.Set(capIO, buf)
}
