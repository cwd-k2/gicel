// Engine setup, source reading, module registration, and error formatting
// shared across the run/check/lsp subcommands.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cwd-k2/gicel"
	"github.com/cwd-k2/gicel/internal/app/header"
)

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
	"session": gicel.EffectSession,
	"console": consolePack,
}

// allPackOrder ensures deterministic pack loading.
var allPackOrder = []string{"prelude", "fail", "state", "io", "stream", "slice", "map", "set", "array", "ref", "mmap", "mset", "json", "session", "console"}

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

// maxTotalSourceSize is the aggregate limit across all source files (10 MiB).
const maxTotalSourceSize = 10 * 1024 * 1024

// sourceBudget tracks cumulative source bytes read in a single invocation.

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

// registerUserModules parses --module Name=path flags and registers each module.
func registerUserModules(eng *gicel.Engine, modules []string, budget *sourceBudget) error {
	for _, spec := range modules {
		before, after, ok := strings.Cut(spec, "=")
		if !ok {
			return fmt.Errorf("invalid module spec: %q (expected Name=path)", spec)
		}
		name := before
		if err := gicel.ValidateModuleName(name); err != nil {
			return err
		}
		path := after
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

	// CLI --module flags are registered first so that header directives
	// can detect and skip modules already provided on the command line.
	if err := registerUserModules(eng, modules, budget); err != nil {
		return nil, nil, err
	}

	// Apply file header directives when source is from a file.
	// Modules already registered by CLI flags are skipped.
	if expr == "" && fs.NArg() > 0 && fs.Arg(0) != "-" {
		if err := applyHeaderDirectives(eng, string(source), fs.Arg(0)); err != nil {
			return nil, nil, err
		}
	}
	return source, eng, nil
}

// applyHeaderDirectives recursively resolves -- gicel: directives from the
// source header and registers discovered modules. Modules already registered
// by CLI --module flags are skipped, ensuring CLI flags take precedence.
func applyHeaderDirectives(eng *gicel.Engine, source, filePath string) error {
	res, err := header.Resolve(source, filePath)
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if res.Recursion {
		eng.EnableRecursion()
	}
	for _, mod := range res.Modules {
		if eng.HasModule(mod.Name) {
			continue // CLI --module flag takes precedence
		}
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

// isUnitValue reports whether a value is the unit value ().
func isUnitValue(v gicel.Value) bool {
	rv, ok := v.(*gicel.RecordVal)
	return ok && rv.Len() == 0
}
