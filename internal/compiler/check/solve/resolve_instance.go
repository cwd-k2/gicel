package solve

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Instance resolution phases, called in sequence by resolveInstance.
// Each returns a dictionary expression on success, or nil to delegate
// to the next phase.

// resolveFromContext looks up context dict variables for an exact dictionary match.
// Uses the dictVarIndex for O(1) class-name lookup when available, then falls back
// to a full context scan for dict vars without DictClassName (e.g., pattern bindings).
func (s *Solver) resolveFromContext(className string, args []types.Type, sp span.Span) ir.Core {
	// Fast path: indexed lookup.
	for _, v := range s.env.LookupDictVar(className) {
		if s.matchesDictVar(v, className, args) {
			return &ir.Var{Name: v.Name, Module: v.Module, S: sp}
		}
	}
	// Slow path: scan for dict vars not in the index (no DictClassName set).
	var result ir.Core
	s.env.ScanContext(func(entry env.CtxEntry) bool {
		if v, ok := entry.(*env.CtxVar); ok && !v.SolverInvisible && v.DictClassName == "" && s.matchesDictVar(v, className, args) {
			result = &ir.Var{Name: v.Name, Module: v.Module, S: sp}
			return false
		}
		return true
	})
	return result
}

// resolveFromSuperclasses searches context for dictionaries whose superclass
// hierarchy contains the target class. Iterates only dict variable classes
// present in the context whose SuperClosure includes the target, avoiding
// a full linear context scan.
func (s *Solver) resolveFromSuperclasses(className string, args []types.Type, sp span.Span) ir.Core {
	for _, parentClass := range s.env.DictVarClasses() {
		// Skip if this class doesn't have className as a transitive super.
		if classInfo, ok := s.env.LookupClass(parentClass); ok {
			if classInfo.SuperClosure != nil && !classInfo.SuperClosure[className] {
				continue
			}
		}
		for _, v := range s.env.LookupDictVar(parentClass) {
			if expr := s.extractSuperDict(v, className, args, sp); expr != nil {
				return expr
			}
		}
	}
	return nil
}

// resolveFromQuantifiedEvidence searches context for quantified evidence
// entries that can be instantiated to produce the needed dictionary.
// Uses the indexed evidence lookup for O(1) class-name access.
func (s *Solver) resolveFromQuantifiedEvidence(className string, args []types.Type, sp span.Span) ir.Core {
	for _, e := range s.env.LookupEvidence(className) {
		if e.Quantified != nil {
			if expr := s.applyQuantifiedEvidence(e, className, args, sp); expr != nil {
				return expr
			}
		}
	}
	return nil
}

// resolveFromGlobalInstances searches the global instance registry for a
// matching instance, unifying type arguments via trial and recursively
// resolving context dictionaries.
func (s *Solver) resolveFromGlobalInstances(className string, args []types.Type, sp span.Span) ir.Core {
	for _, inst := range s.env.InstancesForClass(className) {
		// Private instances are solver-invisible in global search.
		// They are accessible only via explicit evidence injection (value => expr).
		if inst.Private {
			continue
		}
		if len(inst.TypeArgs) != len(args) {
			continue
		}
		freshSubst := s.FreshInstanceSubst(inst)
		ps := types.PrepareSubst(freshSubst)
		var dictExpr ir.Core
		// Wrap both head unification and context resolution in a single trial
		// so that if context resolution fails, head solutions are rolled back.
		savedErrs := s.env.ErrorCount()
		if !s.trialScope(func() bool {
			// Head match.
			for i := range args {
				instArg := ps.Apply(inst.TypeArgs[i])
				if err := s.env.Unify(instArg, args[i]); err != nil {
					return false
				}
			}
			// Context resolution (recursive).
			dictExpr = &ir.Var{Name: inst.DictBindName, Module: inst.Module, S: sp}
			for _, ctx := range inst.Context {
				ctxArgs := make([]types.Type, len(ctx.Args))
				for j, a := range ctx.Args {
					ctxArgs[j] = s.env.Zonk(ps.Apply(a))
				}
				ctxDict := s.resolveInstance(ctx.ClassName, ctxArgs, sp)
				dictExpr = &ir.App{Fun: dictExpr, Arg: ctxDict, S: sp}
			}
			// If context resolution emitted errors, treat as failure.
			return s.env.ErrorCount() == savedErrs
		}) {
			// Roll back any errors emitted during the failed trial.
			s.env.TruncateErrors(savedErrs)
			continue
		}
		return dictExpr
	}
	return nil
}
