package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/check/family"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// Type aliases for family types used throughout check/.
type TypeFamilyInfo = family.TypeFamilyInfo
type TFParam = family.TFParam

// Lowercase aliases for types constructed in class.go/instance.go.
type tfDep = family.TFDep
type tfEquation = family.TFEquation

// matchResult aliases for test backward compatibility.
type matchResult = family.MatchResult

const (
	matchSuccess       = family.MatchSuccess
	matchFail          = family.MatchFail
	matchIndeterminate = family.MatchIndeterminate
)

// cloneFamilies delegates to the family subpackage.
var cloneFamilies = family.CloneFamilies

// collectPatternVars delegates to the family subpackage.
var collectPatternVars = family.CollectPatternVars

// maxReductionDepth re-exports the constant for test backward compatibility.
const maxReductionDepth = family.MaxReductionDepth

// verifyInjectivity delegates to the family subpackage.
func (ch *Checker) verifyInjectivity(info *TypeFamilyInfo) {
	ch.familyEnv().VerifyInjectivity(info)
}

// matchTyPatterns delegates to the family subpackage.
func (ch *Checker) matchTyPatterns(patterns, args []types.Type) (map[string]types.Type, matchResult) {
	return ch.familyEnv().MatchTyPatterns(patterns, args)
}

// matchTyPattern delegates to the family subpackage.
func (ch *Checker) matchTyPattern(pat, arg types.Type, subst map[string]types.Type) matchResult {
	return ch.familyEnv().MatchTyPattern(pat, arg, subst)
}

// processTypeFamily kind-checks and registers a type family declaration.
func (ch *Checker) processTypeFamily(d *syntax.DeclTypeFamily) {
	// Check for duplicate.
	if _, dup := ch.reg.families[d.Name]; dup {
		ch.addCodedError(errs.ErrDuplicateDecl, d.S,
			fmt.Sprintf("duplicate type family: %s", d.Name))
		return
	}
	if _, dup := ch.reg.aliases[d.Name]; dup {
		ch.addCodedError(errs.ErrDuplicateDecl, d.S,
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
			ch.addCodedError(errs.ErrTypeFamilyEquation, eq.S,
				fmt.Sprintf("equation name %s does not match type family %s", eq.Name, d.Name))
			continue
		}
		if len(eq.Patterns) != len(params) {
			ch.addCodedError(errs.ErrTypeFamilyEquation, eq.S,
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

	ch.reg.families[d.Name] = info
}

// familyEnv creates a family.ReduceEnv with the current Checker state.
func (ch *Checker) familyEnv() *family.ReduceEnv {
	return &family.ReduceEnv{
		Families:  ch.reg.families,
		Budget:    ch.budget,
		Unifier:   ch.unifier,
		Stuck:     &ch.stuckFamilies,
		FreshMeta: ch.freshMeta,
		AddError:  ch.addCodedError,
		TryUnify:  ch.tryUnify,
	}
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

// ProcessRework delegates to the family subpackage.
func (ch *Checker) ProcessRework() {
	ch.familyEnv().ProcessRework()
}

// mangledDataFamilyName produces a mangled name for a data family instance.
func (ch *Checker) mangledDataFamilyName(familyName string, patterns []types.Type) string {
	name := fmt.Sprintf("%s$$%d", familyName, len(patterns))
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
