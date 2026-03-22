package check

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/compiler/check/exhaust"
	"github.com/cwd-k2/gicel/internal/compiler/check/family"
	"github.com/cwd-k2/gicel/internal/compiler/check/modscope"
	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
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

// Default compiler limits. Authoritative values; CheckConfig fields
// override when non-zero.
const (
	defaultMaxSolverSteps  = 100_000
	defaultMaxResolveDepth = 64
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
	MaxTFSteps      int                 // type family reduction step limit (0 = default 50000)
	MaxSolverSteps  int                 // constraint solver step limit (0 = default 100000)
	MaxResolveDepth int                 // instance resolution depth limit (0 = default 64)
}

// ModuleExports is the type-level information exported by a compiled module.
// Canonical definition lives in env/module_exports.go.
type ModuleExports = env.ModuleExports

// ModuleOwnership carries ownership signals (which names a module defines).
type ModuleOwnership = env.ModuleOwnership

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

// Scope holds name resolution and module scoping state.
type Scope struct {
	currentModule   string                              // module being compiled ("" = user main source)
	qualifiedScopes map[string]*modscope.QualifiedScope // alias → qualified module scope

	// Qualified name injections: aliases and families resolved from qualified
	// references (e.g., M.Alias) are cached here instead of mutating the Registry.
	injectedAliases  map[string]*AliasInfo
	injectedFamilies map[string]*TypeFamilyInfo
}

// CurrentModule returns the name of the module being compiled.
func (s *Scope) CurrentModule() string {
	return s.currentModule
}

// LookupQualified returns the qualified scope for the given alias.
func (s *Scope) LookupQualified(alias string) (*modscope.QualifiedScope, bool) {
	qs, ok := s.qualifiedScopes[alias]
	return qs, ok
}

// InjectAlias caches a qualified alias in the scope's local map,
// avoiding mutation of the shared Registry.
func (s *Scope) InjectAlias(name string, info *AliasInfo) {
	if s.injectedAliases == nil {
		s.injectedAliases = make(map[string]*AliasInfo)
	}
	s.injectedAliases[name] = info
}

// InjectFamily caches a qualified type family in the scope's local map,
// avoiding mutation of the shared Registry.
func (s *Scope) InjectFamily(name string, info *TypeFamilyInfo) {
	if s.injectedFamilies == nil {
		s.injectedFamilies = make(map[string]*TypeFamilyInfo)
	}
	s.injectedFamilies[name] = info
}

// Solver holds mutable state for the constraint solver.
//
// Access to level and worklist should go through the methods below rather
// than direct field access. This ensures invariants (level non-negative,
// worklist save/restore matching) are maintained at API boundaries.
type Solver struct {
	worklist       Worklist
	inertSet       InertSet
	ambiguityCache map[string]bool // per-solveWanteds cache; lazily allocated

	// level tracks implication nesting depth for OutsideIn(X) touchability.
	// Incremented on entry to GADT branches (bidir_case.go) and higher-rank
	// forall scopes (bidir.go). freshMeta uses this to stamp metas with
	// their birth level. The companion field unifier.SolverLevel controls
	// touchability enforcement: metas with Level < SolverLevel are untouchable.
	level int
}

// EnterScope increments the solver level. Call ExitScope when leaving.
func (s *Solver) EnterScope() { s.level++ }

// ExitScope decrements the solver level.
// Panics if level would go negative (unpaired EnterScope/ExitScope).
func (s *Solver) ExitScope() {
	if s.level <= 0 {
		panic("internal: Solver.ExitScope called at level 0 (unpaired EnterScope/ExitScope)")
	}
	s.level--
}

// SaveWorklist drains the current worklist and returns its contents.
// The caller must eventually call RestoreWorklist to put constraints back.
func (s *Solver) SaveWorklist() []Ct { return s.worklist.Drain() }

// RestoreWorklist replaces the worklist contents.
func (s *Solver) RestoreWorklist(cts []Ct) { s.worklist.Load(cts) }

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

// Session holds cross-cutting infrastructure shared by all check services.
type Session struct {
	ctx             *Context
	unifier         *unify.Unifier
	budget          *budget.CheckBudget
	errors          *diagnostic.Errors
	source          *span.Source
	config          *CheckConfig
	freshID         int
	depth           int
	strictTypeNames bool // enabled after declaration processing
	cancelled       bool // set when context is cancelled
}

// Checker holds mutable state during type checking.
type Checker struct {
	*Session

	// Semantic registries (populated in declaration phases, then read-only).
	reg *Registry

	// Name resolution and module scoping.
	scope *Scope

	// Constraint solver state.
	solver *Solver

	// Cached family reduction environment (lazy, constructed on first use).
	cachedFamilyEnv *family.ReduceEnv
}

