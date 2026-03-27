package engine

import (
	"context"
	"maps"

	"github.com/cwd-k2/gicel/internal/compiler/check"
	"github.com/cwd-k2/gicel/internal/compiler/optimize"
	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
)

// pipelineCtx encapsulates the compile-time environment shared across
// pipeline stages: lex → parse → check → optimize → assemble.
type pipelineCtx struct {
	ctx        context.Context
	host       *HostEnv
	store      *ModuleStore
	limits     *Limits
	traceHook  check.CheckTraceHook
	entryPoint string
}

// lexAndParse is the shared lex/parse pipeline for both module registration
// and main-source compilation. It always injects fixity from all registered
// modules so that operator precedence is consistent regardless of entry path.
func (pc *pipelineCtx) lexAndParse(sourceName, source string, injectCore bool) (*syntax.AstProgram, *span.Source, error) {
	src := span.NewSource(sourceName, source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return nil, nil, &CompileError{Errors: lexErrs}
	}
	parseErrs := &diagnostic.Errors{Source: src}
	p := parse.NewParser(pc.ctx, tokens, parseErrs)
	pc.store.CollectFixity(p)
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

// makeCheckConfig builds a CheckConfig from the pipeline context.
func (pc *pipelineCtx) makeCheckConfig() *check.CheckConfig {
	imported := make(map[string]*check.ModuleExports, len(pc.store.modules))
	deps := make(map[string][]string, len(pc.store.modules))
	for name, mod := range pc.store.modules {
		imported[name] = mod.exports
		deps[name] = mod.deps
	}
	return &check.CheckConfig{
		RegisteredTypes: pc.host.registeredTys,
		Assumptions:     pc.host.assumptions,
		Bindings:        pc.host.bindings,
		GatedBuiltins:   pc.host.gatedBuiltins,
		Trace:           pc.traceHook,
		ImportedModules: imported,
		ModuleDeps:      deps,
		StrictTypeNames: true,
		NestingLimit:    pc.limits.nestingLimit,
		MaxTFSteps:      pc.limits.maxTFSteps,
		MaxSolverSteps:  pc.limits.maxSolverSteps,
		MaxResolveDepth: pc.limits.maxResolveDepth,
	}
}

// compileModule runs the full compilation pipeline for a single module:
// lex → parse → dep check → type check → optimize → annotate.
func (pc *pipelineCtx) compileModule(name, source string) (*compiledModule, error) {
	ast, src, err := pc.lexAndParse(name, source, name != "Core" && pc.store.Has("Core"))
	if err != nil {
		return nil, err
	}

	var deps []string
	for _, imp := range ast.Imports {
		deps = append(deps, imp.ModuleName)
	}
	if err := pc.store.CheckCircularDeps(name, deps); err != nil {
		return nil, err
	}

	config := pc.makeCheckConfig()
	config.Context = pc.ctx
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

	pc.postCheck(prog)

	return &compiledModule{
		prog:           prog,
		exports:        exports,
		deps:           deps,
		fixity:         modFixity,
		sortedBindings: ir.SortBindings(prog.Bindings),
		source:         src,
	}, nil
}

// postCheck applies the shared post-type-checking pipeline:
// label erasure → optimize → annotate free vars → assign de Bruijn indices.
func (pc *pipelineCtx) postCheck(prog *ir.Program) {
	ir.EraseLabelArgsProgram(prog)
	optimize.OptimizeProgram(prog, pc.host.rewriteRules)
	ir.AnnotateFreeVarsProgram(prog)
	ir.AssignIndicesProgram(prog)
}

// compileMain compiles the main source: lex → parse → type check → optimize → annotate.
func (pc *pipelineCtx) compileMain(source string) (*ir.Program, *span.Source, error) {
	ast, src, err := pc.lexAndParse("<input>", source, pc.store.Has("Core"))
	if err != nil {
		return nil, nil, err
	}

	cfg := pc.makeCheckConfig()
	cfg.Context = pc.ctx
	cfg.EntryPoint = pc.entryPoint
	prog, checkErrs := check.Check(ast, src, cfg)
	if checkErrs.HasErrors() {
		return nil, nil, &CompileError{Errors: checkErrs}
	}

	pc.postCheck(prog)

	return prog, src, nil
}

// assembleRuntime constructs an immutable Runtime from compiled artifacts.
func (pc *pipelineCtx) assembleRuntime(prog *ir.Program, src *span.Source) *Runtime {
	entries := pc.store.Entries()

	entryName := pc.entryPoint
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
		prims:              pc.host.prims.Clone(),
		stepLimit:          pc.limits.stepLimit,
		depthLimit:         pc.limits.depthLimit,
		nestingLimit:       pc.limits.nestingLimit,
		allocLimit:         pc.limits.allocLimit,
		source:             src,
		bindings:           maps.Clone(pc.host.bindings),
		moduleEntries:      entries,
		sortedMainBindings: sortedMain,
		entryName:          entryName,
		entryExpr:          entryExpr,
	}
	runtimeGates := maps.Clone(pc.host.gatedBuiltins)
	if pc.store.recursion {
		runtimeGates["fix"] = true
		runtimeGates["rec"] = true
	}
	rt.initBuiltinGlobals(runtimeGates)
	rt.buildGlobalSlots()
	return rt
}
