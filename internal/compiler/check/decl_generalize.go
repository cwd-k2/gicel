package check

import (
	"fmt"
	"sort"

	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// generalizeConstrained replaces unsolved metavariables with universally
// quantified type variables and lifts unresolved constraints into qualified
// types (TyEvidence). Called for unannotated top-level bindings.
//
// Example: \x. x + x  with unresolved Num ?a
//
//	→ \ a. Num a => a -> a  with Core: \dict x. ...
func (ch *Checker) generalizeConstrained(ty types.Type, expr ir.Core, unresolved []*CtClass) (types.Type, ir.Core) {
	metas := collectUnsolvedMetas(ty)
	// Also collect metas from unresolved constraint args.
	for _, uc := range unresolved {
		metas = append(metas, collectUnsolvedMetas(uc.Args...)...)
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
		zonkedArgs := make([]types.Type, len(uc.Args))
		for i, a := range uc.Args {
			zonkedArgs[i] = ch.unifier.Zonk(a)
		}
		// Wrap Core: \placeholder. expr (placeholder becomes the dict param)
		expr = &ir.Lam{Param: uc.Placeholder, Body: expr, Generated: true, S: uc.S}
		// Wrap type: ClassName args => ty
		ty = &types.TyEvidence{
			Constraints: types.SingleConstraint(uc.ClassName, zonkedArgs),
			Body:        ty,
			S:           uc.S,
		}
	}

	// Remove temporary solutions.
	for _, m := range unique {
		ch.unifier.RemoveTempSolution(m.id)
	}

	// Wrap in forall.
	for i := len(unique) - 1; i >= 0; i-- {
		kind := ch.unifier.Zonk(unique[i].kind)
		ty = types.MkForall(unique[i].name, kind, ty)
	}

	return ty, expr
}

type metaInfo struct {
	id   int
	kind types.Type
	name string
}

func genVarName(i int) string {
	if i < 26 {
		return string(rune('a' + i))
	}
	return fmt.Sprintf("t%d", i-26)
}

// collectUnsolvedMetas walks one or more types and collects all TyMeta nodes.
// NOTE: currently all metas eligible for generalization are at level 0
// (top-level). If nested let-generalization is added, this should filter
// by meta.Level <= solver.level to avoid generalizing inner-level metas.
func collectUnsolvedMetas(tys ...types.Type) []metaInfo {
	seen := make(map[int]bool)
	var result []metaInfo
	for _, ty := range tys {
		result = append(result, types.CollectTypes(ty, func(t types.Type) (metaInfo, bool) {
			if m, ok := t.(*types.TyMeta); ok && !seen[m.ID] {
				seen[m.ID] = true
				return metaInfo{id: m.ID, kind: m.Kind}, true
			}
			return metaInfo{}, false
		})...)
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
			k = types.TypeOfTypes
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
func inferFreeVarKinds(ty types.Type, fv map[string]struct{}) map[string]types.Type {
	result := make(map[string]types.Type, len(fv))

	var walkAsRow func(types.Type)
	var walkAsType func(types.Type)

	markRow := func(name string) {
		if _, ok := fv[name]; ok {
			result[name] = types.TypeOfRows
		}
	}
	markType := func(name string) {
		if _, ok := fv[name]; ok {
			if _, already := result[name]; !already {
				result[name] = types.TypeOfTypes
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
