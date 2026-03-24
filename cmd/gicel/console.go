package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// consoleSource is the GICEL module source for the Console pack.
// The console capability is a unit marker; actual IO is performed
// by the host primitives registered below.
const consoleSource = `import Prelude

_consolePutLine :: String -> Computation { console: () | r } { console: () | r } ()
_consolePutLine := assumption

_consoleGetLine :: Computation { console: () | r } { console: () | r } String
_consoleGetLine := assumption

putLine :: String -> Computation { console: () | r } { console: () | r } ()
putLine := _consolePutLine

getLine :: Computation { console: () | r } { console: () | r } String
getLine := _consoleGetLine
`

// consolePack is a CLI-only pack that provides real stdio operations.
// Unlike Effect.IO (which buffers output in the capability environment),
// Console reads from stdin and writes to stdout directly.
//
// Security: this pack is registered only by the CLI binary, never by
// RunSandbox or the Go embedding API. Untrusted code cannot access
// real stdio unless the host explicitly provides it.
var consolePack registry.Pack = func(e registry.Registrar) error {
	e.RegisterPrim("_consolePutLine", putLineImpl)
	e.RegisterPrim("_consoleGetLine", getLineImpl)
	return e.RegisterModule("Console", consoleSource)
}

// stdinScanner is shared across getLine calls within a single CLI run.
var stdinScanner = bufio.NewScanner(os.Stdin)

func putLineImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	hv, ok := args[0].(*eval.HostVal)
	if !ok {
		return nil, ce, fmt.Errorf("putLine: expected string argument, got %T", args[0])
	}
	s, ok := hv.Inner.(string)
	if !ok {
		return nil, ce, fmt.Errorf("putLine: expected string, got %T", hv.Inner)
	}
	fmt.Println(s)
	return &eval.RecordVal{Fields: map[string]eval.Value{}}, ce, nil
}

func getLineImpl(_ context.Context, ce eval.CapEnv, _ []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	if stdinScanner.Scan() {
		return &eval.HostVal{Inner: stdinScanner.Text()}, ce, nil
	}
	if err := stdinScanner.Err(); err != nil {
		return nil, ce, fmt.Errorf("getLine: %w", err)
	}
	return &eval.HostVal{Inner: ""}, ce, nil
}
