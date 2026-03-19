package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// applyQuantifiedEvidence tries to use a quantified evidence entry to produce
// a dictionary for the given className and args.
// For example, if evidence is `\ a. Eq a => Eq (g a)` and we need `Eq (g Bool)`,
// it instantiates `a = Bool`, resolves `Eq Bool`, and builds the application.
func (ch *Checker) applyQuantifiedEvidence(e *CtxEvidence, className string, args []types.Type, s span.Span) core.Core {
	qc := e.Quantified
	// Head must match the target class.
	if qc.Head.ClassName != className {
		return nil
	}
	if len(qc.Head.Args) != len(args) {
		return nil
	}

	// Create fresh metas for the quantified variables.
	freshSubst := make(map[string]types.Type, len(qc.Vars))
	for _, v := range qc.Vars {
		freshSubst[v.Name] = ch.freshMeta(v.Kind)
	}

	// Try to unify head args with wanted args.
	if !ch.withTrial(func() bool {
		for i := range args {
			headArg := types.SubstMany(qc.Head.Args[i], freshSubst)
			if err := ch.unifier.Unify(headArg, args[i]); err != nil {
				return false
			}
		}
		return true
	}) {
		return nil
	}

	// Build the evidence expression by applying the quantified evidence function.
	var dictExpr core.Core = &core.Var{Name: e.DictName, S: s}

	// Apply type arguments.
	for _, v := range qc.Vars {
		tyArg := ch.unifier.Zonk(freshSubst[v.Name])
		dictExpr = &core.TyApp{Expr: dictExpr, TyArg: tyArg, S: s}
	}

	// Resolve and apply context (premise) dictionaries.
	for _, ctx := range qc.Context {
		ctxArgs := make([]types.Type, len(ctx.Args))
		for j, a := range ctx.Args {
			ctxArgs[j] = ch.unifier.Zonk(types.SubstMany(a, freshSubst))
		}
		ctxDict := ch.resolveInstance(ctx.ClassName, ctxArgs, s)
		dictExpr = &core.App{Fun: dictExpr, Arg: ctxDict, S: s}
	}

	return dictExpr
}

// resolveQuantifiedConstraint finds evidence for a quantified constraint.
// For `\ a. Eq a => Eq (F a)`, it searches for an instance whose structure
// matches (e.g., `instance Eq a => Eq (F a)`) and returns its dict binding.
func (ch *Checker) resolveQuantifiedConstraint(qc *types.QuantifiedConstraint, s span.Span) core.Core {
	// Strategy: the quantified constraint `\ a. C1 a => C2 (F a)` is satisfied
	// by a global instance `C2 (F a)` with context `C1 a`, which already has the
	// right type: `\ a. C1$Dict a -> C2$Dict (F a)`.
	//
	// Search global instances for a match on the head.
	for _, inst := range ch.reg.instancesByClass[qc.Head.ClassName] {
		if len(inst.TypeArgs) != len(qc.Head.Args) {
			continue
		}
		// Check if this instance structurally matches the quantified constraint head.
		// Create a fresh substitution for the quantified vars.
		freshSubst := make(map[string]types.Type, len(qc.Vars))
		for _, v := range qc.Vars {
			freshSubst[v.Name] = ch.freshMeta(v.Kind)
		}

		// Also create fresh metas for the instance's own free vars.
		instSubst := ch.freshInstanceSubst(inst)

		if !ch.withTrial(func() bool {
			for i := range qc.Head.Args {
				headArg := types.SubstMany(qc.Head.Args[i], freshSubst)
				instArg := types.SubstMany(inst.TypeArgs[i], instSubst)
				if err := ch.unifier.Unify(headArg, instArg); err != nil {
					return false
				}
			}
			// Verify context compatibility: instance context should subsume quantified context.
			if len(inst.Context) != len(qc.Context) {
				return false
			}
			for i, ic := range inst.Context {
				if ic.ClassName != qc.Context[i].ClassName {
					return false
				}
			}
			return true
		}) {
			continue
		}

		// The instance dict binding has the right type.
		return &core.Var{Name: inst.DictBindName, Module: inst.Module, S: s}
	}

	// Also search context for quantified evidence variables.
	// Full structural match: class name, arity, head args (via trial unification),
	// and context compatibility — same verification as the global instance path above.
	var qcResult core.Core
	ch.ctx.Scan(func(entry CtxEntry) bool {
		e, ok := entry.(*CtxEvidence)
		if !ok || e.Quantified == nil {
			return true
		}
		eq := e.Quantified
		if eq.Head.ClassName != qc.Head.ClassName {
			return true
		}
		if len(eq.Head.Args) != len(qc.Head.Args) {
			return true
		}
		// Fresh metas for both sides' quantified variables.
		wantedSubst := make(map[string]types.Type, len(qc.Vars))
		for _, v := range qc.Vars {
			wantedSubst[v.Name] = ch.freshMeta(v.Kind)
		}
		evidenceSubst := make(map[string]types.Type, len(eq.Vars))
		for _, v := range eq.Vars {
			evidenceSubst[v.Name] = ch.freshMeta(v.Kind)
		}
		if !ch.withTrial(func() bool {
			for i := range qc.Head.Args {
				wantedArg := types.SubstMany(qc.Head.Args[i], wantedSubst)
				evidenceArg := types.SubstMany(eq.Head.Args[i], evidenceSubst)
				if err := ch.unifier.Unify(wantedArg, evidenceArg); err != nil {
					return false
				}
			}
			if len(eq.Context) != len(qc.Context) {
				return false
			}
			for i, ec := range eq.Context {
				if ec.ClassName != qc.Context[i].ClassName {
					return false
				}
			}
			return true
		}) {
			return true
		}
		qcResult = &core.Var{Name: e.DictName, S: s}
		return false
	})
	if qcResult != nil {
		return qcResult
	}

	ch.addCodedError(errs.ErrNoInstance, s,
		fmt.Sprintf("no instance for %s %s", qc.Head.ClassName, ch.prettyTypeArgs(qc.Head.Args)))
	return &core.Var{Name: "<no-instance>", S: s}
}

// freshInstanceSubst creates a substitution mapping each free type variable
// in an instance's type arguments and context to a fresh meta variable.
func (ch *Checker) freshInstanceSubst(inst *InstanceInfo) map[string]types.Type {
	seen := make(map[string]bool)
	subst := make(map[string]types.Type)
	collect := func(ty types.Type) {
		for v := range types.FreeVars(ty) {
			if !seen[v] {
				seen[v] = true
				subst[v] = ch.freshMeta(types.KType{})
			}
		}
	}
	for _, ta := range inst.TypeArgs {
		collect(ta)
	}
	for _, ctx := range inst.Context {
		for _, a := range ctx.Args {
			collect(a)
		}
	}
	return subst
}
