package family

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// ReduceEnv provides the checker capabilities needed for type family operations.
type ReduceEnv struct {
	LookupFamily func(name string) (*env.TypeFamilyInfo, bool) // family lookup (includes Scope injections)
	Budget       *budget.CheckBudget
	Unifier      *unify.Unifier
	TypeOps      *types.TypeOps // type-level operation owner
	FreshMeta    func(k types.Type) *types.TyMeta
	AddError     func(code diagnostic.Code, s span.Span, msg string)
	TryUnify     func(a, b types.Type) bool

	// RegisterStuckFn, when non-nil, registers a stuck type family
	// application as a solver constraint for later re-activation.
	RegisterStuckFn func(name string, args []types.Type, resultKind types.Type, s span.Span) *types.TyMeta

	// tfCache is the per-top-level-call cycle-breaking cache for type
	// family reduction. Used by reduceFamilyAppsN to detect and short-
	// circuit recursive reductions like `Grow a = Pair (Grow a) (Grow a)`.
	//
	// Lifecycle: reduceFamilyApps saves the prior tfCache, sets it to nil,
	// runs the reduction, then restores the saved value. Re-entry from
	// unification (which calls FamilyReducer recursively via normalize)
	// is therefore safe — each top-level reduceFamilyApps call gets its
	// own cycle-breaking scope without interfering with outer calls.
	//
	// The map itself is allocated lazily on the first family hit, so
	// reductions that touch no families pay zero map-allocation cost.
	// Avoiding the prior `&cache` local-variable pattern eliminates the
	// per-call heap escape that dominated alloc_objects on Prelude
	// (1.2M allocs, 14.81% of cold start) before this change.
	tfCache map[string]types.Type
}

// lookupFamily returns the TypeFamilyInfo for name via the LookupFamily callback.
func (e *ReduceEnv) lookupFamily(name string) (*env.TypeFamilyInfo, bool) {
	return e.LookupFamily(name)
}

// maxReductionTypeSize is the maximum allowed size (node count) of a type
// produced by type family reduction.
const maxReductionTypeSize = 10000

// MaxReductionWork is the default step budget for a single reduction pass.
// Exported for test configuration; runtime limit is set via Budget.SetTFStepLimit.
const MaxReductionWork = 50000

// ReduceAll resets the TF step counter and reduces all type family
// applications in a type.
// Intended to be installed as the unifier's FamilyReducer callback.
func (e *ReduceEnv) ReduceAll(t types.Type) types.Type {
	e.Budget.ResetTFSteps()
	return e.reduceFamilyApps(t)
}

// ReduceTyFamily attempts to reduce a saturated type family application.
// Returns (result, true) on success, or (nil, false) if stuck/no match.
func (e *ReduceEnv) ReduceTyFamily(name string, args []types.Type, s span.Span) (types.Type, bool) {
	if err := e.Budget.TFStep(); err != nil {
		e.AddError(diagnostic.ErrTypeFamilyReduction, s,
			"type family "+name+": reduction limit exceeded (possible infinite recursion or exponential growth)")
		return nil, false
	}
	// Builtin row-level type families.
	if result, ok := e.reduceBuiltinRowFamily(name, args, s); ok {
		return result, true
	}
	fam, ok := e.lookupFamily(name)
	if !ok {
		return nil, false
	}
	for _, eq := range fam.Equations {
		subst, result := e.MatchTyPatterns(eq.Patterns, args)
		switch result {
		case env.MatchSuccess:
			rhs, ok := e.safeSubstMany(eq.RHS, subst)
			if !ok || types.TypeSize(rhs, maxReductionTypeSize) > maxReductionTypeSize {
				e.AddError(diagnostic.ErrTypeFamilyReduction, s,
					"type family "+name+": result type too large (possible exponential growth)")
				return nil, false
			}
			return rhs, true
		case env.MatchFail:
			continue
		case env.MatchIndeterminate:
			return nil, false
		}
	}
	return nil, false
}

// MatchTyPatterns attempts to match type patterns against arguments.
func (e *ReduceEnv) MatchTyPatterns(patterns, args []types.Type) (map[string]types.Type, env.MatchResult) {
	if len(patterns) != len(args) {
		return nil, env.MatchFail
	}
	subst := make(map[string]types.Type)
	for i, pat := range patterns {
		result := e.MatchTyPattern(pat, args[i], subst)
		if result != env.MatchSuccess {
			return nil, result
		}
	}
	return subst, env.MatchSuccess
}

