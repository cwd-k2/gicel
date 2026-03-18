package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// Generated name prefixes for internal variables.
const (
	prefixDict      = "$d"    // dictionary parameters
	prefixDictCV    = "$d_cv" // constraint-variable dictionary parameters
	prefixDictDefer = "$dict" // deferred dictionary placeholders
	prefixPat       = "$pat"  // desugared structured pattern variables
	prefixBind      = "$bind" // anonymous bind variables
	prefixSel       = "$sel"  // class method selectors
)

// CheckConfig provides environment for type checking.
type CheckConfig struct {
	RegisteredTypes map[string]types.Kind
	Assumptions     map[string]types.Type
	Bindings        map[string]types.Type
	GatedBuiltins   map[string]bool
	Trace           CheckTraceHook
	ImportedModules map[string]*ModuleExports
	ModuleDeps      map[string][]string // module → direct dependencies
	StrictTypeNames bool                // when true, reject unregistered type constructor names
	CurrentModule   string              // module being compiled ("" = user main source)
}

// ModuleExports carries the type-level information exported by a compiled module.
type ModuleExports struct {
	Types         map[string]types.Kind      // registered type constructors
	ConTypes      map[string]types.Type      // constructor → full type
	ConInfo       map[string]*DataTypeInfo   // constructor → data type info
	Aliases       map[string]*aliasInfo      // type aliases
	Classes       map[string]*ClassInfo      // class declarations
	Instances     []*InstanceInfo            // instance declarations
	Values        map[string]types.Type      // top-level value types
	PromotedKinds map[string]types.Kind      // DataKinds promotions
	PromotedCons  map[string]types.Kind      // promoted constructors
	TypeFamilies  map[string]*TypeFamilyInfo // type family declarations
	DataDecls     []core.DataDecl            // for evaluator constructor registration
}

// CheckTraceKind classifies trace events.
type CheckTraceKind int

const (
	TraceUnify CheckTraceKind = iota
	TraceSolveMeta
	TraceInfer
	TraceCheck
	TraceInstantiate
)

// CheckTraceEvent describes one type checking decision.
type CheckTraceEvent struct {
	Kind    CheckTraceKind
	Depth   int
	Message string
	Span    span.Span // internal byte offsets (retained for internal use)
	Line    int       // 1-based line number
	Col     int       // 1-based column number
}

// CheckTraceHook receives trace events during type checking.
type CheckTraceHook func(CheckTraceEvent)

// Checker holds mutable state during type checking.
type Checker struct {
	ctx               *Context
	unifier           *Unifier
	errors            *errs.Errors
	source            *span.Source
	freshID           int
	config            *CheckConfig
	currentModule     string            // module being compiled ("" = user main source)
	conModules        map[string]string // constructor name → source module name
	conTypes          map[string]types.Type
	conInfo           map[string]*DataTypeInfo
	dataTypeByName    map[string]*DataTypeInfo // type name → DataTypeInfo (reverse index)
	aliases           map[string]*aliasInfo
	classes           map[string]*ClassInfo
	instances         []*InstanceInfo
	instancesByClass  map[string][]*InstanceInfo
	importedInstances map[*InstanceInfo]bool
	promotedKinds     map[string]types.Kind      // DataKinds: data name → KData
	promotedCons      map[string]types.Kind      // DataKinds: nullary con → KData
	kindVars          map[string]bool            // HKT: kind variables in scope (from \ (k: Kind))
	families          map[string]*TypeFamilyInfo // type family declarations
	reductionDepth    int                        // current type family reduction depth
	deferred          []deferredConstraint
	depth             int
	resolveDepth      int                        // instance resolution recursion depth
	qualifiedScopes   map[string]*qualifiedScope // alias → qualified module scope
	importedNames     map[string]string          // name → source module (for ambiguity detection)
	strictTypeNames   bool                       // enabled after declaration processing
}

// qualifiedScope holds a module's exports for qualified name resolution.
type qualifiedScope struct {
	moduleName string         // canonical module name
	exports    *ModuleExports // the module's full exports
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
	quantified    *types.QuantifiedConstraint // non-nil for quantified constraints
	constraintVar types.Type                  // non-nil for constraint variable entries (Dict reification)
}

