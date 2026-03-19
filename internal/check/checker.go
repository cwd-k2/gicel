package check

import (
	"context"
	"fmt"
	"maps"

	"github.com/cwd-k2/gicel/internal/budget"
	"github.com/cwd-k2/gicel/internal/check/env"
	"github.com/cwd-k2/gicel/internal/check/exhaust"
	"github.com/cwd-k2/gicel/internal/check/family"
	"github.com/cwd-k2/gicel/internal/check/unify"
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// Generated name prefixes for internal variables.
const (
	prefixDict              = "$d"    // dictionary parameters (class instance evidence)
	prefixDictConstraintVar = "$d_cv" // constraint-variable dictionary parameters
	prefixDictDefer         = "$dict" // deferred dictionary placeholders
	prefixPat               = "$pat"  // desugared structured pattern variables
	prefixBind              = "$bind" // anonymous bind variables
	prefixSel               = "$sel"  // class method selectors
)

// CheckConfig provides environment for type checking.
type CheckConfig struct {
	Context         context.Context // cancellation context (nil = no cancellation)
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
	Types              map[string]types.Kind      // registered type constructors
	ConTypes           map[string]types.Type      // constructor → full type
	ConstructorInfo    map[string]*DataTypeInfo   // constructor → data type info
	ConstructorsByType map[string][]string        // type name → constructor names (precomputed index)
	Aliases            map[string]*AliasInfo      // type aliases
	Classes            map[string]*ClassInfo      // class declarations
	Instances          []*InstanceInfo            // instance declarations
	Values             map[string]types.Type      // top-level value types
	PromotedKinds      map[string]types.Kind      // DataKinds promotions
	PromotedCons       map[string]types.Kind      // promoted constructors
	TypeFamilies       map[string]*TypeFamilyInfo // type family declarations
	DataDecls          []core.DataDecl            // for evaluator constructor registration
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

// checkerRegistry holds semantic registries populated during declaration
// processing and read during type checking.
type checkerRegistry struct {
	conModules        map[string]string // constructor name → source module name
	conTypes          map[string]types.Type
	conInfo           map[string]*DataTypeInfo
	dataTypeByName    map[string]*DataTypeInfo // type name → DataTypeInfo (reverse index)
	aliases           map[string]*AliasInfo
	classes           map[string]*ClassInfo
	instances         []*InstanceInfo
	instancesByClass  map[string][]*InstanceInfo
	importedInstances map[*InstanceInfo]bool
	promotedKinds     map[string]types.Kind      // DataKinds: data name → KData
	promotedCons      map[string]types.Kind      // DataKinds: nullary con → KData
	kindVars          map[string]bool            // HKT: kind variables in scope
	families          map[string]*TypeFamilyInfo // type family declarations
}

// checkerScope holds name resolution and module scoping state.
type checkerScope struct {
	currentModule   string                     // module being compiled ("" = user main source)
	qualifiedScopes map[string]*qualifiedScope // alias → qualified module scope
	importedNames   map[string]string          // name → source module (for ambiguity detection)
	ownedNamesCache map[string]map[string]bool // module → owned names (lazy, for ambiguity checks)
}

// Checker holds mutable state during type checking.
type Checker struct {
	// Services.
	ctx           *Context
	unifier       *unify.Unifier
	stuckFamilies family.StuckIndex
	budget        *budget.Budget
	errors        *errs.Errors
	source        *span.Source
	config        *CheckConfig
	freshID       int

	// Semantic registries.
	reg checkerRegistry

	// Name resolution scope.
	scope checkerScope

	// Inference accumulator.
	deferred []deferredConstraint

	// Recursion/depth guards.
	depth        int // inference recursion depth
	resolveDepth int // instance resolution recursion depth
	level        int // implication nesting depth for touchability (0 = top-level)

	// Phase state.
	strictTypeNames bool // enabled after declaration processing
	cancelled       bool // set when context is cancelled
}

// checkCancelled checks the budget context for cancellation.
// Returns true if cancelled, recording a terminal error.
func (ch *Checker) checkCancelled() bool {
	if ch.cancelled {
		return true
	}
	ctx := ch.budget.Context()
	select {
	case <-ctx.Done():
		ch.cancelled = true
		ch.errors.Add(&errs.Error{
			Code:    errs.ErrCancelled,
			Phase:   errs.PhaseCheck,
			Message: fmt.Sprintf("type checking cancelled: %v", ctx.Err()),
		})
		return true
	default:
		return false
	}
}

// qualifiedScope holds a module's exports for qualified name resolution.
type qualifiedScope struct {
	moduleName string         // canonical module name
	exports    *ModuleExports // the module's full exports
}

// DataTypeInfo and ConstructorInfo are defined in the exhaust subpackage.
type DataTypeInfo = exhaust.DataTypeInfo
type ConstructorInfo = exhaust.ConstructorInfo

// AliasInfo, ClassInfo, InstanceInfo and related types are defined in the env subpackage.
type AliasInfo = env.AliasInfo
type ClassInfo = env.ClassInfo
type ClassFunDep = env.ClassFunDep
type SuperInfo = env.SuperInfo
type MethodInfo = env.MethodInfo
type InstanceInfo = env.InstanceInfo
type ConstraintInfo = env.ConstraintInfo

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
// Unlike CheckModule, this does not construct module exports, avoiding
// the cost of cloning registries when exports are not needed.
func Check(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*core.Program, *errs.Errors) {
	ch := newChecker(prog, source, config)
	coreProgram := ch.checkDecls(prog.Decls)
	return coreProgram, ch.errors
}

// CheckModule type-checks a program and returns both Core IR and module exports.
func CheckModule(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*core.Program, *ModuleExports, *errs.Errors) {
	ch := newChecker(prog, source, config)
	coreProgram := ch.checkDecls(prog.Decls)
	exports := ch.ExportModule(coreProgram)
	return coreProgram, exports, ch.errors
}

// newChecker initializes a Checker, imports modules, and returns it
// ready for checkDecls. Shared by Check and CheckModule.
func newChecker(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) *Checker {
	if config == nil {
		config = &CheckConfig{}
	}
	ctx := config.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ch := &Checker{
		ctx:    NewContext(),
		budget: budget.New(ctx, family.MaxReductionWork, 0),
		errors: &errs.Errors{Source: source},
		source: source,
		config: config,
		reg: checkerRegistry{
			conModules:        make(map[string]string),
			conTypes:          make(map[string]types.Type),
			conInfo:           make(map[string]*DataTypeInfo),
			dataTypeByName:    make(map[string]*DataTypeInfo),
			aliases:           make(map[string]*AliasInfo),
			classes:           make(map[string]*ClassInfo),
			instancesByClass:  make(map[string][]*InstanceInfo),
			importedInstances: make(map[*InstanceInfo]bool),
			promotedKinds:     make(map[string]types.Kind),
			promotedCons:      make(map[string]types.Kind),
			kindVars:          make(map[string]bool),
			families:          make(map[string]*TypeFamilyInfo),
		},
		scope: checkerScope{
			currentModule:   config.CurrentModule,
			qualifiedScopes: make(map[string]*qualifiedScope),
			importedNames:   make(map[string]string),
		},
	}
	ch.unifier = unify.NewUnifierShared(&ch.freshID)
	ch.unifier.OnSolve = func(metaID int) {
		ch.stuckFamilies.Reactivate(metaID)
	}
	ch.initContext()
	ch.importModules(prog.Imports)
	return ch
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
	// Build precomputed constructor-by-type index.
	consByType := make(map[string][]string, len(ch.reg.conInfo))
	for conName, info := range ch.reg.conInfo {
		consByType[info.Name] = append(consByType[info.Name], conName)
	}
	return &ModuleExports{
		Types:              maps.Clone(ch.config.RegisteredTypes),
		ConTypes:           maps.Clone(ch.reg.conTypes),
		ConstructorInfo:    ch.reg.conInfo,
		ConstructorsByType: consByType,
		Aliases:            ch.reg.aliases,
		Classes:            ch.reg.classes,
		Instances:          ch.reg.instances,
		Values:             values,
		PromotedKinds:      maps.Clone(ch.reg.promotedKinds),
		PromotedCons:       maps.Clone(ch.reg.promotedCons),
		TypeFamilies:       cloneFamilies(ch.reg.families),
		DataDecls:          prog.DataDecls,
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
	return &types.TyMeta{ID: id, Kind: k, Level: ch.level}
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

// checkerSnapshot captures unifier and stuck family state for rollback.
type checkerSnapshot struct {
	unifier       unify.Snapshot
	stuckFamilies family.StuckSnapshot
}

// saveState snapshots the checker's unifier and stuck family state.
func (ch *Checker) saveState() checkerSnapshot {
	return checkerSnapshot{
		unifier:       ch.unifier.Snapshot(),
		stuckFamilies: ch.stuckFamilies.Snapshot(),
	}
}

// restoreState rolls back the checker to a previously saved snapshot.
func (ch *Checker) restoreState(snap checkerSnapshot) {
	ch.unifier.Restore(snap.unifier)
	ch.stuckFamilies.Restore(snap.stuckFamilies)
}

// withTrial runs fn in a trial unification scope. If fn returns false,
// the unifier and stuck family state is rolled back to the snapshot taken before fn was called.
func (ch *Checker) withTrial(fn func() bool) bool {
	saved := ch.saveState()
	if fn() {
		return true
	}
	ch.restoreState(saved)
	return false
}

// withDeferredScope runs fn in an isolated deferred constraint scope.
// Constraints accumulated inside fn are resolved immediately, then
// any remaining constraints are merged back into the outer scope.
func (ch *Checker) withDeferredScope(fn func() core.Core) core.Core {
	saved := ch.deferred
	ch.deferred = nil
	result := fn()
	result = ch.resolveDeferredConstraints(result)
	ch.deferred = append(saved, ch.deferred...)
	return result
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
