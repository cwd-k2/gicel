package check

import (
	"context"
	"fmt"
	"strconv"

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
	RegisteredTypes map[string]types.Type
	Assumptions     map[string]types.Type
	Bindings        map[string]types.Type
	GatedBuiltins   map[string]bool
	Trace           CheckTraceHook
	ImportedModules map[string]*ModuleExports
	ModuleDeps      map[string][]string // module → direct dependencies
	DenyAssumptions bool                // when true, reject `assumption` declarations (sandbox mode)
	StrictTypeNames bool                // when true, reject unregistered type constructor names
	CurrentModule   string              // module being compiled ("" = user main source)
	EntryPoint      string              // non-empty enables bare Computation check; that name is exempt
	NestingLimit    int                 // structural nesting depth limit (0 = disabled)
	MaxTFSteps      int                 // type family reduction step limit (0 = default 50000)
	MaxSolverSteps  int                 // constraint solver step limit (0 = default 100000)
	MaxResolveDepth int                 // instance resolution depth limit (0 = default 64)
	HoverRecorder   HoverRecorder       // when non-nil, receives hover events during checking
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
	strictTypeNames bool       // enabled after declaration processing
	cancelled       bool       // set when context is cancelled
	solverLevel     func() int // injected solver level getter for freshMeta
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

	// Cached type resolver (lazy, constructed on first use).
	cachedTypeResolver *typeResolver

	// pipeState holds phase-transient state that exists only during a
	// declPipeline.run() invocation. Nil outside that scope. The pointer
	// indirection makes the lifecycle explicit: inference code accesses
	// phase state through ch.pipeState, and the nil case documents that
	// the state is unavailable outside the declaration pipeline.
	pipeState *declPipeState
}

// declPipeState holds state whose lifetime is scoped to one
// declPipeline.run() invocation. Created at run() entry, nil'd at exit.
type declPipeState struct {
	// currentBinding is the name being type-checked (Phase 7).
	// Used by lookupVar to generate self-reference diagnostic hints.
	currentBinding string

	// pendingMergeLabels records pre-state row types of Merge nodes
	// during inference. Drained by refineMergeLabels after constraint solving.
	pendingMergeLabels map[*ir.Merge]pendingMergePre
}

// pendingMergePre carries the unresolved capability row types whose
// labels need to be re-extracted after constraint resolution.
type pendingMergePre struct {
	Pre1 types.Type
	Pre2 types.Type
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
	ch.validateLabelArgs(coreProgram)
	if config != nil && config.HoverRecorder != nil {
		config.HoverRecorder.Rezonk(ch.unifier.Zonk)
	}
	return coreProgram, ch.errors
}

// CheckModule type-checks a program and returns both Core IR and module exports.
func CheckModule(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*ir.Program, *ModuleExports, *diagnostic.Errors) {
	ch := newChecker(prog, source, config)
	coreProgram := ch.checkDecls(prog.Decls)
	ch.validateLabelArgs(coreProgram)
	if config != nil && config.HoverRecorder != nil {
		config.HoverRecorder.Rezonk(ch.unifier.Zonk)
	}
	exports := ch.ExportModule(coreProgram)
	return coreProgram, exports, ch.errors
}

// refineMergeLabels rewrites Merge.LeftLabels/RightLabels using the
// pre-state row types recorded in pendingMergeLabels at inferMerge time,
// now that constraint resolution has solved the row metavariables. The
// side table is drained after refinement so the pre-state types can be
// garbage collected. Called from declPipeline.run after checkValues.
func (ch *Checker) refineMergeLabels() {
	if ch.pipeState == nil {
		return
	}
	for m, pre := range ch.pipeState.pendingMergeLabels {
		if pre.Pre1 != nil {
			labels := ch.extractRowLabels(ch.unifier.Zonk(pre.Pre1))
			if labels != nil {
				m.LeftLabels = labels
			}
		}
		if pre.Pre2 != nil {
			labels := ch.extractRowLabels(ch.unifier.Zonk(pre.Pre2))
			if labels != nil {
				m.RightLabels = labels
			}
		}
	}
	clear(ch.pipeState.pendingMergeLabels)
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
		ctx:    NewContext(),
		budget: newBudget(ctx, config),
		errors: &diagnostic.Errors{Source: source},
		source: source,
		config: config,
	}
	ch := &Checker{
		CheckState: session,
		reg:        newRegistry(config),
		scope: &Scope{
			currentModule: config.CurrentModule,
		},
	}
	ch.solver = solve.New(ch)
	ch.solverLevel = ch.solver.Level
	ch.wireUnifier()
	ch.initContext()
	ch.importModules(prog)
	return ch
}

func newBudget(ctx context.Context, config *CheckConfig) *budget.CheckBudget {
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
}

