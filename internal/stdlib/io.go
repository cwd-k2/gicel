package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/eval"
)

// IO provides print/debug capabilities using a CapEnv buffer.
// Output is accumulated in the "io" capability as []string.
var IO Pack = func(e Registrar) error {
	e.RegisterPrim("_ioPrint", printImpl)
	e.RegisterPrim("_ioDebug", debugImpl)
	return e.RegisterModule("Effect.IO", ioSource)
}

var ioSource = mustReadSource("io")

func printImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asString(args[0])
	if err != nil {
		return nil, ce, err
	}
	newCe := appendIO(ce, s)
	return unitVal, newCe, nil
}

func debugImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s := fmt.Sprintf("%v", args[0])
	newCe := appendIO(ce, s)
	return unitVal, newCe, nil
}

// appendIO appends a message to the "io" capability buffer.
// The three-index slice ensures append allocates a new backing array,
// preventing aliasing with the caller's original slice.
func appendIO(ce eval.CapEnv, msg string) eval.CapEnv {
	existing, _ := ce.Get("io")
	var buf []string
	if b, ok := existing.([]string); ok {
		buf = append(b[:len(b):len(b)], msg)
	} else {
		buf = []string{msg}
	}
	return ce.Set("io", buf)
}
