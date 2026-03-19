package engine

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/cwd-k2/gicel/internal/check"
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/opt"
	"github.com/cwd-k2/gicel/internal/reg"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/stdlib"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/syntax/parse"
	"github.com/cwd-k2/gicel/internal/types"
)

// DefaultEntryPoint is the default name of the top-level binding that serves
// as the program's entry point when no explicit name is provided.
const DefaultEntryPoint = "main"

// Engine configures and compiles GICEL programs.
// It is mutable and must not be shared across goroutines.
type Engine struct {
	bindings         map[string]types.Type
	assumptions      map[string]types.Type
	registeredTys    map[string]types.Kind
	prims            *eval.PrimRegistry
	gatedBuiltins    map[string]bool
	stepLimit        int
	depthLimit       int
	nestingLimit     int
	allocLimit       int64
	checkTraceHook   check.CheckTraceHook
	modules          map[string]*compiledModule
	moduleOrder      []string // insertion order for deterministic iteration
	runtimeRecursion bool     // set by RegisterModuleRec; ensures fix/rec in eval env
	rewriteRules     []reg.RewriteRule
	entryPoint       string // entry point name for bare Computation check (default DefaultEntryPoint)
}

type compiledModule struct {
	prog           *core.Program
	exports        *check.ModuleExports
	deps           []string
	fixity         map[string]parse.Fixity
	sortedBindings []core.Binding // pre-sorted for evalBindingsCore
}

// NewEngine creates a new Engine with default limits.
func NewEngine() *Engine {
	e := &Engine{
		bindings:      make(map[string]types.Type),
		assumptions:   make(map[string]types.Type),
		registeredTys: make(map[string]types.Kind),
		prims:         eval.NewPrimRegistry(),
		gatedBuiltins: make(map[string]bool),
		stepLimit:     1_000_000,
		depthLimit:    1_000,
		modules:       make(map[string]*compiledModule),
	}
	// Built-in literal types.
	e.registeredTys["Int"] = types.KType{}
	e.registeredTys["Double"] = types.KType{}
	e.registeredTys["String"] = types.KType{}
	e.registeredTys["Rune"] = types.KType{}
	e.registeredTys["Slice"] = &types.KArrow{From: types.KType{}, To: types.KType{}}
	e.registeredTys["Map"] = &types.KArrow{From: types.KType{}, To: &types.KArrow{From: types.KType{}, To: types.KType{}}}
	e.registeredTys["Set"] = &types.KArrow{From: types.KType{}, To: types.KType{}}
	// Core is always registered — provides IxMonad, Computation primitives.
	if err := e.RegisterModule("Core", stdlib.CoreSource); err != nil {
		panic("internal: core module: " + err.Error())
	}
	return e
}

// Use applies a Pack to the Engine.
func (e *Engine) Use(p reg.Pack) error {
	return p(e)
}

// DeclareBinding registers a host-provided value binding at compile time.
func (e *Engine) DeclareBinding(name string, ty types.Type) {
	e.bindings[name] = ty
}

// DeclareAssumption registers a primitive operation type.
func (e *Engine) DeclareAssumption(name string, ty types.Type) {
	e.assumptions[name] = ty
}

// RegisterType registers an opaque host type with the given kind.
func (e *Engine) RegisterType(name string, kind types.Kind) {
	e.registeredTys[name] = kind
}

// RegisterPrim registers a primitive implementation for an assumption.
func (e *Engine) RegisterPrim(name string, impl eval.PrimImpl) {
	e.prims.Register(name, impl)
}

// EnableRecursion enables the rec and fix built-in identifiers for all
// subsequent compilations on this engine.
func (e *Engine) EnableRecursion() {
	e.gatedBuiltins["rec"] = true
	e.gatedBuiltins["fix"] = true
}

// RegisterRewriteRule adds a fusion rule to the optimization pipeline.
func (e *Engine) RegisterRewriteRule(rule reg.RewriteRule) {
	e.rewriteRules = append(e.rewriteRules, rule)
}

// RegisterModuleRec compiles a module with fix/rec enabled, scoped to
// this single compilation.
func (e *Engine) RegisterModuleRec(name, source string) error {
	saved := maps.Clone(e.gatedBuiltins)
	e.gatedBuiltins["rec"] = true
	e.gatedBuiltins["fix"] = true
	err := e.RegisterModule(name, source)
	e.gatedBuiltins = saved
	if err == nil {
		e.runtimeRecursion = true
	}
	return err
}

// SetStepLimit sets the maximum number of evaluation steps.
func (e *Engine) SetStepLimit(n int) {
	e.stepLimit = n
}

// SetDepthLimit sets the maximum call depth.
func (e *Engine) SetDepthLimit(n int) {
	e.depthLimit = n
}