// checkCancelled checks the budget context for cancellation.
// Returns true if cancelled, recording a terminal error.
func (s *Session) checkCancelled() bool {
	if s.cancelled {
		return true
	}
	ctx := s.budget.Context()
	select {
	case <-ctx.Done():
		s.cancelled = true
		s.errors.Add(&diagnostic.Error{
			Code:    diagnostic.ErrCancelled,
			Phase:   diagnostic.PhaseCheck,
			Message: fmt.Sprintf("type checking cancelled: %v", ctx.Err()),
		})
		return true
	default:
		return false
	}
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
func Check(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*ir.Program, *diagnostic.Errors) {
	ch := newChecker(prog, source, config)
	coreProgram := ch.checkDecls(prog.Decls)
	return coreProgram, ch.errors
}

// CheckModule type-checks a program and returns both Core IR and module exports.
func CheckModule(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*ir.Program, *ModuleExports, *diagnostic.Errors) {
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
	session := &Session{
		ctx: NewContext(),
		budget: func() *budget.CheckBudget {
			b := budget.NewCheck(ctx)
			if config.NestingLimit > 0 {
				b.SetNestingLimit(config.NestingLimit)
			}
			maxTF := config.MaxTFSteps
			if maxTF <= 0 {
				maxTF = family.MaxReductionWork
			}
			b.SetTFStepLimit(maxTF)
			maxSolver := config.MaxSolverSteps
			if maxSolver <= 0 {
				maxSolver = defaultMaxSolverSteps
			}
			b.SetSolverStepLimit(maxSolver)
			maxResolve := config.MaxResolveDepth
			if maxResolve <= 0 {
				maxResolve = defaultMaxResolveDepth
			}
			b.SetResolveDepthLimit(maxResolve)
			return b
		}(),
		errors: &diagnostic.Errors{Source: source},
		source: source,
		config: config,
	}
	reg := func() *Registry {
		r := &Registry{
			typeKinds:         make(map[string]types.Kind),
			conModules:        make(map[string]string),
			conTypes:          make(map[string]types.Type),
			conInfo:           make(map[string]*DataTypeInfo),
			dataTypeByName:    make(map[string]*DataTypeInfo),
			aliases:           make(map[string]*AliasInfo),
			classes:           make(map[string]*ClassInfo),
			dictToClass:       make(map[string]string),
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
	}()
	ch := &Checker{
		Session: session,
		reg:     reg,
		scope: &Scope{
			currentModule: config.CurrentModule,
		},
		solver: &Solver{},
	}
	ch.unifier = unify.NewUnifierShared(&ch.freshID)
	ch.unifier.SolverLevel = 0 // baseline: raised during implication scopes
	ch.unifier.Budget = ch.budget
	ch.unifier.OnSolve = func(metaID int) {
		ch.solver.Reactivate(metaID)
	}
	ch.initContext()
	imp := modscope.NewImporter(modscope.ImportEnv{
		RegisterTypeKind:     ch.reg.RegisterTypeKind,
		RegisterAlias:        ch.reg.RegisterAlias,
		RegisterClass:        ch.reg.RegisterClass,
		RegisterFamily:       ch.reg.RegisterFamily,
		RegisterDataType:     ch.reg.RegisterDataType,
		RegisterPromotedKind: ch.reg.RegisterPromotedKind,
		RegisterPromotedCon:  ch.reg.RegisterPromotedCon,
		SetConBinding:        ch.reg.SetConBinding,
		SetConInfo:           ch.reg.SetConInfo,
		ImportInstance:       ch.reg.ImportInstance,
		AddError:             ch.addCodedError,
		PushVar: func(name string, ty types.Type, module string) {
			ch.ctx.Push(&CtxVar{Name: name, Type: ty, Module: module})
		},
	})
	ch.scope.qualifiedScopes = imp.Import(
		prog.Imports,
		config.ImportedModules,
		config.ModuleDeps,
	)
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
	ch.reg.RegisterTypeKind("Record", &types.KArrow{From: types.KRow{}, To: types.KType{}})
}

func (s *Session) fresh() int {
	s.freshID++
	return s.freshID
}

func (ch *Checker) freshMeta(k types.Kind) *types.TyMeta {
	id := ch.fresh()
	return &types.TyMeta{ID: id, Kind: k, Level: ch.solver.Level()}
}

func (s *Session) freshSkolem(name string, k types.Kind) *types.TySkolem {
	id := s.fresh()
	return &types.TySkolem{ID: id, Name: name, Kind: k}
}

func (s *Session) freshKindMeta() *types.KMeta {
	id := s.fresh()
	return &types.KMeta{ID: id}
}

func (s *Session) mkType(name string) types.Type {
	return &types.TyCon{Name: name}
}

func (s *Session) errorPair(sp span.Span) (types.Type, ir.Core) {
	return &types.TyError{S: sp}, &ir.Var{Name: "<error>", S: sp}
}

// checkerSnapshot captures unifier state for rollback.
type checkerSnapshot struct {
	unifier unify.Snapshot
}

// saveState snapshots the session's unifier state (meta solutions, labels,
// kind solutions, skolem solutions). Constraint state (inert set, worklist),
// context, errors, and SolverLevel are NOT captured. SolverLevel is managed
// separately by withTrial/withProbe.
func (s *Session) saveState() checkerSnapshot {
	return checkerSnapshot{
		unifier: s.unifier.Snapshot(),
	}
}

// restoreState rolls back the unifier to a previously saved snapshot.
// Only meta solutions, labels, kind solutions, and skolem solutions are
// restored. Constraint state, context, and errors are not affected.
func (s *Session) restoreState(snap checkerSnapshot) {
	s.unifier.Restore(snap.unifier)
}

// withTrial runs fn in a trial unification scope. If fn returns false,
// the unifier state (meta solutions, labels, kind solutions, skolem
// solutions) is rolled back to the snapshot taken before fn was called.
// On success, solutions committed inside fn are preserved.
// Constraint state (inert set, worklist), context, and errors are NOT
// rolled back — trials test unifiability, not constraint satisfiability.
// MUST NOT: emit constraints, push/pop context, mutate inert set.
func (s *Session) withTrial(fn func() bool) bool {
	saved := s.saveState()
	savedLevel := s.unifier.SolverLevel
	s.unifier.SolverLevel = -1 // disable touchability in trial scope
	result := fn()
	s.unifier.SolverLevel = savedLevel
	if result {
		return true
	}
	s.restoreState(saved)
	return false
}

// withProbe runs fn in a probe scope. Unifier solutions are ALWAYS
// rolled back regardless of fn's return value. Use for pure
// unifiability tests where the answer matters but the solutions don't.
// MUST NOT: emit constraints, push/pop context, mutate inert set.
func (s *Session) withProbe(fn func() bool) bool {
	saved := s.saveState()
	savedLevel := s.unifier.SolverLevel
	s.unifier.SolverLevel = -1 // disable touchability in probe scope
	result := fn()
	s.unifier.SolverLevel = savedLevel
	s.restoreState(saved)
	return result
}

// lookupAlias searches for a type alias by name: first in the Registry
// (populated during declaration phases), then in Scope's injected aliases
// (populated from qualified references).
func (ch *Checker) lookupAlias(name string) (*AliasInfo, bool) {
	if info, ok := ch.reg.LookupAlias(name); ok {
		return info, true
	}
	if ch.scope != nil && ch.scope.injectedAliases != nil {
		if info, ok := ch.scope.injectedAliases[name]; ok {
			return info, true
		}
	}
	return nil, false
}

// lookupFamily searches for a type family by name: first in the Registry,
// then in Scope's injected families.
func (ch *Checker) lookupFamily(name string) (*TypeFamilyInfo, bool) {
	if info, ok := ch.reg.LookupFamily(name); ok {
		return info, true
	}
	if ch.scope != nil && ch.scope.injectedFamilies != nil {
		if info, ok := ch.scope.injectedFamilies[name]; ok {
			return info, true
		}
	}
	return nil, false
}

// tryUnify attempts to unify a and b, rolling back on failure.
func (s *Session) tryUnify(a, b types.Type) bool {
	return s.withTrial(func() bool {
		return s.unifier.Unify(a, b) == nil
	})
}

func (s *Session) addCodedError(code diagnostic.Code, sp span.Span, msg string) {
	s.errors.Add(&diagnostic.Error{
		Code:    code,
		Phase:   diagnostic.PhaseCheck,
		Span:    sp,
		Message: msg,
	})
}

func (s *Session) trace(kind CheckTraceKind, sp span.Span, format string, args ...any) {
	if s.config.Trace != nil {
		line, col := s.source.Location(sp.Start)
		s.config.Trace(CheckTraceEvent{
			Kind:    kind,
			Depth:   s.depth,
			Message: fmt.Sprintf(format, args...),
			Span:    sp,
			Line:    line,
			Col:     col,
		})
	}
}
