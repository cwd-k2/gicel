package family

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/types"
)

// VerifyInjectivity checks that a type family satisfies its declared
// injectivity annotation by pairwise equation comparison.
// For every pair of equations, if the RHSes can unify,
// the corresponding LHS patterns must also unify.
func (e *ReduceEnv) VerifyInjectivity(info *TypeFamilyInfo) {
	for i := 0; i < len(info.Equations); i++ {
		for j := i + 1; j < len(info.Equations); j++ {
			eqI := info.Equations[i]
			eqJ := info.Equations[j]

			rhsI := e.instantiatePatVars(eqI.RHS, eqI.Patterns, info.Params)
			rhsJ := e.instantiatePatVars(eqJ.RHS, eqJ.Patterns, info.Params)
			patsI := e.instantiatePatVarsList(eqI.Patterns, info.Params)
			patsJ := e.instantiatePatVarsList(eqJ.Patterns, info.Params)

			if e.TryUnify(rhsI, rhsJ) {
				allMatch := true
				for k := range patsI {
					if !e.TryUnify(patsI[k], patsJ[k]) {
						allMatch = false
						break
					}
				}
				if !allMatch {
					e.AddError(errs.ErrInjectivity, eqJ.S,
						fmt.Sprintf("type family %s: injectivity violation between equations %d and %d",
							info.Name, i+1, j+1))
				}
			}
		}
	}
}

// instantiatePatVars replaces free TyVars (pattern variables) with fresh metas.
func (e *ReduceEnv) instantiatePatVars(ty types.Type, patterns []types.Type, params []TFParam) types.Type {
	varKinds := collectPatternVarKinds(patterns, params)
	for _, v := range CollectPatternVars(patterns) {
		kind := varKinds[v]
		ty = types.Subst(ty, v, e.FreshMeta(kind))
	}
	return ty
}

// instantiatePatVarsList replaces free TyVars in each pattern with fresh metas.
func (e *ReduceEnv) instantiatePatVarsList(patterns []types.Type, params []TFParam) []types.Type {
	vars := CollectPatternVars(patterns)
	varKinds := collectPatternVarKinds(patterns, params)
	result := make([]types.Type, len(patterns))
	subs := make(map[string]types.Type, len(vars))
	for _, v := range vars {
		kind := varKinds[v]
		subs[v] = e.FreshMeta(kind)
	}
	for i, p := range patterns {
		result[i] = types.SubstMany(p, subs)
	}
	return result
}

// CollectPatternVars collects all TyVar names from type patterns.
func CollectPatternVars(patterns []types.Type) []string {
	seen := make(map[string]bool)
	var result []string
	for _, p := range patterns {
		CollectPatternVarsRec(p, seen, &result)
	}
	return result
}

func CollectPatternVarsRec(t types.Type, seen map[string]bool, result *[]string) {
	switch ty := t.(type) {
	case *types.TyVar:
		if ty.Name != "_" && !seen[ty.Name] {
			seen[ty.Name] = true
			*result = append(*result, ty.Name)
		}
	case *types.TyApp:
		CollectPatternVarsRec(ty.Fun, seen, result)
		CollectPatternVarsRec(ty.Arg, seen, result)
	}
}

// collectPatternVarKinds maps each pattern variable to its kind based on the
// parameter position where it first appears.
func collectPatternVarKinds(patterns []types.Type, params []TFParam) map[string]types.Kind {
	result := make(map[string]types.Kind)
	for i, p := range patterns {
		var paramKind types.Kind
		if i < len(params) {
			paramKind = params[i].Kind
		} else {
			paramKind = types.KType{}
		}
		collectVarKindsRec(p, paramKind, result)
	}
	return result
}

func collectVarKindsRec(t types.Type, contextKind types.Kind, result map[string]types.Kind) {
	switch ty := t.(type) {
	case *types.TyVar:
		if ty.Name != "_" {
			if _, exists := result[ty.Name]; !exists {
				result[ty.Name] = contextKind
			}
		}
	case *types.TyApp:
		funKind := &types.KArrow{From: types.KType{}, To: contextKind}
		collectVarKindsRec(ty.Fun, funKind, result)
		collectVarKindsRec(ty.Arg, types.KType{}, result)
	}
}
