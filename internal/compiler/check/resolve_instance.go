package check

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Instance resolution phases, called in sequence by resolveInstance.
// Each returns a dictionary expression on success, or nil to delegate
// to the next phase.

// resolveFromContext scans context variables for an exact dictionary match.
func (ch *Checker) resolveFromContext(className string, args []types.Type, s span.Span) ir.Core {
	var result ir.Core
	ch.ctx.Scan(func(entry CtxEntry) bool {
		if v, ok := entry.(*CtxVar); ok && ch.matchesDictVar(v, className, args) {
			result = &ir.Var{Name: v.Name, Module: v.Module, S: s}
			return false
		}
		return true
	})
	return result
}

// resolveFromSuperclasses searches context for dictionaries whose superclass
// hierarchy contains the target class.
func (ch *Checker) resolveFromSuperclasses(className string, args []types.Type, s span.Span) ir.Core {
	var result ir.Core
	ch.ctx.Scan(func(entry CtxEntry) bool {
		if v, ok := entry.(*CtxVar); ok {
			if expr := ch.extractSuperDict(v, className, args, s); expr != nil {
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
func (ch *Checker) resolveFromQuantifiedEvidence(className string, args []types.Type, s span.Span) ir.Core {
	var result ir.Core
	ch.ctx.Scan(func(entry CtxEntry) bool {
		if e, ok := entry.(*CtxEvidence); ok && e.Quantified != nil {
			if expr := ch.applyQuantifiedEvidence(e, className, args, s); expr != nil {
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
func (ch *Checker) resolveFromGlobalInstances(className string, args []types.Type, s span.Span) ir.Core {
	for _, inst := range ch.reg.InstancesForClass(className) {
		if len(inst.TypeArgs) != len(args) {
			continue
		}
		freshSubst := ch.freshInstanceSubst(inst)
		if !ch.withTrial(func() bool {
			for i := range args {
				instArg := types.SubstMany(inst.TypeArgs[i], freshSubst)
				if err := ch.unifier.Unify(instArg, args[i]); err != nil {
					return false
				}
			}
			return true
		}) {
			continue
		}
		var dictExpr ir.Core = &ir.Var{Name: inst.DictBindName, Module: inst.Module, S: s}
		for _, ctx := range inst.Context {
			ctxArgs := make([]types.Type, len(ctx.Args))
			for j, a := range ctx.Args {
				ctxArgs[j] = ch.unifier.Zonk(types.SubstMany(a, freshSubst))
			}
			ctxDict := ch.resolveInstance(ctx.ClassName, ctxArgs, s)
			dictExpr = &ir.App{Fun: dictExpr, Arg: ctxDict, S: s}
		}
		return dictExpr
	}
	return nil
}
