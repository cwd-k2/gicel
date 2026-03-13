package check

import (
	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/types"
)

// resolveDeferredConstraints walks a Core expression and replaces
// placeholder dict variables with resolved instance dictionaries.
func (ch *Checker) resolveDeferredConstraints(expr core.Core) core.Core {
	if len(ch.deferred) == 0 {
		return expr
	}

	// Build resolution map: placeholder name → resolved Core expression.
	resolutions := make(map[string]core.Core)
	for _, dc := range ch.deferred {
		zonkedArgs := make([]types.Type, len(dc.args))
		for i, a := range dc.args {
			zonkedArgs[i] = ch.unifier.Zonk(a)
		}
		resolved := ch.resolveInstance(dc.className, zonkedArgs, dc.s)
		resolutions[dc.placeholder] = resolved
	}
	ch.deferred = ch.deferred[:0]

	return ch.substitutePlaceholders(expr, resolutions)
}

// substitutePlaceholders recursively walks Core and replaces Var nodes matching placeholders.
func (ch *Checker) substitutePlaceholders(expr core.Core, resolutions map[string]core.Core) core.Core {
	switch e := expr.(type) {
	case *core.Var:
		if resolved, ok := resolutions[e.Name]; ok {
			return resolved
		}
		return e
	case *core.Lam:
		return &core.Lam{Param: e.Param, ParamType: e.ParamType, Body: ch.substitutePlaceholders(e.Body, resolutions), S: e.S}
	case *core.App:
		return &core.App{Fun: ch.substitutePlaceholders(e.Fun, resolutions), Arg: ch.substitutePlaceholders(e.Arg, resolutions), S: e.S}
	case *core.TyApp:
		return &core.TyApp{Expr: ch.substitutePlaceholders(e.Expr, resolutions), TyArg: e.TyArg, S: e.S}
	case *core.TyLam:
		return &core.TyLam{TyParam: e.TyParam, Kind: e.Kind, Body: ch.substitutePlaceholders(e.Body, resolutions), S: e.S}
	case *core.Con:
		if len(e.Args) == 0 {
			return e
		}
		args := make([]core.Core, len(e.Args))
		for i, a := range e.Args {
			args[i] = ch.substitutePlaceholders(a, resolutions)
		}
		return &core.Con{Name: e.Name, Args: args, S: e.S}
	case *core.Case:
		scrut := ch.substitutePlaceholders(e.Scrutinee, resolutions)
		alts := make([]core.Alt, len(e.Alts))
		for i, alt := range e.Alts {
			alts[i] = core.Alt{Pattern: alt.Pattern, Body: ch.substitutePlaceholders(alt.Body, resolutions), S: alt.S}
		}
		return &core.Case{Scrutinee: scrut, Alts: alts, S: e.S}
	case *core.LetRec:
		binds := make([]core.Binding, len(e.Bindings))
		for i, b := range e.Bindings {
			binds[i] = core.Binding{Name: b.Name, Type: b.Type, Expr: ch.substitutePlaceholders(b.Expr, resolutions), S: b.S}
		}
		return &core.LetRec{Bindings: binds, Body: ch.substitutePlaceholders(e.Body, resolutions), S: e.S}
	case *core.Pure:
		return &core.Pure{Expr: ch.substitutePlaceholders(e.Expr, resolutions), S: e.S}
	case *core.Bind:
		return &core.Bind{Comp: ch.substitutePlaceholders(e.Comp, resolutions), Var: e.Var, Body: ch.substitutePlaceholders(e.Body, resolutions), S: e.S}
	case *core.Thunk:
		return &core.Thunk{Comp: ch.substitutePlaceholders(e.Comp, resolutions), S: e.S}
	case *core.Force:
		return &core.Force{Expr: ch.substitutePlaceholders(e.Expr, resolutions), S: e.S}
	case *core.PrimOp:
		if len(e.Args) == 0 {
			return e
		}
		args := make([]core.Core, len(e.Args))
		for i, a := range e.Args {
			args[i] = ch.substitutePlaceholders(a, resolutions)
		}
		return &core.PrimOp{Name: e.Name, Arity: e.Arity, Effectful: e.Effectful, Args: args, S: e.S}
	default:
		return expr
	}
}
