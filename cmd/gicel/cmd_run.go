// run subcommand — compile and execute a GICEL program.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cwd-k2/gicel"
)

func printRunUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: gicel run [flags] <file>

Flags:
  --packs <list>     Stdlib packs (default: all)
  --module Name=path Register a user module (repeatable)
  --recursion        Enable recursive definitions (fix/rec)
  -e <source>        Evaluate source string directly
  --entry <name>     Entry point binding (default: main)
  --timeout <dur>    Execution timeout (default: 5s)
  --max-steps <n>    Step limit (default: 100000)
  --max-depth <n>    Depth limit (default: 10000)
  --max-nesting <n>  Structural nesting depth limit (default: 512)
  --max-alloc <n>    Allocation byte limit (default: 100 MiB)
  --json             Output result as JSON
  --explain          Show semantic evaluation trace
  --explain-all      Trace stdlib internals (with --explain)
  --verbose          Show source context in explain trace
  --no-color         Disable color output`)
}

func cmdRun(args []string) int {
	defer flushConsole()
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	packs := fs.String("packs", "all", "comma-separated stdlib packs")
	recursion := fs.Bool("recursion", false, "enable recursive definitions (fix/rec)")
	var modules moduleFlags
	fs.Var(&modules, "module", "register module: Name=path (repeatable)")
	entry := fs.String("entry", gicel.DefaultEntryPoint, "entry point binding")
	timeout := fs.Duration("timeout", 5*time.Second, "execution timeout")
	maxSteps := fs.Int("max-steps", 100000, "step limit")
	maxDepth := fs.Int("max-depth", 10000, "depth limit")
	maxNesting := fs.Int("max-nesting", 512, "structural nesting depth limit")
	maxAlloc := &byteSizeFlag{value: 100 * 1024 * 1024}
	fs.Var(maxAlloc, "max-alloc", "allocation byte limit (default: 100 MiB)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	explain := fs.Bool("explain", false, "show semantic evaluation trace")
	verbose := fs.Bool("verbose", false, "show source context in explain trace")
	noColor := fs.Bool("no-color", false, "disable color output")
	var expr exprFlag
	fs.Var(&expr, "e", "evaluate source string directly")
	explainAll := fs.Bool("explain-all", false, "trace stdlib internals (with --explain)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printRunUsage(os.Stderr)
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", normalizeFlagError(err.Error()))
		printRunUsage(os.Stderr)
		return 1
	}
	warnTrailingFlags(fs)
	if expr.count > 1 {
		fmt.Fprintf(os.Stderr, "warning: -e specified %d times; using last value\n", expr.count)
	}
	if !*explain {
		if *explainAll {
			fmt.Fprintln(os.Stderr, "warning: --explain-all has no effect without --explain")
		}
		if *verbose {
			fmt.Fprintln(os.Stderr, "warning: --verbose has no effect without --explain")
		}
	}

	// --json mode: Console buffers output into capEnv instead of stdout.
	// Buffer mode is signaled by setting the console capability to []string{}.
	// Direct mode uses nil. Primitives detect mode via the capability value type.

	// Validate resource limit flags.
	if *maxSteps <= 0 {
		return preflightError("--max-steps must be a positive integer", *jsonOut)
	}
	if *maxDepth <= 0 {
		return preflightError("--max-depth must be a positive integer", *jsonOut)
	}
	if *maxNesting <= 0 {
		return preflightError("--max-nesting must be a positive integer", *jsonOut)
	}
	if maxAlloc.value <= 0 {
		return preflightError("--max-alloc must be a positive integer", *jsonOut)
	}
	if *timeout <= 0 {
		return preflightError("--timeout must be a positive duration (e.g., 1s, 5m)", *jsonOut)
	}

	// Validate --entry: reject explicitly empty entry point.
	if *entry == "" {
		return preflightError("--entry must not be empty", *jsonOut)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	source, eng, err := prepareEngine(fs, *packs, *recursion, expr.value, modules)
	if err != nil {
		return preflightError(err.Error(), *jsonOut)
	}
	eng.SetCompileContext(ctx)
	eng.SetStepLimit(*maxSteps)
	eng.SetDepthLimit(*maxDepth)
	eng.SetNestingLimit(*maxNesting)
	eng.SetAllocLimit(maxAlloc.value)
	eng.SetEntryPoint(*entry)
	if *explain {
		eng.DisableInlining()
	}

	rt, err := eng.NewRuntime(ctx, string(source))
	if err != nil {
		return handleCompileError(err, *jsonOut)
	}

	// Build per-execution options with explain/trace hooks.
	var explainSteps []gicel.ExplainStep
	var formatter *explainFormatter
	consoleCap := any(nil)
	if *jsonOut {
		consoleCap = []string{} // buffer mode: start with empty slice
	}
	opts := &gicel.RunOptions{
		Entry: *entry,
		Caps:  map[string]any{"console": consoleCap},
	}
	if *explain {
		if *jsonOut {
			opts.Explain = func(step gicel.ExplainStep) {
				explainSteps = append(explainSteps, step)
			}
		} else {
			formatter = newExplainFormatter(os.Stderr, useColor(*noColor), *verbose, string(source), "<input>")
			opts.Explain = formatter.Emit
		}
		if *explainAll {
			opts.ExplainDepth = gicel.ExplainAll
		}
	}

	result, err := rt.RunWith(ctx, opts)
	if err != nil {
		// Flush explain trace before error output so partial traces are visible.
		if formatter != nil {
			formatter.Flush()
		}
		if *jsonOut {
			errJSON := runtimeErrorJSON(err)
			if result != nil {
				errJSON["stats"] = map[string]any{
					"steps":     result.Stats.Steps,
					"maxDepth":  result.Stats.MaxDepth,
					"allocated": result.Stats.Allocated,
				}
			}
			if *explain && len(explainSteps) > 0 {
				errJSON["explain"] = explainSteps
				errJSON["summary"] = summarizeSteps(explainSteps)
			}
			outputJSON(errJSON)
		} else {
			var re *gicel.RuntimeError
			if errors.As(err, &re) {
				if re.Line > 0 {
					fmt.Fprintf(os.Stderr, "%d:%d: runtime error: %s\n", re.Line, re.Col, re.Message)
				} else {
					fmt.Fprintf(os.Stderr, "runtime error: %s\n", re.Message)
				}
			} else {
				fmt.Fprintf(os.Stderr, "runtime error: %v\n", err)
			}
		}
		return 1
	}

	if formatter != nil {
		formatter.Flush()
	}

	if *jsonOut {
		out := map[string]any{
			"ok":    true,
			"value": formatValue(result.Value),
			"stats": map[string]any{
				"steps":     result.Stats.Steps,
				"maxDepth":  result.Stats.MaxDepth,
				"allocated": result.Stats.Allocated,
			},
		}
		if caps := formatCapEnv(result.CapEnv); len(caps) > 0 {
			out["capEnv"] = caps
		}
		if *explain {
			out["explain"] = explainSteps
			out["summary"] = summarizeSteps(explainSteps)
		}
		outputJSON(out)
	} else if !isUnitValue(result.Value) {
		fmt.Println(gicel.PrettyValue(result.Value))
	}
	return 0
}
