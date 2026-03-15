// Package opt provides Core IR optimization passes.
package opt

import (
	"fmt"
	"slices"

	"github.com/cwd-k2/gicel/internal/core"
)

// Optimize applies algebraic simplifications and registered rewrite rules.
// Runs a fixed number of bottom-up passes. Each pass applies all rules
// at every node. Multiple passes handle cases where one transformation
// creates opportunities for another at a higher tree level.
func Optimize(c core.Core, rules []func(core.Core) core.Core) core.Core {
	rewrite := makeRewriter(rules)
	const maxPasses = 4
	for range maxPasses {
		c = core.Transform(c, rewrite)
	}
	return c
}

// OptimizeProgram optimizes all bindings in a program.
func OptimizeProgram(prog *core.Program, rules []func(core.Core) core.Core) {
	for i, b := range prog.Bindings {
		prog.Bindings[i].Expr = Optimize(b.Expr, rules)
	}
}

// makeRewriter builds the bottom-up rewrite function that applies
// algebraic simplifications followed by registered fusion rules.
func makeRewriter(rules []func(core.Core) core.Core) func(core.Core) core.Core {
	return func(c core.Core) core.Core {
		// Phase 1: algebraic simplifications (always active).
		c = betaReduce(c)
		c = caseOfKnownCtor(c)
		c = bindPureElim(c)
		c = forceThunkElim(c)
		c = recordProjKnown(c)
		c = recordUpdateChain(c)
		// Phase 4: registered fusion rules.
		for _, rule := range rules {
			c = rule(c)
		}
		return c
	}
}

// R2: App (Lam x body) arg  →  body[x := arg]
func betaReduce(c core.Core) core.Core {
	app, ok := c.(*core.App)
	if !ok {
		return c
	}
	lam, ok := app.Fun.(*core.Lam)
	if !ok {
		return c
	}
	return subst(lam.Body, lam.Param, app.Arg)
}

// R1: Case (Con C args) alts  →  matching alt with substitution
func caseOfKnownCtor(c core.Core) core.Core {
	cs, ok := c.(*core.Case)
	if !ok {
		return c
	}
	con, ok := cs.Scrutinee.(*core.Con)
	if !ok {
		return c
	}
	for _, alt := range cs.Alts {
		pcon, ok := alt.Pattern.(*core.PCon)
		if !ok {
			continue
		}
		if pcon.Con != con.Name {
			continue
		}
		body := alt.Body
		bindings := pcon.Args
		for i, pat := range bindings {
			if i >= len(con.Args) {
				break
			}
			if pv, ok := pat.(*core.PVar); ok {
				body = subst(body, pv.Name, con.Args[i])
			}
		}
		return body
	}
	return c
}

// R3: Bind (Pure e) x body  →  body[x := e]
func bindPureElim(c core.Core) core.Core {
	bind, ok := c.(*core.Bind)
	if !ok {
		return c
	}
	pure, ok := bind.Comp.(*core.Pure)
	if !ok {
		return c
	}
	return subst(bind.Body, bind.Var, pure.Expr)
}

// R4: Force (Thunk comp)  →  comp
func forceThunkElim(c core.Core) core.Core {
	force, ok := c.(*core.Force)
	if !ok {
		return c
	}
	thunk, ok := force.Expr.(*core.Thunk)
	if !ok {
		return c
	}
	return thunk.Comp
}

// R5: RecordProj (RecordLit fields) l  →  fields[l]
func recordProjKnown(c core.Core) core.Core {
	proj, ok := c.(*core.RecordProj)
	if !ok {
		return c
	}
	rec, ok := proj.Record.(*core.RecordLit)
	if !ok {
		return c
	}
	for _, f := range rec.Fields {
		if f.Label == proj.Label {
			return f.Value
		}
	}
	return c
}

// R6: RecordUpdate (RecordUpdate r inner) outer  →  RecordUpdate r merged
func recordUpdateChain(c core.Core) core.Core {
	outer, ok := c.(*core.RecordUpdate)
	if !ok {
		return c
	}
	inner, ok := outer.Record.(*core.RecordUpdate)
	if !ok {
		return c
	}
	// Merge: outer labels shadow inner labels.
	outerLabels := make(map[string]bool, len(outer.Updates))
	for _, u := range outer.Updates {
		outerLabels[u.Label] = true
	}
	var merged []core.RecordField
	for _, u := range inner.Updates {
		if !outerLabels[u.Label] {
			merged = append(merged, u)
		}
	}
	merged = append(merged, outer.Updates...)
	return &core.RecordUpdate{Record: inner.Record, Updates: merged, S: outer.Span()}
}

