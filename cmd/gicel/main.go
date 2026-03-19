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
	"flag"
	"fmt"
	"io"
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
  --use <packs>    Packs: prelude,fail,state,io,stream,slice,map,set (default: all)
  --module Name=path  Register a user module (repeatable)
  --recursion      Enable recursive definitions (fix/rec)
  -e <source>      Evaluate source string directly

Flags (run only):
  --entry <name>   Entry point binding (default: main)
  --timeout <dur>  Execution timeout (default: 5s)
  --max-steps <n>  Step limit (default: 100000)
  --max-depth <n>  Depth limit (default: 100)
  --max-alloc <n>  Allocation byte limit (default: 100 MiB)
  --json           Output result as JSON
  --explain        Show semantic evaluation trace
  --explain-all    Trace stdlib internals (with --explain)
  --verbose        Show source context in explain trace
  --no-color       Disable color output`)
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
		// Group examples by level for progressive learning.
		type entry struct {
			name, desc string
		}
		groups := map[string][]entry{}
		var ungrouped []entry
		for _, name := range examples {
			src := gicel.Example(name)
			desc := exampleDesc(src)
			level := exampleLevel(src)
			e := entry{name, desc}
			if level != "" {
				groups[level] = append(groups[level], e)
			} else {
				ungrouped = append(ungrouped, e)
			}
		}

		levelOrder := []struct{ key, label string }{
			{"basics", "Basics"},
			{"types", "Type System"},
			{"effects", "Effects & Applications"},
		}
		for _, lv := range levelOrder {
			entries := groups[lv.key]
			if len(entries) == 0 {
				continue
			}
			fmt.Printf("%s:\n", lv.label)
			for _, e := range entries {
				if e.desc != "" {
					fmt.Printf("  %-26s %s\n", e.name, e.desc)
				} else {
					fmt.Printf("  %s\n", e.name)
				}
			}
			fmt.Println()
		}
		if len(ungrouped) > 0 {
			fmt.Println("Other:")
			for _, e := range ungrouped {
				if e.desc != "" {
					fmt.Printf("  %-26s %s\n", e.name, e.desc)
				} else {
					fmt.Printf("  %s\n", e.name)
				}
			}
			fmt.Println()
		}
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

// exampleLevel extracts the level from "-- Level: <level>".
func exampleLevel(source string) string {
	const prefix = "-- Level: "
	for _, line := range strings.SplitN(source, "\n", 20) {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
		if line == "" {
			break // stop at end of header block
		}
	}
	return ""
}

// packMap maps pack names to their Pack functions.
var packMap = map[string]gicel.Pack{
	"prelude": gicel.Prelude,
	"fail":    gicel.EffectFail,
	"state":   gicel.EffectState,
	"io":      gicel.EffectIO,
	"stream":  gicel.DataStream,
	"slice":   gicel.DataSlice,
	"map":     gicel.DataMap,
	"set":     gicel.DataSet,
}

// allPackOrder ensures deterministic pack loading.
var allPackOrder = []string{"prelude", "fail", "state", "io", "stream", "slice", "map", "set"}

func setupEngine(use string) (*gicel.Engine, error) {
	eng := gicel.NewEngine()
	names := strings.Split(strings.ToLower(use), ",")
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if name == "all" {
			for _, n := range allPackOrder {
				if err := packMap[n](eng); err != nil {
					return nil, err
				}
			}
			return eng, nil
		}
		p, ok := packMap[name]
		if !ok {
			return nil, fmt.Errorf("unknown pack: %s (available: prelude,fail,state,io,stream,slice,map,set)", name)
		}
		if err := p(eng); err != nil {
			return nil, err
		}
	}
	return eng, nil
}

// moduleFlags is a repeatable flag for --module Name=path pairs.
type moduleFlags []string

func (m *moduleFlags) String() string { return strings.Join(*m, ", ") }
func (m *moduleFlags) Set(val string) error {
	if !strings.Contains(val, "=") {
		return fmt.Errorf("expected Name=path, got %q", val)
	}
	*m = append(*m, val)
	return nil
}

// readSource loads GICEL source from -e string, stdin ("-"), or a file argument.
func readSource(fs *flag.FlagSet, expr string) ([]byte, error) {
	if expr != "" {
		return []byte(expr), nil
	}
	if fs.NArg() < 1 {
		return nil, fmt.Errorf("no source file specified (use -e or pass a file, - for stdin)")
	}
	if fs.Arg(0) == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(fs.Arg(0))
}

// registerUserModules parses --module Name=path flags and registers each module.
func registerUserModules(eng *gicel.Engine, modules []string) error {
	for _, spec := range modules {
		eqIdx := strings.IndexByte(spec, '=')
		if eqIdx < 0 {
			return fmt.Errorf("invalid module spec: %q (expected Name=path)", spec)
		}
		name := spec[:eqIdx]
		path := spec[eqIdx+1:]
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading module %s: %w", name, err)
		}
		if err := eng.RegisterModule(name, string(data)); err != nil {
			return fmt.Errorf("registering module %s: %w", name, err)
		}
	}
	return nil
}

// prepareEngine loads source and configures the engine with common flags.
func prepareEngine(fs *flag.FlagSet, use string, recursion bool, expr string, modules []string) ([]byte, *gicel.Engine, error) {
	source, err := readSource(fs, expr)
	if err != nil {
		return nil, nil, err
	}
	eng, err := setupEngine(use)
	if err != nil {
		return nil, nil, err
	}
	if recursion {
		eng.EnableRecursion()
	}
	if err := registerUserModules(eng, modules); err != nil {
		return nil, nil, err
	}
	return source, eng, nil
}

func handleCompileError(err error, jsonOut bool) int {
	if jsonOut {
		outputJSON(compileErrorJSON(err))
	} else {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
	return 1
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	use := fs.String("use", "all", "comma-separated stdlib packs")
	recursion := fs.Bool("recursion", false, "enable recursive definitions (fix/rec)")
	var modules moduleFlags
	fs.Var(&modules, "module", "register module: Name=path (repeatable)")
	entry := fs.String("entry", "main", "entry point binding")
	timeout := fs.Duration("timeout", 5*time.Second, "execution timeout")
	maxSteps := fs.Int("max-steps", 100000, "step limit")
	maxDepth := fs.Int("max-depth", 100, "depth limit")
	maxAlloc := fs.Int64("max-alloc", 100*1024*1024, "allocation byte limit (default: 100 MiB)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	explain := fs.Bool("explain", false, "show semantic evaluation trace")
	verbose := fs.Bool("verbose", false, "show source context in explain trace")
	noColor := fs.Bool("no-color", false, "disable color output")
	expr := fs.String("e", "", "evaluate source string directly")
	explainAll := fs.Bool("explain-all", false, "trace stdlib internals (with --explain)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Validate resource limit flags.
	if *maxSteps <= 0 {
		fmt.Fprintln(os.Stderr, "error: --max-steps must be a positive integer")
		return 1
	}
	if *maxDepth <= 0 {
		fmt.Fprintln(os.Stderr, "error: --max-depth must be a positive integer")
		return 1
	}
	if *maxAlloc <= 0 {
		fmt.Fprintln(os.Stderr, "error: --max-alloc must be a positive integer")
		return 1
	}
	if *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "error: --timeout must be a positive duration (e.g., 1s, 5m)")
		return 1
	}

	// Validate --entry: reject explicitly empty entry point.
	if *entry == "" {
		fmt.Fprintln(os.Stderr, "error: --entry must not be empty")
		return 1
	}

	source, eng, err := prepareEngine(fs, *use, *recursion, *expr, modules)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	eng.SetStepLimit(*maxSteps)
	eng.SetDepthLimit(*maxDepth)
	eng.SetAllocLimit(*maxAlloc)
	eng.SetEntryPoint(*entry)

	rt, err := eng.NewRuntime(context.Background(), string(source))
	if err != nil {
		return handleCompileError(err, *jsonOut)
	}

	// Build per-execution options with explain/trace hooks.
	var explainSteps []gicel.ExplainStep
	var formatter *explainFormatter
	opts := &gicel.RunOptions{Entry: *entry}
	if *explain {
		if *jsonOut {
			opts.Explain = func(step gicel.ExplainStep) {
				explainSteps = append(explainSteps, step)
			}
		} else {
			formatter = newExplainFormatter(os.Stderr, useColor(*noColor), *verbose, string(source))
			opts.Explain = formatter.Emit
		}
		if *explainAll {
			opts.ExplainDepth = gicel.ExplainAll
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	result, err := rt.RunWith(ctx, opts)
	if err != nil {
		if *jsonOut {
			outputJSON(map[string]any{"ok": false, "error": err.Error(), "phase": "eval"})
		} else {
			fmt.Fprintf(os.Stderr, "runtime error: %v\n", err)
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
				"steps":    result.Stats.Steps,
				"maxDepth": result.Stats.MaxDepth,
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
	} else {
		fmt.Println(gicel.PrettyValue(result.Value))
	}
	return 0
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	use := fs.String("use", "all", "comma-separated stdlib packs")
	recursion := fs.Bool("recursion", false, "enable recursive definitions (fix/rec)")
	var modules moduleFlags
	fs.Var(&modules, "module", "register module: Name=path (repeatable)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	expr := fs.String("e", "", "evaluate source string directly")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	source, eng, err := prepareEngine(fs, *use, *recursion, *expr, modules)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	cr, err := eng.Compile(context.Background(), string(source))
	if err != nil {
		return handleCompileError(err, *jsonOut)
	}

	if *jsonOut {
		out := map[string]any{"ok": true}
		if types := cr.BindingTypes(); len(types) > 0 {
			out["bindings"] = types
		}
		outputJSON(out)
	} else {
		fmt.Println("ok")
	}
	return 0
}

// compileErrorJSON produces structured JSON from a compile error.
// If the error is a *gicel.CompileError, diagnostics are included.
