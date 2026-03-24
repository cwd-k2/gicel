package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// consoleSource is the GICEL module source for the Console pack.
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
// In buffer mode (--json), output is captured in the capability environment
// instead of written to stdout.
var consolePack registry.Pack = func(e registry.Registrar) error {
	e.RegisterPrim("_consolePutLine", putLineImpl)
	e.RegisterPrim("_consoleGetLine", getLineImpl)
	return e.RegisterModule("Console", consoleSource)
}

// consoleMode controls whether Console writes to real stdio or buffers.
var consoleMode struct {
	buffer bool // true = capture to capEnv (--json mode)
}

var (
	stdinScanner = bufio.NewScanner(os.Stdin)
	stdoutWriter = bufio.NewWriter(os.Stdout)
)

func flushConsole() { stdoutWriter.Flush() }

var unitVal = &eval.RecordVal{Fields: map[string]eval.Value{}}

func putLineImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	hv, ok := args[0].(*eval.HostVal)
	if !ok {
		return nil, ce, fmt.Errorf("putLine: expected string argument, got %T", args[0])
	}
	s, ok := hv.Inner.(string)
	if !ok {
		return nil, ce, fmt.Errorf("putLine: expected string, got %T", hv.Inner)
	}
	if consoleMode.buffer {
		return unitVal, appendConsoleBuffer(ce, s), nil
	}
	stdoutWriter.WriteString(s)
	stdoutWriter.WriteByte('\n')
	stdoutWriter.Flush()
	return unitVal, ce, nil
}

func getLineImpl(ctx context.Context, ce eval.CapEnv, _ []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	if consoleMode.buffer {
		return &eval.HostVal{Inner: ""}, ce, fmt.Errorf("getLine: not available in --json mode")
	}
	if stdinScanner.Scan() {
		s := stdinScanner.Text()
		if err := budget.ChargeAlloc(ctx, int64(len(s))); err != nil {
			return nil, ce, fmt.Errorf("getLine: %w", err)
		}
		return &eval.HostVal{Inner: s}, ce, nil
	}
	if err := stdinScanner.Err(); err != nil {
		return nil, ce, fmt.Errorf("getLine: %w", err)
	}
	return &eval.HostVal{Inner: ""}, ce, nil
}

// appendConsoleBuffer captures a console line into the capability environment.
func appendConsoleBuffer(ce eval.CapEnv, line string) eval.CapEnv {
	existing, _ := ce.Get("console")
	var buf []string
	if b, ok := existing.([]string); ok {
		buf = append(b[:len(b):len(b)], line)
	} else {
		buf = []string{line}
	}
	return ce.Set("console", buf)
}
