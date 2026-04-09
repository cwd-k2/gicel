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
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// pipelineCtx encapsulates the compile-time environment shared across
// pipeline stages: lex → parse → check → optimize → assemble.
//
// The engine back-pointer gives pipeline stages access to Engine-level
// caches (fingerprints, etc.). host/store/limits remain as explicit
// fields for direct access inside hot paths.
type pipelineCtx struct {
	engine          *Engine
	ctx             context.Context
	host            *HostEnv
	store           *ModuleStore
	limits          *Limits
	traceHook       check.CheckTraceHook
	entryPoint      string
	denyAssumptions bool
	noInline        bool
	verifyIR        bool // when true, run structural IR verification after label erasure
	typeRecorder    bool // when true, analyze() populates TypeIndex
}

// lexAndParse is the shared lex/parse pipeline for both module registration
// and main-source compilation. Fixity is scoped to the transitive import
// closure of the module being compiled, preventing unimported modules from
// affecting operator precedence.
func (pc *pipelineCtx) lexAndParse(sourceName, source string, injectCore bool) (*syntax.AstProgram, *span.Source, error) {
	src := span.NewSource(sourceName, source)
	parseErrs := &diagnostic.Errors{Source: src}
	p := parse.NewParser(pc.ctx, src, parseErrs)

	// Stream imports → inject external fixity → parse rest → resolve.
	imports := p.ParseImports()
	importNames := make([]string, len(imports))
	for i, imp := range imports {
		importNames[i] = imp.ModuleName
	}
	if injectCore {
		importNames = append(importNames, "Core")
	}
	p.AddFixity(pc.store.CollectFixityMap(importNames))
	decls := p.ParseDecls()
	ast := &syntax.AstProgram{Imports: imports, Decls: decls}
	p.ResolveInfix(ast)

	if p.LexErrors().HasErrors() {
		return nil, nil, &CompileError{Errors: p.LexErrors()}
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

	annots := pc.postCheck(prog, nil) // module: no inlining

	mod := &compiledModule{
		prog:           prog,
		annots:         annots,
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
// label erasure → [verify structure] → optimize → annotate FV → assign indices → [verify annotations].
// userBindings limits selective inlining to the given names (nil = no inlining).
// Returns the freshly computed FVAnnotations so callers can store them
// alongside the Program they own — the ir layer keeps no hidden state.
func (pc *pipelineCtx) postCheck(prog *ir.Program, userBindings map[string]bool) *ir.FVAnnotations {
	ir.EraseLabelArgsProgram(prog)
	if pc.verifyIR {
		if errs := ir.VerifyProgram(prog); len(errs) > 0 {
			panic("IR verification failed: " + errs[0].Error())
		}
	}
	externalInline := pc.collectExternalInlineBindings()
	optimize.OptimizeProgram(prog, pc.host.rewriteRules, userBindings, externalInline)
	annots := ir.AnnotateFreeVarsProgram(prog)
	ir.AssignIndicesProgram(prog, annots)
	if pc.verifyIR {
		if errs := ir.VerifyAnnotations(prog, annots); len(errs) > 0 {
			panic("IR annotation verification failed: " + errs[0].Error())
		}
	}
	return annots
}

// analyze runs lex → parse → check, returning partial results on error.
// Unlike compileMain, it does NOT run postCheck (optimize, annotate).
func (pc *pipelineCtx) analyze(source string) *AnalysisResult {
	result := &AnalysisResult{}

	ast, src, err := pc.lexAndParse("<input>", source, pc.store.Has("Core"))
	result.Source = src
	if err != nil {
		if ce, ok := err.(*CompileError); ok {
			result.Errors = ce.Errors
		}
		result.Program = &ir.Program{}
		return result
	}

	cfg := pc.makeCheckConfig()
	cfg.Context = pc.ctx
	cfg.EntryPoint = pc.entryPoint
	cfg.DenyAssumptions = pc.denyAssumptions

	var idx *TypeIndex
	if pc.typeRecorder {
		idx = NewTypeIndex()
		cfg.TypeRecorder = func(sp span.Span, ty types.Type) {
			idx.Record(sp, ty)
		}
	}

	prog, checkErrs := check.Check(ast, src, cfg)
	if idx != nil {
		// Record definition-site types from top-level bindings so
		// hover works on binding names, not just expression bodies.
		for _, b := range prog.Bindings {
			if b.Type != nil && b.S != (span.Span{}) {
				idx.Record(b.S, b.Type)
			}
		}
		idx.Finalize()
	}

	result.Program = prog
	result.Errors = checkErrs
	result.Complete = !checkErrs.HasErrors()
	result.TypeIndex = idx
	return result
}

// compileMain compiles the main source: lex → parse → type check → optimize → annotate.
// Returns the Program, its FV annotations side table, and the source map.
func (pc *pipelineCtx) compileMain(source string) (*ir.Program, *ir.FVAnnotations, *span.Source, error) {
	ar := pc.analyze(source)
	if !ar.Complete {
		return nil, nil, nil, &CompileError{Errors: ar.Errors}
	}

	var userBindings map[string]bool
	if !pc.noInline {
		userBindings = collectUserBindings(ar.Program)
	}
	annots := pc.postCheck(ar.Program, userBindings)

	return ar.Program, annots, ar.Source, nil
}

// collectUserBindings returns the set of non-generated binding names
// eligible for selective inlining.
func collectUserBindings(prog *ir.Program) map[string]bool {
	m := make(map[string]bool)
	for _, b := range prog.Bindings {
		if !b.Generated {
			m[b.Name] = true
		}
	}
	return m
}

// transparentInlineWhitelist names the Prelude bindings whose bodies
// are pure syntactic wrappers with no semantic content beyond their
// application shape. Inlining these lets the optimizer reduce every
// call site to the corresponding IR primitive (`$`/`&` to a direct
// `App`, `fix`/`rec` to `ir.Fix`, `force` to `ir.Force`, `pure`/`bind`
// to their respective Core nodes via betaReduce/bindPureElim) so that
// user code written against the first-class values compiles to the
// same bytecode as the syntactic special forms.
//
// The list is intentionally small and closed: arbitrary module bindings
// are NOT inlined across module boundaries because that would destroy
// source-attribution invariants that explain/diagnostic code relies on.
// Wider inlining is a separate design trade-off and is out of scope
// for the CBPV coercion work.
var transparentInlineWhitelist = map[string]bool{
	"$":     true,
	"&":     true,
	"fix":   true,
	"rec":   true,
	"force": true,
	"pure":  true,
	"bind":  true,
}

// collectExternalInlineBindings gathers the whitelisted transparent
// wrappers from imported modules so the optimizer can reduce their
// applied forms at call sites in the main program. The inliner applies
// its own size / non-recursive / lambda-body filters as a secondary
// guard, but the whitelist is the primary mechanism that keeps the
// scope of cross-module inlining narrow and predictable.
//
// Each ExternalBinding is keyed by (moduleName, bindingName) so the
// inliner's VarKey lookup matches qualified references emitted by the
// checker for imported identifiers.
func (pc *pipelineCtx) collectExternalInlineBindings() []optimize.ExternalBinding {
	if pc.noInline {
		return nil
	}
	entries := pc.store.Entries()
	if len(entries) == 0 {
		return nil
	}
	var out []optimize.ExternalBinding
	for _, e := range entries {
		if e.prog == nil {
			continue
		}
		for _, b := range e.prog.Bindings {
			if b.Generated {
				continue
			}
			if !transparentInlineWhitelist[b.Name] {
				continue
			}
			out = append(out, optimize.ExternalBinding{
				Module: e.name,
				Name:   b.Name,
				Expr:   b.Expr,
			})
		}
	}
	return out
}

// assembleRuntime constructs an immutable Runtime from compiled artifacts.
// Returns a CompileError if precompileVM detects a structural compile-time
// limit (e.g. bytecode pool overflow); other panics from the bytecode
// compiler propagate as real bugs.
func (pc *pipelineCtx) assembleRuntime(prog *ir.Program, annots *ir.FVAnnotations, src *span.Source) (*Runtime, error) {
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
		annots:             annots,
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

	if err := rt.precompileVM(runtimeGates); err != nil {
		return nil, err
	}
	return rt, nil
}
