package engine

import (
	"context"
	"maps"

	"github.com/cwd-k2/gicel/internal/compiler/check"
	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/compiler/desugar"
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
// All fields are value snapshots taken at pipeline() time. There is no
// back-pointer to Engine — the pipeline is a pure function of its inputs.
type pipelineCtx struct {
	ctx            context.Context
	host           *HostEnv
	store          *ModuleStore
	compilerLimits *CompilerLimits
	runtimeLimits  *RuntimeLimits
	pipelineFlags  *PipelineFlags
	cacheStore     *CacheStore  // cache for compiled modules and runtimes
	modEnvFp       [32]byte     // pre-computed module environment fingerprint
	runtimeFp      [32]byte     // pre-computed runtime fingerprint
	warnFunc       func(string) // warning callback (nil = stderr)
	traceHook      check.CheckTraceHook
	typeRecorder   bool // when true, analyze() populates TypeIndex
}

// lexAndParse is the shared lex/parse pipeline for both module registration
// and main-source compilation. Fixity is scoped to the transitive import
// closure of the module being compiled, preventing unimported modules from
// affecting operator precedence.
func (pc *pipelineCtx) lexAndParse(sourceName, source string, injectCore bool) (*syntax.Program, *span.Source, error) {
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
		importNames = append(importNames, env.CoreModuleName)
	}
	p.AddFixity(pc.store.CollectFixityMap(importNames))
	decls := p.ParseDecls()
	ast := &syntax.Program{Imports: imports, Decls: decls}
	p.ResolveInfix(ast)
	desugar.Program(ast)

	if p.LexErrors().HasErrors() {
		return nil, src, &CompileError{errs: p.LexErrors()}
	}
	if parseErrs.HasErrors() {
		return nil, src, &CompileError{errs: parseErrs}
	}
	if injectCore {
		injectCoreImport(ast)
	}
	return ast, src, nil
}

func injectCoreImport(ast *syntax.Program) {
	for _, imp := range ast.Imports {
		if imp.ModuleName == env.CoreModuleName {
			return
		}
	}
	ast.Imports = append([]syntax.DeclImport{{ModuleName: env.CoreModuleName}}, ast.Imports...)
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
		NestingLimit:    pc.compilerLimits.nestingLimit,
		MaxTFSteps:      pc.compilerLimits.maxTFSteps,
		MaxSolverSteps:  pc.compilerLimits.maxSolverSteps,
		MaxResolveDepth: pc.compilerLimits.maxResolveDepth,
	}
}

// compileModule runs the full compilation pipeline for a single module:
// lex → parse → dep check → type check → optimize → annotate.
// Results are cached at the process level keyed by (source hash, env fingerprint).
func (pc *pipelineCtx) compileModule(name, source string) (*compiledModule, error) {
	cacheKey := pc.computeModuleCacheKey(source)
	if cached, ok := pc.cacheStore.GetModule(cacheKey); ok {
		return cached, nil
	}

	ast, src, err := pc.lexAndParse(name, source, name != env.CoreModuleName && pc.store.Has(env.CoreModuleName))
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
		return nil, &CompileError{errs: checkErrs}
	}

	modFixity := make(map[string]syntax.Fixity)
	for _, d := range ast.Decls {
		if fix, ok := d.(*syntax.DeclFixity); ok {
			modFixity[fix.Op] = syntax.Fixity{Assoc: fix.Assoc, Prec: fix.Prec}
		}
	}

	pc.postCheck(prog, nil) // module: no inlining
	annots := annotateForVM(prog, pc.pipelineFlags.verifyIR)

	mod := &compiledModule{
		prog:           prog,
		annots:         annots,
		exports:        exports,
		deps:           deps,
		fixity:         modFixity,
		sortedBindings: ir.SortBindings(prog.Bindings),
		source:         src,
	}
	pc.cacheStore.PutModule(cacheKey, mod)
	return mod, nil
}

// postCheck applies the backend-agnostic post-type-checking pipeline:
// label erasure → [verify structure] → optimize.
// userBindings limits selective inlining to the given names (nil = no inlining).
//
// After postCheck, the ir.Program is ready for backend-specific lowering.
// The VM backend calls annotateForVM to add FV analysis and de Bruijn indices.
func (pc *pipelineCtx) postCheck(prog *ir.Program, userBindings map[string]bool) {
	ir.EraseLabelArgsProgram(prog)
	if pc.pipelineFlags.verifyIR {
		if errs := ir.VerifyProgram(prog); len(errs) > 0 {
			panic("IR verification failed: " + errs[0].Error())
		}
	}
	externalInline := pc.collectExternalInlineBindings()
	externalDicts := pc.collectExternalDictionaries()
	optimize.OptimizeProgram(pc.ctx, prog, pc.host.rewriteRules, userBindings, externalInline, externalDicts)
}

