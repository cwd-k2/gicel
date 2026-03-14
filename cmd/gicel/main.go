// Command gicel provides a CLI for the GICEL language.
//
// Usage:
//
//	gicel run  [flags] <file>    compile and execute
//	gicel check [flags] <file>   type-check only
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
	case "help", "-h", "--help":
		os.Exit(cmdHelp(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: gicel <command> [flags] [args]

Commands:
  run    Compile and execute a GICEL program
  check  Type-check a GICEL program
  help   Show language reference (help <topic> for details)

Flags (run/check):
  --use <packs>    Comma-separated stdlib packs: Num,Str,List,Fail,State,IO (default: all)
  --entry <name>   Entry point binding (default: main)
  --timeout <dur>  Execution timeout (default: 5s, run only)
  --max-steps <n>  Step limit (default: 100000, run only)
  --max-depth <n>  Depth limit (default: 100, run only)
  --json           Output result as JSON (run only)`)
}

func cmdHelp(args []string) int {
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
	entry := fs.String("entry", "main", "entry point binding")
	timeout := fs.Duration("timeout", 5*time.Second, "execution timeout")
	maxSteps := fs.Int("max-steps", 100000, "step limit")
	maxDepth := fs.Int("max-depth", 100, "depth limit")
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

	eng.SetStepLimit(*maxSteps)
	eng.SetDepthLimit(*maxDepth)

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
		outputJSON(map[string]any{
			"ok":    true,
			"value": formatValue(result.Value),
			"stats": map[string]any{
				"steps":    result.Stats.Steps,
				"maxDepth": result.Stats.MaxDepth,
			},
		})
	} else {
		fmt.Println(result.Value)
	}
	return 0
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	use := fs.String("use", "all", "comma-separated stdlib packs")
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

	_, err = eng.NewRuntime(string(source))
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
	_ = enc.Encode(v)
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