// SetNestingLimit sets the maximum structural nesting depth.
func (e *Engine) SetNestingLimit(n int) {
	e.nestingLimit = n
}

// SetAllocLimit sets the maximum cumulative allocation in bytes.
func (e *Engine) SetAllocLimit(bytes int64) {
	e.allocLimit = bytes
}

// SetCheckTraceHook sets the type checking trace hook.
func (e *Engine) SetCheckTraceHook(hook check.CheckTraceHook) {
	e.checkTraceHook = hook
}

// SetEntryPoint sets the entry point name for bare Computation checking.
// Non-entry top-level bindings with bare Computation type are rejected.
func (e *Engine) SetEntryPoint(name string) {
	e.entryPoint = name
}

// RegisterModuleFile reads a .gicel file and registers it as a module.
func (e *Engine) RegisterModuleFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read module file: %w", err)
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return e.RegisterModule(name, string(data))
}

// RegisterModule compiles a module and makes it available for import.
func (e *Engine) RegisterModule(name, source string) error {
	if name == "" {
		return fmt.Errorf("module name must not be empty")
	}
	if strings.ContainsAny(name, "\x00/\\") {
		return fmt.Errorf("module name contains invalid character: %q", name)
	}
	if _, exists := e.modules[name]; exists {
		return fmt.Errorf("module %s already registered", name)
	}

	ast, src, err := e.lexAndParse(name, source, name != "Core" && e.modules["Core"] != nil)
	if err != nil {
		return err
	}

	var deps []string
	for _, imp := range ast.Imports {
		deps = append(deps, imp.ModuleName)
	}

	if err := e.checkCircularDeps(name, deps); err != nil {
		return err
	}

	config := e.makeCheckConfig()
	config.CurrentModule = name
	prog, exports, checkErrs := check.CheckModule(ast, src, config)
	if checkErrs.HasErrors() {
		return &CompileError{Errors: checkErrs}
	}

	modFixity := make(map[string]parse.Fixity)
	for _, d := range ast.Decls {
		if fix, ok := d.(*syntax.DeclFixity); ok {
			modFixity[fix.Op] = parse.Fixity{Assoc: fix.Assoc, Prec: fix.Prec}
		}
	}

	opt.OptimizeProgram(prog, e.rewriteRules)
	core.AnnotateFreeVarsProgram(prog)

	e.modules[name] = &compiledModule{
		prog:           prog,
		exports:        exports,
		deps:           deps,
		fixity:         modFixity,
		sortedBindings: core.SortBindings(prog.Bindings),
	}
	e.moduleOrder = append(e.moduleOrder, name)
	return nil
}

func (e *Engine) checkCircularDeps(name string, deps []string) error {
	visited := map[string]bool{name: true}
	var walk func(modName string) error
	walk = func(modName string) error {
		if visited[modName] {
			return fmt.Errorf("circular module dependency involving %s", modName)
		}
		visited[modName] = true
		if mod, ok := e.modules[modName]; ok {
			for _, dep := range mod.deps {
				if err := walk(dep); err != nil {
					return err
				}
			}
		}
		visited[modName] = false
		return nil
	}
	for _, dep := range deps {
		if err := walk(dep); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) makeCheckConfig() *check.CheckConfig {
	imported := make(map[string]*check.ModuleExports, len(e.modules))
	deps := make(map[string][]string, len(e.modules))
	for name, mod := range e.modules {
		imported[name] = mod.exports
		deps[name] = mod.deps
	}
	return &check.CheckConfig{
		RegisteredTypes: maps.Clone(e.registeredTys),
		Assumptions:     maps.Clone(e.assumptions),
		Bindings:        maps.Clone(e.bindings),
		GatedBuiltins:   maps.Clone(e.gatedBuiltins),
		Trace:           e.checkTraceHook,
		ImportedModules: imported,
		ModuleDeps:      deps,
		StrictTypeNames: true,
		NestingLimit:    e.nestingLimit,
	}
}

// lexAndParse is the shared lex/parse pipeline for both module registration
// and main-source compilation. It always injects fixity from all registered
// modules so that operator precedence is consistent regardless of entry path.
func (e *Engine) lexAndParse(sourceName, source string, injectCore bool) (*syntax.AstProgram, *span.Source, error) {
	src := span.NewSource(sourceName, source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return nil, nil, &CompileError{Errors: lexErrs}
	}
	parseErrs := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, parseErrs)
	for _, name := range e.moduleOrder {
		p.AddFixity(e.modules[name].fixity)
	}
	ast := p.ParseProgram()
	if parseErrs.HasErrors() {
		return nil, nil, &CompileError{Errors: parseErrs}
	}
	if injectCore {
		injectCoreImport(ast)
	}
	return ast, src, nil
}