// MatchTyPattern matches a single type pattern against an argument.
func (e *ReduceEnv) MatchTyPattern(pat, arg types.Type, subst map[string]types.Type) env.MatchResult {
	arg = e.Unifier.Zonk(arg)

	switch p := pat.(type) {
	case *types.TyVar:
		if p.Name == "_" {
			return env.MatchSuccess
		}
		if existing, ok := subst[p.Name]; ok {
			if e.TypeOps.Equal(existing, arg) {
				return env.MatchSuccess
			}
			return env.MatchFail
		}
		subst[p.Name] = arg
		return env.MatchSuccess

	case *types.TyCon:
		switch a := arg.(type) {
		case *types.TyCon:
			if p.Name == a.Name {
				return env.MatchSuccess
			}
			return env.MatchFail
		case *types.TyMeta:
			return env.MatchIndeterminate
		default:
			return env.MatchFail
		}

	case *types.TyApp:
		switch a := arg.(type) {
		case *types.TyApp:
			r := e.MatchTyPattern(p.Fun, a.Fun, subst)
			if r != env.MatchSuccess {
				return r
			}
			return e.MatchTyPattern(p.Arg, a.Arg, subst)
		case *types.TyMeta:
			return env.MatchIndeterminate
		default:
			return env.MatchFail
		}

	default:
		return env.MatchFail
	}
}

// reduceFamilyApps walks a type and reduces any TyFamilyApp nodes
// or TyApp chains that form a saturated type family application.
//
// The cycle-breaking cache lives on the ReduceEnv (e.tfCache); this
// entry point saves the outer cache (in case of re-entry from a
// recursive normalize call inside the unifier) and restores it on
// return. Inner recursion uses reduceFamilyAppsN directly without
// disturbing the cache scope.
func (e *ReduceEnv) reduceFamilyApps(t types.Type) types.Type {
	saved := e.tfCache
	e.tfCache = nil
	result := e.reduceFamilyAppsN(t)
	e.tfCache = saved
	return result
}

func (e *ReduceEnv) reduceFamilyAppsN(t types.Type) types.Type {
	if err := e.Budget.TFStep(); err != nil {
		return t
	}
	// Case 1: explicit TyFamilyApp.
	if tf, ok := t.(*types.TyFamilyApp); ok {
		var newArgs []types.Type // nil until first change (lazy-init)
		for i, a := range tf.Args {
			rA := e.reduceFamilyAppsN(a)
			if newArgs == nil && rA != a {
				newArgs = make([]types.Type, len(tf.Args))
				copy(newArgs[:i], tf.Args[:i])
			}
			if newArgs != nil {
				newArgs[i] = rA
			}
		}
		rArgs := newArgs
		if rArgs == nil {
			rArgs = tf.Args
		}
		key := e.familyAppKey(tf.Name, rArgs)
		if e.tfCache != nil {
			if cached, ok := e.tfCache[key]; ok {
				return cached
			}
		}
		result, reduced := e.ReduceTyFamily(tf.Name, rArgs, tf.S)
		if reduced {
			// Plant sentinel BEFORE recursing into the RHS. If the same
			// family application appears during reduction of its own RHS
			// (e.g. Grow a = Pair (Grow a) (Grow a)), the sentinel is
			// returned instead of re-entering, breaking the exponential
			// blowup. The sentinel is the unreduced TyFamilyApp — a stuck
			// application, which is the correct semantics for a cycle.
			if e.tfCache == nil {
				e.tfCache = make(map[string]types.Type)
			}
			stuck := e.TypeOps.FamilyApp(tf.Name, rArgs, tf.Kind, tf.S)
			e.tfCache[key] = stuck
			r := e.reduceFamilyAppsN(result)
			e.tfCache[key] = r
			return r
		}
		if placeholder := e.registerStuckFamily(tf.Name, rArgs, tf.Kind, tf.S); placeholder != nil {
			return placeholder
		}
		return e.TypeOps.FamilyApp(tf.Name, rArgs, tf.Kind, tf.S)
	}
	// Case 2: TyApp chain with TyCon head that is a known type family.
	// Two-phase: first check head+arity (no alloc), then unwind only on hit.
	if app, ok := t.(*types.TyApp); ok {
		head, depth := types.AppSpineHead(t)
		if con, ok := head.(*types.TyCon); ok {
			if fam, ok := e.lookupFamily(con.Name); ok && len(fam.Params) == depth {
				_, args := types.UnwindApp(t)
				for i, a := range args {
					args[i] = e.reduceFamilyAppsN(a)
				}
				key := e.familyAppKey(con.Name, args)
				if e.tfCache != nil {
					if cached, ok := e.tfCache[key]; ok {
						return cached
					}
				}
				result, reduced := e.ReduceTyFamily(con.Name, args, t.Span())
				if reduced {
					if e.tfCache == nil {
						e.tfCache = make(map[string]types.Type)
					}
					stuck := e.TypeOps.FamilyApp(con.Name, args, fam.ResultKind, t.Span())
					e.tfCache[key] = stuck
					r := e.reduceFamilyAppsN(result)
					e.tfCache[key] = r
					return r
				}
				if placeholder := e.registerStuckFamily(con.Name, args, fam.ResultKind, t.Span()); placeholder != nil {
					return placeholder
				}
				return e.TypeOps.FamilyApp(con.Name, args, fam.ResultKind, t.Span())
			}
		}
		rFun := e.reduceFamilyAppsN(app.Fun)
		rArg := e.reduceFamilyAppsN(app.Arg)
		if rFun == app.Fun && rArg == app.Arg {
			return t
		}
		return e.TypeOps.App(rFun, rArg, app.S)
	}
	// Case 3: structural recursion into other type formers.
	// Inlined (not via MapType) to avoid closure heap-escape.
	// Fast path: skip if subtree is stable (no metas, no family apps).
	if !types.HasFamilyApp(t) {
		return t
	}
	return e.reduceChildren(t)
}

