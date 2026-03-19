package family

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/budget"
	"github.com/cwd-k2/gicel/internal/check/unify"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// ReduceEnv provides the checker capabilities needed for type family operations.
type ReduceEnv struct {
	Families  map[string]*TypeFamilyInfo
	Budget    *budget.Budget
	Unifier   *unify.Unifier
	Stuck     *StuckIndex
	FreshMeta func(k types.Kind) *types.TyMeta
	AddError  func(code errs.Code, s span.Span, msg string)
	TryUnify  func(a, b types.Type) bool

	// RegisterStuckFn, when non-nil, overrides the default StuckIndex
	// registration. Used by the worklist solver to create CtFunEq constraints
	// instead of StuckIndex entries.
	RegisterStuckFn func(name string, args []types.Type, resultKind types.Kind, s span.Span) *types.TyMeta
}

// maxReductionTypeSize is the maximum allowed size (node count) of a type
// produced by type family reduction.
const maxReductionTypeSize = 10000

// MaxReductionWork is the step budget for a single reduction pass.
// Prevents exponential blowup from families like Grow a = Pair (Grow a) (Grow a).
const MaxReductionWork = 50000

// maxReworkIterations bounds the number of rework processing iterations
// to prevent runaway loops from cascading re-activations.
const maxReworkIterations = 200

// ReduceAll resets the budget counters and reduces all type family
// applications in a type.
// Intended to be installed as the unifier's FamilyReducer callback.
func (e *ReduceEnv) ReduceAll(t types.Type) types.Type {
	e.Budget.ResetCounters()
	return e.reduceFamilyApps(t)
}

// ReduceTyFamily attempts to reduce a saturated type family application.
// Returns (result, true) on success, or (nil, false) if stuck/no match.
func (e *ReduceEnv) ReduceTyFamily(name string, args []types.Type, s span.Span) (types.Type, bool) {
	if err := e.Budget.Step(); err != nil {
		e.AddError(errs.ErrTypeFamilyReduction, s,
			fmt.Sprintf("type family %s: reduction limit exceeded (possible infinite recursion or exponential growth)", name))
		return nil, false
	}
	fam, ok := e.Families[name]
	if !ok {
		return nil, false
	}
	for _, eq := range fam.Equations {
		subst, result := e.MatchTyPatterns(eq.Patterns, args)
		switch result {
		case MatchSuccess:
			rhs := types.SubstMany(eq.RHS, subst)
			if types.TypeSize(rhs, maxReductionTypeSize) > maxReductionTypeSize {
				e.AddError(errs.ErrTypeFamilyReduction, s,
					fmt.Sprintf("type family %s: result type too large (possible exponential growth)", name))
				return nil, false
			}
			return rhs, true
		case MatchFail:
			continue
		case MatchIndeterminate:
			return nil, false
		}
	}
	return nil, false
}

// MatchTyPatterns attempts to match type patterns against arguments.
func (e *ReduceEnv) MatchTyPatterns(patterns, args []types.Type) (map[string]types.Type, MatchResult) {
	if len(patterns) != len(args) {
		return nil, MatchFail
	}
	subst := make(map[string]types.Type)
	for i, pat := range patterns {
		result := e.MatchTyPattern(pat, args[i], subst)
		if result != MatchSuccess {
			return nil, result
		}
	}
	return subst, MatchSuccess
}

// MatchTyPattern matches a single type pattern against an argument.
func (e *ReduceEnv) MatchTyPattern(pat, arg types.Type, subst map[string]types.Type) MatchResult {
	arg = e.Unifier.Zonk(arg)

	switch p := pat.(type) {
	case *types.TyVar:
		if p.Name == "_" {
			return MatchSuccess
		}
		if existing, ok := subst[p.Name]; ok {
			if types.Equal(existing, arg) {
				return MatchSuccess
			}
			return MatchFail
		}
		subst[p.Name] = arg
		return MatchSuccess

	case *types.TyCon:
		switch a := arg.(type) {
		case *types.TyCon:
			if p.Name == a.Name {
				return MatchSuccess
			}
			return MatchFail
		case *types.TyMeta:
			return MatchIndeterminate
		default:
			return MatchFail
		}

	case *types.TyApp:
		switch a := arg.(type) {
		case *types.TyApp:
			r := e.MatchTyPattern(p.Fun, a.Fun, subst)
			if r != MatchSuccess {
				return r
			}
			return e.MatchTyPattern(p.Arg, a.Arg, subst)
		case *types.TyMeta:
			return MatchIndeterminate
		default:
			return MatchFail
		}

	default:
		return MatchFail
	}
}

