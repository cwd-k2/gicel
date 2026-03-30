// Package engine orchestrates compilation and execution of GICEL programs.
//
// Files are grouped by responsibility:
//
//   - Orchestration: engine.go, pipeline.go, modstore.go
//   - Execution:     runtime.go, sandbox.go, limits.go
//   - Host bridge:   hostenv.go, convert.go, typehelpers.go, errors.go
package engine

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/cwd-k2/gicel/internal/compiler/check"
	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// DefaultEntryPoint is the default name of the top-level binding that serves
// as the program's entry point when no explicit name is provided.
const DefaultEntryPoint = "main"

// Engine configures and compiles GICEL programs.
// It is mutable and must not be shared across goroutines.
type Engine struct {
	host       HostEnv
	store      ModuleStore
	limits     Limits
	compileCtx context.Context // module compilation context (default: Background)

	entryPoint      string               // entry binding name (default: "main")
	denyAssumptions bool                 // when true, user code cannot use assumption declarations
	checkTraceHook  check.CheckTraceHook // diagnostic hook for type checking
}

// NewEngine creates a new Engine with default limits.
func NewEngine() *Engine {
	e := &Engine{
		host:       newHostEnv(),
		store:      newModuleStore(),
		compileCtx: context.Background(),
		limits: Limits{
			stepLimit:  1_000_000,
			depthLimit: 1_000,
		},
	}
	// Core primitives: SMC parallel composition and dagger.
	e.RegisterPrim("merge", stdlib.MergeImpl)
	e.RegisterPrim("dag", stdlib.DagImpl)
	// Core is always registered — provides IxMonad, Computation primitives.
	if err := e.RegisterModule("Core", stdlib.CoreSource); err != nil {
		panic("internal: core module: " + err.Error())
	}
	return e
}

// Use applies a Pack to the Engine.
func (e *Engine) Use(p registry.Pack) error {
	return p(e)
}

// DeclareBinding registers a host-provided value binding at compile time.
func (e *Engine) DeclareBinding(name string, ty types.Type) {
	e.host.bindings[name] = ty
}

// DeclareAssumption registers a primitive operation type.
func (e *Engine) DeclareAssumption(name string, ty types.Type) {
	e.host.assumptions[name] = ty
}

// RegisterType registers an opaque host type with the given kind.
func (e *Engine) RegisterType(name string, kind types.Type) {
	e.host.registeredTys[name] = kind
}

// RegisterPrim registers a primitive implementation for an assumption.
func (e *Engine) RegisterPrim(name string, impl eval.PrimImpl) {
	e.host.prims.Register(name, impl)
}

// EnableRecursion enables the rec and fix built-in identifiers for all
// subsequent compilations on this engine.
func (e *Engine) EnableRecursion() {
	e.host.gatedBuiltins["rec"] = true
	e.host.gatedBuiltins["fix"] = true
}

// DenyAssumptions prevents user code from using `assumption` declarations.
// Stdlib modules are unaffected. Use this in sandbox/agent contexts.
func (e *Engine) DenyAssumptions() {
	e.denyAssumptions = true
}

// RegisterRewriteRule adds a fusion rule to the optimization pipeline.
func (e *Engine) RegisterRewriteRule(rule registry.RewriteRule) {
	e.host.rewriteRules = append(e.host.rewriteRules, rule)
}

// RegisterModuleRec compiles a module with fix/rec enabled, scoped to
// this single compilation.
func (e *Engine) RegisterModuleRec(name, source string) error {
	saved := maps.Clone(e.host.gatedBuiltins)
	e.host.gatedBuiltins["rec"] = true
	e.host.gatedBuiltins["fix"] = true
	err := e.RegisterModule(name, source)
	e.host.gatedBuiltins = saved
	if err == nil {
		e.store.recursion = true
	}
	return err
}

// SetStepLimit sets the maximum number of evaluation steps.
func (e *Engine) SetStepLimit(n int) { e.limits.stepLimit = n }

// SetDepthLimit sets the maximum call depth.
func (e *Engine) SetDepthLimit(n int) { e.limits.depthLimit = n }

// SetNestingLimit sets the maximum structural nesting depth.
func (e *Engine) SetNestingLimit(n int) { e.limits.nestingLimit = n }

// SetAllocLimit sets the maximum cumulative allocation in bytes.
func (e *Engine) SetAllocLimit(bytes int64) { e.limits.allocLimit = bytes }

// SetCheckTraceHook sets the type checking trace hook.
func (e *Engine) SetCheckTraceHook(hook check.CheckTraceHook) {
	e.checkTraceHook = hook
}

// SetMaxTFSteps sets the type family reduction step limit.
func (e *Engine) SetMaxTFSteps(n int) { e.limits.maxTFSteps = n }

// SetMaxSolverSteps sets the constraint solver step limit.
func (e *Engine) SetMaxSolverSteps(n int) { e.limits.maxSolverSteps = n }

// SetMaxResolveDepth sets the instance resolution depth limit.
func (e *Engine) SetMaxResolveDepth(n int) { e.limits.maxResolveDepth = n }

