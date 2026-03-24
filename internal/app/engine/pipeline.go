package engine

import (
	"context"
	"maps"

	"github.com/cwd-k2/gicel/internal/compiler/check"
	"github.com/cwd-k2/gicel/internal/compiler/optimize"
	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
)

// lexAndParse is the shared lex/parse pipeline for both module registration
// and main-source compilation. It always injects fixity from all registered
// modules so that operator precedence is consistent regardless of entry path.
func lexAndParse(ctx context.Context, sourceName, source string, store *ModuleStore, injectCore bool) (*syntax.AstProgram, *span.Source, error) {
	src := span.NewSource(sourceName, source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return nil, nil, &CompileError{Errors: lexErrs}
	}
	parseErrs := &diagnostic.Errors{Source: src}
	p := parse.NewParser(ctx, tokens, parseErrs)
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
func makeCheckConfig(host *HostEnv, store *ModuleStore, limits *Limits, traceHook check.CheckTraceHook) *check.CheckConfig {
	imported := make(map[string]*check.ModuleExports, len(store.modules))
	deps := make(map[string][]string, len(store.modules))
	for name, mod := range store.modules {
		imported[name] = mod.exports
		deps[name] = mod.deps
	}
	return &check.CheckConfig{
		RegisteredTypes: host.registeredTys,
		Assumptions:     host.assumptions,
		Bindings:        host.bindings,
		GatedBuiltins:   host.gatedBuiltins,
		Trace:           traceHook,
		ImportedModules: imported,
		ModuleDeps:      deps,
		StrictTypeNames: true,
		NestingLimit:    limits.nestingLimit,
		MaxTFSteps:      limits.maxTFSteps,
		MaxSolverSteps:  limits.maxSolverSteps,
		MaxResolveDepth: limits.maxResolveDepth,
	}
}

// compileModule runs the full compilation pipeline for a single module:
// lex → parse → dep check → type check → optimize → annotate.
// See compileMain for the main-source counterpart.
func compileModule(ctx context.Context, name, source string, host *HostEnv, store *ModuleStore, limits *Limits, traceHook check.CheckTraceHook) (*compiledModule, error) {
	ast, src, err := lexAndParse(ctx, name, source, store, name != "Core" && store.Has("Core"))
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

	config := makeCheckConfig(host, store, limits, traceHook)
	config.Context = ctx
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

	postCheck(prog, host.rewriteRules)

	return &compiledModule{
		prog:           prog,
		exports:        exports,
		deps:           deps,
		fixity:         modFixity,
		sortedBindings: ir.SortBindings(prog.Bindings),
		source:         src,
	}, nil
}

// postCheck applies the shared post-type-checking pipeline: optimize and annotate free vars.
func postCheck(prog *ir.Program, rules []registry.RewriteRule) {
	optimize.OptimizeProgram(prog, rules)
	ir.AnnotateFreeVarsProgram(prog)
}

// compileMain compiles the main source: lex → parse → type check → optimize → annotate.
// See compileModule for the module counterpart.
func compileMain(ctx context.Context, source string, host *HostEnv, store *ModuleStore, limits *Limits, traceHook check.CheckTraceHook, entryPoint string) (*ir.Program, *span.Source, error) {
	ast, src, err := lexAndParse(ctx, "<input>", source, store, store.Has("Core"))
	if err != nil {
		return nil, nil, err
	}

	cfg := makeCheckConfig(host, store, limits, traceHook)
	cfg.Context = ctx
	cfg.EntryPoint = entryPoint
	if cfg.EntryPoint == "" {
		cfg.EntryPoint = DefaultEntryPoint
	}
	prog, checkErrs := check.Check(ast, src, cfg)
	if checkErrs.HasErrors() {
		return nil, nil, &CompileError{Errors: checkErrs}
	}

	postCheck(prog, host.rewriteRules)

	return prog, src, nil
}

// assembleRuntime constructs an immutable Runtime from compiled artifacts.
func assembleRuntime(prog *ir.Program, src *span.Source, host *HostEnv, store *ModuleStore, limits *Limits, entryPoint string) *Runtime {
	entries := store.Entries()

	entryName := entryPoint
	if entryName == "" {
		entryName = DefaultEntryPoint
	}
	sortedMain := ir.SortBindings(prog.Bindings)
	var entryExpr ir.Core
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
