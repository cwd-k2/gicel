package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// patternResult holds the outputs of pattern checking.
type patternResult struct {
	Pattern     core.Pattern
	Bindings    map[string]types.Type
	SkolemIDs   map[int]string
	HasEvidence bool
}

func (ch *Checker) checkPattern(pat syntax.Pattern, scrutTy types.Type) patternResult {
	switch p := pat.(type) {
	case *syntax.PatVar:
		return patternResult{
			Pattern:  &core.PVar{Name: p.Name, S: p.S},
			Bindings: map[string]types.Type{p.Name: scrutTy},
		}
	case *syntax.PatWild:
		return patternResult{Pattern: &core.PWild{S: p.S}}
	case *syntax.PatCon:
		return ch.checkConPattern(p, scrutTy)
	case *syntax.PatRecord:
		return ch.checkRecordPattern(p, scrutTy)
	case *syntax.PatParen:
		return ch.checkPattern(p.Inner, scrutTy)
	default:
		return patternResult{Pattern: &core.PWild{S: pat.Span()}}
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
	// Collect forall vars.
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
	conTy, ok := ch.conTypes[p.Con]
	if !ok {
		ch.addCodedError(errs.ErrUnboundCon, p.S, fmt.Sprintf("unknown constructor in pattern: %s", p.Con))
		return patternResult{Pattern: &core.PWild{S: p.S}}
	}
	conTy = ch.unifier.Zonk(conTy)
	var args []core.Pattern
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
					dictParam := fmt.Sprintf("%s_%d", prefixDictCV, ch.fresh())
					pendingCVs = append(pendingCVs, pendingCV{
						constraintVar: entry.ConstraintVar,
						dictParam:     dictParam,
					})
					args = append(args, &core.PVar{Name: dictParam, S: p.S})
				} else {
					dictParam := fmt.Sprintf("%s_%s_%d", prefixDict, entry.ClassName, ch.fresh())
					dictTy := ch.buildDictType(entry.ClassName, entry.Args)
					bindings[dictParam] = dictTy
					args = append(args, &core.PVar{Name: dictParam, S: p.S})
				}
			}
			currentTy = ev.Body
		} else {
			break
		}
	}

	mkResult := func() patternResult {
		return patternResult{
			Pattern:     &core.PCon{Con: p.Con, Args: args, S: p.S},
			Bindings:    bindings,
			SkolemIDs:   skolemIDs,
			HasEvidence: len(pendingCVs) > 0,
		}
	}

	// 5. Peel arrow arguments matching user-supplied pattern args.
	for _, argPat := range p.Args {
		argTy, restTy := ch.matchArrow(currentTy, p.S)
		child := ch.checkPattern(argPat, argTy)
		args = append(args, child.Pattern)
		for k, v := range child.Bindings {
			bindings[k] = v
		}
		for k, v := range child.SkolemIDs {
			skolemIDs[k] = v
		}
		currentTy = restTy
	}
	// 6. Unify result type with scrutinee type.
	if ch.isInaccessibleGADTBranch(p.Con, scrutTy) {
		return mkResult()
	}
	if err := ch.unifier.Unify(currentTy, scrutTy); err != nil {
		ch.addUnifyError(err, p.S, "constructor type mismatch")
		return mkResult()
	}
	// 7. Resolve pending constraint variable entries now that metas are solved.
	ch.resolvePendingCVs(pendingCVs, bindings)
	return mkResult()
}

// isInaccessibleGADTBranch returns true if the constructor's return type
// cannot unify with the scrutinee, making the branch inaccessible.
func (ch *Checker) isInaccessibleGADTBranch(conName string, scrutTy types.Type) bool {
	info := ch.conInfo[conName]
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

// resolvePendingCVs resolves deferred constraint variable entries after metas are solved.
func (ch *Checker) resolvePendingCVs(pending []pendingCV, bindings map[string]types.Type) {
	for _, pcv := range pending {
		cv := ch.unifier.Zonk(pcv.constraintVar)
		if cn, cArgs, ok := DecomposeConstraintType(cv); ok {
			dictTy := ch.buildDictType(cn, cArgs)
			bindings[pcv.dictParam] = dictTy
		}
	}
}
