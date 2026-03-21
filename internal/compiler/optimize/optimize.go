// Package opt provides Core IR optimization passes.
package optimize

import (
	"fmt"
	"slices"

	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// optimize applies algebraic simplifications and registered rewrite rules.
// Runs up to maxPasses bottom-up passes, stopping early when a pass
// makes no changes (fixed-point detection).
func optimize(c ir.Core, rules []func(ir.Core) ir.Core) ir.Core {
	const maxPasses = 4
	for range maxPasses {
		rw := &rewriter{rules: rules}
		c = ir.Transform(c, rw.apply)
		if !rw.changed {
			break
		}
	}
	return c
}

// OptimizeProgram optimizes all bindings in a program.
func OptimizeProgram(prog *ir.Program, rules []func(ir.Core) ir.Core) {
	for i, b := range prog.Bindings {
		prog.Bindings[i].Expr = optimize(b.Expr, rules)
	}
}

// rewriter tracks whether any transformation fired during a pass.
type rewriter struct {
	rules   []func(ir.Core) ir.Core
	changed bool
}

// apply runs algebraic simplifications and registered fusion rules on a
// single node, setting changed if any rule fires.
func (rw *rewriter) apply(c ir.Core) ir.Core {
	orig := c
	// Phase 1: algebraic simplifications (always active).
	c = betaReduce(c)
	c = caseOfKnownCtor(c)
	c = bindPureElim(c)
	c = forceThunkElim(c)
	c = recordProjKnown(c)
	c = recordUpdateChain(c)
	// Phase 4: registered fusion rules.
	for _, rule := range rw.rules {
		c = rule(c)
	}
	if c != orig {
		rw.changed = true
	}
	return c
}

// R2: App (Lam x body) arg  →  body[x := arg]
func betaReduce(c ir.Core) ir.Core {
	app, ok := c.(*ir.App)
	if !ok {
		return c
	}
	lam, ok := app.Fun.(*ir.Lam)
	if !ok {
		return c
	}
	return subst(lam.Body, lam.Param, app.Arg)
}

// R1: Case (Con C args) alts  →  matching alt with substitution
func caseOfKnownCtor(c ir.Core) ir.Core {
	cs, ok := c.(*ir.Case)
	if !ok {
		return c
	}
	con, ok := cs.Scrutinee.(*ir.Con)
	if !ok {
		return c
	}
	for _, alt := range cs.Alts {
		pcon, ok := alt.Pattern.(*ir.PCon)
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
			if pv, ok := pat.(*ir.PVar); ok {
				body = subst(body, pv.Name, con.Args[i])
			}
		}
		return body
	}
	return c
}

// R3: Bind (Pure e) x body  →  body[x := e]
func bindPureElim(c ir.Core) ir.Core {
	bind, ok := c.(*ir.Bind)
	if !ok {
		return c
	}
	pure, ok := bind.Comp.(*ir.Pure)
	if !ok {
		return c
	}
	return subst(bind.Body, bind.Var, pure.Expr)
}

// R4: Force (Thunk comp)  →  comp
func forceThunkElim(c ir.Core) ir.Core {
	force, ok := c.(*ir.Force)
	if !ok {
		return c
	}
	thunk, ok := force.Expr.(*ir.Thunk)
	if !ok {
		return c
	}
	return thunk.Comp
}

