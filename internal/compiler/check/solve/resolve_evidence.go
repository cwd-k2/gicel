package solve

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// applyQuantifiedEvidence tries to use a quantified evidence entry to produce
// a dictionary for the given className and args.
// For example, if evidence is `\ a. Eq a => Eq (g a)` and we need `Eq (g Bool)`,
// it instantiates `a = Bool`, resolves `Eq Bool`, and builds the application.
func (s *Solver) applyQuantifiedEvidence(e *env.CtxEvidence, className string, args []types.Type, sp span.Span) ir.Core {
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
		freshSubst[v.Name] = s.env.FreshMeta(v.Kind)
	}

	// Try to unify head args with wanted args.
	if !s.env.WithTrial(func() bool {
		for i := range args {
			headArg := types.SubstMany(qc.Head.Args[i], freshSubst)
			if err := s.env.Unify(headArg, args[i]); err != nil {
				return false
			}
		}
		return true
	}) {
		return nil
	}

	// Build the evidence expression by applying the quantified evidence function.
	var dictExpr ir.Core = &ir.Var{Name: e.DictName, S: sp}

	// Apply type arguments.
	for _, v := range qc.Vars {
		tyArg := s.env.Zonk(freshSubst[v.Name])
		dictExpr = &ir.TyApp{Expr: dictExpr, TyArg: tyArg, S: sp}
	}

	// Resolve and apply context (premise) dictionaries.
	for _, ctx := range qc.Context {
		ctxArgs := make([]types.Type, len(ctx.Args))
		for j, a := range ctx.Args {
			ctxArgs[j] = s.env.Zonk(types.SubstMany(a, freshSubst))
		}
		ctxDict := s.resolveInstance(ctx.ClassName, ctxArgs, sp)
		dictExpr = &ir.App{Fun: dictExpr, Arg: ctxDict, S: sp}
	}

	return dictExpr
}

// resolveQuantifiedConstraint finds evidence for a quantified constraint.
// For `\ a. Eq a => Eq (F a)`, it searches for an instance whose structure
// matches (e.g., `instance Eq a => Eq (F a)`) and returns its dict binding.
func (s *Solver) resolveQuantifiedConstraint(qc *types.QuantifiedConstraint, sp span.Span) ir.Core {
	// Strategy: the quantified constraint `\ a. C1 a => C2 (F a)` is satisfied
	// by a global instance `C2 (F a)` with context `C1 a`, which already has the
	// right type: `\ a. C1$Dict a -> C2$Dict (F a)`.
	//
	// Search global instances for a match on the head.
	for _, inst := range s.env.InstancesForClass(qc.Head.ClassName) {
		if len(inst.TypeArgs) != len(qc.Head.Args) {
			continue
		}
		// Check if this instance structurally matches the quantified constraint head.
		// Create a fresh substitution for the quantified vars.
		freshSubst := make(map[string]types.Type, len(qc.Vars))
		for _, v := range qc.Vars {
			freshSubst[v.Name] = s.env.FreshMeta(v.Kind)
		}

		// Also create fresh metas for the instance's own free vars.
		instSubst := s.FreshInstanceSubst(inst)

		if !s.env.WithProbe(func() bool {
			for i := range qc.Head.Args {
				headArg := types.SubstMany(qc.Head.Args[i], freshSubst)
				instArg := types.SubstMany(inst.TypeArgs[i], instSubst)
				if err := s.env.Unify(headArg, instArg); err != nil {
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
		// Solutions are not needed — only the binding name matters.
		return &ir.Var{Name: inst.DictBindName, Module: inst.Module, S: sp}
	}

	// Also search context for quantified evidence variables.
	// Full structural match: class name, arity, head args (via trial unification),
	// and context compatibility — same verification as the global instance path above.
	var qcResult ir.Core
	s.env.ScanContext(func(entry env.CtxEntry) bool {
		e, ok := entry.(*env.CtxEvidence)
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
			wantedSubst[v.Name] = s.env.FreshMeta(v.Kind)
		}
		evidenceSubst := make(map[string]types.Type, len(eq.Vars))
		for _, v := range eq.Vars {
			evidenceSubst[v.Name] = s.env.FreshMeta(v.Kind)
		}
		if !s.env.WithProbe(func() bool {
			for i := range qc.Head.Args {
				wantedArg := types.SubstMany(qc.Head.Args[i], wantedSubst)
				evidenceArg := types.SubstMany(eq.Head.Args[i], evidenceSubst)
				if err := s.env.Unify(wantedArg, evidenceArg); err != nil {
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
		// Solutions are not needed — only the binding name matters.
		qcResult = &ir.Var{Name: e.DictName, S: sp}
		return false
	})
	if qcResult != nil {
		return qcResult
	}

	s.env.AddCodedError(diagnostic.ErrNoInstance, sp,
		fmt.Sprintf("no instance for %s %s", qc.Head.ClassName, s.prettyTypeArgs(qc.Head.Args)))
	return &ir.Var{Name: "<no-instance>", S: sp}
}

// FreshInstanceSubst creates a substitution mapping each free type variable
// in an instance's type arguments and context to a fresh meta variable.
// Uses pre-computed FreeVarNames to avoid repeated FreeVars traversals.
func (s *Solver) FreshInstanceSubst(inst *env.InstanceInfo) map[string]types.Type {
	names := inst.FreeVarNames
	if len(names) == 0 {
		return nil
	}
	subst := make(map[string]types.Type, len(names))
	for _, v := range names {
		subst[v] = s.env.FreshMeta(types.KType{})
	}
	return subst
}
