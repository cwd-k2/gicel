// Command gicel provides a CLI for the GICEL language.
//
// Usage:
//
//	gicel run     [flags] <file>   compile and execute
//	gicel check   [flags] <file>   type-check only
//	gicel docs    [topic]          show language reference
//	gicel example [name]           show example programs
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cwd-k2/gicel"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		os.Exit(cmdRun(os.Args[2:]))
	case "check":
		os.Exit(cmdCheck(os.Args[2:]))
	case "docs":
		os.Exit(cmdDocs(os.Args[2:]))
	case "example":
		os.Exit(cmdExample(os.Args[2:]))
	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: gicel <command> [flags] [args]

Commands:
  run      Compile and execute a GICEL program
  check    Type-check a GICEL program
  docs     Show language reference (docs <topic> for details)
  example  Show example programs (example <name> for source)

Flags (run, check):
  --use <packs>    Stdlib packs: Num,Str,List,Fail,State,IO (default: all)
  --recursion      Enable recursive definitions (fix/rec)

Flags (run only):
  --entry <name>   Entry point binding (default: main)
  --timeout <dur>  Execution timeout (default: 5s)
  --max-steps <n>  Step limit (default: 100000)
  --max-depth <n>  Depth limit (default: 100)
  --json           Output result as JSON
  --explain        Show semantic evaluation trace`)
}

func cmdDocs(args []string) int {
	if len(args) == 0 {
		content := gicel.Doc("index")
		if content == "" {
			fmt.Fprintln(os.Stderr, "no documentation available")
			return 1
		}
		fmt.Print(content)
		return 0
	}
	topic := args[0]
	content := gicel.Doc(topic)
	if content == "" {
		fmt.Fprintf(os.Stderr, "unknown topic: %s\n\nAvailable topics:\n", topic)
		for _, t := range gicel.DocTopics() {
			fmt.Fprintf(os.Stderr, "  %s\n", t)
		}
		return 1
	}
	fmt.Print(content)
	return 0
}

func cmdExample(args []string) int {
	if len(args) == 0 {
		examples := gicel.Examples()
		if len(examples) == 0 {
			fmt.Fprintln(os.Stderr, "no examples available")
			return 1
		}
		fmt.Println("Available examples:")
		fmt.Println()
		for _, name := range examples {
			desc := exampleDesc(gicel.Example(name))
			if desc != "" {
				fmt.Printf("  %-26s %s\n", name, desc)
			} else {
				fmt.Printf("  %s\n", name)
			}
		}
		fmt.Println()
		fmt.Println("Run 'gicel example <name>' to view source.")
		return 0
	}
	name := args[0]
	content := gicel.Example(name)
	if content == "" {
		fmt.Fprintf(os.Stderr, "unknown example: %s\n\nAvailable examples:\n", name)
		for _, e := range gicel.Examples() {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		return 1
	}
	fmt.Print(content)
	return 0
}

// exampleDesc extracts the title from "-- GICEL Example: <title>".
func exampleDesc(source string) string {
	const prefix = "-- GICEL Example: "
	if i := strings.Index(source, "\n"); i >= 0 {
		source = source[:i]
	}
	if strings.HasPrefix(source, prefix) {
		return strings.TrimPrefix(source, prefix)
	}
	return ""
}

// packMap maps pack names to their Pack functions.
var packMap = map[string]gicel.Pack{
	"num":   gicel.Num,
	"str":   gicel.Str,
	"list":  gicel.List,
	"fail":  gicel.Fail,
	"state": gicel.State,
	"io":    gicel.IO,
}

func setupEngine(use string) (*gicel.Engine, error) {
	eng := gicel.NewEngine()
	names := strings.Split(strings.ToLower(use), ",")
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if name == "all" {
			for _, p := range packMap {
				if err := p(eng); err != nil {
					return nil, err
				}
			}
			return eng, nil
		}
		p, ok := packMap[name]
		if !ok {
			return nil, fmt.Errorf("unknown pack: %s (available: Num,Str,List,Fail,State,IO)", name)
		}
		if err := p(eng); err != nil {
			return nil, err
		}
	}
	return eng, nil
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	use := fs.String("use", "all", "comma-separated stdlib packs")
	recursion := fs.Bool("recursion", false, "enable recursive definitions (fix/rec)")
	entry := fs.String("entry", "main", "entry point binding")
	timeout := fs.Duration("timeout", 5*time.Second, "execution timeout")
	maxSteps := fs.Int("max-steps", 100000, "step limit")
	maxDepth := fs.Int("max-depth", 100, "depth limit")
	jsonOut := fs.Bool("json", false, "output as JSON")
	explain := fs.Bool("explain", false, "show semantic evaluation trace")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "error: no source file specified")
		return 1
	}

	source, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	eng, err := setupEngine(*use)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if *recursion {
		eng.EnableRecursion()
	}
	eng.SetStepLimit(*maxSteps)
	eng.SetDepthLimit(*maxDepth)

	var explainSteps []gicel.ExplainStep
	if *explain {
		eng.SetExplainHook(func(step gicel.ExplainStep) {
			if *jsonOut {
				explainSteps = append(explainSteps, step)
				return
			}
			if step.Kind == gicel.ExplainResult {
				return // result goes to stdout, not the trace
			}
			if step.Line > 0 {
				fmt.Fprintf(os.Stderr, "L%d: %s\n", step.Line, step.Message)
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", step.Message)
			}
		})
	}

	rt, err := eng.NewRuntime(string(source))
	if err != nil {
		if *jsonOut {
			outputJSON(map[string]any{"ok": false, "error": err.Error(), "phase": "compile"})
		} else {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	result, err := rt.RunContextFull(ctx, nil, nil, *entry)
	if err != nil {
		if *jsonOut {
			outputJSON(map[string]any{"ok": false, "error": err.Error(), "phase": "eval"})
		} else {
			fmt.Fprintf(os.Stderr, "runtime error: %v\n", err)
		}
		return 1
	}

	if *jsonOut {
		out := map[string]any{
			"ok":    true,
			"value": formatValue(result.Value),
			"stats": map[string]any{
				"steps":    result.Stats.Steps,
				"maxDepth": result.Stats.MaxDepth,
			},
		}
		if *explain {
			out["explain"] = explainSteps
		}
		outputJSON(out)
	} else {
		fmt.Println(gicel.PrettyValue(result.Value))
	}
	return 0
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	use := fs.String("use", "all", "comma-separated stdlib packs")
	recursion := fs.Bool("recursion", false, "enable recursive definitions (fix/rec)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "error: no source file specified")
		return 1
	}

	source, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	eng, err := setupEngine(*use)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if *recursion {
		eng.EnableRecursion()
	}

	_, err = eng.Check(string(source))
	if err != nil {
		if *jsonOut {
			outputJSON(map[string]any{"ok": false, "error": err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		return 1
	}

	if *jsonOut {
		outputJSON(map[string]any{"ok": true})
	} else {
		fmt.Println("ok")
	}
	return 0
}

func outputJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v) // write error to stdout is intentionally ignored
}

func formatValue(v gicel.Value) any {
	switch val := v.(type) {
	case *gicel.HostVal:
		return val.Inner
	case *gicel.ConVal:
		if len(val.Args) == 0 {
			return val.Con
		}
		args := make([]any, len(val.Args))
		for i, a := range val.Args {
			args[i] = formatValue(a)
		}
		return map[string]any{"con": val.Con, "args": args}
	default:
		return v.String()
	}
}
