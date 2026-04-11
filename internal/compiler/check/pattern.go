package check

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// patternResult holds the outputs of pattern checking.
type patternResult struct {
	Pattern        ir.Pattern
	Bindings       map[string]types.Type
	SkolemIDs      map[int]string
	GivenEqs       map[int]types.Type // GADT given equalities: skolem ID → type
	HasEvidence    bool
	VariantFieldTy types.Type // non-nil for label patterns: the Lookup result for this branch
}

// mergePatternBindings merges a child pattern's bindings into a parent
// binding map, diagnosing duplicate names. Patterns must be linear: each
// bound variable may appear at most once across the whole pattern. The
// diagnostic points at sp (the child pattern's span). Generated names
// (dict params from constraint elaboration) are allowed to duplicate
// themselves because the freshDictName generator guarantees uniqueness;
// in practice those never collide with user names.
func (ch *Checker) mergePatternBindings(parent map[string]types.Type, child map[string]types.Type, sp span.Span) {
	for name, ty := range child {
		if _, exists := parent[name]; exists {
			ch.addDiag(diagnostic.ErrDuplicateLabel, sp,
				diagFmt{Format: "variable %q is bound more than once in the same pattern", Args: []any{name}})
			continue
		}
		parent[name] = ty
	}
}

func (ch *Checker) checkPattern(pat syntax.Pattern, scrutTy types.Type) patternResult {
	switch p := pat.(type) {
	case *syntax.PatVar:
		// Record the pattern variable type for hover.
		if !p.Generated {
			ch.recordType(p.S, &scrutTy)
		}
		return patternResult{
			Pattern:  &ir.PVar{Name: p.Name, Generated: syntaxGenKind(p.Generated), S: p.S},
			Bindings: map[string]types.Type{p.Name: scrutTy},
		}
	case *syntax.PatWild:
		return patternResult{Pattern: &ir.PWild{S: p.S}}
	case *syntax.PatCon:
		return ch.checkConPattern(p, scrutTy)
	case *syntax.PatQualCon:
		return ch.checkQualConPattern(p, scrutTy)
	case *syntax.PatRecord:
		return ch.checkRecordPattern(p, scrutTy)
	case *syntax.PatParen:
		return ch.checkPattern(p.Inner, scrutTy)
	case *syntax.PatLit:
		return ch.checkLitPattern(p, scrutTy)
	case *syntax.PatList:
		// PatList is desugared here (not in compiler/desugar) because pattern
		// desugaring requires type-directed elaboration — the desugar pass
		// operates on syntax.Expr only, without type information.
		return ch.checkPattern(desugarListPattern(p), scrutTy)
	case *syntax.PatLabel:
		return ch.checkLabelPattern(p, scrutTy)
	default:
		ch.addDiag(diagnostic.ErrTypeMismatch, pat.Span(), diagFmt{Format: "unsupported pattern form: %T", Args: []any{pat}})
		return patternResult{Pattern: &ir.PWild{S: pat.Span()}}
	}
}

func (ch *Checker) checkLitPattern(p *syntax.PatLit, scrutTy types.Type) patternResult {
	litTy, litVal, parseErr := parseLitValue(p.Kind, p.Value)
	if parseErr != nil {
		ch.addDiag(diagnostic.ErrTypeMismatch, p.S, diagMsg("invalid literal in pattern: "+p.Value))
		return patternResult{Pattern: &ir.PWild{S: p.S}}
	}
	ch.emitEq(litTy, scrutTy, p.S, solve.WithContext(0, "literal pattern type mismatch"))
	return patternResult{Pattern: &ir.PLit{Value: litVal, S: p.S}}
}

// parseLitValue converts a raw literal text into a (type, runtime value) pair.
// Shared between literal patterns (checkLitPattern) and literal expressions (infer).
func parseLitValue(kind syntax.LitKind, raw string) (types.Type, any, error) {
	switch kind {
	case syntax.LitInt:
		n, err := strconv.ParseInt(strings.ReplaceAll(raw, "_", ""), 10, 64)
		return types.Con("Int"), n, err
	case syntax.LitDouble:
		f, err := strconv.ParseFloat(strings.ReplaceAll(raw, "_", ""), 64)
		return types.Con("Double"), f, err
	case syntax.LitString:
		return types.Con("String"), raw, nil
	case syntax.LitRune:
		// Runtime stores rune values as Go rune (int32), matching ExprRuneLit.Value.
		runes := []rune(raw)
		if len(runes) > 0 {
			return types.Con("Rune"), runes[0], nil
		}
		return types.Con("Rune"), rune(0), nil
	default:
		panic(fmt.Sprintf("parseLitValue: unknown LitKind %d", kind))
	}
}

