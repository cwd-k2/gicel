// Command gicel provides a CLI for the GICEL language.
//
// Usage:
//
//	gicel run     [flags] <file>   compile and execute
//	gicel check   [flags] <file>   type-check only
//	gicel lsp     [flags]          start language server (stdio)
//	gicel docs    [topic]          list topics or show topic
//	gicel example [name]           list examples or show source
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"log"

	"github.com/cwd-k2/gicel"
	"github.com/cwd-k2/gicel/internal/app/engine"
	"github.com/cwd-k2/gicel/internal/app/header"
	"github.com/cwd-k2/gicel/internal/lsp"
	"github.com/cwd-k2/gicel/internal/lsp/jsonrpc"
)

// version is the CLI version, set via -ldflags at build time.
var version = "dev"

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
	case "lsp":
		os.Exit(cmdLsp(os.Args[2:]))
	case "docs":
		os.Exit(cmdDocs(os.Args[2:]))
	case "example":
		os.Exit(cmdExample(os.Args[2:]))
	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)
	case "version", "--version":
		fmt.Println("gicel " + version)
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
  lsp      Start language server (stdio transport)
  docs     List topics, or show a topic (docs <topic>)
  example  List examples, or show source (example <name>)
  version  Print version information

Use 'gicel <command> --help' for flag details.

Packs: prelude,fail,state,io,stream,slice,map,set,array,ref,mmap,mset,json,console`)
}

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

func printDocsUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: gicel docs [topic]

List available documentation topics, or show a specific topic.`)
}

func printExampleUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: gicel example [name]