// subst replaces free occurrences of name with replacement in expr.
func subst(expr core.Core, name string, replacement core.Core) core.Core {
	switch n := expr.(type) {
	case *core.Var:
		if n.Name == name {
			return replacement
		}
		return n
	case *core.Lam:
		if n.Param == name {
			return n // shadowed
		}
		if capturedBy(n.Param, replacement) {
			return n // would capture — bail out to preserve correctness
		}
		return &core.Lam{Param: n.Param, ParamType: n.ParamType, Body: subst(n.Body, name, replacement), FV: n.FV, S: n.S}
	case *core.App:
		return &core.App{Fun: subst(n.Fun, name, replacement), Arg: subst(n.Arg, name, replacement), S: n.S}
	case *core.TyApp:
		return &core.TyApp{Expr: subst(n.Expr, name, replacement), TyArg: n.TyArg, S: n.S}
	case *core.TyLam:
		return &core.TyLam{TyParam: n.TyParam, Kind: n.Kind, Body: subst(n.Body, name, replacement), S: n.S}
	case *core.Con:
		args := make([]core.Core, len(n.Args))
		for i, a := range n.Args {
			args[i] = subst(a, name, replacement)
		}
		return &core.Con{Name: n.Name, Args: args, S: n.S}
	case *core.Case:
		alts := make([]core.Alt, len(n.Alts))
		for i, alt := range n.Alts {
			if binds(alt.Pattern, name) {
				alts[i] = alt
			} else {
				alts[i] = core.Alt{Pattern: alt.Pattern, Body: subst(alt.Body, name, replacement), S: alt.S}
			}
		}
		return &core.Case{Scrutinee: subst(n.Scrutinee, name, replacement), Alts: alts, S: n.S}
	case *core.LetRec:
		for _, b := range n.Bindings {
			if b.Name == name {
				return n // shadowed by letrec binding
			}
		}
		// Guard: if replacement contains a free variable that would be
		// captured by a letrec binder, bail out to preserve correctness.
		for _, b := range n.Bindings {
			if capturedBy(b.Name, replacement) {
				return n
			}
		}
		bindings := make([]core.Binding, len(n.Bindings))
		for i, b := range n.Bindings {
			bindings[i] = core.Binding{Name: b.Name, Type: b.Type, Expr: subst(b.Expr, name, replacement), S: b.S}
		}
		return &core.LetRec{Bindings: bindings, Body: subst(n.Body, name, replacement), S: n.S}
	case *core.Pure:
		return &core.Pure{Expr: subst(n.Expr, name, replacement), S: n.S}
	case *core.Bind:
		comp := subst(n.Comp, name, replacement)
		if n.Var == name {
			return &core.Bind{Comp: comp, Var: n.Var, Body: n.Body, S: n.S}
		}
		if capturedBy(n.Var, replacement) {
			return &core.Bind{Comp: comp, Var: n.Var, Body: n.Body, S: n.S}
		}
		return &core.Bind{Comp: comp, Var: n.Var, Body: subst(n.Body, name, replacement), S: n.S}
	case *core.Thunk:
		return &core.Thunk{Comp: subst(n.Comp, name, replacement), FV: n.FV, S: n.S}
	case *core.Force:
		return &core.Force{Expr: subst(n.Expr, name, replacement), S: n.S}
	case *core.PrimOp:
		args := make([]core.Core, len(n.Args))
		for i, a := range n.Args {
			args[i] = subst(a, name, replacement)
		}
		return &core.PrimOp{Name: n.Name, Arity: n.Arity, Effectful: n.Effectful, Args: args, S: n.S}
	case *core.Lit:
		return n
	case *core.RecordLit:
		fields := make([]core.RecordField, len(n.Fields))
		for i, f := range n.Fields {
			fields[i] = core.RecordField{Label: f.Label, Value: subst(f.Value, name, replacement)}
		}
		return &core.RecordLit{Fields: fields, S: n.S}
	case *core.RecordProj:
		return &core.RecordProj{Record: subst(n.Record, name, replacement), Label: n.Label, S: n.S}
	case *core.RecordUpdate:
		updates := make([]core.RecordField, len(n.Updates))
		for i, f := range n.Updates {
			updates[i] = core.RecordField{Label: f.Label, Value: subst(f.Value, name, replacement)}
		}
		return &core.RecordUpdate{Record: subst(n.Record, name, replacement), Updates: updates, S: n.S}
	}
	panic(fmt.Sprintf("subst: unhandled Core node %T", expr))
}

// capturedBy checks if binder would capture a free variable in replacement.
// If so, subst must bail out to avoid incorrect variable capture.
func capturedBy(binder string, replacement core.Core) bool {
	_, free := core.FreeVars(replacement)[binder]
	return free
}

// binds checks if a pattern binds the given name.
func binds(p core.Pattern, name string) bool {
	return slices.Contains(p.Bindings(), name)
}
