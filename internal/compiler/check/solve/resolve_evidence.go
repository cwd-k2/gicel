package solve

import (
	"sort"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// constraintSortKey returns a canonical key for ordering constraint entries.
// EqualityEntries sort before class entries (empty className).
func constraintSortKey(e types.ConstraintEntry) string {
	return types.HeadClassName(e)
}

// sortedConstraintIndices returns indices into ctx sorted by canonical key,
// enabling order-independent matching of constraint contexts.
func sortedConstraintIndices(ctx []types.ConstraintEntry) []int {
	indices := make([]int, len(ctx))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(a, b int) bool {
		ka := constraintSortKey(ctx[indices[a]])
		kb := constraintSortKey(ctx[indices[b]])
		if ka != kb {
			return ka < kb
		}
		return indices[a] < indices[b] // stable by original order
	})
	return indices
}

// sortedInfoIndices returns indices into a ConstraintInfo slice sorted by class name.
func sortedInfoIndices(ctx []env.ConstraintInfo) []int {
	indices := make([]int, len(ctx))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(a, b int) bool {
		ka := ctx[indices[a]].ClassName
		kb := ctx[indices[b]].ClassName
		if ka != kb {
			return ka < kb
		}
		return indices[a] < indices[b]
	})
	return indices
}

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

	// Wrap head unification and context resolution in a single trial
	// so that if context resolution fails, head solutions are rolled back.
	ps := types.PrepareSubst(freshSubst)
	var dictExpr ir.Core
	savedErrs := s.env.ErrorCount()
	if !s.trialScope(func() bool {
		// Head match.
		for i := range args {
			headArg := ps.Apply(qc.Head.Args[i])
			if err := s.env.Unify(headArg, args[i]); err != nil {
				return false
			}
		}
		// Build the evidence expression.
		dictExpr = &ir.Var{Name: e.DictName, S: sp}
		for _, v := range qc.Vars {
			tyArg := s.env.Zonk(freshSubst[v.Name])
			dictExpr = &ir.TyApp{Expr: dictExpr, TyArg: tyArg, S: sp}
		}
		// Resolve context premises:
		// - ClassEntry: resolve and apply the runtime dictionary.
		// - EqualityEntry: verify the equality holds (no runtime dict).
		// - Other variants: skip (not currently generated).
		for _, ctx := range qc.Context {
			if eqEntry, ok := ctx.(*types.EqualityEntry); ok {
				lhs := s.env.Zonk(ps.Apply(eqEntry.Lhs))
				rhs := s.env.Zonk(ps.Apply(eqEntry.Rhs))
				if err := s.env.Unify(lhs, rhs); err != nil {
					return false
				}
				continue
			}
			ctxCls, ok := ctx.(*types.ClassEntry)
			if !ok {
				continue
			}
			ctxArgs := make([]types.Type, len(ctxCls.Args))
			for j, a := range ctxCls.Args {
				ctxArgs[j] = s.env.Zonk(ps.Apply(a))
			}
			ctxDict := s.resolveInstance(ctxCls.ClassName, ctxArgs, sp)
			dictExpr = &ir.App{Fun: dictExpr, Arg: ctxDict, S: sp}
		}
		// If context resolution emitted errors, treat as failure.
		return s.env.ErrorCount() == savedErrs
	}) {
		s.env.TruncateErrors(savedErrs)
		return nil
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

		psHead := types.PrepareSubst(freshSubst)
		psInst := types.PrepareSubst(instSubst)
		if !s.probeScope(func() bool {
			for i := range qc.Head.Args {
				headArg := psHead.Apply(qc.Head.Args[i])
				instArg := psInst.Apply(inst.TypeArgs[i])
				if err := s.env.Unify(headArg, instArg); err != nil {
					return false
				}
			}
			// Verify context compatibility: instance context should subsume quantified context.
			// Contexts are compared in canonical order (sorted by class name) to allow
			// matching regardless of declaration order — see T1-3 structural matching fix.
			if len(inst.Context) != len(qc.Context) {
				return false
			}
			instOrder := sortedInfoIndices(inst.Context)
			qcOrder := sortedConstraintIndices(qc.Context)
			for idx := range instOrder {
				ic := inst.Context[instOrder[idx]]
				qcc := qc.Context[qcOrder[idx]]
				if ic.ClassName != types.HeadClassName(qcc) {
					return false
				}
				qccArgs := types.HeadClassArgs(qcc)
				if len(ic.Args) != len(qccArgs) {
					return false
				}
				for j := range ic.Args {
					instArg := psInst.Apply(ic.Args[j])
					qcArg := psHead.Apply(qccArgs[j])
					if err := s.env.Unify(instArg, qcArg); err != nil {
						return false
					}
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

	// Search context for quantified evidence variables using the evidence index.
	// Full structural match: class name, arity, head args (via trial unification),
	// and context compatibility — same verification as the global instance path above.
	var qcResult ir.Core
	for _, e := range s.env.LookupEvidence(qc.Head.ClassName) {
		if e.Quantified == nil {
			continue
		}
		eq := e.Quantified
		if len(eq.Head.Args) != len(qc.Head.Args) {
			continue
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
		psWanted := types.PrepareSubst(wantedSubst)
		psEvidence := types.PrepareSubst(evidenceSubst)
		if !s.probeScope(func() bool {
			for i := range qc.Head.Args {
				wantedArg := psWanted.Apply(qc.Head.Args[i])
				evidenceArg := psEvidence.Apply(eq.Head.Args[i])
				if err := s.env.Unify(wantedArg, evidenceArg); err != nil {
					return false
				}
			}
			if len(eq.Context) != len(qc.Context) {
				return false
			}
			// Sort both context lists by canonical key for order-independent matching.
			eqOrder := sortedConstraintIndices(eq.Context)
			qcCtxOrder := sortedConstraintIndices(qc.Context)
			for idx := range eqOrder {
				ec := eq.Context[eqOrder[idx]]
				qcc := qc.Context[qcCtxOrder[idx]]
				// EqualityEntry: unify both sides of the equality.
				if ecEq, ok := ec.(*types.EqualityEntry); ok {
					qccEq, ok := qcc.(*types.EqualityEntry)
					if !ok {
						return false
					}
					if err := s.env.Unify(psEvidence.Apply(ecEq.Lhs), psWanted.Apply(qccEq.Lhs)); err != nil {
						return false
					}
					if err := s.env.Unify(psEvidence.Apply(ecEq.Rhs), psWanted.Apply(qccEq.Rhs)); err != nil {
						return false
					}
					continue
				}
				// ClassEntry and other class-headed entries: name + pairwise arg unification.
				if types.HeadClassName(ec) != types.HeadClassName(qcc) {
					return false
				}
				ecArgs := types.HeadClassArgs(ec)
				qccArgs := types.HeadClassArgs(qcc)
				if len(ecArgs) != len(qccArgs) {
					return false
				}
				for j := range ecArgs {
					eArg := psEvidence.Apply(ecArgs[j])
					qArg := psWanted.Apply(qccArgs[j])
					if err := s.env.Unify(eArg, qArg); err != nil {
						return false
					}
				}
			}
			return true
		}) {
			continue
		}
		// Solutions are not needed — only the binding name matters.
		qcResult = &ir.Var{Name: e.DictName, S: sp}
		break
	}
	if qcResult != nil {
		return qcResult
	}

	s.env.AddCodedError(diagnostic.ErrNoInstance, sp,
		"no instance for "+qc.Head.ClassName+" "+s.prettyTypeArgs(qc.Head.Args))
	return &ir.Var{Name: "<no-instance>", S: sp}
}

// FreshInstanceSubst creates a substitution mapping each free type variable
// in an instance's type arguments and context to a fresh meta variable.
// Uses pre-computed FreeVars to avoid repeated traversals. Each variable's
// kind is used to create the meta at the correct kind (Type vs Row).
func (s *Solver) FreshInstanceSubst(inst *env.InstanceInfo) map[string]types.Type {
	fvs := inst.FreeVars
	if len(fvs) == 0 {
		return nil
	}
	subst := make(map[string]types.Type, len(fvs))
	for _, v := range fvs {
		subst[v.Name] = s.env.FreshMeta(v.Kind)
	}
	return subst
}