// Check type-checks a surface AST program and produces Core IR.
func Check(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*core.Program, *errs.Errors) {
	coreProg, _, errors := CheckModule(prog, source, config)
	return coreProg, errors
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
		currentModule:     config.CurrentModule,
		conModules:        make(map[string]string),
		conTypes:          make(map[string]types.Type),
		conInfo:           make(map[string]*DataTypeInfo),
		dataTypeByName:    make(map[string]*DataTypeInfo),
		aliases:           make(map[string]*aliasInfo),
		classes:           make(map[string]*ClassInfo),
		instancesByClass:  make(map[string][]*InstanceInfo),
		importedInstances: make(map[*InstanceInfo]bool),
		promotedKinds:     make(map[string]types.Kind),
		promotedCons:      make(map[string]types.Kind),
		kindVars:          make(map[string]bool),
		families:          make(map[string]*TypeFamilyInfo),
		qualifiedScopes:   make(map[string]*qualifiedScope),
		importedNames:     make(map[string]string),
	}
	ch.unifier = NewUnifierShared(&ch.freshID)
	ch.initContext()
	ch.importModules(prog.Imports)
	coreProgram := ch.checkDecls(prog.Decls)
	exports := ch.ExportModule(coreProgram)
	return coreProgram, exports, ch.errors
}

// ExportModule captures the current checker state as a ModuleExports.
// Names starting with '_' are private and excluded from exports.
func (ch *Checker) ExportModule(prog *core.Program) *ModuleExports {
	values := make(map[string]types.Type)
	for _, b := range prog.Bindings {
		if !isPrivateName(b.Name) {
			values[b.Name] = b.Type
		}
	}
	return &ModuleExports{
		Types:         copyMap(ch.config.RegisteredTypes),
		ConTypes:      copyMap(ch.conTypes),
		ConInfo:       ch.conInfo,
		Aliases:       ch.aliases,
		Classes:       ch.classes,
		Instances:     ch.instances,
		Values:        values,
		PromotedKinds: copyMap(ch.promotedKinds),
		PromotedCons:  copyMap(ch.promotedCons),
		TypeFamilies:  cloneFamilies(ch.families),
		DataDecls:     prog.DataDecls,
	}
}

// isPrivateName reports whether a name is module-private.
// Private: '_' prefix (user convention) or compiler-generated identifier containing '$'.
// Operator names (e.g., <$>, $, +>) are never private even if they contain '$'.
func isPrivateName(name string) bool {
	if len(name) == 0 {
		return false
	}
	if name[0] == '_' {
		return true
	}
	// Compiler-generated names contain '$' in identifier context.
	// Operators (all non-alphanumeric) are exempt.
	if isOperatorName(name) {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] == '$' {
			return true
		}
	}
	return false
}

// isOperatorName returns true if the name is an operator (all symbol characters).
func isOperatorName(name string) bool {
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return false
		}
	}
	return true
}

func copyMap[V any](m map[string]V) map[string]V {
	out := make(map[string]V, len(m))
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
	// Built-in type constructors.
	ch.config.RegisteredTypes["Record"] = &types.KArrow{From: types.KRow{}, To: types.KType{}}
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

func (ch *Checker) freshKindMeta() *types.KMeta {
	id := ch.fresh()
	return &types.KMeta{ID: id}
}

func (ch *Checker) mkType(name string) types.Type {
	return &types.TyCon{Name: name}
}

func (ch *Checker) errorPair(s span.Span) (types.Type, core.Core) {
	return &types.TyError{S: s}, &core.Var{Name: "<error>", S: s}
}

// saveUnifierState snapshots the unifier state for later rollback.
func (ch *Checker) saveUnifierState() UnifierSnapshot {
	return ch.unifier.Snapshot()
}

// restoreUnifierState rolls back the unifier to a previously saved snapshot.
func (ch *Checker) restoreUnifierState(snap UnifierSnapshot) {
	ch.unifier.Restore(snap)
}

// withTrial runs fn in a trial unification scope. If fn returns false,
// the unifier state is rolled back to the snapshot taken before fn was called.
func (ch *Checker) withTrial(fn func() bool) bool {
	saved := ch.saveUnifierState()
	if fn() {
		return true
	}
	ch.restoreUnifierState(saved)
	return false
}

// tryUnify attempts to unify a and b, rolling back on failure.
func (ch *Checker) tryUnify(a, b types.Type) bool {
	return ch.withTrial(func() bool {
		return ch.unifier.Unify(a, b) == nil
	})
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
		line, col := ch.source.Location(s.Start)
		ch.config.Trace(CheckTraceEvent{
			Kind:    kind,
			Depth:   ch.depth,
			Message: fmt.Sprintf(format, args...),
			Span:    s,
			Line:    line,
			Col:     col,
		})
	}
}
