package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// TypeFamilyInfo holds the elaborated information for a type family.
type TypeFamilyInfo struct {
	Name       string
	Params     []string
	ParamKinds []types.Kind
	ResultKind types.Kind
	ResultName string   // non-empty if injective
	Deps       []tfDep  // injectivity deps (elaborated)
	Equations  []tfEquation
	IsAssoc    bool // true if declared as associated type in a class
	ClassName  string // non-empty if IsAssoc
}

// tfDep is an elaborated functional dependency.
type tfDep struct {
	From string   // result variable name
	To   []string // determined parameter names
}

// tfEquation is an elaborated type family equation with resolved types.
type tfEquation struct {
	Patterns []types.Type // LHS patterns (resolved)
	RHS      types.Type   // RHS body (resolved)
	S        span.Span
}

// matchResult classifies the outcome of type-level pattern matching.
type matchResult int

const (
	matchSuccess       matchResult = iota // patterns matched, substitution available
	matchFail                             // patterns definitely do not match
	matchIndeterminate                    // cannot decide (unsolved metavariable vs concrete pattern)
)

// processTypeFamily kind-checks and registers a type family declaration.
func (ch *Checker) processTypeFamily(d *syntax.DeclTypeFamily) {
	// Check for duplicate.
	if _, dup := ch.families[d.Name]; dup {
		ch.addCodedError(errs.ErrDuplicateDecl, d.S,
			fmt.Sprintf("duplicate type family: %s", d.Name))
		return
	}
	if _, dup := ch.aliases[d.Name]; dup {
		ch.addCodedError(errs.ErrDuplicateDecl, d.S,
			fmt.Sprintf("type family %s conflicts with type alias of the same name", d.Name))
		return
	}

	// Resolve parameter kinds.
	var params []string
	var paramKinds []types.Kind
	for _, p := range d.Params {
		params = append(params, p.Name)
		paramKinds = append(paramKinds, ch.resolveKindExpr(p.Kind))
	}

	// Resolve result kind.
	resultKind := ch.resolveKindExpr(d.ResultKind)

	// Elaborate dependencies.
	var deps []tfDep
	for _, fd := range d.Deps {
		deps = append(deps, tfDep{From: fd.From, To: fd.To})
	}

	// Resolve equations.
	var equations []tfEquation
	for _, eq := range d.Equations {
		// Validate equation name matches family name.
		if eq.Name != d.Name {
			ch.addCodedError(errs.ErrTypeFamilyEquation, eq.S,
				fmt.Sprintf("equation name %s does not match type family %s", eq.Name, d.Name))
			continue
		}
		// Validate arity.
		if len(eq.Patterns) != len(params) {
			ch.addCodedError(errs.ErrTypeFamilyEquation, eq.S,
				fmt.Sprintf("type family %s expects %d arguments, equation has %d",
					d.Name, len(params), len(eq.Patterns)))
			continue
		}
		// Resolve patterns and RHS.
		resolvedPats := make([]types.Type, len(eq.Patterns))
		for i, pat := range eq.Patterns {
			resolvedPats[i] = ch.resolveTypeExpr(pat)
		}
		resolvedRHS := ch.resolveTypeExpr(eq.RHS)
		equations = append(equations, tfEquation{
			Patterns: resolvedPats,
			RHS:      resolvedRHS,
			S:        eq.S,
		})
	}

	info := &TypeFamilyInfo{
		Name:       d.Name,
		Params:     params,
		ParamKinds: paramKinds,
		ResultKind: resultKind,
		ResultName: d.ResultName,
		Deps:       deps,
		Equations:  equations,
	}

	// Verify injectivity if declared.
	if d.ResultName != "" {
		ch.verifyInjectivity(info)
	}

	ch.families[d.Name] = info
}

// maxReductionDepth is the fuel limit for type family reduction.
// Prevents non-termination in recursive type families.
const maxReductionDepth = 100

