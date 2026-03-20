package engine

import (
	"context"
	"maps"

	"github.com/cwd-k2/gicel/internal/check"
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/opt"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/syntax/parse"
)

// lexAndParse is the shared lex/parse pipeline for both module registration
// and main-source compilation. It always injects fixity from all registered
// modules so that operator precedence is consistent regardless of entry path.
func lexAndParse(sourceName, source string, store *ModuleStore, injectCore bool) (*syntax.AstProgram, *span.Source, error) {
	src := span.NewSource(sourceName, source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return nil, nil, &CompileError{Errors: lexErrs}
	}
	parseErrs := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, parseErrs)
	store.CollectFixity(p)
	ast := p.ParseProgram()
	if parseErrs.HasErrors() {
		return nil, nil, &CompileError{Errors: parseErrs}
	}
	if injectCore {
		injectCoreImport(ast)
	}
	return ast, src, nil
}

func injectCoreImport(ast *syntax.AstProgram) {
	for _, imp := range ast.Imports {
		if imp.ModuleName == "Core" {
			return
		}
	}
	ast.Imports = append([]syntax.DeclImport{{ModuleName: "Core"}}, ast.Imports...)
}

// makeCheckConfig builds a CheckConfig from the three Engine subsystems.
func makeCheckConfig(host *HostEnv, store *ModuleStore, limits *Limits) *check.CheckConfig {
	imported := make(map[string]*check.ModuleExports, len(store.modules))
	deps := make(map[string][]string, len(store.modules))
	for name, mod := range store.modules {
		imported[name] = mod.exports
		deps[name] = mod.deps
	}
	return &check.CheckConfig{
		RegisteredTypes: maps.Clone(host.registeredTys),
		Assumptions:     maps.Clone(host.assumptions),
		Bindings:        maps.Clone(host.bindings),
		GatedBuiltins:   maps.Clone(host.gatedBuiltins),
		Trace:           limits.checkTraceHook,
		ImportedModules: imported,
		ModuleDeps:      deps,
		StrictTypeNames: true,
		NestingLimit:    limits.nestingLimit,
	}
}

// compileModule runs the full compilation pipeline for a single module:
// lex → parse → dep check → type check → optimize → annotate.
func compileModule(name, source string, host *HostEnv, store *ModuleStore, limits *Limits) (*compiledModule, error) {
	ast, src, err := lexAndParse(name, source, store, name != "Core" && store.Has("Core"))
	if err != nil {
		return nil, err
	}

	var deps []string
	for _, imp := range ast.Imports {
		deps = append(deps, imp.ModuleName)
	}
	if err := store.CheckCircularDeps(name, deps); err != nil {
		return nil, err
	}

	config := makeCheckConfig(host, store, limits)
	config.CurrentModule = name
	prog, exports, checkErrs := check.CheckModule(ast, src, config)
	if checkErrs.HasErrors() {
		return nil, &CompileError{Errors: checkErrs}
	}

	modFixity := make(map[string]parse.Fixity)
	for _, d := range ast.Decls {
		if fix, ok := d.(*syntax.DeclFixity); ok {
			modFixity[fix.Op] = parse.Fixity{Assoc: fix.Assoc, Prec: fix.Prec}
		}
	}

	opt.OptimizeProgram(prog, host.rewriteRules)
	core.AnnotateFreeVarsProgram(prog)

	return &compiledModule{
		prog:           prog,
		exports:        exports,
		deps:           deps,
		fixity:         modFixity,
		sortedBindings: core.SortBindings(prog.Bindings),
		source:         src,
	}, nil
}

// compileMain compiles the main source: lex → parse → type check → optimize → annotate.
func compileMain(ctx context.Context, source string, host *HostEnv, store *ModuleStore, limits *Limits) (*core.Program, *span.Source, error) {
	ast, src, err := lexAndParse("<input>", source, store, store.Has("Core"))
	if err != nil {
		return nil, nil, err
	}

	cfg := makeCheckConfig(host, store, limits)
	cfg.Context = ctx
	cfg.EntryPoint = limits.entryPoint
	if cfg.EntryPoint == "" {
		cfg.EntryPoint = DefaultEntryPoint
	}
	prog, checkErrs := check.Check(ast, src, cfg)
	if checkErrs.HasErrors() {
		return nil, nil, &CompileError{Errors: checkErrs}
	}

	opt.OptimizeProgram(prog, host.rewriteRules)
	core.AnnotateFreeVarsProgram(prog)

	return prog, src, nil
}

// assembleRuntime constructs an immutable Runtime from compiled artifacts.
func assembleRuntime(prog *core.Program, src *span.Source, host *HostEnv, store *ModuleStore, limits *Limits) *Runtime {
	entries := store.Entries()

	entryName := limits.entryPoint
	if entryName == "" {
		entryName = DefaultEntryPoint
	}
	sortedMain := core.SortBindings(prog.Bindings)
	var entryExpr core.Core
	for _, b := range sortedMain {
		if b.Name == entryName {
			entryExpr = b.Expr
			break
		}
	}

	rt := &Runtime{
		prog:               prog,
		prims:              host.prims.Clone(),
		stepLimit:          limits.stepLimit,
		depthLimit:         limits.depthLimit,
		nestingLimit:       limits.nestingLimit,
		allocLimit:         limits.allocLimit,
		source:             src,
		bindings:           maps.Clone(host.bindings),
		moduleEntries:      entries,
		sortedMainBindings: sortedMain,
		entryName:          entryName,
		entryExpr:          entryExpr,
	}
	runtimeGates := maps.Clone(host.gatedBuiltins)
	if store.recursion {
		runtimeGates["fix"] = true
		runtimeGates["rec"] = true
	}
	rt.initBuiltinEnv(runtimeGates)
	return rt
}
