package check

import (
	"fmt"
	"sort"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/types"
)

// generalizeConstrained replaces unsolved metavariables with universally
// quantified type variables and lifts unresolved constraints into qualified
// types (TyEvidence). Called for unannotated top-level bindings.
//
// Example: \x. x + x  with unresolved Num ?a
//
//	→ \ a. Num a => a -> a  with Core: \dict x. ...
func (ch *Checker) generalizeConstrained(ty types.Type, expr core.Core, unresolved []deferredConstraint) (types.Type, core.Core) {
	metas := collectUnsolvedMetas(ty)
	// Also collect metas from unresolved constraint args.
	for _, uc := range unresolved {
		metas = append(metas, collectUnsolvedMetas(uc.args...)...)
	}
	if len(metas) == 0 && len(unresolved) == 0 {
		return ty, expr
	}

	// Deduplicate and sort by ID for deterministic naming.
	seen := make(map[int]bool)
	var unique []metaInfo
	for _, m := range metas {
		if !seen[m.id] {
			seen[m.id] = true
			unique = append(unique, m)
		}
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i].id < unique[j].id })

	// Register temporary solutions: meta → TyVar.
	for i, m := range unique {
		name := genVarName(i)
		unique[i].name = name
		ch.unifier.InstallTempSolution(m.id, &types.TyVar{Name: name})
	}

	// Re-zonk type to replace metas with vars.
	ty = ch.unifier.Zonk(ty)

	// Wrap unresolved constraints into the type and Core expression.
	// Each constraint becomes a dict parameter (lambda in Core, TyEvidence in type).
	for _, uc := range unresolved {
		zonkedArgs := make([]types.Type, len(uc.args))
		for i, a := range uc.args {
			zonkedArgs[i] = ch.unifier.Zonk(a)
		}
		// Wrap Core: \placeholder. expr (placeholder becomes the dict param)
		expr = &core.Lam{Param: uc.placeholder, Body: expr, S: uc.s}
		// Wrap type: ClassName args => ty
		ty = &types.TyEvidence{
			Constraints: types.SingleConstraint(uc.className, zonkedArgs),
			Body:        ty,
			S:           uc.s,
		}
	}

	// Remove temporary solutions.
	for _, m := range unique {
		ch.unifier.RemoveTempSolution(m.id)
	}

	// Wrap in forall.
	for i := len(unique) - 1; i >= 0; i-- {
		kind := ch.unifier.ZonkKind(unique[i].kind)
		ty = types.MkForall(unique[i].name, kind, ty)
	}

	return ty, expr
}

type metaInfo struct {
	id   int
	kind types.Kind
	name string
}

func genVarName(i int) string {
	if i < 26 {
		return string(rune('a' + i))
	}
	return fmt.Sprintf("t%d", i-26)
}

// collectUnsolvedMetas walks one or more types and collects all TyMeta nodes.
func collectUnsolvedMetas(tys ...types.Type) []metaInfo {
	var result []metaInfo
	seen := make(map[int]bool)
	var walk func(types.Type)
	walk = func(t types.Type) {
		switch ty := t.(type) {
		case *types.TyMeta:
			if !seen[ty.ID] {
				seen[ty.ID] = true
				result = append(result, metaInfo{id: ty.ID, kind: ty.Kind})
			}
		case *types.TyApp:
			walk(ty.Fun)
			walk(ty.Arg)
		case *types.TyArrow:
			walk(ty.From)
			walk(ty.To)
		case *types.TyForall:
			walk(ty.Body)
		case *types.TyEvidence:
			if ty.Constraints != nil {
				for _, ch := range ty.Constraints.Children() {
					walk(ch)
				}
			}
			walk(ty.Body)
		case *types.TyCBPV:
			walk(ty.Pre)
			walk(ty.Post)
			walk(ty.Result)
		case *types.TyFamilyApp:
			for _, a := range ty.Args {
				walk(a)
			}
		case *types.TyEvidenceRow:
			for _, ch := range ty.Children() {
				walk(ch)
			}
		}
	}
	for _, ty := range tys {
		walk(ty)
	}
	return result
}

// quantifyFreeVars wraps free type variables in implicit forall quantifiers.
// This implements Haskell-style implicit universal quantification for type annotations:
// `f :: List a -> Int` is treated as `f :: \ a. List a -> Int`.
// Kind inference: variables appearing in row positions (TyCBPV.Pre/Post,
// TyEvidenceRow.Tail) are quantified as KRow; all others as KType.
func quantifyFreeVars(ty types.Type) types.Type {
	fv := types.FreeVars(ty)
	if len(fv) == 0 {
		return ty
	}
	kinds := inferFreeVarKinds(ty, fv)
	vars := make([]string, 0, len(fv))
	for v := range fv {
		vars = append(vars, v)
	}
	sort.Strings(vars)
	for i := len(vars) - 1; i >= 0; i-- {
		k := kinds[vars[i]]
		if k == nil {
			k = types.KType{}
		}
		ty = types.MkForall(vars[i], k, ty)
	}
	return ty
}

// inferFreeVarKinds walks a type and determines the kind for each free variable
// based on the position where it appears. Variables in row positions get KRow;
// variables in type positions get KType. If a variable appears in both, KRow wins
// (a row variable used where a type is expected is more likely a mistake caught by
// the kind checker, whereas a type variable in a row position is the real bug).
func inferFreeVarKinds(ty types.Type, fv map[string]struct{}) map[string]types.Kind {
	result := make(map[string]types.Kind, len(fv))

	var walkAsRow func(types.Type)
	var walkAsType func(types.Type)

	markRow := func(name string) {
		if _, ok := fv[name]; ok {
			result[name] = types.KRow{}
		}
	}
	markType := func(name string) {
		if _, ok := fv[name]; ok {
			if _, already := result[name]; !already {
				result[name] = types.KType{}
			}
		}
	}

	walkAsRow = func(t types.Type) {
		if t == nil {
			return
		}
		switch tt := t.(type) {
		case *types.TyVar:
			markRow(tt.Name)
		case *types.TyEvidenceRow:
			for _, ch := range tt.Entries.AllChildren() {
				walkAsType(ch)
			}
			if tt.Tail != nil {
				walkAsRow(tt.Tail)
			}
		default:
			walkAsType(t)
		}
	}

	walkAsType = func(t types.Type) {
		if t == nil {
			return
		}
		switch tt := t.(type) {
		case *types.TyVar:
			markType(tt.Name)
		case *types.TyApp:
			walkAsType(tt.Fun)
			walkAsType(tt.Arg)
		case *types.TyArrow:
			walkAsType(tt.From)
			walkAsType(tt.To)
		case *types.TyForall:
			walkAsType(tt.Body)
		case *types.TyCBPV:
			walkAsRow(tt.Pre)
			walkAsRow(tt.Post)
			walkAsType(tt.Result)
		case *types.TyEvidence:
			walkAsRow(tt.Constraints)
			walkAsType(tt.Body)
		case *types.TyFamilyApp:
			for _, a := range tt.Args {
				walkAsType(a)
			}
		case *types.TyEvidenceRow:
			for _, ch := range tt.Entries.AllChildren() {
				walkAsType(ch)
			}
			if tt.Tail != nil {
				walkAsRow(tt.Tail)
			}
		}
	}

	walkAsType(ty)
	return result
}