// checkLabelPattern checks a label literal pattern (#tag) against a Variant scrutinee.
// The scrutinee should have type Variant choices s. The label must be present
// in choices. The VariantFieldTy is set on the result so that checkCaseAlts
// can compute per-branch pre-states (substituting s with the field type).
func (ch *Checker) checkLabelPattern(p *syntax.PatLabel, scrutTy types.Type) patternResult {
	scrutTy = ch.unifier.Zonk(scrutTy)
	choices, _, ok := decomposeVariantType(scrutTy)
	if !ok {
		ch.addDiag(diagnostic.ErrTypeMismatch, p.S,
			diagFmt{Format: "label pattern #%s requires Variant scrutinee, got %s", Args: []any{p.Label, types.Pretty(scrutTy)}})
		return patternResult{Pattern: &ir.PLabel{Label: p.Label, S: p.S}}
	}

	choices = ch.unifier.Zonk(choices)
	row, rowOk := choices.(*types.TyEvidenceRow)
	if !rowOk || !row.IsCapabilityRow() {
		ch.addDiag(diagnostic.ErrTypeMismatch, p.S,
			diagFmt{Format: "label pattern #%s: choices is not a concrete row", Args: []any{p.Label}})
		return patternResult{Pattern: &ir.PLabel{Label: p.Label, S: p.S}}
	}

	fieldTy := types.RowFieldType(row.CapFields(), p.Label)
	if fieldTy == nil {
		ch.addDiag(diagnostic.ErrTypeMismatch, p.S,
			diagFmt{Format: "label #%s not present in Variant row %s", Args: []any{p.Label, types.Pretty(choices)}})
		return patternResult{Pattern: &ir.PLabel{Label: p.Label, S: p.S}}
	}

	result := patternResult{
		Pattern:        &ir.PLabel{Label: p.Label, S: p.S},
		VariantFieldTy: fieldTy,
	}

	// Payload binding: #tag x binds x to the Variant payload (type = fieldTy).
	// #tag _ discards the payload (PatWild). Other patterns are rejected.
	if p.Payload != nil {
		switch pv := p.Payload.(type) {
		case *syntax.PatVar:
			result.Pattern = &ir.PLabel{Label: p.Label, PayloadVar: pv.Name, S: p.S}
			if result.Bindings == nil {
				result.Bindings = make(map[string]types.Type)
			}
			result.Bindings[pv.Name] = fieldTy
		case *syntax.PatWild:
			// Wildcard: payload is discarded (PayloadVar stays "")
		default:
			ch.addDiag(diagnostic.ErrInvalidPattern, p.Payload.Span(),
				diagMsg("label payload pattern must be a variable or wildcard"))
		}
	}

	return result
}

// decomposeVariantType extracts (choices, s) from Variant choices s.
// Returns (choices, s, true) or (nil, nil, false).
func decomposeVariantType(ty types.Type) (choices types.Type, s types.Type, ok bool) {
	// Variant choices s = TyApp(TyApp(TyCon("Variant"), choices), s)
	outer, ok1 := ty.(*types.TyApp)
	if !ok1 {
		return nil, nil, false
	}
	inner, ok2 := outer.Fun.(*types.TyApp)
	if !ok2 {
		return nil, nil, false
	}
	con, ok3 := inner.Fun.(*types.TyCon)
	if !ok3 || con.Name != types.TyConVariant {
		return nil, nil, false
	}
	return inner.Arg, outer.Arg, true
}

// pendingCV tracks a constraint variable entry whose class/args are unknown
// until return type unification resolves the meta.
type pendingCV struct {
	constraintVar types.Type
	dictParam     string
}

// instantiateConForalls peels outer foralls from a constructor type,
// classifying each variable as universal (meta) or existential (skolem).
// Returns the body type after substitution and a map of skolem IDs.
func (ch *Checker) instantiateConForalls(conTy types.Type) (types.Type, map[int]string) {
	// Get the return type's free vars (strip arrows from after foralls).
	_, retTy := decomposeConSig(conTy)
	retFreeVars := types.FreeVars(retTy)

	skolemIDs := map[int]string{}
	body := types.PeelForalls(conTy, func(f *types.TyForall) (types.Type, types.LevelExpr) {
		if _, isUniversal := retFreeVars[f.Var]; isUniversal {
			return ch.freshMeta(f.Kind), nil
		}
		skolem := ch.freshSkolem(f.Var, f.Kind)
		skolemIDs[skolem.ID] = f.Var
		return skolem, nil
	})
	return body, skolemIDs
}