// reduceFamilyApps walks a type and reduces any TyFamilyApp nodes
// or TyApp chains that form a saturated type family application.
func (e *ReduceEnv) reduceFamilyApps(t types.Type) types.Type {
	cache := make(map[string]types.Type)
	return e.reduceFamilyAppsN(t, cache)
}

func (e *ReduceEnv) reduceFamilyAppsN(t types.Type, cache map[string]types.Type) types.Type {
	if err := e.Budget.Step(); err != nil {
		return t
	}
	// Case 1: explicit TyFamilyApp.
	if tf, ok := t.(*types.TyFamilyApp); ok {
		args := make([]types.Type, len(tf.Args))
		for i, a := range tf.Args {
			args[i] = e.reduceFamilyAppsN(a, cache)
		}
		key := familyAppKey(tf.Name, args)
		if cached, ok := cache[key]; ok {
			return cached
		}
		result, reduced := e.ReduceTyFamily(tf.Name, args, tf.S)
		if reduced {
			r := e.reduceFamilyAppsN(result, cache)
			cache[key] = r
			return r
		}
		if placeholder := e.registerStuckFamily(tf.Name, args, tf.Kind, tf.S); placeholder != nil {
			return placeholder
		}
		return &types.TyFamilyApp{Name: tf.Name, Args: args, Kind: tf.Kind, S: tf.S}
	}
	// Case 2: TyApp chain with TyCon head that is a known type family.
	if app, ok := t.(*types.TyApp); ok {
		head, args := types.UnwindApp(t)
		if con, ok := head.(*types.TyCon); ok {
			if fam, ok := e.Families[con.Name]; ok && len(fam.Params) == len(args) {
				for i, a := range args {
					args[i] = e.reduceFamilyAppsN(a, cache)
				}
				key := familyAppKey(con.Name, args)
				if cached, ok := cache[key]; ok {
					return cached
				}
				result, reduced := e.ReduceTyFamily(con.Name, args, t.Span())
				if reduced {
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
	return types.MapType(t, func(child types.Type) types.Type {
		return e.reduceFamilyAppsN(child, cache)
	})
}

// registerStuckFamily checks whether a stuck family application has unsolved
// meta arguments and, if so, registers it in the stuck family index (or
// delegates to RegisterStuckFn when the worklist solver is active).
func (e *ReduceEnv) registerStuckFamily(name string, args []types.Type, resultKind types.Kind, s span.Span) *types.TyMeta {
	if e.RegisterStuckFn != nil {
		return e.RegisterStuckFn(name, args, resultKind, s)
	}
	blocking := e.Unifier.CollectBlockingMetas(args)
	if len(blocking) == 0 {
		return nil
	}
	resultMeta := e.FreshMeta(resultKind)
	entry := &stuckEntry{
		familyName: name,
		args:       args,
		span:       s,
		resultMeta: resultMeta,
		blockingOn: blocking,
	}
	e.Stuck.register(entry)
	return resultMeta
}

// ProcessRework attempts to reduce stuck type family applications that were
// unblocked by recent meta solutions. On success, the result meta is unified
// with the reduced type. Entries that remain stuck are re-registered.
func (e *ReduceEnv) ProcessRework() {
	e.Budget.ResetCounters()
	for range maxReworkIterations {
		entries := e.Stuck.drainRework()
		if len(entries) == 0 {
			return
		}
		seen := make(map[*stuckEntry]bool, len(entries))
		for _, entry := range entries {
			if seen[entry] {
				continue
			}
			seen[entry] = true
			zonked := make([]types.Type, len(entry.args))
			for i, a := range entry.args {
				zonked[i] = e.Unifier.Zonk(a)
			}
			result, reduced := e.ReduceTyFamily(entry.familyName, zonked, entry.span)
			if reduced {
				_ = e.Unifier.Unify(entry.resultMeta, result)
				continue
			}
			blocking := e.Unifier.CollectBlockingMetas(zonked)
			if len(blocking) == 0 {
				continue
			}
			entry.args = zonked
			entry.blockingOn = blocking
			e.Stuck.register(entry)
		}
	}
}

// familyAppKey produces a structural cache key for a type family application.
func familyAppKey(name string, args []types.Type) string {
	var b strings.Builder
	b.WriteString(name)
	for _, a := range args {
		b.WriteByte(' ')
		types.WriteTypeKey(&b, a)
	}
	return b.String()
}