// reduceChildren applies reduceFamilyAppsN to each child of a composite type,
// reconstructing the node only when a child changes. Inlined equivalent of
// types.MapType to avoid closure heap allocation on every call.
//
// The cycle-breaking cache is read from e.tfCache (managed by reduceFamilyApps).
func (e *ReduceEnv) reduceChildren(t types.Type) types.Type {
	switch ty := t.(type) {
	case *types.TyArrow:
		rFrom := e.reduceFamilyAppsN(ty.From)
		rTo := e.reduceFamilyAppsN(ty.To)
		if rFrom == ty.From && rTo == ty.To {
			return t
		}
		return e.TypeOps.Arrow(rFrom, rTo, ty.S)
	case *types.TyForall:
		rKind := e.reduceFamilyAppsN(ty.Kind)
		rBody := e.reduceFamilyAppsN(ty.Body)
		if rKind == ty.Kind && rBody == ty.Body {
			return t
		}
		return e.TypeOps.Forall(ty.Var, rKind, rBody, ty.S)
	case *types.TyCBPV:
		rPre := e.reduceFamilyAppsN(ty.Pre)
		rPost := e.reduceFamilyAppsN(ty.Post)
		rResult := e.reduceFamilyAppsN(ty.Result)
		rGrade := ty.Grade
		if rGrade != nil {
			rGrade = e.reduceFamilyAppsN(rGrade)
		}
		if rPre == ty.Pre && rPost == ty.Post && rResult == ty.Result && rGrade == ty.Grade {
			return t
		}
		if ty.Tag == types.TagComp {
			return e.TypeOps.Comp(rPre, rPost, rResult, rGrade, ty.S)
		}
		if rGrade != nil {
			return e.TypeOps.ThunkGraded(rPre, rPost, rResult, rGrade, ty.S)
		}
		return e.TypeOps.Thunk(rPre, rPost, rResult, ty.S)
	case *types.TyEvidence:
		rConstraints := e.reduceFamilyAppsN(ty.Constraints)
		rBody := e.reduceFamilyAppsN(ty.Body)
		if rConstraints == ty.Constraints && rBody == ty.Body {
			return t
		}
		cr, ok := rConstraints.(*types.TyEvidenceRow)
		if !ok {
			cr = ty.Constraints
		}
		return e.TypeOps.EvidenceWrap(cr, rBody, ty.S)
	case *types.TyEvidenceRow:
		newEntries, changed := ty.Entries.MapChildren(func(child types.Type) types.Type {
			return e.reduceFamilyAppsN(child)
		})
		var tail types.Type
		if ty.IsOpen() {
			tail = e.reduceFamilyAppsN(ty.Tail)
			if tail != ty.Tail {
				changed = true
			}
		}
		if !changed {
			return t
		}
		return &types.TyEvidenceRow{Entries: newEntries, Tail: tail, Flags: types.EvidenceRowFlags(newEntries, tail), S: ty.S}
	default:
		return t
	}
}

// registerStuckFamily delegates to RegisterStuckFn when set.
// When nil (standalone usage without a solver), stuck applications
// are not tracked and the original TyFamilyApp is preserved as-is.
func (e *ReduceEnv) registerStuckFamily(name string, args []types.Type, resultKind types.Type, s span.Span) *types.TyMeta {
	if e.RegisterStuckFn != nil {
		return e.RegisterStuckFn(name, args, resultKind, s)
	}
	return nil
}

// safeSubstMany applies SubstMany, recovering specifically from
// `*types.DepthExceededError` panics. Returns (result, true) on success, or
// (nil, false) when the substitution would exceed `types.maxTraversalDepth`.
//
// The recover deliberately type-asserts on the depth-exceeded variant rather
// than catching all panics: a nil dereference or index-out-of-range from a
// programming bug should crash loudly, not be silently misclassified as a
// recoverable depth condition.
func (e *ReduceEnv) safeSubstMany(t types.Type, subs map[string]types.Type) (result types.Type, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, isDepth := r.(*types.DepthExceededError); isDepth {
				result, ok = nil, false
				return
			}
			panic(r) // re-panic unrelated panics (real bugs)
		}
	}()
	return e.TypeOps.SubstMany(t, subs, nil), true
}

// familyAppKey produces a structural cache key for a type family application.
func (e *ReduceEnv) familyAppKey(name string, args []types.Type) string {
	return e.TypeOps.TypeListKey(name, ' ', args)
}
