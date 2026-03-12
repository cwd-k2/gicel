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

	"github.com/cwd-k2/gomputation/internal/check"
	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/eval"
	"github.com/cwd-k2/gomputation/internal/prelude"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/pkg/types"
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
}

// NewEngine creates a new Engine with default limits.
func NewEngine() *Engine {
	return &Engine{
		bindings:      make(map[string]types.Type),
		assumptions:   make(map[string]types.Type),
		registeredTys: make(map[string]types.Kind),
		prims:         eval.NewPrimRegistry(),
		gatedBuiltins: make(map[string]bool),
		stepLimit:     1_000_000,
		depthLimit:    1_000,
	}
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

// CompileError wraps compilation errors (lex, parse, or type check).
type CompileError struct {
	Errors *errs.Errors
}

func (e *CompileError) Error() string {
	return e.Errors.Format()
}

// NoPrelude disables automatic prelude inclusion.
func (e *Engine) NoPrelude() {
	e.noPrelude = true
}

// NewRuntime compiles source code into an immutable, goroutine-safe Runtime.
func (e *Engine) NewRuntime(source string) (*Runtime, error) {
	fullSource := source
	if !e.noPrelude {
		fullSource = prelude.Source + "\n" + source
	}
	src := span.NewSource("<input>", fullSource)

	// Lex.
	l := syntax.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return nil, &CompileError{Errors: lexErrs}
	}

	// Parse.
	parseErrs := &errs.Errors{Source: src}
	p := syntax.NewParser(tokens, parseErrs)
	ast := p.ParseProgram()
	if parseErrs.HasErrors() {
		return nil, &CompileError{Errors: parseErrs}
	}

	// Type check.
	config := &check.CheckConfig{
		RegisteredTypes: copyKindMap(e.registeredTys),
		Assumptions:     copyTypeMap(e.assumptions),
		Bindings:        copyTypeMap(e.bindings),
		GatedBuiltins:   copyBoolMap(e.gatedBuiltins),
		Trace:           e.checkTraceHook,
	}
	prog, checkErrs := check.Check(ast, src, config)
	if checkErrs.HasErrors() {
		return nil, &CompileError{Errors: checkErrs}
	}

	return &Runtime{
		prog:       prog,
		prims:      e.prims,
		stepLimit:  e.stepLimit,
		depthLimit: e.depthLimit,
		traceHook:  e.traceHook,
		bindings:   copyTypeMap(e.bindings),
	}, nil
}

// Runtime is an immutable, compiled Gomputation program.
// It is goroutine-safe and can be executed concurrently.
type Runtime struct {
	prog       *core.Program
	prims      *eval.PrimRegistry
	stepLimit  int
	depthLimit int
	traceHook  eval.TraceHook
	bindings   map[string]types.Type
}

// Program returns the compiled Core IR for debugging/inspection.
func (r *Runtime) Program() *core.Program {
	return r.prog
}

// RunResult holds the result of a single execution.
type RunResult struct {
	Value Value
	Stats EvalStats
}

// RunContext executes the program with the given capabilities and bindings.
// entry specifies the top-level binding to evaluate (typically "main").
func (r *Runtime) RunContext(ctx context.Context, caps map[string]any, bindings map[string]Value, entry string) (*RunResult, error) {
	// Build initial environment from bindings.
	env := eval.EmptyEnv()
	for name := range r.bindings {
		v, ok := bindings[name]
		if !ok {
			return nil, fmt.Errorf("missing binding: %s", name)
		}
		env = env.Extend(name, v)
	}

	// Register built-in values.
	// pure: identity at runtime (direct-style strict evaluation).
	env = env.Extend("pure", &eval.Closure{
		Env: eval.EmptyEnv(), Param: "_v",
		Body: &core.Var{Name: "_v"},
	})
	// bind: in direct style, bind(comp, f) = f(comp).
	bindEnv := eval.EmptyEnv()
	env = env.Extend("bind", &eval.Closure{
		Env: bindEnv, Param: "_comp",
		Body: &core.Lam{
			Param: "_f",
			Body:  &core.App{Fun: &core.Var{Name: "_f"}, Arg: &core.Var{Name: "_comp"}},
		},
	})
	// thunk/force are handled by the evaluator's Core.Thunk/Core.Force nodes,
	// but for standalone use we provide runtime values.
	env = env.Extend("thunk", &eval.Closure{
		Env: eval.EmptyEnv(), Param: "_comp",
		Body: &core.Thunk{Comp: &core.Var{Name: "_comp"}},
	})
	env = env.Extend("force", &eval.Closure{
		Env: eval.EmptyEnv(), Param: "_thk",
		Body: &core.Force{Expr: &core.Var{Name: "_thk"}},
	})

	// Register constructors from data declarations.
	// Zero-arity constructors are ConVal values; multi-arity constructors
	// start as empty ConVal and accumulate args via apply.
	for _, d := range r.prog.DataDecls {
		for _, con := range d.Cons {
			env = env.Extend(con.Name, &eval.ConVal{Con: con.Name})
		}
	}

	// Find entry binding.
	var entryExpr core.Core
	for _, b := range r.prog.Bindings {
		if b.Name == entry {
			entryExpr = b.Expr
		} else {
			// Evaluate non-entry bindings and add to env.
			ev := eval.NewEvaluator(ctx, r.prims, eval.NewLimit(r.stepLimit, r.depthLimit), r.traceHook)
			result, err := ev.Eval(env, eval.NewCapEnv(nil), b.Expr)
			if err != nil {
				return nil, fmt.Errorf("evaluating %s: %w", b.Name, err)
			}
			env = env.Extend(b.Name, result.Value)
		}
	}

	if entryExpr == nil {
		return nil, fmt.Errorf("entry point %q not found", entry)
	}

	// Evaluate entry.
	capEnv := eval.NewCapEnv(caps)
	ev := eval.NewEvaluator(ctx, r.prims, eval.NewLimit(r.stepLimit, r.depthLimit), r.traceHook)
	result, err := ev.Eval(env, capEnv, entryExpr)
	if err != nil {
		return nil, err
	}

	return &RunResult{
		Value: result.Value,
		Stats: ev.Stats(),
	}, nil
}

func copyTypeMap(m map[string]types.Type) map[string]types.Type {
	c := make(map[string]types.Type, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func copyKindMap(m map[string]types.Kind) map[string]types.Kind {
	c := make(map[string]types.Kind, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func copyBoolMap(m map[string]bool) map[string]bool {
	c := make(map[string]bool, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
