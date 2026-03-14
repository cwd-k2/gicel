package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/types"
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
		if dc.quantified != nil {
			// Resolve quantified constraint by finding matching evidence.
			resolved := ch.resolveQuantifiedConstraint(dc.quantified, dc.s)
			resolutions[dc.placeholder] = resolved
		} else if dc.constraintVar != nil {
			// Constraint variable: zonk and decompose into className + args.
			cv := ch.unifier.Zonk(dc.constraintVar)
			cn, cArgs, ok := DecomposeConstraintType(cv)
			if ok {
				resolved := ch.resolveInstance(cn, cArgs, dc.s)
				resolutions[dc.placeholder] = resolved
			} else {
				// Still unresolved — try using className/args if available.
				if dc.className != "" {
					zonkedArgs := make([]types.Type, len(dc.args))
					for i, a := range dc.args {
						zonkedArgs[i] = ch.unifier.Zonk(a)
					}
					resolved := ch.resolveInstance(dc.className, zonkedArgs, dc.s)
					resolutions[dc.placeholder] = resolved
				} else {
					ch.addCodedError(errs.ErrNoInstance, dc.s,
						fmt.Sprintf("cannot resolve constraint variable %s", types.Pretty(cv)))
					resolutions[dc.placeholder] = &core.Var{Name: "<no-instance>", S: dc.s}
				}
			}
		} else {
			zonkedArgs := make([]types.Type, len(dc.args))
			for i, a := range dc.args {
				zonkedArgs[i] = ch.unifier.Zonk(a)
			}
			resolved := ch.resolveInstance(dc.className, zonkedArgs, dc.s)
			resolutions[dc.placeholder] = resolved
		}
	}
	ch.deferred = ch.deferred[:0]

	return ch.substitutePlaceholders(expr, resolutions)
}

// substitutePlaceholders replaces Var nodes matching placeholders via core.Transform.
func (ch *Checker) substitutePlaceholders(expr core.Core, resolutions map[string]core.Core) core.Core {
	return core.Transform(expr, func(c core.Core) core.Core {
		if v, ok := c.(*core.Var); ok {
			if resolved, ok := resolutions[v.Name]; ok {
				return resolved
			}
		}
		return c
	})
}
