package family

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// VerifyInjectivity checks that a type family satisfies its declared
// injectivity annotation by pairwise equation comparison.
// For every pair of equations, if the RHSes can unify,
// the corresponding LHS patterns must also unify.
func (e *ReduceEnv) VerifyInjectivity(info *env.TypeFamilyInfo) {
	for i := 0; i < len(info.Equations); i++ {
		for j := i + 1; j < len(info.Equations); j++ {
			eqI := info.Equations[i]
			eqJ := info.Equations[j]

			// Build a single substitution per equation so that pattern
			// variables in the RHS and LHS share the same fresh metas.
			subsI := e.freshPatVarSubst(eqI.Patterns, info.Params)
			subsJ := e.freshPatVarSubst(eqJ.Patterns, info.Params)
			rhsI := types.SubstMany(eqI.RHS, subsI, nil)
			rhsJ := types.SubstMany(eqJ.RHS, subsJ, nil)
			patsI := substManyList(eqI.Patterns, subsI)
			patsJ := substManyList(eqJ.Patterns, subsJ)

			if e.TryUnify(rhsI, rhsJ) {
				allMatch := true
				for k := range patsI {
					if !e.TryUnify(patsI[k], patsJ[k]) {
						allMatch = false
						break
					}
				}
				if !allMatch {
					e.AddError(diagnostic.ErrInjectivity, eqJ.S,
						fmt.Sprintf("type family %s: injectivity violation between equations %d and %d",
							info.Name, i+1, j+1))
				}
			}
		}
	}
}

// freshPatVarSubst builds a substitution mapping each pattern variable to a
// fresh meta. The same map is used for both RHS and LHS patterns, ensuring
// the connection between pattern variables and their RHS occurrences is preserved.
func (e *ReduceEnv) freshPatVarSubst(patterns []types.Type, params []env.TFParam) map[string]types.Type {
	vars := CollectPatternVars(patterns)
	varKinds := collectPatternVarKinds(patterns, params)
	subs := make(map[string]types.Type, len(vars))
	for _, v := range vars {
		subs[v] = e.FreshMeta(varKinds[v])
	}
	return subs
}

// substManyList applies a substitution to each element of a type list.
func substManyList(ts []types.Type, subs map[string]types.Type) []types.Type {
	result := make([]types.Type, len(ts))
	for i, t := range ts {
		result[i] = types.SubstMany(t, subs, nil)
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
func collectPatternVarKinds(patterns []types.Type, params []env.TFParam) map[string]types.Type {
	result := make(map[string]types.Type)
	for i, p := range patterns {
		var paramKind types.Type
		if i < len(params) {
			paramKind = params[i].Kind
		} else {
			paramKind = types.TypeOfTypes
		}
		collectVarKindsRec(p, paramKind, result)
	}
	return result
}

func collectVarKindsRec(t types.Type, contextKind types.Type, result map[string]types.Type) {
	switch ty := t.(type) {
	case *types.TyVar:
		if ty.Name != "_" {
			if _, exists := result[ty.Name]; !exists {
				result[ty.Name] = contextKind
			}
		}
	case *types.TyApp:
		// When the context kind is a concrete arrow (k1 -> k2), the function
		// position expects (argKind -> contextKind) and the argument position
		// expects argKind = k1. Fall back to Type when the kind is not an arrow
		// (e.g., unsolved meta or a non-arrow kind).
		var argKind types.Type = types.TypeOfTypes
		if arr, ok := contextKind.(*types.TyArrow); ok {
			argKind = arr.From
		}
		funKind := &types.TyArrow{From: argKind, To: contextKind}
		collectVarKindsRec(ty.Fun, funKind, result)
		collectVarKindsRec(ty.Arg, argKind, result)
	}
}
