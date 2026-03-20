package check

import (
	"context"
	"fmt"

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
	prefixSec               = "$sec"  // section-desugared lambda parameters
	prefixField             = "$f"    // method selector field extraction parameters
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
	EntryPoint      string              // non-empty enables bare Computation check; that name is exempt
	NestingLimit    int                 // structural nesting depth limit (0 = disabled)
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

// Registry holds semantic registries populated during declaration
// processing and read during type checking.
type Registry struct {
	typeKinds         map[string]types.Kind      // registered type constructor kinds (absorbed from config)
	conModules        map[string]string           // constructor name → source module name
	conTypes          map[string]types.Type
	conInfo           map[string]*DataTypeInfo
	dataTypeByName    map[string]*DataTypeInfo    // type name → DataTypeInfo (reverse index)
	aliases           map[string]*AliasInfo
	classes           map[string]*ClassInfo
	instances         []*InstanceInfo
	instancesByClass  map[string][]*InstanceInfo
	importedInstances map[*InstanceInfo]bool
	promotedKinds     map[string]types.Kind      // DataKinds: data name → KData
	promotedCons      map[string]types.Kind      // DataKinds: nullary con → KData
	kindVars          map[string]bool             // HKT: kind variables in scope
	families          map[string]*TypeFamilyInfo  // type family declarations
}

// RegisterConstructor records a constructor's type, owning module, and
// parent data type info. This is the single point of registration for
// the conTypes / conModules / conInfo triple.
func (r *Registry) RegisterConstructor(name string, ty types.Type, module string, info *DataTypeInfo) {
	r.conTypes[name] = ty
	r.conModules[name] = module
	r.conInfo[name] = info
}

// RegisterInstance appends an instance to the global list and the
// per-class index.
func (r *Registry) RegisterInstance(inst *InstanceInfo) {
	r.instances = append(r.instances, inst)
	r.instancesByClass[inst.ClassName] = append(r.instancesByClass[inst.ClassName], inst)
}

// Scope holds name resolution and module scoping state.
type Scope struct {
	currentModule       string                     // module being compiled ("" = user main source)
	qualifiedScopes     map[string]*qualifiedScope // alias → qualified module scope
	importedNames       map[string]string          // name → source module (value namespace ambiguity)
	importedTypeNames   map[string]string          // name → source module (type namespace ambiguity)
	ownedNamesCache     map[string]map[string]bool // module → owned names (lazy, for value ambiguity)
	ownedTypeNamesCache map[string]map[string]bool // module → owned type names (lazy, for type ambiguity)
}

// CurrentModule returns the name of the module being compiled.
func (s *Scope) CurrentModule() string {
	return s.currentModule
}

// LookupQualified returns the qualified scope for the given alias.
func (s *Scope) LookupQualified(alias string) (*qualifiedScope, bool) {
	qs, ok := s.qualifiedScopes[alias]
	return qs, ok
}

// Solver holds mutable state for the constraint solver.
type Solver struct {
	worklist       Worklist
	inertSet       InertSet
	ambiguityCache map[string]bool // per-solveWanteds cache; lazily allocated
	resolveDepth   int             // instance resolution recursion depth
	level          int             // implication nesting depth for touchability (0 = top-level)
}

// Reactivate kicks out constraints blocked on the given meta and
// re-enqueues them for priority processing.
func (s *Solver) Reactivate(metaID int) {
	kicked := s.inertSet.KickOut(metaID)
	s.worklist.PushFront(kicked...)
}

// Emit pushes a constraint to the worklist for processing.
func (s *Solver) Emit(ct Ct) {
	s.worklist.Push(ct)
}

// Level returns the current implication nesting depth for touchability.
func (s *Solver) Level() int {
	return s.level
}

// EnterResolve increments the resolution depth and returns whether the
// limit is exceeded. The caller must defer the returned exit function.
func (s *Solver) EnterResolve() (ok bool, exit func()) {
	s.resolveDepth++
	exit = func() { s.resolveDepth-- }
	return s.resolveDepth <= maxResolveDepth, exit
}

// RegisterStuckFunEq inserts a stuck type family equation into the inert
// set for later re-activation when blocking metas are solved.
func (s *Solver) RegisterStuckFunEq(ct *CtFunEq) {
	s.inertSet.InsertFunEq(ct)
}

// LookupAmbiguity checks the per-solve ambiguity cache.
func (s *Solver) LookupAmbiguity(key string) (result bool, found bool) {
	if s.ambiguityCache == nil {
		return false, false
	}
	result, found = s.ambiguityCache[key]
	return
}

// CacheAmbiguity stores an ambiguity result in the per-solve cache.
func (s *Solver) CacheAmbiguity(key string, result bool) {
	if s.ambiguityCache == nil {
		s.ambiguityCache = make(map[string]bool)
	}
	s.ambiguityCache[key] = result
}

// Checker holds mutable state during type checking.
type Checker struct {
	// Services.
	ctx     *Context
	unifier *unify.Unifier
	budget  *budget.Budget
	errors  *errs.Errors
	source  *span.Source
	config  *CheckConfig
	freshID int

	// Semantic registries (populated in declaration phases, then read-only).
	reg *Registry

	// Name resolution and module scoping.
	scope *Scope

	// Constraint solver state.
	solver *Solver

	// Inference recursion depth (for trace events).
	depth int

	// Phase state.
	strictTypeNames bool // enabled after declaration processing
	cancelled       bool // set when context is cancelled

	// Cached family reduction environment (lazy, constructed on first use).
	cachedFamilyEnv *family.ReduceEnv
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
		budget: func() *budget.Budget {
			b := budget.New(ctx, family.MaxReductionWork, 0)
			if config.NestingLimit > 0 {
				b.SetNestingLimit(config.NestingLimit)
			}
			return b
		}(),
		errors: &errs.Errors{Source: source},
		source: source,
		config: config,
		reg: func() *Registry {
			r := &Registry{
				typeKinds:         make(map[string]types.Kind),
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
			}
			for name, kind := range config.RegisteredTypes {
				r.typeKinds[name] = kind
			}
			return r
		}(),
		scope: &Scope{
			currentModule:     config.CurrentModule,
			qualifiedScopes:   make(map[string]*qualifiedScope),
			importedNames:     make(map[string]string),
			importedTypeNames: make(map[string]string),
		},
		solver: &Solver{},
	}
	ch.unifier = unify.NewUnifierShared(&ch.freshID)
	ch.unifier.Budget = ch.budget
	ch.unifier.OnSolve = func(metaID int) {
		ch.solver.Reactivate(metaID)
	}
	ch.initContext()
	ch.importModules(prog.Imports)
	return ch
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
	// Built-in type constructors.
	ch.reg.typeKinds["Record"] = &types.KArrow{From: types.KRow{}, To: types.KType{}}
}

func (ch *Checker) fresh() int {
	ch.freshID++
	return ch.freshID
}

func (ch *Checker) freshMeta(k types.Kind) *types.TyMeta {
	id := ch.fresh()
	return &types.TyMeta{ID: id, Kind: k, Level: ch.solver.Level()}
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

// checkerSnapshot captures unifier state for rollback.
type checkerSnapshot struct {
	unifier unify.Snapshot
}

// saveState snapshots the checker's unifier state.
func (ch *Checker) saveState() checkerSnapshot {
	return checkerSnapshot{
		unifier: ch.unifier.Snapshot(),
	}
}

// restoreState rolls back the checker to a previously saved snapshot.
func (ch *Checker) restoreState(snap checkerSnapshot) {
	ch.unifier.Restore(snap.unifier)
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

// withDeferredScope runs fn in an isolated constraint scope.
// Constraints accumulated inside fn are resolved immediately, then
// any remaining constraints are merged back into the outer scope.
func (ch *Checker) withDeferredScope(fn func() core.Core) core.Core {
	saved := ch.solver.worklist.Drain()
	result := fn()
	result = ch.resolveDeferredConstraints(result)
	residual := ch.solver.worklist.Drain()
	ch.solver.worklist.Load(append(saved, residual...))
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
