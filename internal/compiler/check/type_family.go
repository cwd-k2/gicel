package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/compiler/check/family"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Type aliases for family types used throughout check/.
type TypeFamilyInfo = family.TypeFamilyInfo
type TFParam = family.TFParam

// Lowercase aliases for types constructed in class.go/instance.go.
type tfDep = family.TFDep
type tfEquation = family.TFEquation

// collectPatternVars delegates to the family subpackage.
var collectPatternVars = family.CollectPatternVars

// verifyInjectivity delegates to the family subpackage.
func (ch *Checker) verifyInjectivity(info *TypeFamilyInfo) {
	ch.familyEnv().VerifyInjectivity(info)
}

// matchTyPatterns delegates to the family subpackage.
func (ch *Checker) matchTyPatterns(patterns, args []types.Type) (map[string]types.Type, family.MatchResult) {
	return ch.familyEnv().MatchTyPatterns(patterns, args)
}

// matchTyPattern delegates to the family subpackage.
func (ch *Checker) matchTyPattern(pat, arg types.Type, subst map[string]types.Type) family.MatchResult {
	return ch.familyEnv().MatchTyPattern(pat, arg, subst)
}

// processTypeFamily kind-checks and registers a type family declaration.
func (ch *Checker) processTypeFamily(d *syntax.DeclTypeFamily) {
	// Check for duplicate.
	if _, dup := ch.reg.families[d.Name]; dup {
		ch.addCodedError(diagnostic.ErrDuplicateDecl, d.S,
			fmt.Sprintf("duplicate type family: %s", d.Name))
		return
	}
	if _, dup := ch.reg.aliases[d.Name]; dup {
		ch.addCodedError(diagnostic.ErrDuplicateDecl, d.S,
			fmt.Sprintf("type family %s conflicts with type alias of the same name", d.Name))
		return
	}

	// Resolve parameter kinds.
	var params []TFParam
	for _, p := range d.Params {
		params = append(params, TFParam{Name: p.Name, Kind: ch.resolveKindExpr(p.Kind)})
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
		if eq.Name != d.Name {
			ch.addCodedError(diagnostic.ErrTypeFamilyEquation, eq.S,
				fmt.Sprintf("equation name %s does not match type family %s", eq.Name, d.Name))
			continue
		}
		if len(eq.Patterns) != len(params) {
			ch.addCodedError(diagnostic.ErrTypeFamilyEquation, eq.S,
				fmt.Sprintf("type family %s expects %d arguments, equation has %d",
					d.Name, len(params), len(eq.Patterns)))
			continue
		}
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
		ResultKind: resultKind,
		ResultName: d.ResultName,
		Deps:       deps,
		Equations:  equations,
	}

	// Verify injectivity if declared.
	if d.ResultName != "" {
		ch.familyEnv().VerifyInjectivity(info)
	}

	ch.reg.RegisterFamily(d.Name, info)
}

// familyEnv returns the cached family.ReduceEnv, constructing it on first use.
// Caching is safe because all fields are references to Checker state (pointers,
// maps, or method values), so mutations are visible through the cached object.
func (ch *Checker) familyEnv() *family.ReduceEnv {
	if ch.cachedFamilyEnv == nil {
		ch.cachedFamilyEnv = &family.ReduceEnv{
			Families:        ch.reg.families,
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

// installFamilyReducer sets the family reducer callback in the unifier.
func (ch *Checker) installFamilyReducer() {
	if len(ch.reg.families) == 0 {
		return
	}
	env := ch.familyEnv()
	ch.unifier.FamilyReducer = env.ReduceAll
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
func (ch *Checker) registerStuckViaInert(name string, args []types.Type, resultKind types.Kind, s span.Span) *types.TyMeta {
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