// reduceTyFamily attempts to reduce a saturated type family application.
// Returns (result, true) on success, or (nil, false) if stuck/no match.
// Uses a checker-level depth counter to bound recursive reductions.
// The counter is incremented on each successful reduction and never decremented
// within a single normalize() call — it resets at the normalize entry point.
func (ch *Checker) reduceTyFamily(name string, args []types.Type) (types.Type, bool) {
	ch.reductionDepth++
	if ch.reductionDepth > maxReductionDepth {
		ch.addCodedError(errs.ErrTypeFamilyReduction, span.Span{},
			fmt.Sprintf("type family %s: reduction depth limit exceeded (possible infinite recursion)", name))
		return nil, false
	}
	fam, ok := ch.families[name]
	if !ok {
		return nil, false
	}
	for _, eq := range fam.Equations {
		subst, result := ch.matchTyPatterns(eq.Patterns, args)
		switch result {
		case matchSuccess:
			rhs := eq.RHS
			for varName, repl := range subst {
				rhs = types.Subst(rhs, varName, repl)
			}
			return rhs, true
		case matchFail:
			continue
		case matchIndeterminate:
			return nil, false // stuck — do not try further equations
		}
	}
	return nil, false // no equation matched — stuck
}

// matchTyPatterns attempts to match type patterns against arguments.
// Returns a substitution and match result.
func (ch *Checker) matchTyPatterns(patterns, args []types.Type) (map[string]types.Type, matchResult) {
	subst := make(map[string]types.Type)
	for i, pat := range patterns {
		result := ch.matchTyPattern(pat, args[i], subst)
		if result != matchSuccess {
			return nil, result
		}
	}
	return subst, matchSuccess
}

// matchTyPattern matches a single type pattern against an argument.
func (ch *Checker) matchTyPattern(pat, arg types.Type, subst map[string]types.Type) matchResult {
	// Zonk the argument to resolve solved metavariables.
	arg = ch.unifier.Zonk(arg)

	switch p := pat.(type) {
	case *types.TyVar:
		// Wildcard-like: "_" is conventionally a TyVar named "_".
		if p.Name == "_" {
			return matchSuccess
		}
		// Pattern variable: bind or check consistency.
		if existing, ok := subst[p.Name]; ok {
			// Already bound — must be equal.
			if types.Equal(existing, arg) {
				return matchSuccess
			}
			return matchFail
		}
		subst[p.Name] = arg
		return matchSuccess

	case *types.TyCon:
		// Exact constructor match.
		switch a := arg.(type) {
		case *types.TyCon:
			if p.Name == a.Name {
				return matchSuccess
			}
			return matchFail
		case *types.TyMeta:
			return matchIndeterminate
		default:
			return matchFail
		}

	case *types.TyApp:
		// Decompose application: match head and argument recursively.
		switch a := arg.(type) {
		case *types.TyApp:
			r := ch.matchTyPattern(p.Fun, a.Fun, subst)
			if r != matchSuccess {
				return r
			}
			return ch.matchTyPattern(p.Arg, a.Arg, subst)
		case *types.TyMeta:
			return matchIndeterminate
		default:
			return matchFail
		}

	default:
		// Unsupported pattern form.
		return matchFail
	}
}

// verifyInjectivity checks that a type family satisfies its declared
// injectivity annotation by pairwise equation comparison.
// For every pair of equations, if the RHSes can unify,
// the corresponding LHS patterns must also unify.
//
// Pattern variables are instantiated as fresh metavariables before comparison,
// since TyVar (pattern variable) ≠ TyMeta (solvable) in unification.
func (ch *Checker) verifyInjectivity(info *TypeFamilyInfo) {
	for i := 0; i < len(info.Equations); i++ {
		for j := i + 1; j < len(info.Equations); j++ {
			eqI := info.Equations[i]
			eqJ := info.Equations[j]

			// Instantiate pattern variables as fresh metas for both equations.
			rhsI := ch.instantiatePatVars(eqI.RHS, eqI.Patterns)
			rhsJ := ch.instantiatePatVars(eqJ.RHS, eqJ.Patterns)
			patsI := ch.instantiatePatVarsList(eqI.Patterns)
			patsJ := ch.instantiatePatVarsList(eqJ.Patterns)

			// Trial unification on RHSes.
			if ch.tryUnify(rhsI, rhsJ) {
				// RHSes can unify — check that LHSes also unify.
				allMatch := true
				for k := range patsI {
					if !ch.tryUnify(patsI[k], patsJ[k]) {
						allMatch = false
						break
					}
				}
				if !allMatch {
					ch.addCodedError(errs.ErrInjectivity, eqJ.S,
						fmt.Sprintf("type family %s: injectivity violation between equations %d and %d",
							info.Name, i+1, j+1))
				}
			}
		}
	}
}