// SetEntryPoint sets the entry point name for bare Computation checking.
// Non-entry top-level bindings with bare Computation type are rejected.
func (e *Engine) SetEntryPoint(name string) { e.entryPoint = name }

// SetCompileContext sets the context used for module compilation.
// This allows callers to impose a timeout or cancellation on
// RegisterModule / RegisterModuleRec calls.
func (e *Engine) SetCompileContext(ctx context.Context) { e.compileCtx = ctx }

// RegisterModuleFile reads a .gicel file and registers it as a module.
func (e *Engine) RegisterModuleFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read module file: %w", err)
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return e.RegisterModule(name, string(data))
}

// pipeline creates a pipelineCtx from the current Engine state.
func (e *Engine) pipeline(ctx context.Context) *pipelineCtx {
	ep := e.entryPoint
	if ep == "" {
		ep = DefaultEntryPoint
	}
	return &pipelineCtx{
		ctx:             ctx,
		host:            &e.host,
		store:           &e.store,
		limits:          &e.limits,
		traceHook:       e.checkTraceHook,
		entryPoint:      ep,
		denyAssumptions: e.denyAssumptions,
	}
}

// ValidateModuleName checks that name is a valid module identifier.
// Names are dot-separated segments (e.g. "Effect.State"), where each
// segment starts with an uppercase ASCII letter and contains only
// ASCII letters, digits, and underscores.
func ValidateModuleName(name string) error {
	if name == "" {
		return fmt.Errorf("module name must not be empty")
	}
	for _, seg := range strings.Split(name, ".") {
		if seg == "" {
			return fmt.Errorf("invalid module name %q: empty segment", name)
		}
		if seg[0] < 'A' || seg[0] > 'Z' {
			return fmt.Errorf("invalid module name %q: each segment must start with an uppercase letter", name)
		}
		for _, r := range seg {
			if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
				return fmt.Errorf("invalid module name %q: contains invalid character %q", name, string(r))
			}
		}
	}
	return nil
}

// RegisterModule compiles a module and makes it available for import.
// Module names must start with an uppercase letter and contain only
// ASCII letters, digits, and underscores.
func (e *Engine) RegisterModule(name, source string) error {
	if err := ValidateModuleName(name); err != nil {
		return err
	}
	if e.store.Has(name) {
		return fmt.Errorf("module %s already registered", name)
	}
	mod, err := e.pipeline(e.compileCtx).compileModule(name, source)
	if err != nil {
		return err
	}
	e.store.Register(name, mod)
	return nil
}

// CoreProgram is an opaque compiled Core IR for inspection.
type CoreProgram struct{ prog *ir.Program }

// Pretty returns a human-readable representation of the Core IR.
func (c *CoreProgram) Pretty() string { return ir.PrettyProgram(c.prog) }

// CompileResult holds all static information produced by compilation.
type CompileResult struct {
	prog *ir.Program
}

// Pretty returns the Core IR as a human-readable string.
func (cr *CompileResult) Pretty() string { return ir.PrettyProgram(cr.prog) }

// BindingNames returns the names of all top-level bindings.
func (cr *CompileResult) BindingNames() []string {
	names := make([]string, len(cr.prog.Bindings))
	for i, b := range cr.prog.Bindings {
		names[i] = b.Name
	}
	return names
}

// PrettyBindingTypes returns binding names mapped to their display-formatted types.
// The values are human-readable strings, not suitable for identity or comparison.
func (cr *CompileResult) PrettyBindingTypes() map[string]string {
	m := make(map[string]string, len(cr.prog.Bindings))
	for _, b := range cr.prog.Bindings {
		m[b.Name] = types.Pretty(b.Type)
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
	_, _, err := e.pipeline(e.compileCtx).lexAndParse("<input>", source, e.store.Has("Core"))
	return err
}

// Compile type-checks source code, returning exports and Core IR for
// static inspection. Unlike NewRuntime, it does not optimize or assemble
// a runtime. Pass context.Background() when cancellation is not needed.
func (e *Engine) Compile(ctx context.Context, source string) (*CompileResult, error) {
	pc := e.pipeline(ctx)
	ast, src, err := pc.lexAndParse("<input>", source, e.store.Has("Core"))
	if err != nil {
		return nil, err
	}
	cfg := pc.makeCheckConfig()
	cfg.Context = ctx
	cfg.EntryPoint = pc.entryPoint
	prog, _, checkErrs := check.CheckModule(ast, src, cfg)
	if checkErrs.HasErrors() {
		return nil, &CompileError{Errors: checkErrs}
	}
	return &CompileResult{prog: prog}, nil
}

// NewRuntime compiles source code into an immutable, goroutine-safe Runtime.
// The context bounds compilation time (type checking in particular);
// pass context.Background() when cancellation is not needed.
func (e *Engine) NewRuntime(ctx context.Context, source string) (*Runtime, error) {
	pc := e.pipeline(ctx)
	prog, src, err := pc.compileMain(source)
	if err != nil {
		return nil, err
	}
	return pc.assembleRuntime(prog, src), nil
}