func (ch *Checker) checkConPattern(p *syntax.PatCon, scrutTy types.Type) patternResult {
	conTy, ok := ch.reg.LookupConType(p.Con)
	if !ok {
		ch.addDiag(diagnostic.ErrUnboundCon, p.S, diagUnknown{Kind: "constructor", Name: p.Con})
		return patternResult{Pattern: &ir.PWild{S: p.S}}
	}
	return ch.checkConPatternWith(p.Con, "", conTy, p.Args, scrutTy, p.S)
}

func (ch *Checker) checkQualConPattern(p *syntax.PatQualCon, scrutTy types.Type) patternResult {
	qs, ok := ch.scope.LookupQualified(p.Qualifier)
	if !ok {
		ch.addDiag(diagnostic.ErrUnboundCon, p.S, diagUnknown{Kind: "qualifier", Name: p.Qualifier})
		return patternResult{Pattern: &ir.PWild{S: p.S}}
	}
	conTy, ok := qs.Exports.ConTypes[p.Con]
	if !ok {
		ch.addDiag(diagnostic.ErrUnboundCon, p.S,
			diagFmt{Format: "module %s does not export constructor: %s", Args: []any{qs.ModuleName, p.Con}})
		return patternResult{Pattern: &ir.PWild{S: p.S}}
	}
	return ch.checkConPatternWith(p.Con, qs.ModuleName, conTy, p.Args, scrutTy, p.S)
}

// checkConPatternWith is the shared implementation for unqualified and qualified constructor patterns.
func (ch *Checker) checkConPatternWith(conName, moduleName string, conTy types.Type, patArgs []syntax.Pattern, scrutTy types.Type, s span.Span) patternResult {
	conTy = ch.unifier.Zonk(conTy)
	var args []ir.Pattern
	bindings := make(map[string]types.Type)

	currentTy, skolemIDs := ch.instantiateConForalls(conTy)

	// 4. Peel constraints — generate dict bindings and pattern args for existential constraints.
	// For ConstraintVar entries, the concrete className/args are unknown until
	// return type unification (step 6). Record them and resolve after unification.
	// EqualityEntry constraints are also deferred: the metas are unsolved at
	// this point, so we record them and install as givens after unification.
	var pendingCVs []pendingCV
	type pendingEq struct{ lhs, rhs types.Type }
	var pendingEqs []pendingEq
	for {
		if ev, ok := currentTy.(*types.TyEvidence); ok {
			for _, entry := range ev.Constraints.ConEntries() {
				switch e := entry.(type) {
				case *types.VarEntry:
					dictParam := ch.freshName(prefixDictConstraintVar)
					pendingCVs = append(pendingCVs, pendingCV{
						constraintVar: e.Var,
						dictParam:     dictParam,
					})
					args = append(args, &ir.PVar{Name: dictParam, Generated: ir.GenDict, S: s})
				case *types.ClassEntry:
					dictParam := ch.freshDictName(e.ClassName)
					dictTy := ch.buildDictType(e.ClassName, e.Args)
					bindings[dictParam] = dictTy
					args = append(args, &ir.PVar{Name: dictParam, Generated: ir.GenDict, S: s})
				case *types.EqualityEntry:
					pendingEqs = append(pendingEqs, pendingEq{lhs: e.Lhs, rhs: e.Rhs})
				}
			}
			currentTy = ev.Body
		} else {
			break
		}
	}

	var givenEqs map[int]types.Type

	mkResult := func() patternResult {
		return patternResult{
			Pattern:     &ir.PCon{Con: conName, Module: moduleName, Args: args, S: s},
			Bindings:    bindings,
			SkolemIDs:   skolemIDs,
			GivenEqs:    givenEqs,
			HasEvidence: len(pendingCVs) > 0 || len(pendingEqs) > 0,
		}
	}

	// 5. Peel arrow arguments matching user-supplied pattern args.
	// Pre-check arity to give a clear error instead of "expected function type".
	if len(patArgs) > 0 {
		arity := 0
		ty := currentTy
		for {
			if arr, ok := ch.unifier.Zonk(ty).(*types.TyArrow); ok {
				arity++
				ty = arr.To
			} else {
				break
			}
		}
		if len(patArgs) > arity {
			ch.addDiag(diagnostic.ErrBadApplication, s,
				diagFmt{Format: "constructor %s expects %d field(s), but pattern has %d", Args: []any{conName, arity, len(patArgs)}})
			return mkResult()
		}
	}
	for _, argPat := range patArgs {
		argTy, restTy := ch.matchArrow(currentTy, s)
		child := ch.checkPattern(argPat, argTy)
		args = append(args, child.Pattern)
		ch.mergePatternBindings(bindings, child.Bindings, argPat.Span())
		maps.Copy(skolemIDs, child.SkolemIDs)
		currentTy = restTy
	}
	// 6. Extract GADT given equalities and unify result type with scrutinee.
	if ch.isInaccessibleGADTBranch(conName, scrutTy) {
		return mkResult()
	}
	givenEqs = extractGivenEqs(ch.unifier.Zonk(currentTy), ch.unifier.Zonk(scrutTy))
	for skolemID, ty := range givenEqs {
		ch.unifier.InstallGivenEq(skolemID, ty)
		ch.emitGivenEq(&types.TySkolem{ID: skolemID}, ty, s)
	}
	if err := ch.unifier.Unify(currentTy, scrutTy); err != nil {
		ch.addUnifyError(err, s, "constructor type mismatch")
		return mkResult()
	}
	// 6b. Install explicit equality constraints as givens. Metas are now
	// solved by unification, so zonk reveals the concrete (skolem) types.
	for _, eq := range pendingEqs {
		lhs := ch.unifier.Zonk(eq.lhs)
		rhs := ch.unifier.Zonk(eq.rhs)
		if sk, ok := lhs.(*types.TySkolem); ok {
			if givenEqs == nil {
				givenEqs = make(map[int]types.Type)
			}
			givenEqs[sk.ID] = rhs
			ch.unifier.InstallGivenEq(sk.ID, rhs)
			ch.emitGivenEq(lhs, rhs, s)
		} else if sk, ok := rhs.(*types.TySkolem); ok {
			if givenEqs == nil {
				givenEqs = make(map[int]types.Type)
			}
			givenEqs[sk.ID] = lhs
			ch.unifier.InstallGivenEq(sk.ID, lhs)
			ch.emitGivenEq(rhs, lhs, s)
		}
	}
	// 7. Resolve pending constraint variable entries now that metas are solved.
	ch.resolvePendingCVs(pendingCVs, bindings)
	return mkResult()
}

