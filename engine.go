package gomputation

import (
	"fmt"
	"maps"

	"github.com/cwd-k2/gomputation/internal/check"
	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/eval"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/stdlib"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/internal/types"
)

// Engine configures and compiles Gomputation programs.
// It is mutable and must not be shared across goroutines.
type Engine struct {
	bindings       map[string]types.Type
	assumptions    map[string]types.Type
	registeredTys  map[string]types.Kind
	prims          *eval.PrimRegistry
	gatedBuiltins  map[string]bool
	stepLimit      int
	depthLimit     int
	noPrelude      bool
	traceHook      eval.TraceHook
	checkTraceHook check.CheckTraceHook
	modules        map[string]*compiledModule
}

type compiledModule struct {
	prog    *core.Program
	exports *check.ModuleExports
	deps    []string
	fixity  map[string]syntax.Fixity
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
	e.registeredTys["String"] = types.KType{}
	e.registeredTys["Rune"] = types.KType{}
	return e
}

// Pack configures an Engine with a coherent set of types, primitives, and modules.
type Pack func(e *Engine) error

// Use applies a Pack to the Engine.
func (e *Engine) Use(p Pack) error {
	return p(e)
}

// DeclareBinding registers a host-provided value binding at compile time.
// The name becomes available in Gomputation source as a variable of the given type.
// The actual value must be provided at runtime via RunContext.
func (e *Engine) DeclareBinding(name string, ty types.Type) {
	e.bindings[name] = ty
}

// DeclareAssumption registers a primitive operation type.
// The source code must declare `name := assumption`.
func (e *Engine) DeclareAssumption(name string, ty types.Type) {
	e.assumptions[name] = ty
}

// RegisterType registers an opaque host type with the given kind.
func (e *Engine) RegisterType(name string, kind types.Kind) {
	e.registeredTys[name] = kind
}

// RegisterPrim registers a primitive implementation for an assumption.
func (e *Engine) RegisterPrim(name string, impl PrimImpl) {
	e.prims.Register(name, impl)
}

// EnableRecursion enables the rec and fix built-in identifiers.
func (e *Engine) EnableRecursion() {
	e.gatedBuiltins["rec"] = true
	e.gatedBuiltins["fix"] = true
}

// SetStepLimit sets the maximum number of evaluation steps.
func (e *Engine) SetStepLimit(n int) {
	e.stepLimit = n
}

// SetDepthLimit sets the maximum call depth.
func (e *Engine) SetDepthLimit(n int) {
	e.depthLimit = n
}

// SetTraceHook sets the evaluation trace hook.
func (e *Engine) SetTraceHook(hook eval.TraceHook) {
	e.traceHook = hook
}

// SetCheckTraceHook sets the type checking trace hook.
func (e *Engine) SetCheckTraceHook(hook check.CheckTraceHook) {
	e.checkTraceHook = hook
}

// RegisterModule compiles a module and makes it available for import.
// Circular dependencies are detected and rejected.
func (e *Engine) RegisterModule(name, source string) error {
	// Circular dependency check.
	if _, exists := e.modules[name]; exists {
		return fmt.Errorf("module %s already registered", name)
	}

	// Ensure prelude is available for non-prelude modules.
	if name != "Prelude" && !e.noPrelude {
		e.ensurePrelude()
	}

	src := span.NewSource(name, source)
	l := syntax.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return &CompileError{Errors: lexErrs}
	}
	parseErrs := &errs.Errors{Source: src}
	p := syntax.NewParser(tokens, parseErrs)
	ast := p.ParseProgram()
	if parseErrs.HasErrors() {
		return &CompileError{Errors: parseErrs}
	}

	// Inject implicit prelude import for non-prelude modules.
	if name != "Prelude" && !e.noPrelude {
		injectPreludeImport(ast)
	}

	// Collect dependencies.
	var deps []string
	for _, imp := range ast.Imports {
		deps = append(deps, imp.ModuleName)
	}

	// Circular dependency detection.
	if err := e.checkCircularDeps(name, deps); err != nil {
		return err
	}

	config := e.makeCheckConfig()
	prog, exports, checkErrs := check.CheckModule(ast, src, config)
	if checkErrs.HasErrors() {
		return &CompileError{Errors: checkErrs}
	}

	// Collect fixity declarations from the module AST.
	modFixity := make(map[string]syntax.Fixity)
	for _, d := range ast.Decls {
		if fix, ok := d.(*syntax.DeclFixity); ok {
			modFixity[fix.Op] = syntax.Fixity{Assoc: fix.Assoc, Prec: fix.Prec}
		}
	}

	e.modules[name] = &compiledModule{
		prog:    prog,
		exports: exports,
		deps:    deps,
		fixity:  modFixity,
	}
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
	for name, mod := range e.modules {
		imported[name] = mod.exports
	}
	return &check.CheckConfig{
		RegisteredTypes: maps.Clone(e.registeredTys),
		Assumptions:     maps.Clone(e.assumptions),
		Bindings:        maps.Clone(e.bindings),
		GatedBuiltins:   maps.Clone(e.gatedBuiltins),
		Trace:           e.checkTraceHook,
		ImportedModules: imported,
	}
}

