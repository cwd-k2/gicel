package check

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// patternResult holds the outputs of pattern checking.
type patternResult struct {
	Pattern     ir.Pattern
	Bindings    map[string]types.Type
	SkolemIDs   map[int]string
	GivenEqs    map[int]types.Type // GADT given equalities: skolem ID → type
	HasEvidence bool
}

func (ch *Checker) checkPattern(pat syntax.Pattern, scrutTy types.Type) patternResult {
	switch p := pat.(type) {
	case *syntax.PatVar:
		return patternResult{
			Pattern:  &ir.PVar{Name: p.Name, S: p.S},
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
		return ch.checkPattern(desugarListPattern(p), scrutTy)
	default:
		ch.addCodedError(diagnostic.ErrTypeMismatch, pat.Span(), fmt.Sprintf("unsupported pattern form: %T", pat))
		return patternResult{Pattern: &ir.PWild{S: pat.Span()}}
	}
}

func (ch *Checker) checkLitPattern(p *syntax.PatLit, scrutTy types.Type) patternResult {
	litTy, litVal, parseErr := parseLitValue(p.Kind, p.Value)
	if parseErr != nil {
		ch.addCodedError(diagnostic.ErrTypeMismatch, p.S, fmt.Sprintf("invalid literal in pattern: %s", p.Value))
		return patternResult{Pattern: &ir.PWild{S: p.S}}
	}
	if err := ch.unifier.Unify(litTy, scrutTy); err != nil {
		ch.addUnifyError(err, p.S, "literal pattern type mismatch")
	}
	return patternResult{Pattern: &ir.PLit{Value: litVal, S: p.S}}
}

// parseLitValue converts a raw literal text into a (type, runtime value) pair.
// Shared between literal patterns (checkLitPattern) and literal expressions (infer).
func parseLitValue(kind syntax.LitKind, raw string) (types.Type, any, error) {
	switch kind {
	case syntax.LitInt:
		n, err := strconv.ParseInt(strings.ReplaceAll(raw, "_", ""), 10, 64)
		return &types.TyCon{Name: "Int"}, n, err
	case syntax.LitDouble:
		f, err := strconv.ParseFloat(strings.ReplaceAll(raw, "_", ""), 64)
		return &types.TyCon{Name: "Double"}, f, err
	case syntax.LitString:
		return &types.TyCon{Name: "String"}, raw, nil
	case syntax.LitRune:
		// Runtime stores rune values as Go rune (int32), matching ExprRuneLit.Value.
		runes := []rune(raw)
		if len(runes) > 0 {
			return &types.TyCon{Name: "Rune"}, runes[0], nil
		}
		return &types.TyCon{Name: "Rune"}, rune(0), nil
	default:
		panic(fmt.Sprintf("parseLitValue: unknown LitKind %d", kind))
	}
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
	// Collect \ vars.
	type fvar struct {
		name string
		kind types.Kind
	}
	var forallVars []fvar
	tmpTy := conTy
	for {
		if f, ok := tmpTy.(*types.TyForall); ok {
			forallVars = append(forallVars, fvar{name: f.Var, kind: f.Kind})
			tmpTy = f.Body
		} else {
			break
		}
	}

	// Get the return type's free vars (strip arrows from after foralls).
	_, retTy := decomposeConSig(conTy)
	retFreeVars := types.FreeVars(retTy)

	// Classify each forall var: universal (in return type) → meta, existential → skolem.
	currentTy := conTy
	skolemIDs := map[int]string{}
	for _, fv := range forallVars {
		if f, ok := currentTy.(*types.TyForall); ok {
			if _, isUniversal := retFreeVars[fv.name]; isUniversal {
				meta := ch.freshMeta(fv.kind)
				currentTy = types.Subst(f.Body, f.Var, meta)
			} else {
				skolem := ch.freshSkolem(fv.name, fv.kind)
				skolemIDs[skolem.ID] = fv.name
				currentTy = types.Subst(f.Body, f.Var, skolem)
			}
		}
	}
	return currentTy, skolemIDs
}

func (ch *Checker) checkConPattern(p *syntax.PatCon, scrutTy types.Type) patternResult {
	conTy, ok := ch.reg.LookupConType(p.Con)
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundCon, p.S, fmt.Sprintf("unknown constructor in pattern: %s", p.Con))
		return patternResult{Pattern: &ir.PWild{S: p.S}}
	}
	return ch.checkConPatternWith(p.Con, "", conTy, p.Args, scrutTy, p.S)
}

func (ch *Checker) checkQualConPattern(p *syntax.PatQualCon, scrutTy types.Type) patternResult {
	qs, ok := ch.scope.LookupQualified(p.Qualifier)
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundCon, p.S, fmt.Sprintf("unknown qualifier: %s", p.Qualifier))
		return patternResult{Pattern: &ir.PWild{S: p.S}}
	}
	conTy, ok := qs.Exports.ConTypes[p.Con]
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundCon, p.S,
			fmt.Sprintf("module %s does not export constructor: %s", qs.ModuleName, p.Con))
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
	var pendingCVs []pendingCV
	for {
		if ev, ok := currentTy.(*types.TyEvidence); ok {
			for _, entry := range ev.Constraints.ConEntries() {
				if entry.ConstraintVar != nil && entry.ClassName == "" {
					dictParam := fmt.Sprintf("%s_%d", prefixDictConstraintVar, ch.fresh())
					pendingCVs = append(pendingCVs, pendingCV{
						constraintVar: entry.ConstraintVar,
						dictParam:     dictParam,
					})
					args = append(args, &ir.PVar{Name: dictParam, S: s})
				} else {
					dictParam := fmt.Sprintf("%s_%s_%d", prefixDict, entry.ClassName, ch.fresh())
					dictTy := ch.buildDictType(entry.ClassName, entry.Args)
					bindings[dictParam] = dictTy
					args = append(args, &ir.PVar{Name: dictParam, S: s})
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
			HasEvidence: len(pendingCVs) > 0,
		}
	}

	// 5. Peel arrow arguments matching user-supplied pattern args.
	for _, argPat := range patArgs {
		argTy, restTy := ch.matchArrow(currentTy, s)
		child := ch.checkPattern(argPat, argTy)
		args = append(args, child.Pattern)
		maps.Copy(bindings, child.Bindings)
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
	}
	if err := ch.unifier.Unify(currentTy, scrutTy); err != nil {
		ch.addUnifyError(err, s, "constructor type mismatch")
		return mkResult()
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
