package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// solveWanteds processes all constraints in the worklist, producing
// a placeholder → Core resolution map and residual constraints.
//
// shouldDefer governs whether a plain class constraint is deferred
// (returned as residual) or discharged. Nil means discharge all.
func (ch *Checker) solveWanteds(
	shouldDefer func(className string, zonkedArgs []types.Type) bool,
) (map[string]ir.Core, []*CtClass) {
	resolutions := make(map[string]ir.Core)
	var residuals []*CtClass
	ch.solver.inertSet.Reset()
	ch.solver.ambiguityCache = nil // lazily allocated; zero-cost when shouldDefer is nil
	ch.budget.ResetSolverSteps()

	for {
		ct, ok := ch.solver.worklist.Pop()
		if !ok {
			break
		}
		if err := ch.budget.SolverStep(); err != nil {
			ch.addCodedError(diagnostic.ErrSolverLimit, ct.ctSpan(),
				"constraint solver step limit exceeded (possible infinite loop or exponential blowup)")
			break
		}
		if ch.checkCancelled() {
			break
		}
		switch c := ct.(type) {
		case *CtClass:
			ch.processCtClass(c, resolutions, &residuals, shouldDefer)
		case *CtFunEq:
			ch.processCtFunEq(c)
		}
	}

	// Collect remaining class constraints from inert set as residuals.
	for _, ct := range ch.solver.inertSet.CollectClassResiduals() {
		if shouldDefer != nil {
			zonkedArgs := ch.zonkAll(ct.Args)
			if shouldDefer(ct.ClassName, zonkedArgs) {
				residuals = append(residuals, ct)
				continue
			}
		}
		// Discharge from inert set: resolve if not already resolved.
		if _, exists := resolutions[ct.Placeholder]; !exists {
			key := constraintKey(ct.ClassName, ct.Args)
			ch.resolveCtClassKeyed(ct, key, resolutions)
		}
	}

	ch.solver.inertSet.Reset()
	return resolutions, residuals
}

// processCtClass handles a single class constraint from the worklist.
func (ch *Checker) processCtClass(
	ct *CtClass,
	resolutions map[string]ir.Core,
	residuals *[]*CtClass,
	shouldDefer func(className string, zonkedArgs []types.Type) bool,
) {
	// Branch A: quantified constraint.
	if ct.Quantified != nil {
		resolutions[ct.Placeholder] = ch.resolveQuantifiedConstraint(ct.Quantified, ct.S)
		return
	}

	// Branch B: constraint variable.
	if ct.ConstraintVar != nil {
		cv := ch.unifier.Zonk(ct.ConstraintVar)
		cn, cArgs, ok := types.DecomposeConstraintType(cv)
		if ok {
			ct.ClassName = cn
			ct.Args = cArgs
		} else if ct.ClassName != "" {
			ct.Args = ch.zonkAll(ct.Args)
		} else {
			ch.addCodedError(diagnostic.ErrNoInstance, ct.S,
				fmt.Sprintf("cannot resolve constraint variable %s", types.Pretty(cv)))
			resolutions[ct.Placeholder] = &ir.Var{Name: "<no-instance>", S: ct.S}
			return
		}
	} else {
		// Branch C: plain className + args.
		ct.Args = ch.zonkAll(ct.Args)
	}

	// Build canonical key for cache lookup.
	key := constraintKey(ct.ClassName, ct.Args)

	// Check if the inert set already has an identical resolved constraint.
	if cachedPlaceholder := ch.solver.inertSet.LookupResolution(key); cachedPlaceholder != "" {
		if cachedExpr, ok := resolutions[cachedPlaceholder]; ok {
			resolutions[ct.Placeholder] = cachedExpr
			return
		}
	}

	// Check defer predicate for branch C (plain constraints only).
	if ct.Quantified == nil && ct.ConstraintVar == nil && shouldDefer != nil {
		if shouldDefer(ct.ClassName, ct.Args) {
			*residuals = append(*residuals, ct)
			return
		}
	}

	// Resolve the constraint.
	ch.resolveCtClassKeyed(ct, key, resolutions)
}

// resolveCtClassKeyed resolves a class constraint, records the resolution,
// and inserts it into the inert set with a canonical cache key.
func (ch *Checker) resolveCtClassKeyed(ct *CtClass, key string, resolutions map[string]ir.Core) {
	resolved := ch.resolveInstance(ct.ClassName, ct.Args, ct.S)
	resolutions[ct.Placeholder] = resolved
	ch.solver.inertSet.InsertClass(ct, key)
}

// constraintKey builds a canonical key for a class constraint.
// Injective: distinct (className, zonked args) produce distinct keys.
func constraintKey(className string, args []types.Type) string {
	var b strings.Builder
	b.WriteString(className)
	for _, a := range args {
		b.WriteByte(' ')
		types.WriteTypeKey(&b, a)
	}
	return b.String()
}

// processCtFunEq handles a stuck type family equation from the worklist.
func (ch *Checker) processCtFunEq(ct *CtFunEq) {
	zonked := ch.zonkAll(ct.Args)
	result, reduced := ch.reduceTyFamily(ct.FamilyName, zonked, ct.S)
	if reduced {
		// Unification failure here means the reduced result conflicts with an
		// earlier reduction of the same family application. This is prevented
		// by type family overlap checks; if it occurs, the unsolved meta will
		// produce a downstream type mismatch error.
		_ = ch.unifier.Unify(ct.ResultMeta, result)
		return
	}
	// Still stuck: update args and re-register in inert set.
	ct.Args = zonked
	ct.BlockingOn = collectMetaIDs(zonked)
	if len(ct.BlockingOn) > 0 {
		ch.solver.inertSet.InsertFunEq(ct)
	}
}