// isInaccessibleGADTBranch returns true if the constructor's return type
// cannot unify with the scrutinee, making the branch inaccessible.
func (ch *Checker) isInaccessibleGADTBranch(conName string, scrutTy types.Type) bool {
	info, _ := ch.reg.LookupConInfo(conName)
	if info == nil {
		return false
	}
	for _, c := range info.Constructors {
		if c.Name == conName && c.ReturnType != nil {
			if !ch.canUnifyWith(c.ReturnType, scrutTy) {
				return true
			}
		}
	}
	return false
}

// desugarListPattern rewrites [p1, p2, p3] to Cons p1 (Cons p2 (Cons p3 Nil)).
func desugarListPattern(p *syntax.PatList) syntax.Pattern {
	var result syntax.Pattern = &syntax.PatCon{Con: "Nil", S: p.S}
	for i := len(p.Elems) - 1; i >= 0; i-- {
		result = &syntax.PatCon{
			Con:  "Cons",
			Args: []syntax.Pattern{p.Elems[i], result},
			S:    p.S,
		}
	}
	return result
}

// extractGivenEqs compares a GADT constructor's return type with the
// scrutinee type and extracts given equalities. When the scrutinee has a
// skolem at a position where the constructor has a concrete type, the
// pair (skolemID → concreteType) is recorded as a given equality.
func extractGivenEqs(conRetTy, scrutTy types.Type) map[int]types.Type {
	_, conArgs := types.UnwindApp(conRetTy)
	_, scrutArgs := types.UnwindApp(scrutTy)
	if len(conArgs) != len(scrutArgs) {
		return nil
	}
	var result map[int]types.Type
	for i, scrutArg := range scrutArgs {
		conArg := conArgs[i]
		if sk, ok := scrutArg.(*types.TySkolem); ok {
			// The scrutinee arg is a skolem; the constructor arg refines it
			// (unless the constructor arg is also a skolem or a meta — no refinement).
			if _, isMeta := conArg.(*types.TyMeta); isMeta {
				continue
			}
			if sk2, ok2 := conArg.(*types.TySkolem); ok2 && sk2.ID == sk.ID {
				continue
			}
			if result == nil {
				result = make(map[int]types.Type)
			}
			result[sk.ID] = conArg
		}
	}
	return result
}

// resolvePendingCVs resolves deferred constraint variable entries after metas are solved.
func (ch *Checker) resolvePendingCVs(pending []pendingCV, bindings map[string]types.Type) {
	for _, pcv := range pending {
		cv := ch.unifier.Zonk(pcv.constraintVar)
		if cn, cArgs, ok := types.DecomposeConstraintType(cv); ok {
			dictTy := ch.buildDictType(cn, cArgs)
			bindings[pcv.dictParam] = dictTy
		}
	}
}