// annotateForVM runs VM-backend-specific preparation: free-variable analysis
// and de Bruijn index assignment. These passes populate the FVAnnotations
// side table and Var.Index fields consumed by the bytecode compiler.
//
// CONTRACT: AssignIndices requires AnnotateFreeVars (populates FVAnnotations);
// the VM compiler requires AssignIndices (populates Var.Index). Calling these
// out of order panics via LookupLam/LookupThunk/LookupMerge.
func annotateForVM(prog *ir.Program, verifyIR bool) *ir.FVAnnotations {
	annots := ir.AnnotateFreeVarsProgram(prog)
	ir.AssignIndicesProgram(prog, annots)
	if verifyIR {
		if errs := ir.VerifyAnnotations(prog, annots); len(errs) > 0 {
			panic("IR annotation verification failed: " + errs[0].Error())
		}
	}
	return annots
}

// compileMain compiles the main source: lex → parse → type check → optimize → annotate.
// Returns the Program, its FV annotations side table, and the source map.
func (pc *pipelineCtx) compileMain(source string) (*ir.Program, *ir.FVAnnotations, *span.Source, error) {
	ar := pc.analyze(source)
	if !ar.Complete {
		return nil, nil, nil, &CompileError{errs: ar.Errors}
	}

	var userBindings map[string]bool
	if !pc.pipelineFlags.noInline {
		userBindings = collectUserBindings(ar.Program)
	}
	pc.postCheck(ar.Program, userBindings)
	annots := annotateForVM(ar.Program, pc.pipelineFlags.verifyIR)

	return ar.Program, annots, ar.Source, nil
}

// assembleRuntime constructs an immutable Runtime from compiled artifacts.
// Returns a CompileError if precompileVM detects a structural compile-time
// limit (e.g. bytecode pool overflow); other panics from the bytecode
// compiler propagate as real bugs.
func (pc *pipelineCtx) assembleRuntime(prog *ir.Program, annots *ir.FVAnnotations, src *span.Source) (*Runtime, error) {
	entries := pc.store.Entries()

	entryName := pc.pipelineFlags.effectiveEntryPoint()
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
		stepLimit:          pc.runtimeLimits.stepLimit,
		depthLimit:         pc.runtimeLimits.depthLimit,
		nestingLimit:       pc.compilerLimits.nestingLimit,
		allocLimit:         pc.runtimeLimits.allocLimit,
		source:             src,
		warnFunc:           pc.warnFunc,
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

// ComputeFormKind builds the kind of a data declaration from its type
// parameters. E.g., Maybe with [a :: Type] → Type -> Type.
func ComputeFormKind(dd *ir.DataDecl) types.Type {
	var kind types.Type = types.TypeOfTypes
	for i := len(dd.TyParams) - 1; i >= 0; i-- {
		kind = types.MkArrow(dd.TyParams[i].Kind, kind)
	}
	return kind
}

// BuildConType returns the full type of a constructor. If the checker
// populated ConDecl.FullType (which includes GADT/existential foralls),
// it is used directly. Otherwise falls back to reconstruction from
// data-type-level parameters.
func BuildConType(dd *ir.DataDecl, con *ir.ConDecl) types.Type {
	if con.FullType != nil {
		return con.FullType
	}
	// Fallback: reconstruct from data type params + fields.
	var ret types.Type = &types.TyCon{Name: dd.Name}
	for _, p := range dd.TyParams {
		ret = &types.TyApp{Fun: ret, Arg: &types.TyVar{Name: p.Name}}
	}
	if con.IsGADT() {
		ret = con.ReturnType
	}
	ty := ret
	for i := len(con.Fields) - 1; i >= 0; i-- {
		ty = types.MkArrow(con.Fields[i], ty)
	}
	for i := len(dd.TyParams) - 1; i >= 0; i-- {
		ty = types.MkForall(dd.TyParams[i].Name, dd.TyParams[i].Kind, ty)
	}
	return ty
}
