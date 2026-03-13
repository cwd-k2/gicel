package check

import (
	"fmt"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/internal/types"
)

// CheckConfig provides environment for type checking.
type CheckConfig struct {
	RegisteredTypes map[string]types.Kind
	Assumptions     map[string]types.Type
	Bindings        map[string]types.Type
	GatedBuiltins   map[string]bool
	Trace           CheckTraceHook
	ImportedModules map[string]*ModuleExports
}

// ModuleExports carries the type-level information exported by a compiled module.
type ModuleExports struct {
	Types         map[string]types.Kind    // registered type constructors
	ConTypes      map[string]types.Type    // constructor → full type
	ConInfo       map[string]*DataTypeInfo // constructor → data type info
	Aliases       map[string]*aliasInfo    // type aliases
	Classes       map[string]*ClassInfo    // class declarations
	Instances     []*InstanceInfo          // instance declarations
	Values        map[string]types.Type    // top-level value types
	PromotedKinds map[string]types.Kind    // DataKinds promotions
	PromotedCons  map[string]types.Kind    // promoted constructors
	DataDecls     []core.DataDecl          // for evaluator constructor registration
	Bindings      []core.Binding           // for evaluator top-level bindings
}

// CheckTraceKind classifies trace events.
type CheckTraceKind int

const (
	TraceUnify CheckTraceKind = iota
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
	ctx              *Context
	unifier          *Unifier
	errors           *errs.Errors
	source           *span.Source
	freshID          int
	config           *CheckConfig
	conTypes         map[string]types.Type
	conInfo          map[string]*DataTypeInfo
	aliases          map[string]*aliasInfo
	classes          map[string]*ClassInfo
	instances          []*InstanceInfo
	instancesByClass   map[string][]*InstanceInfo
	importedInstances  map[*InstanceInfo]bool
	promotedKinds    map[string]types.Kind // DataKinds: data name → KData
	promotedCons     map[string]types.Kind // DataKinds: nullary con → KData
	deferred         []deferredConstraint
	depth            int
}

// DataTypeInfo carries constructor information for exhaustiveness.
type DataTypeInfo struct {
	Name         string
	Constructors []ConInfo
}

// ConInfo is a constructor's name, arity, and optional GADT return type.
type ConInfo struct {
	Name       string
	Arity      int
	ReturnType types.Type // GADT: non-nil if constructor has refined return type
}

type aliasInfo struct {
	params     []string
	paramKinds []types.Kind
	body       types.Type
}

// deferredConstraint records a constraint to be resolved after type inference.
type deferredConstraint struct {
	placeholder   string
	className     string
	args          []types.Type
	s             span.Span
	group         int                          // constraints from same qualified type chain; 0 = ungrouped
	quantified    *types.QuantifiedConstraint  // non-nil for quantified constraints
	constraintVar types.Type                   // non-nil for constraint variable entries (Dict reification)
}

// Check type-checks a surface AST program and produces Core IR.
func Check(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*core.Program, *errs.Errors) {
	if config == nil {
		config = &CheckConfig{}
	}
	ch := &Checker{
		ctx:               NewContext(),
		errors:            &errs.Errors{Source: source},
		source:            source,
		config:            config,
		conTypes:          make(map[string]types.Type),
		conInfo:           make(map[string]*DataTypeInfo),
		aliases:           make(map[string]*aliasInfo),
		classes:           make(map[string]*ClassInfo),
		instancesByClass:  make(map[string][]*InstanceInfo),
		importedInstances: make(map[*InstanceInfo]bool),
		promotedKinds:     make(map[string]types.Kind),
		promotedCons:      make(map[string]types.Kind),
	}
	ch.unifier = NewUnifierShared(&ch.freshID)
	ch.initContext()
	ch.importModules(prog.Imports)
	coreProgram := ch.checkDecls(prog.Decls)
	return coreProgram, ch.errors
}

