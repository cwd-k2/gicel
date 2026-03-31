// Name resolution — variable and constructor lookup with diagnostic hints.
// Does NOT cover: instantiation (bidir_inst.go), diagnostics (bidir_suggest.go).
package check

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// lookupVar resolves a variable name to its type and Core node.
func (ch *Checker) lookupVar(e *syntax.ExprVar) (types.Type, ir.Core, bool) {
	ty, mod, ok := ch.ctx.LookupVarFull(e.Name)
	if !ok {
		msg := "unbound variable: " + e.Name
		if gatedBuiltins[e.Name] {
			msg += " (requires --recursion flag)"
		}
		if hints := ch.suggestVar(e.Name); len(hints) > 0 {
			ch.addCodedErrorWithHints(diagnostic.ErrUnboundVar, e.S, msg, hints)
		} else {
			ch.addCodedError(diagnostic.ErrUnboundVar, e.S, msg)
		}
		return &types.TyError{S: e.S}, &ir.Var{Name: e.Name, S: e.S}, false
	}
	return ty, &ir.Var{Name: e.Name, Module: mod, S: e.S}, true
}

// lookupCon resolves a constructor name to its type and Core node.
func (ch *Checker) lookupCon(e *syntax.ExprCon) (types.Type, ir.Core, bool) {
	ty, ok := ch.reg.LookupConType(e.Name)
	if !ok {
		msg := "unknown constructor: " + e.Name
		if hints := ch.suggestCon(e.Name); len(hints) > 0 {
			ch.addCodedErrorWithHints(diagnostic.ErrUnboundCon, e.S, msg, hints)
		} else {
			ch.addCodedError(diagnostic.ErrUnboundCon, e.S, msg)
		}
		return &types.TyError{S: e.S}, &ir.Con{Name: e.Name, S: e.S}, false
	}
	mod, _ := ch.reg.LookupConModule(e.Name)
	return ty, &ir.Con{Name: e.Name, Module: mod, S: e.S}, true
}

// lookupQualVar resolves a qualified variable reference (N.add) to its type and Core node.
func (ch *Checker) lookupQualVar(e *syntax.ExprQualVar) (types.Type, ir.Core, bool) {
	qs, ok := ch.scope.LookupQualified(e.Qualifier)
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundVar, e.S, "unknown qualifier: "+e.Qualifier)
		return &types.TyError{S: e.S}, &ir.Var{Name: e.Name, S: e.S}, false
	}
	ty, ok := qs.Exports.Values[e.Name]
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundVar, e.S,
			"module "+qs.ModuleName+" does not export value: "+e.Name)
		return &types.TyError{S: e.S}, &ir.Var{Name: e.Name, S: e.S}, false
	}
	return ty, &ir.Var{Name: e.Name, Module: qs.ModuleName, S: e.S}, true
}

// lookupQualCon resolves a qualified constructor reference (N.Just) to its type and Core node.
func (ch *Checker) lookupQualCon(e *syntax.ExprQualCon) (types.Type, ir.Core, bool) {
	qs, ok := ch.scope.LookupQualified(e.Qualifier)
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundCon, e.S, "unknown qualifier: "+e.Qualifier)
		return &types.TyError{S: e.S}, &ir.Con{Name: e.Name, S: e.S}, false
	}
	ty, ok := qs.Exports.ConTypes[e.Name]
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundCon, e.S,
			"module "+qs.ModuleName+" does not export constructor: "+e.Name)
		return &types.TyError{S: e.S}, &ir.Con{Name: e.Name, S: e.S}, false
	}
	return ty, &ir.Con{Name: e.Name, Module: qs.ModuleName, S: e.S}, true
}
