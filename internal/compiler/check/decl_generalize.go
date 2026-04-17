package check

import (
	"fmt"
	"slices"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
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
func (ch *Checker) generalizeConstrained(ty types.Type, expr ir.Core, unresolved []*CtPlainClass, maxLevel int) (types.Type, ir.Core) {
	collectFn := func(tys ...types.Type) []metaInfo {
		return collectUnsolvedMetas(maxLevel, tys...)
	}
	return ch.generalizeWith(ty, expr, unresolved, collectFn)
}

// generalizeLocal replaces unsolved metavariables born after watermark
// with universally quantified type variables. Used for nested
// let-generalization in block expressions and do-block pure binds.
func (ch *Checker) generalizeLocal(ty types.Type, expr ir.Core, unresolved []*CtPlainClass, watermark int) (types.Type, ir.Core) {
	collectFn := func(tys ...types.Type) []metaInfo {
		return collectUnsolvedMetasAfter(watermark, tys...)
	}
	return ch.generalizeWith(ty, expr, unresolved, collectFn)
}

// generalizeWith is the shared implementation for generalizeConstrained and
// generalizeLocal. The collectFn parameter determines which metas to collect.
func (ch *Checker) generalizeWith(ty types.Type, expr ir.Core, unresolved []*CtPlainClass, collectFn func(...types.Type) []metaInfo) (types.Type, ir.Core) {
	typeMetas := collectFn(ty)
	// Build set of meta IDs that appear in the result type.
	typeMetaIDs := make(map[int]bool, len(typeMetas))
	for _, m := range typeMetas {
		typeMetaIDs[m.id] = true
	}

	// Also collect metas from unresolved constraint args.
	metas := typeMetas
	for _, uc := range unresolved {
		metas = append(metas, collectFn(uc.Args...)...)
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
	slices.SortFunc(unique, func(a, b metaInfo) int { return a.id - b.id })

	// Ambiguity check: reject type variables that appear only in constraints,
	// not in the result type. These cannot be determined at any call site.
	if len(unresolved) > 0 {
		for _, m := range unique {
			if !typeMetaIDs[m.id] {
				ch.addDiag(diagnostic.ErrAmbiguousType, unresolved[0].S,
					diagMsg("ambiguous type variable in constraint-only position (does not appear in the result type)"))
				break // report once
			}
		}
	}

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
		expr = &ir.Lam{Param: uc.Placeholder, Body: expr, Generated: ir.GenDict, S: uc.S}
		// Wrap type: ClassName args => ty
		constraints := types.SingleConstraint(uc.ClassName, zonkedArgs)
		ty = ch.typeOps.EvidenceWrap(constraints, ty, uc.S)
	}

	// Re-zonk recorded expression types while temp solutions
	// (meta → TyVar) are still active. This ensures that hover
	// entries for sub-expressions of this binding show type variable
	// names (a, b, ...) rather than unsolved meta placeholders (_).
	//
	// Safety: temp solutions live in the Unifier's tempSoln overlay,
	// separate from the permanent soln map. Zonk's path compression
	// is suppressed while the overlay is active, so transient TyVar
	// values cannot leak into permanent outer-scope solutions.
	if ch.config.HoverRecorder != nil {
		ch.config.HoverRecorder.Rezonk(ch.unifier.Zonk)
	}

	// Remove temporary solutions.
	for _, m := range unique {
		ch.unifier.RemoveTempSolution(m.id)
	}

	// Wrap in forall.
	for i := len(unique) - 1; i >= 0; i-- {
		kind := ch.unifier.Zonk(unique[i].kind)
		ty = ch.typeOps.Forall(unique[i].name, kind, ty, span.Span{})
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

// collectUnsolvedMetas walks one or more types and collects TyMeta nodes
// whose Level does not exceed maxLevel. Metas born in deeper scopes
// (Level > maxLevel) are excluded — they belong to an inner binding and
// must not be generalized at this level.
func collectUnsolvedMetas(maxLevel int, tys ...types.Type) []metaInfo {
	seen := make(map[int]bool)
	var result []metaInfo
	for _, ty := range tys {
		result = append(result, types.CollectTypes(ty, func(t types.Type) (metaInfo, bool) {
			if m, ok := t.(*types.TyMeta); ok && !seen[m.ID] && m.Level <= maxLevel {
				seen[m.ID] = true
				return metaInfo{id: m.ID, kind: m.Kind}, true
			}
			return metaInfo{}, false
		})...)
	}
	return result
}

// collectUnsolvedMetasAfter walks one or more types and collects TyMeta
// nodes whose ID exceeds watermark. Used for nested let-generalization:
// only metas born during a particular binding's inference are candidates.
func collectUnsolvedMetasAfter(watermark int, tys ...types.Type) []metaInfo {
	seen := make(map[int]bool)
	var result []metaInfo
	for _, ty := range tys {
		result = append(result, types.CollectTypes(ty, func(t types.Type) (metaInfo, bool) {
			if m, ok := t.(*types.TyMeta); ok && !seen[m.ID] && m.ID > watermark {
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
// TyEvidenceRow.Tail) are quantified as Row; all others as Type.
func quantifyFreeVars(ops *types.TypeOps, ty types.Type) types.Type {
	fv := ops.FreeVars(ty)
	if len(fv) == 0 {
		return ty
	}
	kinds := inferFreeVarKinds(ty, fv)
	vars := make([]string, 0, len(fv))
	for v := range fv {
		vars = append(vars, v)
	}
	slices.Sort(vars)
	for i := len(vars) - 1; i >= 0; i-- {
		k := kinds[vars[i]]
		if k == nil {
			k = types.TypeOfTypes
		}
		ty = ops.Forall(vars[i], k, ty, span.Span{})
	}
	return ty
}

// inferFreeVarKinds walks a type and determines the kind for each free variable
// based on the position where it appears. Variables in row positions get Row;
// variables in type positions get Type. If a variable appears in both, Row wins
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
			if tt.IsOpen() {
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
			if tt.IsGraded() {
				walkAsType(tt.Grade)
			}
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
			if tt.IsOpen() {
				walkAsRow(tt.Tail)
			}
		}
	}

	walkAsType(ty)
	return result
}
