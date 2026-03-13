package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gomputation/internal/eval"
)

// IO provides print/debug capabilities using a CapEnv buffer.
// Output is accumulated in the "io" capability as []string.
var IO Pack = func(e Registrar) error {
	e.RegisterPrim("_ioPrint", printImpl)
	e.RegisterPrim("_ioDebug", debugImpl)
	return e.RegisterModule("Std.IO", ioSource)
}

const ioSource = `
import Prelude

_ioPrint :: String -> Computation { io : () | r } { io : () | r } ()
_ioPrint := assumption

_ioDebug :: forall a. a -> Computation { io : () | r } { io : () | r } ()
_ioDebug := assumption

print :: String -> Computation { io : () | r } { io : () | r } ()
print := _ioPrint

debug :: forall a. a -> Computation { io : () | r } { io : () | r } ()
debug := _ioDebug
`

func printImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	hv, ok := args[0].(*eval.HostVal)
	if !ok {
		return nil, ce, fmt.Errorf("print: expected String, got %T", args[0])
	}
	s, ok := hv.Inner.(string)
	if !ok {
		return nil, ce, fmt.Errorf("print: expected string, got %T", hv.Inner)
	}
	newCe := appendIO(ce, s)
	return &eval.RecordVal{Fields: map[string]eval.Value{}}, newCe, nil
}

func debugImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s := fmt.Sprintf("%v", args[0])
	newCe := appendIO(ce, s)
	return &eval.RecordVal{Fields: map[string]eval.Value{}}, newCe, nil
}

// appendIO appends a message to the "io" capability buffer.
func appendIO(ce eval.CapEnv, msg string) eval.CapEnv {
	existing, _ := ce.Get("io")
	var buf []string
	if b, ok := existing.([]string); ok {
		buf = b
	}
	buf = append(buf, msg)
	return ce.Set("io", buf)
}