// NoPrelude disables automatic prelude inclusion.
func (e *Engine) NoPrelude() {
	e.noPrelude = true
}

// ensurePrelude registers the prelude module if it hasn't been registered yet.
func (e *Engine) ensurePrelude() {
	if e.noPrelude {
		return
	}
	if _, exists := e.modules["Prelude"]; exists {
		return
	}
	// Register prelude as an implicit module (errors are programming errors, so panic).
	if err := e.RegisterModule("Prelude", stdlib.PreludeSource); err != nil {
		panic(fmt.Sprintf("failed to compile prelude: %v", err))
	}
}

// parseSource lexes and parses source, adding implicit prelude import if needed.
func (e *Engine) parseSource(source string) (*syntax.AstProgram, *span.Source, error) {
	e.ensurePrelude()
	src := span.NewSource("<input>", source)
	l := syntax.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return nil, nil, &CompileError{Errors: lexErrs}
	}
	parseErrs := &errs.Errors{Source: src}
	p := syntax.NewParser(tokens, parseErrs)
	// Seed parser with fixity declarations from registered modules.
	for _, mod := range e.modules {
		p.AddFixity(mod.fixity)
	}
	ast := p.ParseProgram()
	if parseErrs.HasErrors() {
		return nil, nil, &CompileError{Errors: parseErrs}
	}
	// Inject implicit prelude import.
	if !e.noPrelude {
		injectPreludeImport(ast)
	}
	return ast, src, nil
}

// injectPreludeImport adds an implicit "import Prelude" if not already present.
func injectPreludeImport(ast *syntax.AstProgram) {
	for _, imp := range ast.Imports {
		if imp.ModuleName == "Prelude" {
			return
		}
	}
	ast.Imports = append([]syntax.DeclImport{{ModuleName: "Prelude"}}, ast.Imports...)
}

// ParsedProgram is an opaque parsed program for inspection.
type ParsedProgram struct{ prog *syntax.AstProgram }

// CoreProgram is an opaque compiled Core IR for inspection.
type CoreProgram struct{ prog *core.Program }

// Pretty returns a human-readable representation of the Core IR.
func (c *CoreProgram) Pretty() string { return core.PrettyProgram(c.prog) }

// Parse lexes and parses source code, returning an opaque parsed program.
// Useful for tooling and editor integration.
func (e *Engine) Parse(source string) (*ParsedProgram, error) {
	ast, _, err := e.parseSource(source)
	if err != nil {
		return nil, err
	}
	return &ParsedProgram{prog: ast}, nil
}

// Check compiles and type-checks source code without creating a Runtime.
// Returns the compiled Core IR program for inspection.
func (e *Engine) Check(source string) (*CoreProgram, error) {
	ast, src, err := e.parseSource(source)
	if err != nil {
		return nil, err
	}
	prog, checkErrs := check.Check(ast, src, e.makeCheckConfig())
	if checkErrs.HasErrors() {
		return nil, &CompileError{Errors: checkErrs}
	}
	return &CoreProgram{prog: prog}, nil
}

// NewRuntime compiles source code into an immutable, goroutine-safe Runtime.
func (e *Engine) NewRuntime(source string) (*Runtime, error) {
	ast, src, err := e.parseSource(source)
	if err != nil {
		return nil, err
	}

	// Type check.
	prog, checkErrs := check.Check(ast, src, e.makeCheckConfig())
	if checkErrs.HasErrors() {
		return nil, &CompileError{Errors: checkErrs}
	}

	// Collect module programs for runtime constructor/binding registration.
	var modProgs []*core.Program
	for _, mod := range e.modules {
		modProgs = append(modProgs, mod.prog)
	}

	return &Runtime{
		prog:          prog,
		prims:         e.prims,
		stepLimit:     e.stepLimit,
		depthLimit:    e.depthLimit,
		traceHook:     e.traceHook,
		bindings:      maps.Clone(e.bindings),
		gatedBuiltins: maps.Clone(e.gatedBuiltins),
		moduleProgs:   modProgs,
	}, nil
}
