package check

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/compiler/check/family"
	"github.com/cwd-k2/gicel/internal/compiler/check/modscope"
	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
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

// CheckState holds cross-cutting infrastructure shared by all check services.
type CheckState struct {
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
	*CheckState

	// Semantic registries (populated in declaration phases, then read-only).
	reg *Registry

	// Name resolution and module scoping.
	scope *Scope

	// Constraint solver state.
	solver *solve.Solver

	// Cached family reduction environment (lazy, constructed on first use).
	cachedFamilyEnv *family.ReduceEnv
}

// checkCancelled checks the budget context for cancellation.
// Returns true if cancelled, recording a terminal error.
func (s *CheckState) checkCancelled() bool {
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

// DataTypeInfo and ConstructorInfo are defined in the env subpackage.
type DataTypeInfo = env.DataTypeInfo
type ConstructorInfo = env.ConstructorInfo

// AliasInfo, ClassInfo, InstanceInfo and related types are defined in the env subpackage.
type AliasInfo = env.AliasInfo
type ClassInfo = env.ClassInfo
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
	session := &CheckState{
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
		CheckState: session,
		reg:        reg,
		scope: &Scope{
			currentModule: config.CurrentModule,
		},
	}
	ch.solver = solve.New(ch)
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

func (s *CheckState) fresh() int {
	s.freshID++
	return s.freshID
}

func (ch *Checker) freshMeta(k types.Kind) *types.TyMeta {
	id := ch.fresh()
	return &types.TyMeta{ID: id, Kind: k, Level: ch.solver.Level()}
}

func (s *CheckState) freshSkolem(name string, k types.Kind) *types.TySkolem {
	id := s.fresh()
	return &types.TySkolem{ID: id, Name: name, Kind: k}
}

func (s *CheckState) freshKindMeta() *types.KMeta {
	id := s.fresh()
	return &types.KMeta{ID: id}
}

func (s *CheckState) mkType(name string) types.Type {
	return types.Con(name)
}

func (s *CheckState) errorPair(sp span.Span) (types.Type, ir.Core) {
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
func (s *CheckState) saveState() checkerSnapshot {
	return checkerSnapshot{
		unifier: s.unifier.Snapshot(),
	}
}

// restoreState rolls back the unifier to a previously saved snapshot.
// Only meta solutions, labels, kind solutions, and skolem solutions are
// restored. Constraint state, context, and errors are not affected.
func (s *CheckState) restoreState(snap checkerSnapshot) {
	s.unifier.Restore(snap.unifier)
}

// withTrial runs fn in a trial unification scope. If fn returns false,
// the unifier state (meta solutions, labels, kind solutions, skolem
// solutions) is rolled back to the snapshot taken before fn was called.
// On success, solutions committed inside fn are preserved.
// Constraint state (inert set, worklist), context, and errors are NOT
// rolled back — trials test unifiability, not constraint satisfiability.
// MUST NOT: emit constraints, push/pop context, mutate inert set.
func (s *CheckState) withTrial(fn func() bool) bool {
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
func (s *CheckState) withProbe(fn func() bool) bool {
	saved := s.saveState()
	savedLevel := s.unifier.SolverLevel
	s.unifier.SolverLevel = -1 // disable touchability in probe scope
	result := fn()
	s.unifier.SolverLevel = savedLevel
	s.restoreState(saved)
	return result
}

// --- solve.Env interface methods ---

func (ch *Checker) Zonk(t types.Type) types.Type         { return ch.unifier.Zonk(t) }
func (ch *Checker) Unify(a, b types.Type) error          { return ch.unifier.Unify(a, b) }
func (ch *Checker) SolverLevel() int                     { return ch.unifier.SolverLevel }
func (ch *Checker) SetSolverLevel(l int)                 { ch.unifier.SolverLevel = l }
func (ch *Checker) InstallGivenEq(id int, ty types.Type) { ch.unifier.InstallGivenEq(id, ty) }
func (ch *Checker) RemoveGivenEq(id int)                 { ch.unifier.RemoveGivenEq(id) }
func (ch *Checker) ScanContext(fn func(CtxEntry) bool)   { ch.ctx.Scan(fn) }
func (ch *Checker) AddCodedError(code diagnostic.Code, s span.Span, msg string) {
	ch.addCodedError(code, s, msg)
}
func (ch *Checker) ErrorCount() int                               { return ch.errors.Len() }
func (ch *Checker) TruncateErrors(n int)                          { ch.errors.Truncate(n) }
func (ch *Checker) ResetSolverSteps()                             { ch.budget.ResetSolverSteps() }
func (ch *Checker) SolverStep() error                             { return ch.budget.SolverStep() }
func (ch *Checker) EnterResolve() error                           { return ch.budget.EnterResolve() }
func (ch *Checker) LeaveResolve()                                 { ch.budget.LeaveResolve() }
func (ch *Checker) CheckCancelled() bool                          { return ch.checkCancelled() }
func (ch *Checker) WithTrial(fn func() bool) bool                 { return ch.withTrial(fn) }
func (ch *Checker) WithProbe(fn func() bool) bool                 { return ch.withProbe(fn) }
func (ch *Checker) Fresh() int                                    { return ch.fresh() }
func (ch *Checker) FreshMeta(k types.Kind) *types.TyMeta          { return ch.freshMeta(k) }
func (ch *Checker) InstancesForClass(name string) []*InstanceInfo {
	return ch.reg.InstancesForClass(name)
}
func (ch *Checker) LookupClass(name string) (*ClassInfo, bool) {
	return ch.reg.LookupClass(name)
}
func (ch *Checker) ClassFromDict(name string) (string, bool) {
	return ch.reg.ClassFromDict(name)
}
func (ch *Checker) ReduceTyFamily(name string, args []types.Type, s span.Span) (types.Type, bool) {
	return ch.reduceTyFamily(name, args, s)
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
func (s *CheckState) tryUnify(a, b types.Type) bool {
	return s.withTrial(func() bool {
		return s.unifier.Unify(a, b) == nil
	})
}

func (s *CheckState) addCodedError(code diagnostic.Code, sp span.Span, msg string) {
	s.errors.Add(&diagnostic.Error{
		Code:    code,
		Phase:   diagnostic.PhaseCheck,
		Span:    sp,
		Message: msg,
	})
}

func (s *CheckState) trace(kind CheckTraceKind, sp span.Span, format string, args ...any) {
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