// CheckModule type-checks a program and returns both Core IR and module exports.
func CheckModule(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*core.Program, *ModuleExports, *errs.Errors) {
	if config == nil {
		config = &CheckConfig{}
	}
	ch := &Checker{
		ctx:               NewContext(),
		errors:            &errs.Errors{Source: source},
		source:            source,
		config:            config,
		conTypes:          make(map[string]types.Type),
		conInfo:           make(map[string]*DataTypeInfo),
		aliases:           make(map[string]*aliasInfo),
		classes:           make(map[string]*ClassInfo),
		instancesByClass:  make(map[string][]*InstanceInfo),
		importedInstances: make(map[*InstanceInfo]bool),
		promotedKinds:     make(map[string]types.Kind),
		promotedCons:      make(map[string]types.Kind),
	}
	ch.unifier = NewUnifierShared(&ch.freshID)
	ch.initContext()
	ch.importModules(prog.Imports)
	coreProgram := ch.checkDecls(prog.Decls)
	exports := ch.ExportModule(coreProgram)
	return coreProgram, exports, ch.errors
}

// importModules injects exported declarations from imported modules into the checker state.
func (ch *Checker) importModules(imports []syntax.DeclImport) {
	if ch.config.ImportedModules == nil {
		if len(imports) > 0 {
			ch.addCodedError(errs.ErrImport, imports[0].S, fmt.Sprintf("unknown module: %s", imports[0].ModuleName))
		}
		return
	}
	for _, imp := range imports {
		mod, ok := ch.config.ImportedModules[imp.ModuleName]
		if !ok {
			ch.addCodedError(errs.ErrImport, imp.S,
				fmt.Sprintf("unknown module: %s", imp.ModuleName))
			continue
		}
		// Merge module exports into checker state.
		for name, kind := range mod.Types {
			ch.config.RegisteredTypes[name] = kind
		}
		for name, ty := range mod.ConTypes {
			ch.conTypes[name] = ty
			ch.ctx.Push(&CtxVar{Name: name, Type: ty})
		}
		for name, info := range mod.ConInfo {
			ch.conInfo[name] = info
		}
		for name, alias := range mod.Aliases {
			ch.aliases[name] = alias
		}
		for name, cls := range mod.Classes {
			ch.classes[name] = cls
		}
		for _, inst := range mod.Instances {
			ch.instances = append(ch.instances, inst)
			ch.instancesByClass[inst.ClassName] = append(ch.instancesByClass[inst.ClassName], inst)
			ch.importedInstances[inst] = true
		}
		for name, ty := range mod.Values {
			ch.ctx.Push(&CtxVar{Name: name, Type: ty})
		}
		for name, kind := range mod.PromotedKinds {
			ch.promotedKinds[name] = kind
		}
		for name, kind := range mod.PromotedCons {
			ch.promotedCons[name] = kind
		}
	}
}

// ExportModule captures the current checker state as a ModuleExports.
func (ch *Checker) ExportModule(prog *core.Program) *ModuleExports {
	values := make(map[string]types.Type)
	for _, b := range prog.Bindings {
		values[b.Name] = b.Type
	}
	return &ModuleExports{
		Types:         copyKindMap(ch.config.RegisteredTypes),
		ConTypes:      copyTypeMap(ch.conTypes),
		ConInfo:       ch.conInfo,
		Aliases:       ch.aliases,
		Classes:       ch.classes,
		Instances:     ch.instances,
		Values:        values,
		PromotedKinds: copyKindMap(ch.promotedKinds),
		PromotedCons:  copyKindMap(ch.promotedCons),
		DataDecls:     prog.DataDecls,
		Bindings:      prog.Bindings,
	}
}

func copyKindMap(m map[string]types.Kind) map[string]types.Kind {
	out := make(map[string]types.Kind, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyTypeMap(m map[string]types.Type) map[string]types.Type {
	out := make(map[string]types.Type, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
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

func (ch *Checker) freshSkolem(name string, k types.Kind) *types.TySkolem {
	id := ch.fresh()
	return &types.TySkolem{ID: id, Name: name, Kind: k}
}

func (ch *Checker) mkType(name string) types.Type {
	return &types.TyCon{Name: name}
}

func (ch *Checker) errorPair(s span.Span) (types.Type, core.Core) {
	return &types.TyError{S: s}, &core.Var{Name: "<error>", S: s}
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
