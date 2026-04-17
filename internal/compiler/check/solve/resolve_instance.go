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
	// Pre-compute the expected dict type constructor name so we can reject
	// non-dictionary entries cheaply without zonking their types.
	dictTyCon := env.DictName(className)
	var result ir.Core
	s.env.ScanContext(func(entry env.CtxEntry) bool {
		v, ok := entry.(*env.CtxVar)
		if !ok || v.IsSolverInvisible || v.HasDictClass() {
			return true
		}
		// Cheap pre-filter: walk the type's TyApp spine without zonking.
		// If the head is a concrete non-dict TyCon or a TyArrow, this
		// variable cannot be a dictionary — skip it. Only fall through
		// to the full matchesDictVar (which zonks and unifies) when the
		// head is a TyMeta or the matching TyCon.
		if !couldBeDictType(v.Type, dictTyCon) {
			return true
		}
		if s.matchesDictVar(v, className, args) {
			result = &ir.Var{Name: v.Name, Module: v.Module, S: sp}
			return false
		}
		return true
	})
	return result
}

// couldBeDictType is a cheap pre-filter that walks the TyApp spine of a type
// WITHOUT zonking to check whether the head could be the expected dictionary
// type constructor. Returns false only when the head is a concrete type that
// is definitely not dictTyCon (e.g. TyArrow, a different TyCon). Returns true
// for TyMeta heads (need zonking to know) and matching TyCon heads.
func couldBeDictType(ty types.Type, dictTyCon string) bool {
	for {
		switch t := ty.(type) {
		case *types.TyApp:
			ty = t.Fun
			continue
		case *types.TyCon:
			return t.Name == dictTyCon
		case *types.TyMeta:
			return true // unknown — must fall through to full check
		default:
			return false // TyArrow, TyForall, TyVar, etc. — not a dict
		}
	}
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
		if inst.IsPrivate {
			continue
		}
		if len(inst.TypeArgs) != len(args) {
			continue
		}
		freshSubst := s.FreshInstanceSubst(inst)
		ps := s.TypeOps.PrepareSubst(freshSubst)
		var dictExpr ir.Core
		// Wrap both head unification and context resolution in a single trial
		// so that if context resolution fails, head solutions are rolled back.
		savedErrs := s.env.ErrorCount()
		if !s.trialScope(func() bool {
			// Head match.
			for i := range args {
				instArg := ps.Apply(s.TypeOps, inst.TypeArgs[i])
				if err := s.env.Unify(instArg, args[i]); err != nil {
					return false
				}
			}
			// Context resolution (recursive).
			dictExpr = &ir.Var{Name: inst.DictBindName, Module: inst.Module, S: sp}
			for _, ctx := range inst.Context {
				ctxArgs := make([]types.Type, len(ctx.Args))
				for j, a := range ctx.Args {
					ctxArgs[j] = s.env.Zonk(ps.Apply(s.TypeOps, a))
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
