// check subcommand — type-check a GICEL program without executing it.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func printCheckUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: gicel check [flags] <file>

Flags:
  --packs <list>     Stdlib packs (default: all)
  --module Name=path Register a user module (repeatable)
  --recursion        Enable recursive definitions (fix/rec)
  -e <source>        Evaluate source string directly
  --json             Output as JSON
  --max-nesting <n>  Structural nesting depth limit (default: 512)
  --timeout <dur>    Compilation timeout (default: 5s)`)
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	packs := fs.String("packs", "", "comma-separated stdlib packs (default: from header or all)")
	recursion := fs.Bool("recursion", false, "enable recursive definitions (fix/rec)")
	var modules moduleFlags
	fs.Var(&modules, "module", "register module: Name=path (repeatable)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	maxNesting := fs.Int("max-nesting", 512, "structural nesting depth limit")
	var expr exprFlag
	fs.Var(&expr, "e", "evaluate source string directly")
	timeout := fs.Duration("timeout", 5*time.Second, "compilation timeout")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printCheckUsage(os.Stderr)
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", normalizeFlagError(err.Error()))
		printCheckUsage(os.Stderr)
		return 1
	}
	warnTrailingFlags(fs)
	if expr.count > 1 {
		fmt.Fprintf(os.Stderr, "warning: -e specified %d times; using last value\n", expr.count)
	}

	if *timeout <= 0 {
		return preflightError("--timeout must be a positive duration (e.g., 1s, 5m)", *jsonOut)
	}
	if *maxNesting <= 0 {
		return preflightError("--max-nesting must be a positive integer", *jsonOut)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	source, eng, err := prepareEngine(fs, *packs, *recursion, expr.value, expr.count > 0, modules)
	if err != nil {
		return preflightError(err.Error(), *jsonOut)
	}
	eng.SetCompileContext(ctx)
	eng.SetNestingLimit(*maxNesting)

	cr, err := eng.Compile(ctx, string(source))
	if err != nil {
		return handleCompileError(err, *jsonOut)
	}

	if *jsonOut {
		out := map[string]any{"ok": true}
		if types := cr.PrettyBindingTypes(); len(types) > 0 {
			out["bindings"] = types
		}
		outputJSON(out)
	} else {
		fmt.Println("ok")
	}
	return 0
}
