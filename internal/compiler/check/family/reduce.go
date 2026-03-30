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
	FreshMeta    func(k types.Type) *types.TyMeta
	AddError     func(code diagnostic.Code, s span.Span, msg string)
	TryUnify     func(a, b types.Type) bool

	// RegisterStuckFn, when non-nil, registers a stuck type family
	// application as a solver constraint for later re-activation.
	RegisterStuckFn func(name string, args []types.Type, resultKind types.Type, s span.Span) *types.TyMeta
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
			rhs := types.SubstMany(eq.RHS, subst)
			if types.TypeSize(rhs, maxReductionTypeSize) > maxReductionTypeSize {
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
			if types.Equal(existing, arg) {
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
func (e *ReduceEnv) reduceFamilyApps(t types.Type) types.Type {
	cache := make(map[string]types.Type)
	return e.reduceFamilyAppsN(t, cache)
}

func (e *ReduceEnv) reduceFamilyAppsN(t types.Type, cache map[string]types.Type) types.Type {
	if err := e.Budget.TFStep(); err != nil {
		return t
	}
	// Case 1: explicit TyFamilyApp.
	if tf, ok := t.(*types.TyFamilyApp); ok {
		var newArgs []types.Type // nil until first change (lazy-init)
		for i, a := range tf.Args {
			rA := e.reduceFamilyAppsN(a, cache)
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
		key := familyAppKey(tf.Name, rArgs)
		if cached, ok := cache[key]; ok {
			return cached
		}
		result, reduced := e.ReduceTyFamily(tf.Name, rArgs, tf.S)
		if reduced {
			// Plant sentinel BEFORE recursing into the RHS. If the same
			// family application appears during reduction of its own RHS
			// (e.g. Grow a = Pair (Grow a) (Grow a)), the sentinel is
			// returned instead of re-entering, breaking the exponential
			// blowup. The sentinel is the unreduced TyFamilyApp — a stuck
			// application, which is the correct semantics for a cycle.
			stuck := &types.TyFamilyApp{Name: tf.Name, Args: rArgs, Kind: tf.Kind, S: tf.S}
			cache[key] = stuck
			r := e.reduceFamilyAppsN(result, cache)
			cache[key] = r
			return r
		}
		if placeholder := e.registerStuckFamily(tf.Name, rArgs, tf.Kind, tf.S); placeholder != nil {
			return placeholder
		}
		return &types.TyFamilyApp{Name: tf.Name, Args: rArgs, Kind: tf.Kind, S: tf.S}
	}
	// Case 2: TyApp chain with TyCon head that is a known type family.
	if app, ok := t.(*types.TyApp); ok {
		head, args := types.UnwindApp(t)
		if con, ok := head.(*types.TyCon); ok {
			if fam, ok := e.lookupFamily(con.Name); ok && len(fam.Params) == len(args) {
				for i, a := range args {
					args[i] = e.reduceFamilyAppsN(a, cache)
				}
				key := familyAppKey(con.Name, args)
				if cached, ok := cache[key]; ok {
					return cached
				}
				result, reduced := e.ReduceTyFamily(con.Name, args, t.Span())
				if reduced {
					stuck := &types.TyFamilyApp{Name: con.Name, Args: args, Kind: fam.ResultKind, S: t.Span()}
					cache[key] = stuck
					r := e.reduceFamilyAppsN(result, cache)
					cache[key] = r
					return r
				}
				if placeholder := e.registerStuckFamily(con.Name, args, fam.ResultKind, t.Span()); placeholder != nil {
					return placeholder
				}
				return &types.TyFamilyApp{Name: con.Name, Args: args, Kind: fam.ResultKind, S: t.Span()}
			}
		}
		rFun := e.reduceFamilyAppsN(app.Fun, cache)
		rArg := e.reduceFamilyAppsN(app.Arg, cache)
		if rFun == app.Fun && rArg == app.Arg {
			return t
		}
		return &types.TyApp{Fun: rFun, Arg: rArg, S: app.S}
	}
	// Case 3: structural recursion into other type formers.
	// Fast path: skip if subtree is stable (no metas, no family apps).
	if !types.HasFamilyApp(t) {
		return t
	}
	return types.MapType(t, func(child types.Type) types.Type {
		return e.reduceFamilyAppsN(child, cache)
	})
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

// familyAppKey produces a structural cache key for a type family application.
func familyAppKey(name string, args []types.Type) string {
	return types.TypeListKey(name, ' ', args)
}
