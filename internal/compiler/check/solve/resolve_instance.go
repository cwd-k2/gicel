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

// resolveFromContext scans context variables for an exact dictionary match.
func (s *Solver) resolveFromContext(className string, args []types.Type, sp span.Span) ir.Core {
	var result ir.Core
	s.env.ScanContext(func(entry env.CtxEntry) bool {
		if v, ok := entry.(*env.CtxVar); ok && !v.SolverInvisible && s.matchesDictVar(v, className, args) {
			result = &ir.Var{Name: v.Name, Module: v.Module, S: sp}
			return false
		}
		return true
	})
	return result
}

// resolveFromSuperclasses searches context for dictionaries whose superclass
// hierarchy contains the target class.
func (s *Solver) resolveFromSuperclasses(className string, args []types.Type, sp span.Span) ir.Core {
	var result ir.Core
	s.env.ScanContext(func(entry env.CtxEntry) bool {
		if v, ok := entry.(*env.CtxVar); ok && !v.SolverInvisible {
			if expr := s.extractSuperDict(v, className, args, sp); expr != nil {
				result = expr
				return false
			}
		}
		return true
	})
	return result
}

// resolveFromQuantifiedEvidence searches context for quantified evidence
// entries that can be instantiated to produce the needed dictionary.
func (s *Solver) resolveFromQuantifiedEvidence(className string, args []types.Type, sp span.Span) ir.Core {
	var result ir.Core
	s.env.ScanContext(func(entry env.CtxEntry) bool {
		if e, ok := entry.(*env.CtxEvidence); ok && e.Quantified != nil {
			if expr := s.applyQuantifiedEvidence(e, className, args, sp); expr != nil {
				result = expr
				return false
			}
		}
		return true
	})
	return result
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
		if !s.env.WithTrial(func() bool {
			for i := range args {
				instArg := types.SubstMany(inst.TypeArgs[i], freshSubst)
				if err := s.env.Unify(instArg, args[i]); err != nil {
					return false
				}
			}
			return true
		}) {
			continue
		}
		var dictExpr ir.Core = &ir.Var{Name: inst.DictBindName, Module: inst.Module, S: sp}
		for _, ctx := range inst.Context {
			ctxArgs := make([]types.Type, len(ctx.Args))
			for j, a := range ctx.Args {
				ctxArgs[j] = s.env.Zonk(types.SubstMany(a, freshSubst))
			}
			ctxDict := s.resolveInstance(ctx.ClassName, ctxArgs, sp)
			dictExpr = &ir.App{Fun: dictExpr, Arg: ctxDict, S: sp}
		}
		return dictExpr
	}
	return nil
}