// instantiatePatVars replaces free TyVars (pattern variables) with fresh metas.
func (ch *Checker) instantiatePatVars(ty types.Type, patterns []types.Type) types.Type {
	vars := collectPatternVars(patterns)
	for _, v := range vars {
		ty = types.Subst(ty, v, ch.freshMeta(types.KType{}))
	}
	return ty
}

// instantiatePatVarsList replaces free TyVars in each pattern with fresh metas.
func (ch *Checker) instantiatePatVarsList(patterns []types.Type) []types.Type {
	vars := collectPatternVars(patterns)
	result := make([]types.Type, len(patterns))
	subs := make(map[string]types.Type, len(vars))
	for _, v := range vars {
		subs[v] = ch.freshMeta(types.KType{})
	}
	for i, p := range patterns {
		result[i] = types.SubstMany(p, subs)
	}
	return result
}

// collectPatternVars collects all TyVar names from type patterns.
func collectPatternVars(patterns []types.Type) []string {
	seen := make(map[string]bool)
	var result []string
	for _, p := range patterns {
		collectPatternVarsRec(p, seen, &result)
	}
	return result
}

func collectPatternVarsRec(t types.Type, seen map[string]bool, result *[]string) {
	switch ty := t.(type) {
	case *types.TyVar:
		if ty.Name != "_" && !seen[ty.Name] {
			seen[ty.Name] = true
			*result = append(*result, ty.Name)
		}
	case *types.TyApp:
		collectPatternVarsRec(ty.Fun, seen, result)
		collectPatternVarsRec(ty.Arg, seen, result)
	}
}

// lubPostStates computes the join of multiple post-states from case branches.
// When the LUB type family is available and multiplicities are in use,
// this performs field-by-field LUB. Currently falls back to unification
// (requiring equal post-states), which is the v0 behavior.
func (ch *Checker) lubPostStates(posts []types.Type, s span.Span) types.Type {
	if len(posts) == 0 {
		return ch.freshMeta(types.KRow{})
	}
	result := posts[0]
	for i := 1; i < len(posts); i++ {
		if _, ok := ch.families["LUB"]; ok {
			// TODO(Phase 6): implement field-by-field LUB using the LUB type family.
			// For now, fall through to unification.
		}
		if err := ch.unifier.Unify(result, posts[i]); err != nil {
			ch.addCodedError(errs.ErrTypeMismatch, s,
				fmt.Sprintf("divergent post-states in case branches: %s vs %s (requires LUB type family for automatic join)",
					types.Pretty(result), types.Pretty(posts[i])))
		}
	}
	return result
}

// checkMultiplicity validates multiplicity constraints in bind elaboration.
// When a capability in pre has a non-nil Mult:
//   - Linear: must appear in pre but NOT in post (consumed exactly once)
//   - Affine: may appear in pre but NOT in post (consumed at most once)
//   - Unrestricted (nil Mult): no constraint
//
// This is a foundation stub — full checking is activated when multiplicity
// annotation syntax is finalized (Phase 5d).
func (ch *Checker) checkMultiplicity(pre, post types.Type, s span.Span) {
	// TODO(Phase 5d): implement once annotation syntax is decided.
	// For now, the structural foundation (Mult field on RowField) is in place,
	// and row unification propagates Mult through unification.
	_ = pre
	_ = post
	_ = s
}