// R5: RecordProj (RecordLit fields) l  →  fields[l]
func recordProjKnown(c ir.Core) ir.Core {
	proj, ok := c.(*ir.RecordProj)
	if !ok {
		return c
	}
	rec, ok := proj.Record.(*ir.RecordLit)
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
func recordUpdateChain(c ir.Core) ir.Core {
	outer, ok := c.(*ir.RecordUpdate)
	if !ok {
		return c
	}
	inner, ok := outer.Record.(*ir.RecordUpdate)
	if !ok {
		return c
	}
	// Merge: outer labels shadow inner labels.
	outerLabels := make(map[string]bool, len(outer.Updates))
	for _, u := range outer.Updates {
		outerLabels[u.Label] = true
	}
	var merged []ir.RecordField
	for _, u := range inner.Updates {
		if !outerLabels[u.Label] {
			merged = append(merged, u)
		}
	}
	merged = append(merged, outer.Updates...)
	return &ir.RecordUpdate{Record: inner.Record, Updates: merged, S: outer.Span()}
}

// subst replaces free occurrences of name with replacement in expr.
func subst(expr ir.Core, name string, replacement ir.Core) ir.Core {
	switch n := expr.(type) {
	case *ir.Var:
		if n.Name == name {
			return replacement
		}
		return n
	case *ir.Lam:
		if n.Param == name {
			return n // shadowed
		}
		if capturedBy(n.Param, replacement) {
			return n // would capture — bail out to preserve correctness
		}
		return &ir.Lam{Param: n.Param, ParamType: n.ParamType, Body: subst(n.Body, name, replacement), FV: n.FV, Generated: n.Generated, S: n.S}
	case *ir.App:
		return &ir.App{Fun: subst(n.Fun, name, replacement), Arg: subst(n.Arg, name, replacement), S: n.S}
	case *ir.TyApp:
		return &ir.TyApp{Expr: subst(n.Expr, name, replacement), TyArg: n.TyArg, S: n.S}
	case *ir.TyLam:
		return &ir.TyLam{TyParam: n.TyParam, Kind: n.Kind, Body: subst(n.Body, name, replacement), S: n.S}
	case *ir.Con:
		args := make([]ir.Core, len(n.Args))
		for i, a := range n.Args {
			args[i] = subst(a, name, replacement)
		}
		return &ir.Con{Name: n.Name, Args: args, S: n.S}
	case *ir.Case:
		alts := make([]ir.Alt, len(n.Alts))
		for i, alt := range n.Alts {
			if binds(alt.Pattern, name) {
				alts[i] = alt
			} else {
				alts[i] = ir.Alt{Pattern: alt.Pattern, Body: subst(alt.Body, name, replacement), Generated: alt.Generated, S: alt.S}
			}
		}
		return &ir.Case{Scrutinee: subst(n.Scrutinee, name, replacement), Alts: alts, S: n.S}
	case *ir.Fix:
		if n.Name == name {
			return n // shadowed by fix binding
		}
		if capturedBy(n.Name, replacement) {
			return n
		}
		return &ir.Fix{Name: n.Name, Body: subst(n.Body, name, replacement), S: n.S}
	case *ir.Pure:
		return &ir.Pure{Expr: subst(n.Expr, name, replacement), S: n.S}
	case *ir.Bind:
		comp := subst(n.Comp, name, replacement)
		if n.Var == name {
			return &ir.Bind{Comp: comp, Var: n.Var, Body: n.Body, Generated: n.Generated, S: n.S}
		}
		if capturedBy(n.Var, replacement) {
			return &ir.Bind{Comp: comp, Var: n.Var, Body: n.Body, Generated: n.Generated, S: n.S}
		}
		return &ir.Bind{Comp: comp, Var: n.Var, Body: subst(n.Body, name, replacement), Generated: n.Generated, S: n.S}
	case *ir.Thunk:
		return &ir.Thunk{Comp: subst(n.Comp, name, replacement), FV: n.FV, S: n.S}
	case *ir.Force:
		return &ir.Force{Expr: subst(n.Expr, name, replacement), S: n.S}
	case *ir.PrimOp:
		args := make([]ir.Core, len(n.Args))
		for i, a := range n.Args {
			args[i] = subst(a, name, replacement)
		}
		return &ir.PrimOp{Name: n.Name, Arity: n.Arity, Effectful: n.Effectful, Args: args, S: n.S}
	case *ir.Lit:
		return n
	case *ir.RecordLit:
		fields := make([]ir.RecordField, len(n.Fields))
		for i, f := range n.Fields {
			fields[i] = ir.RecordField{Label: f.Label, Value: subst(f.Value, name, replacement)}
		}
		return &ir.RecordLit{Fields: fields, S: n.S}
	case *ir.RecordProj:
		return &ir.RecordProj{Record: subst(n.Record, name, replacement), Label: n.Label, S: n.S}
	case *ir.RecordUpdate:
		updates := make([]ir.RecordField, len(n.Updates))
		for i, f := range n.Updates {
			updates[i] = ir.RecordField{Label: f.Label, Value: subst(f.Value, name, replacement)}
		}
		return &ir.RecordUpdate{Record: subst(n.Record, name, replacement), Updates: updates, S: n.S}
	}
	panic(fmt.Sprintf("subst: unhandled Core node %T", expr))
}

// capturedBy checks if binder would capture a free variable in replacement.
// If so, subst must bail out to avoid incorrect variable capture.
func capturedBy(binder string, replacement ir.Core) bool {
	_, free := ir.FreeVars(replacement)[binder]
	return free
}

// binds checks if a pattern binds the given name.
func binds(p ir.Pattern, name string) bool {
	return slices.Contains(p.Bindings(), name)
}