List available examples, or show example source.`)
}

// normalizeFlagError rewrites Go flag error messages to use -- prefix for
// multi-character flags, preserving single-character flags as -e style.
func normalizeFlagError(msg string) string {
	// "flag provided but not defined: -xyz" → "unknown flag: --xyz"
	if rest, ok := strings.CutPrefix(msg, "flag provided but not defined: -"); ok {
		rest = strings.TrimPrefix(rest, "-") // handle ---xyz edge case
		if len(rest) > 1 {
			return "unknown flag: --" + rest
		}
		return "unknown flag: -" + rest
	}
	// "invalid value ... for flag -xyz" → "invalid value ... for flag --xyz"
	if i := strings.Index(msg, " for flag -"); i >= 0 {
		after := msg[i+len(" for flag -"):]
		after = strings.TrimPrefix(after, "-")
		if len(after) > 1 {
			return msg[:i] + " for flag --" + after
		}
	}
	return msg
}

func isHelpFlag(s string) bool {
	return s == "--help" || s == "-h" || s == "-help" || s == "help"
}

// catalogEntry is a name–description pair for docs/example listing.
type catalogEntry struct {
	name, desc string
}

// printCatalog groups entries by dot-separated category prefix and prints them.
func printCatalog(entries []catalogEntry, defaultLabel string) {
	groups := map[string][]catalogEntry{}
	var groupOrder []string
	for _, e := range entries {
		category := ""
		if i := strings.LastIndex(e.name, "."); i >= 0 {
			category = e.name[:i]
		}
		if _, seen := groups[category]; !seen {
			groupOrder = append(groupOrder, category)
		}
		groups[category] = append(groups[category], e)
	}
	for _, cat := range groupOrder {
		label := cat
		if label == "" {
			label = defaultLabel
		}
		fmt.Printf("%s:\n", label)
		for _, e := range groups[cat] {
			if e.desc != "" {
				fmt.Printf("  %-30s %s\n", e.name, e.desc)
			} else {
				fmt.Printf("  %s\n", e.name)
			}
		}
		fmt.Println()
	}
}

// warnTrailingFlags warns about flag-like arguments after the positional filename.
func warnTrailingFlags(fs *flag.FlagSet) {
	if fs.NArg() <= 1 {
		return
	}
	for _, a := range fs.Args()[1:] {
		if strings.HasPrefix(a, "-") {
			fmt.Fprintf(os.Stderr, "warning: %q after filename is ignored; place flags before the filename\n", a)
		}
	}
}

func cmdDocs(args []string) int {
	if len(args) > 0 && isHelpFlag(args[0]) {
		printDocsUsage(os.Stderr)
		return 0
	}
	if len(args) == 0 {
		topics := gicel.DocTopics()
		if len(topics) == 0 {
			fmt.Fprintln(os.Stderr, "no documentation available")
			return 1
		}
		entries := make([]catalogEntry, len(topics))
		for i, name := range topics {
			entries[i] = catalogEntry{name, gicel.DocDesc(name)}
		}
		printCatalog(entries, "general")
		fmt.Println("Run 'gicel docs <topic>' for details.")
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
	if len(args) > 0 && isHelpFlag(args[0]) {
		printExampleUsage(os.Stderr)
		return 0
	}
	if len(args) == 0 {
		examples := gicel.Examples()
		if len(examples) == 0 {
			fmt.Fprintln(os.Stderr, "no examples available")
			return 1
		}
		entries := make([]catalogEntry, len(examples))
		for i, name := range examples {
			entries[i] = catalogEntry{name, exampleDesc(gicel.Example(name))}
		}
		printCatalog(entries, "Other")
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

// exampleDesc extracts the title from "-- GICEL Example: <title>" and
// appends any required flags from "-- Flags: <flags>" lines.
func exampleDesc(source string) string {
	const titlePrefix = "-- GICEL Example: "
	const flagsPrefix = "-- Flags: "
	var title, flags string
	for line := range strings.SplitSeq(source, "\n") {
		if t, ok := strings.CutPrefix(line, titlePrefix); ok && title == "" {
			title = t
		}
		if f, ok := strings.CutPrefix(line, flagsPrefix); ok && flags == "" {
			flags = strings.TrimSpace(f)
		}
		// Stop scanning after the header comment block.
		if !strings.HasPrefix(line, "--") && strings.TrimSpace(line) != "" {
			break
		}
	}
	if flags != "" && title != "" {
		return title + " [" + flags + "]"
	}
	return title
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
	"array":   gicel.EffectArray,
	"mmap":    gicel.EffectMap,
	"mset":    gicel.EffectSet,
	"ref":     gicel.EffectRef,
	"json":    gicel.DataJSON,
	"console": consolePack,
}

// allPackOrder ensures deterministic pack loading.
var allPackOrder = []string{"prelude", "fail", "state", "io", "stream", "slice", "map", "set", "array", "ref", "mmap", "mset", "json", "console"}

func setupEngine(packs string) (*gicel.Engine, error) {
	eng := gicel.NewEngine()

	// Pre-validate pack names and dependencies.
	names := strings.Split(strings.ToLower(packs), ",")
	hasAll, hasPrelude, hasAny := false, false, false
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		hasAny = true
		if n == "all" {
			hasAll = true
			continue
		}
		if _, ok := packMap[n]; !ok {
			return nil, fmt.Errorf("unknown pack: %s (available: prelude,fail,state,io,stream,slice,map,set,array,ref,mmap,mset,json,console)", n)
		}
		if n == "prelude" {
			hasPrelude = true
		}
	}
	if !hasAny {
		return nil, fmt.Errorf("no packs specified; use --packs prelude or --packs all")
	}
	if !hasAll && !hasPrelude {
		for _, n := range names {
			n = strings.TrimSpace(n)
			if n != "" && n != "prelude" {
				return nil, fmt.Errorf("pack %q requires prelude; use --packs prelude,%s or --packs all", n, packs)
			}
		}
	}

	seen := make(map[string]bool)
	for name := range strings.SplitSeq(strings.ToLower(packs), ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if seen[name] {
			continue // deduplicate
		}
		seen[name] = true
		if name == "all" {
			for _, n := range allPackOrder {
				if !seen[n] {
					seen[n] = true
					if err := packMap[n](eng); err != nil {
						return nil, err
					}
				}
			}
			return eng, nil
		}
		p := packMap[name] // pre-validated above
		if err := p(eng); err != nil {
			return nil, err
		}
	}
	return eng, nil
}

// exprFlag tracks the -e flag value and warns on duplicates.
type exprFlag struct {
	value string
	count int
}

func (e *exprFlag) String() string { return e.value }
func (e *exprFlag) Set(val string) error {
	e.value = val
	e.count++
	return nil
}

// byteSizeFlag parses byte sizes with optional suffixes: KiB, MiB, GiB, KB, MB, GB.
// Plain integers are treated as raw bytes.
type byteSizeFlag struct {
	value int64
}

func (b *byteSizeFlag) String() string { return strconv.FormatInt(b.value, 10) }
func (b *byteSizeFlag) Set(val string) error {
	suffixes := []struct {
		suffix string
		mult   int64
	}{
		{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
		{"GB", 1_000_000_000}, {"MB", 1_000_000}, {"KB", 1_000},
	}
	for _, s := range suffixes {
		trimmed := strings.TrimSpace(val)
		if strings.HasSuffix(trimmed, s.suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(trimmed, s.suffix))
			n, err := strconv.ParseInt(numStr, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid number before %s: %q", s.suffix, numStr)
			}
			if n < 0 {
				return fmt.Errorf("byte size must be non-negative: %q", val)
			}
			b.value = n * s.mult
			return nil
		}
	}
	n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
	if err != nil {
		return fmt.Errorf("expected byte count or size with suffix (e.g. 100MiB): %q", val)
	}
	if n < 0 {
		return fmt.Errorf("byte size must be non-negative: %q", val)
	}
	b.value = n
	return nil
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
func readSource(fs *flag.FlagSet, expr string, budget *sourceBudget) ([]byte, error) {
	if expr != "" {
		if fs.NArg() > 0 {
			fmt.Fprintf(os.Stderr, "warning: -e and file argument both specified; using -e, ignoring %s\n", fs.Arg(0))
		}
		data := []byte(expr)
		budget.used += int64(len(data))
		return data, nil
	}
	if fs.NArg() < 1 {
		return nil, fmt.Errorf("no source file specified (use -e or pass a file, - for stdin)")
	}
	if fs.Arg(0) == "-" {
		return budget.read(os.Stdin)
	}
	return budget.readFile(fs.Arg(0))
}

// maxTotalSourceSize is the aggregate limit across all source files (10 MiB).
const maxTotalSourceSize = 10 * 1024 * 1024

// sourceBudget tracks cumulative source bytes read in a single invocation.
type sourceBudget struct {
	used int64
}

func (b *sourceBudget) readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return b.read(f)
}

func (b *sourceBudget) read(r io.Reader) ([]byte, error) {
	remaining := maxTotalSourceSize - b.used
	if remaining <= 0 {
		return nil, fmt.Errorf("total source size exceeds limit (%d MiB)", maxTotalSourceSize/(1024*1024))
	}
	limited := io.LimitReader(r, remaining+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	b.used += int64(len(data))
	if b.used > maxTotalSourceSize {
		return nil, fmt.Errorf("total source size exceeds limit (%d MiB)", maxTotalSourceSize/(1024*1024))
	}
	return data, nil
}

// registerUserModules parses --module Name=path flags and registers each module.
func registerUserModules(eng *gicel.Engine, modules []string, budget *sourceBudget) error {
	for _, spec := range modules {
		eqIdx := strings.IndexByte(spec, '=')
		if eqIdx < 0 {
			return fmt.Errorf("invalid module spec: %q (expected Name=path)", spec)
		}
		name := spec[:eqIdx]
		if err := gicel.ValidateModuleName(name); err != nil {
			return err
		}
		path := spec[eqIdx+1:]
		data, err := budget.readFile(path)
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
// File header directives (-- gicel: --module, --recursion) are applied
// when the source comes from a file (not -e or stdin).
func prepareEngine(fs *flag.FlagSet, packs string, recursion bool, expr string, modules []string) ([]byte, *gicel.Engine, error) {
	budget := &sourceBudget{}
	source, err := readSource(fs, expr, budget)
	if err != nil {
		return nil, nil, err
	}
	eng, err := setupEngine(packs)
	if err != nil {
		return nil, nil, err
	}
	if recursion {
		eng.EnableRecursion()
	}
	eng.DenyAssumptions() // CLI user code cannot use assumption declarations

	// Apply file header directives when source is from a file.
	if expr == "" && fs.NArg() > 0 && fs.Arg(0) != "-" {
		if err := applyHeaderDirectives(eng, string(source), fs.Arg(0)); err != nil {
			return nil, nil, err
		}
	}

	if err := registerUserModules(eng, modules, budget); err != nil {
		return nil, nil, err
	}
	return source, eng, nil
}

// applyHeaderDirectives recursively resolves -- gicel: directives from the
// source header and registers discovered modules. CLI flags take precedence
// (modules registered here can be overridden by --module flags registered later).
func applyHeaderDirectives(eng *gicel.Engine, source, filePath string) error {
	res, err := header.Resolve(source, filePath)
	if err != nil {
		return err
	}
	if res.Recursion {
		eng.EnableRecursion()
	}
	for _, mod := range res.Modules {
		if err := eng.RegisterModule(mod.Name, mod.Source); err != nil {
			return fmt.Errorf("header module %s: %w", mod.Name, err)
		}
	}
	return nil
}

func handleCompileError(err error, jsonOut bool) int {
	if jsonOut {
		outputJSON(compileErrorJSON(err))
	} else {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
	return 1
}

func preflightError(msg string, jsonOut bool) int {
	if jsonOut {
		outputJSON(map[string]any{"ok": false, "phase": "preflight", "error": msg})
	} else {
		fmt.Fprintf(os.Stderr, "error: %s\n", msg)
	}
	return 1
}

// isUnitValue reports whether a value is the unit value ().
func isUnitValue(v gicel.Value) bool {
	rv, ok := v.(*gicel.RecordVal)
	return ok && rv.Len() == 0
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

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	packs := fs.String("packs", "all", "comma-separated stdlib packs")
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

	source, eng, err := prepareEngine(fs, *packs, *recursion, expr.value, modules)
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

func cmdLsp(args []string) int {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	packs := fs.String("packs", "all", "comma-separated stdlib packs")
	recursion := fs.Bool("recursion", false, "enable recursive definitions (fix/rec)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printLspUsage(os.Stderr)
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", normalizeFlagError(err.Error()))
		printLspUsage(os.Stderr)
		return 1
	}

	// Pre-validate packs.
	if _, err := setupEngine(*packs); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	packsCopy := *packs
	recCopy := *recursion
	logger := log.New(os.Stderr, "[gicel-lsp] ", log.LstdFlags)
	transport := jsonrpc.NewTransport(os.Stdin, os.Stdout)

	srv := lsp.NewServer(lsp.ServerConfig{
		Transport: transport,
		Logger:    logger,
		EngineSetup: func() *engine.Engine {
			eng, _ := setupEngine(packsCopy) // pre-validated
			if recCopy {
				eng.EnableRecursion()
			}
			eng.DenyAssumptions()
			return eng
		},
	})

	if err := srv.Run(context.Background()); err != nil {
		logger.Printf("server error: %v", err)
		return 1
	}
	return srv.ExitCode()
}

func printLspUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: gicel lsp [flags]

Start the Language Server Protocol server (stdio transport).

Flags:
  --packs <list>  Stdlib packs (default: all)
  --recursion     Enable recursive definitions (fix/rec)`)
}