// applyFunDepImprovement uses functional dependencies to improve type inference.
// For each fundep a -> b in the class, if the "from" args are determined (no metas),
// search instances whose "from" positions match, and unify the "to" positions.
func (ch *Checker) applyFunDepImprovement(className string, args []types.Type) {
	classInfo, ok := ch.classes[className]
	if !ok || len(classInfo.FunDeps) == 0 {
		return
	}
	for _, fd := range classInfo.FunDeps {
		// Check if all "from" positions are determined (no unsolved metas after zonk).
		allDetermined := true
		for _, fromIdx := range fd.From {
			if fromIdx >= len(args) {
				allDetermined = false
				break
			}
			zonked := ch.unifier.Zonk(args[fromIdx])
			if _, isMeta := zonked.(*types.TyMeta); isMeta {
				allDetermined = false
				break
			}
		}
		if !allDetermined {
			continue
		}
		// Search instances: if "from" positions match, unify "to" positions.
		for _, inst := range ch.instancesByClass[className] {
			if len(inst.TypeArgs) != len(args) {
				continue
			}
			freshSubst := ch.freshInstanceSubst(inst)
			fromMatch := ch.withTrial(func() bool {
				for _, fromIdx := range fd.From {
					instArg := types.SubstMany(inst.TypeArgs[fromIdx], freshSubst)
					if err := ch.unifier.Unify(instArg, args[fromIdx]); err != nil {
						return false
					}
				}
				return true
			})
			if fromMatch {
				// "From" positions match — unify "to" positions.
				for _, toIdx := range fd.To {
					if toIdx >= len(args) {
						continue
					}
					instArg := ch.unifier.Zonk(types.SubstMany(inst.TypeArgs[toIdx], freshSubst))
					_ = ch.unifier.Unify(args[toIdx], instArg)
				}
				break // first matching instance wins
			}
		}
	}
}

// reduceFamilyInType reduces type family applications within a type.
// Used by exhaustiveness checking to resolve data family instances.
func (ch *Checker) reduceFamilyInType(t types.Type) types.Type {
	if ch.unifier.familyReducer != nil {
		return ch.unifier.familyReducer(t)
	}
	return t
}

// mangledDataFamilyName produces a mangled name for a data family instance.
// E.g., Elem applied to (List a) → "Elem$List".
func (ch *Checker) mangledDataFamilyName(familyName string, patterns []types.Type) string {
	name := familyName
	for _, p := range patterns {
		name += "$" + typeNameForMangling(p)
	}
	return name
}

// typeNameForMangling extracts a short name from a type for mangling purposes.
func typeNameForMangling(t types.Type) string {
	switch ty := t.(type) {
	case *types.TyCon:
		return ty.Name
	case *types.TyApp:
		head, _ := types.UnwindApp(t)
		return typeNameForMangling(head)
	case *types.TyVar:
		return ty.Name
	default:
		return "X"
	}
}

// installFamilyReducer sets the family reducer callback in the unifier.
func (ch *Checker) installFamilyReducer() {
	if len(ch.families) == 0 {
		return
	}
	ch.unifier.familyReducer = func(t types.Type) types.Type {
		ch.reductionDepth = 0 // reset per normalize() call
		return ch.reduceFamilyApps(t)
	}
}

// reduceFamilyApps walks a type and reduces any TyFamilyApp nodes
// or TyApp chains that form a saturated type family application.
func (ch *Checker) reduceFamilyApps(t types.Type) types.Type {
	// Bail out if we've exceeded the reduction depth (set by reduceTyFamily calls).
	if ch.reductionDepth > maxReductionDepth {
		return t
	}
	// Case 1: explicit TyFamilyApp.
	if tf, ok := t.(*types.TyFamilyApp); ok {
		result, reduced := ch.reduceTyFamily(tf.Name, tf.Args)
		if reduced {
			return ch.reduceFamilyApps(result)
		}
		return t
	}
	// Case 2: TyApp chain with TyCon head that is a known type family.
	// This arises from substitution (e.g., Elem c [c := List a]).
	if _, ok := t.(*types.TyApp); ok {
		head, args := types.UnwindApp(t)
		if con, ok := head.(*types.TyCon); ok {
			if fam, ok := ch.families[con.Name]; ok && len(fam.Params) == len(args) {
				result, reduced := ch.reduceTyFamily(con.Name, args)
				if reduced {
					return ch.reduceFamilyApps(result)
				}
				// Not reduced — rewrite to TyFamilyApp for cleaner types.
				return &types.TyFamilyApp{Name: con.Name, Args: args, Kind: fam.ResultKind, S: t.Span()}
			}
		}
	}
	return t
}