// wireUnifier initializes the unifier and connects it to the solver.
//
// SolverLevel timing patterns (L1-a invariant):
//
//	Baseline:           SolverLevel = 0   — all level-0 metas are touchable.
//	Trial/Probe:        SolverLevel = -1  — touchability disabled for speculative unification.
//	Implication scope:  SolverLevel = solver.Level() — metas below this level are untouchable.
//
// The DK interleaving constraint: in CheckWithLocalScope (solve/implication.go),
// SolverLevel is set AFTER body check, not before. Body check's eager unification
// must be free to solve outer metas (e.g. case result type).
func (ch *Checker) wireUnifier() {
	ch.unifier = unify.NewUnifierShared(&ch.freshID)
	ch.unifier.SolverLevel = 0
	ch.unifier.Budget = ch.budget
	ch.unifier.OnSolve = func(metaID int) {
		ch.solver.Reactivate(metaID)
	}
}

func (ch *Checker) importModules(prog *syntax.AstProgram) {
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
		AddError: func(code diagnostic.Code, s span.Span, msg string) {
			ch.addDiag(code, s, diagMsg(msg))
		},
		PushVar: func(name string, ty types.Type, module string) {
			ch.ctx.Push(&CtxVar{Name: name, Type: ty, Module: module})
		},
	})
	ch.scope.qualifiedScopes = imp.Import(
		prog.Imports,
		ch.config.ImportedModules,
		ch.config.ModuleDeps,
	)
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
	ch.reg.RegisterTypeKind(types.TyConRecord, &types.TyArrow{From: types.TypeOfRows, To: types.TypeOfTypes})
}

func (s *CheckState) fresh() int {
	s.freshID++
	return s.freshID
}

// freshName generates a unique name with the given prefix.
// Format: prefix_N where N is a per-checker monotonic counter.
//
// Namespace: "_" suffix. Other fresh-name generators use different
// namespaces to avoid collisions:
//   - types/subst.go: "$" suffix (type-level substitution, global)
//   - optimize/subst.go: "$r" suffix (term-level Core IR substitution, global)
func (s *CheckState) freshName(prefix string) string {
	return prefix + "_" + strconv.Itoa(s.fresh())
}

// freshDictName generates a unique dictionary parameter name.
// Format: $d_className_N.
func (s *CheckState) freshDictName(className string) string {
	return prefixDict + "_" + className + "_" + strconv.Itoa(s.fresh())
}

func (s *CheckState) freshMeta(k types.Type) *types.TyMeta {
	id := s.fresh()
	return &types.TyMeta{ID: id, Kind: k, Level: s.solverLevel()}
}

func (s *CheckState) freshSkolem(name string, k types.Type) *types.TySkolem {
	id := s.fresh()
	return &types.TySkolem{ID: id, Name: name, Kind: k}
}

// tryTrivialUnify attempts to solve a trivial unification eagerly.
// Returns true if solved (both sides are identical, or one is a fresh meta).
// This is a legitimate shortcut: the result is identical to emitting a CtEq
// and having the solver process it, but avoids the overhead.
func (ch *Checker) tryTrivialUnify(a, b types.Type) bool {
	a = ch.unifier.Zonk(a)
	b = ch.unifier.Zonk(b)
	if a == b {
		return true
	}
	if m, ok := a.(*types.TyMeta); ok && ch.unifier.Solve(m.ID) == nil {
		if ch.unifier.SolveFreshMeta(m, b) {
			return true
		}
	}
	if m, ok := b.(*types.TyMeta); ok && ch.unifier.Solve(m.ID) == nil {
		if ch.unifier.SolveFreshMeta(m, a) {
			return true
		}
	}
	return false
}

// isSortKind checks whether a kind-as-Type is a sort (universe level >= 2),
// i.e. the kind of kinds (SortZero or higher).
func isSortKind(k types.Type) bool {
	if tc, ok := k.(*types.TyCon); ok {
		return types.IsSortLevel(tc.Level)
	}
	return false
}

// isLevelKind checks whether a kind is TypeOfLevels (the Level kind).
func isLevelKind(k types.Type) bool {
	return k == types.TypeOfLevels
}

func (s *CheckState) errorPair(sp span.Span) (types.Type, ir.Core) {
	return &types.TyError{S: sp}, &ir.Error{S: sp}
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
		s.unifier.AbandonSnapshot()
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

func (s *CheckState) addCodedErrorWithHints(code diagnostic.Code, sp span.Span, msg string, hints []diagnostic.Hint) {
	s.errors.Add(&diagnostic.Error{
		Code:    code,
		Phase:   diagnostic.PhaseCheck,
		Span:    sp,
		Message: msg,
		Hints:   hints,
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
