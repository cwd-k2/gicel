// Package gomputation provides an embedded typed effect language for Go.
//
// The compilation pipeline follows a three-tier lifecycle:
//
//	Engine   (mutable, configurable)
//	  ↓ NewRuntime(source)
//	Runtime  (immutable, goroutine-safe)
//	  ↓ RunContext(ctx, ...)
//	result   (per-execution)
package gomputation

import (
	"context"
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

// Value is a runtime value produced by evaluation.
type Value = eval.Value

// HostVal wraps an opaque Go value injected from the host.
type HostVal = eval.HostVal

// ConVal is a fully-applied constructor value.
type ConVal = eval.ConVal

// CapEnv is a capability environment with copy-on-write semantics.
type CapEnv = eval.CapEnv

// PrimImpl is the signature for host-provided primitive operations.
type PrimImpl = eval.PrimImpl

// EvalStats holds post-evaluation statistics.
type EvalStats = eval.EvalStats

// TraceEvent describes one evaluation step.
type TraceEvent = eval.TraceEvent

// TraceHook is called before each evaluation step.
type TraceHook = eval.TraceHook

// CheckTraceKind classifies type checking trace events.
type CheckTraceKind = check.CheckTraceKind

// CheckTraceEvent describes one type checking decision.
type CheckTraceEvent = check.CheckTraceEvent

// CheckTraceHook receives trace events during type checking.
type CheckTraceHook = check.CheckTraceHook

// Stdlib re-exports — users import only the root package.

// Num provides integer arithmetic: Num class, Eq/Ord Int instances, and operators.
var Num Pack = func(e *Engine) error { return stdlib.Num(e) }

// Str provides string and rune operations.
var Str Pack = func(e *Engine) error { return stdlib.Str(e) }

// Fail provides the fail effect capability.
var Fail Pack = func(e *Engine) error { return stdlib.Fail(e) }

// State provides get/put state capabilities.
var State Pack = func(e *Engine) error { return stdlib.State(e) }

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

// Diagnostic is a single structured error from compilation.
type Diagnostic struct {
	Code    int
	Phase   string // "lex", "parse", or "check"
	Line    int
	Col     int
	Message string
}

// CompileError wraps compilation errors (lex, parse, or type check).
type CompileError struct {
	Errors *errs.Errors
}

func (e *CompileError) Error() string {
	return e.Errors.Format()
}

// Diagnostics returns structured diagnostics for programmatic access.
func (e *CompileError) Diagnostics() []Diagnostic {
	diags := make([]Diagnostic, len(e.Errors.Errs))
	for i, err := range e.Errors.Errs {
		line, col := e.Errors.Source.Location(err.Span.Start)
		diags[i] = Diagnostic{
			Code:    int(err.Code),
			Phase:   err.Phase.String(),
			Line:    line,
			Col:     col,
			Message: err.Message,
		}
	}
	return diags
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
		hasPrelude := false
		for _, imp := range ast.Imports {
			if imp.ModuleName == "Prelude" {
				hasPrelude = true
				break
			}
		}
		if !hasPrelude {
			ast.Imports = append([]syntax.DeclImport{{ModuleName: "Prelude"}}, ast.Imports...)
		}
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
		hasPrelude := false
		for _, imp := range ast.Imports {
			if imp.ModuleName == "Prelude" {
				hasPrelude = true
				break
			}
		}
		if !hasPrelude {
			ast.Imports = append([]syntax.DeclImport{{ModuleName: "Prelude"}}, ast.Imports...)
		}
	}
	return ast, src, nil
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

// Runtime is an immutable, compiled Gomputation program.
// It is goroutine-safe and can be executed concurrently.
type Runtime struct {
	prog          *core.Program
	prims         *eval.PrimRegistry
	stepLimit     int
	depthLimit    int
	traceHook     eval.TraceHook
	bindings      map[string]types.Type
	gatedBuiltins map[string]bool
	moduleProgs   []*core.Program
}

// Program returns the compiled Core IR for debugging/inspection.
func (r *Runtime) Program() *CoreProgram {
	return &CoreProgram{prog: r.prog}
}

// PrettyProgram returns a human-readable representation of the Core IR.
func (r *Runtime) PrettyProgram() string {
	return core.PrettyProgram(r.prog)
}

// RunResult holds the result of a single execution.
type RunResult struct {
	Value Value
	Stats EvalStats
}

// RunResultFull holds the full result of an execution including the final capability environment.
type RunResultFull struct {
	Value  Value
	CapEnv CapEnv
	Stats  EvalStats
}

// buildEnv constructs the base runtime environment with builtins, bindings, and constructors.
func (r *Runtime) buildEnv(bindings map[string]Value) (*eval.Env, error) {
	env := eval.EmptyEnv()
	for name := range r.bindings {
		v, ok := bindings[name]
		if !ok {
			return nil, fmt.Errorf("missing binding: %s", name)
		}
		env = env.Extend(name, v)
	}

	// Built-in values.
	env = env.Extend("pure", &eval.Closure{
		Env: eval.EmptyEnv(), Param: "_v",
		Body: &core.Var{Name: "_v"},
	})
	env = env.Extend("bind", &eval.Closure{
		Env: eval.EmptyEnv(), Param: "_comp",
		Body: &core.Lam{
			Param: "_f",
			Body:  &core.App{Fun: &core.Var{Name: "_f"}, Arg: &core.Var{Name: "_comp"}},
		},
	})
	env = env.Extend("force", &eval.Closure{
		Env: eval.EmptyEnv(), Param: "_thk",
		Body: &core.Force{Expr: &core.Var{Name: "_thk"}},
	})

	// Gated built-ins: rec and fix (enabled via EnableRecursion).
	if r.gatedBuiltins["fix"] {
		// fix f = letrec x = \arg -> (f x) arg in x
		// Standard strict-language fixpoint using knot-tying.
		env = env.Extend("fix", &eval.Closure{
			Env: eval.EmptyEnv(), Param: "_f",
			Body: &core.LetRec{
				Bindings: []core.Binding{{
					Name: "_x",
					Expr: &core.Lam{Param: "_arg", Body: &core.App{
						Fun: &core.App{Fun: &core.Var{Name: "_f"}, Arg: &core.Var{Name: "_x"}},
						Arg: &core.Var{Name: "_arg"},
					}},
				}},
				Body: &core.Var{Name: "_x"},
			},
		})
	}
	if r.gatedBuiltins["rec"] {
		// rec f = letrec x = \arg -> (f x) arg in x
		// Same implementation as fix; the type system enforces pre = post.
		env = env.Extend("rec", &eval.Closure{
			Env: eval.EmptyEnv(), Param: "_f",
			Body: &core.LetRec{
				Bindings: []core.Binding{{
					Name: "_x",
					Expr: &core.Lam{Param: "_arg", Body: &core.App{
						Fun: &core.App{Fun: &core.Var{Name: "_f"}, Arg: &core.Var{Name: "_x"}},
						Arg: &core.Var{Name: "_arg"},
					}},
				}},
				Body: &core.Var{Name: "_x"},
			},
		})
	}

	// Constructors from imported modules.
	for _, modProg := range r.moduleProgs {
		for _, d := range modProg.DataDecls {
			for _, con := range d.Cons {
				env = env.Extend(con.Name, &eval.ConVal{Con: con.Name})
			}
		}
	}

	// Constructors from main program.
	for _, d := range r.prog.DataDecls {
		for _, con := range d.Cons {
			env = env.Extend(con.Name, &eval.ConVal{Con: con.Name})
		}
	}
	return env, nil
}

// run is the shared implementation for RunContext and RunContextFull.
func (r *Runtime) run(ctx context.Context, caps map[string]any, bindings map[string]Value, entry string) (eval.EvalResult, EvalStats, error) {
	env, err := r.buildEnv(bindings)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}

	// Evaluate module bindings first (in registration order).
	for _, modProg := range r.moduleProgs {
		modCells := make(map[string]*eval.IndirectVal)
		for _, b := range modProg.Bindings {
			cell := &eval.IndirectVal{}
			modCells[b.Name] = cell
			env = env.Extend(b.Name, cell)
		}
		for _, b := range modProg.Bindings {
			ev := eval.NewEvaluator(ctx, r.prims, eval.NewLimit(r.stepLimit, r.depthLimit), r.traceHook)
			result, err := ev.Eval(env, eval.NewCapEnv(nil), b.Expr)
			if err != nil {
				return eval.EvalResult{}, EvalStats{}, fmt.Errorf("evaluating module binding %s: %w", b.Name, err)
			}
			v := result.Value
			modCells[b.Name].Ref = &v
		}
	}

	// Pre-populate env with forward-reference cells for all non-entry bindings.
	// This enables mutually-recursive and self-referential top-level bindings
	// (e.g., type class instance dicts with recursive methods).
	var entryExpr core.Core
	cells := make(map[string]*eval.IndirectVal)
	for _, b := range r.prog.Bindings {
		if b.Name == entry {
			entryExpr = b.Expr
			continue
		}
		cell := &eval.IndirectVal{}
		cells[b.Name] = cell
		env = env.Extend(b.Name, cell)
	}

	// Evaluate each binding, populating its forward-reference cell.
	for _, b := range r.prog.Bindings {
		if b.Name == entry {
			continue
		}
		ev := eval.NewEvaluator(ctx, r.prims, eval.NewLimit(r.stepLimit, r.depthLimit), r.traceHook)
		result, err := ev.Eval(env, eval.NewCapEnv(nil), b.Expr)
		if err != nil {
			return eval.EvalResult{}, EvalStats{}, fmt.Errorf("evaluating %s: %w", b.Name, err)
		}
		v := result.Value
		cells[b.Name].Ref = &v
	}

	if entryExpr == nil {
		return eval.EvalResult{}, EvalStats{}, fmt.Errorf("entry point %q not found", entry)
	}

	capEnv := eval.NewCapEnv(caps)
	ev := eval.NewEvaluator(ctx, r.prims, eval.NewLimit(r.stepLimit, r.depthLimit), r.traceHook)
	result, err := ev.Eval(env, capEnv, entryExpr)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}
	// Force effectful PrimVals at top-level (e.g. main := get).
	result, err = ev.ForceEffectful(result)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}
	return result, ev.Stats(), nil
}

// RunContext executes the program with the given capabilities and bindings.
// entry specifies the top-level binding to evaluate (typically "main").
func (r *Runtime) RunContext(ctx context.Context, caps map[string]any, bindings map[string]Value, entry string) (*RunResult, error) {
	result, stats, err := r.run(ctx, caps, bindings, entry)
	if err != nil {
		return nil, err
	}
	return &RunResult{Value: result.Value, Stats: stats}, nil
}

// RunContextFull is like RunContext but also returns the final capability environment.
func (r *Runtime) RunContextFull(ctx context.Context, caps map[string]any, bindings map[string]Value, entry string) (*RunResultFull, error) {
	result, stats, err := r.run(ctx, caps, bindings, entry)
	if err != nil {
		return nil, err
	}
	return &RunResultFull{Value: result.Value, CapEnv: result.CapEnv, Stats: stats}, nil
}

