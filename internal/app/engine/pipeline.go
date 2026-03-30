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
	ctx             context.Context
	host            *HostEnv
	store           *ModuleStore
	limits          *Limits
	traceHook       check.CheckTraceHook
	entryPoint      string
	denyAssumptions bool
}

// lexAndParse is the shared lex/parse pipeline for both module registration
// and main-source compilation. Fixity is scoped to the transitive import
// closure of the module being compiled, preventing unimported modules from
// affecting operator precedence.
func (pc *pipelineCtx) lexAndParse(sourceName, source string, injectCore bool) (*syntax.AstProgram, *span.Source, error) {
	src := span.NewSource(sourceName, source)
	scanner := parse.NewScanner(src)
	parseErrs := &diagnostic.Errors{Source: src}
	p := parse.NewParserStreaming(pc.ctx, scanner, nil, parseErrs, len(source))

	// Phase 1: Parse imports (streaming — only reads import tokens).
	imports := p.ParseImports()
	importNames := make([]string, len(imports))
	for i, imp := range imports {
		importNames[i] = imp.ModuleName
	}
	if injectCore {
		importNames = append(importNames, "Core")
	}

	// Phase 2: Inject external fixity from transitively imported modules.
	pc.store.CollectFixityForImports(p, importNames)

	// Phase 3: Parse declarations (streaming — reads remaining tokens).
	decls := p.ParseDecls()
	ast := &syntax.AstProgram{Imports: imports, Decls: decls}

	// Phase 4: Collect in-module fixity + resolve infix spines.
	mergedFixity := make(map[string]parse.Fixity)
	maps.Copy(mergedFixity, pc.store.CollectFixityMap(importNames))
	maps.Copy(mergedFixity, parse.CollectModuleFixity(decls))
	parse.ResolveFixity(ast, mergedFixity, parseErrs)

	if scanner.Errors().HasErrors() {
		return nil, nil, &CompileError{Errors: scanner.Errors()}
	}
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
// Results are cached at the process level keyed by (source hash, env fingerprint).
func (pc *pipelineCtx) compileModule(name, source string) (*compiledModule, error) {
	cacheKey := pc.computeModuleCacheKey(source)
	if cached, ok := moduleCacheGet(cacheKey); ok {
		return cached, nil
	}

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
	// Modules (stdlib) may use host-provided assumptions; don't deny.
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

	mod := &compiledModule{
		prog:           prog,
		exports:        exports,
		deps:           deps,
		fixity:         modFixity,
		sortedBindings: ir.SortBindings(prog.Bindings),
		source:         src,
	}
	moduleCachePut(cacheKey, mod)
	return mod, nil
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
	cfg.DenyAssumptions = pc.denyAssumptions
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

	rt.precompileVM(runtimeGates)
	return rt
}
