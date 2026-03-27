package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/compiler/check/family"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Type aliases for env types used throughout check/.
type TypeFamilyInfo = env.TypeFamilyInfo
type TFParam = env.TFParam

// Lowercase alias for types constructed in class.go/instance.go.
type tfEquation = env.TFEquation

// collectPatternVars delegates to the family subpackage.
var collectPatternVars = family.CollectPatternVars

// verifyInjectivity delegates to the family subpackage.
func (ch *Checker) verifyInjectivity(info *TypeFamilyInfo) {
	ch.familyEnv().VerifyInjectivity(info)
}

// matchTyPatterns delegates to the family subpackage.
func (ch *Checker) matchTyPatterns(patterns, args []types.Type) (map[string]types.Type, env.MatchResult) {
	return ch.familyEnv().MatchTyPatterns(patterns, args)
}

// matchTyPattern delegates to the family subpackage.
func (ch *Checker) matchTyPattern(pat, arg types.Type, subst map[string]types.Type) env.MatchResult {
	return ch.familyEnv().MatchTyPattern(pat, arg, subst)
}

// processTypeFamilyDecl kind-checks and registers a closed type family from
// its unified syntax components. Called from processTypeFamilyFromAlias after
// extracting the case expression from the alias body.
func (ch *Checker) processTypeFamilyDecl(
	name string,
	syntaxParams []syntax.TyBinder,
	kindAnn syntax.TypeExpr,
	alts []syntax.TyAlt,
	s span.Span,
) {
	// Check for duplicate.
	if _, dup := ch.reg.LookupFamily(name); dup {
		ch.addCodedError(diagnostic.ErrDuplicateDecl, s,
			fmt.Sprintf("duplicate type family: %s", name))
		return
	}
	if _, dup := ch.reg.LookupAlias(name); dup {
		ch.addCodedError(diagnostic.ErrDuplicateDecl, s,
			fmt.Sprintf("type family %s conflicts with type alias of the same name", name))
		return
	}

	// Resolve parameter kinds.
	var params []TFParam
	for _, p := range syntaxParams {
		params = append(params, TFParam{Name: p.Name, Kind: ch.resolveKindExpr(p.Kind)})
	}

	// Resolve result kind.
	resultKind := ch.resolveKindExpr(kindAnn)

	// Resolve equations from case alternatives.
	var equations []tfEquation
	for _, alt := range alts {
		patCount := 0
		if alt.Pattern != nil {
			patCount = countTupleArity(alt.Pattern)
		}
		patterns := extractTFPatterns(alt.Pattern, patCount)

		if len(patterns) != len(params) {
			ch.addCodedError(diagnostic.ErrTypeFamilyEquation, alt.S,
				fmt.Sprintf("type family %s expects %d arguments, equation has %d",
					name, len(params), len(patterns)))
			continue
		}
		resolvedPats := make([]types.Type, len(patterns))
		for i, pat := range patterns {
			resolvedPats[i] = ch.resolveTypeExpr(pat)
		}
		resolvedRHS := ch.resolveTypeExpr(alt.Body)
		equations = append(equations, tfEquation{
			Patterns: resolvedPats,
			RHS:      resolvedRHS,
			S:        alt.S,
		})
	}

	info := &TypeFamilyInfo{
		Name:       name,
		Params:     params,
		ResultKind: resultKind,
		Equations:  equations,
	}

	ch.reg.RegisterFamily(name, info)
}

// familyEnv returns the cached family.ReduceEnv, constructing it on first use.
// Caching is safe because all fields are references to Checker state (pointers,
// maps, or method values), so mutations are visible through the cached object.
func (ch *Checker) familyEnv() *family.ReduceEnv {
	if ch.cachedFamilyEnv == nil {
		ch.cachedFamilyEnv = &family.ReduceEnv{
			LookupFamily:    ch.lookupFamily,
			Budget:          ch.budget,
			Unifier:         ch.unifier,
			FreshMeta:       ch.freshMeta,
			AddError:        ch.addCodedError,
			TryUnify:        ch.tryUnify,
			RegisterStuckFn: ch.registerStuckViaInert,
		}
	}
	return ch.cachedFamilyEnv
}

// installFamilyReducer registers internal type families and sets the
// family reducer callback in the unifier. Grade algebra is defined
// exclusively by the Prelude's GradeAlgebra class instances.
func (ch *Checker) installFamilyReducer() {
	ch.registerBuiltinRowFamilies()
	if len(ch.reg.families) == 0 {
		return
	}
	env := ch.familyEnv()
	ch.unifier.FamilyReducer = env.ReduceAll
}

// registerBuiltinRowFamilies registers type family info for builtin row-level
// type families (Merge, etc.). The actual reduction logic is in family/row_family.go;
// the registration here makes the family names known to the checker so they can
// appear in type signatures and be resolved as type family applications.
func (ch *Checker) registerBuiltinRowFamilies() {
	// Merge :: Row -> Row -> Row
	if _, exists := ch.reg.LookupFamily("Merge"); !exists {
		ch.reg.RegisterFamily("Merge", &TypeFamilyInfo{
			Name: "Merge",
			Params: []TFParam{
				{Name: "r1", Kind: types.TypeOfRows},
				{Name: "r2", Kind: types.TypeOfRows},
			},
			ResultKind: types.TypeOfRows,
		})
	}
}

// reduceFamilyInType reduces type family applications within a type.
// Used by exhaustiveness checking to resolve data family instances.
func (ch *Checker) reduceFamilyInType(t types.Type) types.Type {
	if ch.unifier.FamilyReducer != nil {
		return ch.unifier.FamilyReducer(t)
	}
	return t
}

// reduceTyFamily delegates to the family subpackage.
func (ch *Checker) reduceTyFamily(name string, args []types.Type, s span.Span) (types.Type, bool) {
	return ch.familyEnv().ReduceTyFamily(name, args, s)
}

// registerStuckViaInert registers a stuck type family application as a
// CtFunEq constraint in the inert set for later re-activation via OnSolve.
func (ch *Checker) registerStuckViaInert(name string, args []types.Type, resultKind types.Type, s span.Span) *types.TyMeta {
	blocking := ch.unifier.CollectBlockingMetas(args)
	if len(blocking) == 0 {
		return nil
	}
	resultMeta := ch.freshMeta(resultKind)
	ct := &CtFunEq{
		FamilyName: name,
		Args:       args,
		ResultMeta: resultMeta,
		BlockingOn: blocking,
		S:          s,
	}
	ch.solver.RegisterStuckFunEq(ct)
	return resultMeta
}

// mangledDataFamilyName produces a mangled name for a data family instance.
// Uses WriteTypeKey for each pattern to guarantee injectivity: distinct
// pattern lists always produce distinct mangled names.
func (ch *Checker) mangledDataFamilyName(familyName string, patterns []types.Type) string {
	var b strings.Builder
	b.WriteString(familyName)
	fmt.Fprintf(&b, "$$%d", len(patterns))
	for _, p := range patterns {
		b.WriteByte('$')
		types.WriteTypeKey(&b, p)
	}
	return b.String()
}
