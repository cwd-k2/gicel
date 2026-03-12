package check

import (
	"fmt"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/pkg/types"
)

// CheckConfig provides environment for type checking.
type CheckConfig struct {
	RegisteredTypes map[string]types.Kind
	Assumptions     map[string]types.Type
	Bindings        map[string]types.Type
	GatedBuiltins   map[string]bool
	Trace           CheckTraceHook
}

// CheckTraceKind classifies trace events.
type CheckTraceKind int

const (
	TraceUnify       CheckTraceKind = iota
	TraceSolveMeta
	TraceInfer
	TraceCheck
	TraceInstantiate
	TraceRowUnify
)

// CheckTraceEvent describes one type checking decision.
type CheckTraceEvent struct {
	Kind    CheckTraceKind
	Depth   int
	Message string
	Span    span.Span
}

// CheckTraceHook receives trace events during type checking.
type CheckTraceHook func(CheckTraceEvent)

// Checker holds mutable state during type checking.
type Checker struct {
	ctx       *Context
	unifier   *Unifier
	errors    *errs.Errors
	source    *span.Source
	freshID   int
	config    *CheckConfig
	conTypes  map[string]types.Type
	conInfo   map[string]*DataTypeInfo
	aliases   map[string]*aliasInfo
	classes   map[string]*ClassInfo
	instances []*InstanceInfo
	depth     int
}

// DataTypeInfo carries constructor information for exhaustiveness.
type DataTypeInfo struct {
	Name         string
	Constructors []ConInfo
}

// ConInfo is a constructor's name and arity.
type ConInfo struct {
	Name  string
	Arity int
}

type aliasInfo struct {
	params []string
	body   types.Type
}

// Check type-checks a surface AST program and produces Core IR.
func Check(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*core.Program, *errs.Errors) {
	if config == nil {
		config = &CheckConfig{}
	}
	ch := &Checker{
		ctx:      NewContext(),
		errors:   &errs.Errors{Source: source},
		source:   source,
		config:   config,
		conTypes: make(map[string]types.Type),
		conInfo:  make(map[string]*DataTypeInfo),
		aliases:  make(map[string]*aliasInfo),
		classes:  make(map[string]*ClassInfo),
	}
	ch.unifier = NewUnifierShared(&ch.freshID)
	ch.initContext()
	coreProgram := ch.checkDecls(prog.Decls)
	return coreProgram, ch.errors
}

func (ch *Checker) initContext() {
	// Host-declared bindings.
	for name, ty := range ch.config.Bindings {
		ch.ctx.Push(&CtxVar{Name: name, Type: ty})
	}
	// Built-in identifiers.
	for name, ty := range builtinTypes {
		if gatedBuiltins[name] {
			if ch.config.GatedBuiltins != nil && ch.config.GatedBuiltins[name] {
				ch.ctx.Push(&CtxVar{Name: name, Type: ty})
			}
		} else {
			ch.ctx.Push(&CtxVar{Name: name, Type: ty})
		}
	}
	// Assumptions.
	for name, ty := range ch.config.Assumptions {
		ch.ctx.Push(&CtxVar{Name: name, Type: ty})
	}
	// Registered opaque types.
	if ch.config.RegisteredTypes == nil {
		ch.config.RegisteredTypes = make(map[string]types.Kind)
	}
}

func (ch *Checker) fresh() int {
	ch.freshID++
	return ch.freshID
}

func (ch *Checker) freshMeta(k types.Kind) *types.TyMeta {
	id := ch.fresh()
	return &types.TyMeta{ID: id, Kind: k}
}

func (ch *Checker) addError(s span.Span, msg string) {
	ch.addCodedError(errs.ErrTypeMismatch, s, msg)
}

func (ch *Checker) addCodedError(code errs.Code, s span.Span, msg string) {
	ch.errors.Add(&errs.Error{
		Code:    code,
		Phase:   errs.PhaseCheck,
		Span:    s,
		Message: msg,
	})
}

func (ch *Checker) trace(kind CheckTraceKind, s span.Span, format string, args ...any) {
	if ch.config.Trace != nil {
		ch.config.Trace(CheckTraceEvent{
			Kind:    kind,
			Depth:   ch.depth,
			Message: fmt.Sprintf(format, args...),
			Span:    s,
		})
	}
}