// parseSource lexes and parses main-source input.
func (e *Engine) parseSource(source string) (*syntax.AstProgram, *span.Source, error) {
	return e.lexAndParse("<input>", source, e.modules["Core"] != nil)
}

func injectCoreImport(ast *syntax.AstProgram) {
	for _, imp := range ast.Imports {
		if imp.ModuleName == "Core" {
			return
		}
	}
	ast.Imports = append([]syntax.DeclImport{{ModuleName: "Core"}}, ast.Imports...)
}

// CoreProgram is an opaque compiled Core IR for inspection.
type CoreProgram struct{ prog *core.Program }

// Pretty returns a human-readable representation of the Core IR.
func (c *CoreProgram) Pretty() string { return core.PrettyProgram(c.prog) }

// CompileResult holds all static information produced by compilation.
type CompileResult struct {
	prog   *core.Program
	values map[string]types.Type
}

// Pretty returns the Core IR as a human-readable string.
func (cr *CompileResult) Pretty() string { return core.PrettyProgram(cr.prog) }

// BindingNames returns the names of all top-level bindings.
func (cr *CompileResult) BindingNames() []string {
	names := make([]string, len(cr.prog.Bindings))
	for i, b := range cr.prog.Bindings {
		names[i] = b.Name
	}
	return names
}

// BindingTypes returns a map of binding names to their pretty-printed types.
func (cr *CompileResult) BindingTypes() map[string]string {
	m := make(map[string]string, len(cr.values))
	for name, ty := range cr.values {
		m[name] = types.Pretty(ty)
	}
	return m
}

// CoreProgram returns the compiled Core IR for inspection.
func (cr *CompileResult) CoreProgram() *CoreProgram {
	return &CoreProgram{prog: cr.prog}
}

// Parse lexes and parses source code, checking for syntax errors only.
// Does not type-check or optimize. Use Compile for static analysis
// or NewRuntime for execution.
func (e *Engine) Parse(source string) error {
	_, _, err := e.parseSource(source)
	return err
}

// Compile type-checks source code, returning exports and Core IR for
// static inspection. Unlike NewRuntime, it does not optimize or assemble
// a runtime. Pass context.Background() when cancellation is not needed.
func (e *Engine) Compile(ctx context.Context, source string) (*CompileResult, error) {
	ast, src, err := e.parseSource(source)
	if err != nil {
		return nil, err
	}
	cfg := e.makeCheckConfig()
	cfg.Context = ctx
	cfg.EntryPoint = e.entryPoint
	if cfg.EntryPoint == "" {
		cfg.EntryPoint = DefaultEntryPoint
	}
	prog, exports, checkErrs := check.CheckModule(ast, src, cfg)
	if checkErrs.HasErrors() {
		return nil, &CompileError{Errors: checkErrs}
	}
	return &CompileResult{prog: prog, values: exports.Values}, nil
}

// NewRuntime compiles source code into an immutable, goroutine-safe Runtime.
// The context bounds compilation time (type checking in particular);
// pass context.Background() when cancellation is not needed.
func (e *Engine) NewRuntime(ctx context.Context, source string) (*Runtime, error) {
	ast, src, err := e.parseSource(source)
	if err != nil {
		return nil, err
	}

	cfg := e.makeCheckConfig()
	cfg.Context = ctx
	cfg.EntryPoint = e.entryPoint
	if cfg.EntryPoint == "" {
		cfg.EntryPoint = DefaultEntryPoint
	}
	prog, checkErrs := check.Check(ast, src, cfg)
	if checkErrs.HasErrors() {
		return nil, &CompileError{Errors: checkErrs}
	}

	opt.OptimizeProgram(prog, e.rewriteRules)
	core.AnnotateFreeVarsProgram(prog)

	entries := make([]moduleEntry, 0, len(e.moduleOrder))
	for _, name := range e.moduleOrder {
		mod := e.modules[name]
		entries = append(entries, moduleEntry{
			name:           name,
			prog:           mod.prog,
			sortedBindings: mod.sortedBindings,
		})
	}

	entryName := cfg.EntryPoint
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
		prims:              e.prims.Clone(),
		stepLimit:          e.stepLimit,
		depthLimit:         e.depthLimit,
		nestingLimit:       e.nestingLimit,
		allocLimit:         e.allocLimit,
		source:             src,
		bindings:           maps.Clone(e.bindings),
		moduleEntries:      entries,
		sortedMainBindings: sortedMain,
		entryName:          entryName,
		entryExpr:          entryExpr,
	}
	runtimeGates := maps.Clone(e.gatedBuiltins)
	if e.runtimeRecursion {
		runtimeGates["fix"] = true
		runtimeGates["rec"] = true
	}
	rt.initBuiltinEnv(runtimeGates)
	return rt, nil
}
