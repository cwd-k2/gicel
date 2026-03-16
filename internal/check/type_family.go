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

// reduceTyFamily attempts to reduce a saturated type family application.
// Returns (result, true) on success, or (nil, false) if stuck/no match.
func (ch *Checker) reduceTyFamily(name string, args []types.Type) (types.Type, bool) {
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
func (ch *Checker) verifyInjectivity(info *TypeFamilyInfo) {
	for i := 0; i < len(info.Equations); i++ {
		for j := i + 1; j < len(info.Equations); j++ {
			eqI := info.Equations[i]
			eqJ := info.Equations[j]
			// Trial unification on RHSes.
			if ch.tryUnify(eqI.RHS, eqJ.RHS) {
				// RHSes can unify — check that LHSes also unify.
				allMatch := true
				for k := range eqI.Patterns {
					if !ch.tryUnify(eqI.Patterns[k], eqJ.Patterns[k]) {
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

// installFamilyReducer sets the family reducer callback in the unifier.
func (ch *Checker) installFamilyReducer() {
	if len(ch.families) == 0 {
		return
	}
	ch.unifier.familyReducer = func(t types.Type) types.Type {
		return ch.reduceFamilyApps(t)
	}
}

// reduceFamilyApps walks a type and reduces any TyFamilyApp nodes.
func (ch *Checker) reduceFamilyApps(t types.Type) types.Type {
	tf, ok := t.(*types.TyFamilyApp)
	if !ok {
		return t
	}
	result, reduced := ch.reduceTyFamily(tf.Name, tf.Args)
	if reduced {
		// Recursively reduce in case the result contains more family apps.
		return ch.reduceFamilyApps(result)
	}
	return t
}
