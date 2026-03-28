// Package optimize provides Core IR optimization passes.
//
// INVARIANT: Optimization rules MUST NOT introduce new TyApp nodes.
// The label erasure pass (ir.EraseLabelArgsProgram) runs BEFORE optimization
// and converts Label-kinded TyApp nodes into App+Lit. If an optimization
// rule were to synthesize a new TyApp with a label argument, it would
// bypass erasure and cause a runtime type-application error.
package optimize

import (
	"fmt"

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
	fv := ir.FreeVars(app.Arg)
	return substFV(lam.Body, lam.Param, app.Arg, fv)
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
		// Simultaneous substitution: collect all pattern→arg mappings,
		// then apply in a single tree traversal.
		subs := make(map[string]ir.Core)
		subsFV := make(map[string]struct{})
		bindings := pcon.Args
		for i, pat := range bindings {
			if i >= len(con.Args) {
				break
			}
			if pv, ok := pat.(*ir.PVar); ok {
				subs[pv.Name] = con.Args[i]
				for k := range ir.FreeVars(con.Args[i]) {
					subsFV[k] = struct{}{}
				}
			}
		}
		if len(subs) == 0 {
			return alt.Body
		}
		return substMany(alt.Body, subs, subsFV)
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
	fv := ir.FreeVars(pure.Expr)
	return substFV(bind.Body, bind.Var, pure.Expr, fv)
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

// substFV replaces free occurrences of name with replacement in expr.
// replFV is the pre-computed free variable set of replacement.
func substFV(expr ir.Core, name string, replacement ir.Core, replFV map[string]struct{}) ir.Core {
	return substMany(expr, map[string]ir.Core{name: replacement}, replFV)
}

// substMany simultaneously replaces free occurrences of all names in subs.
// subsFV is the union of free variables across all replacement expressions.
func substMany(expr ir.Core, subs map[string]ir.Core, subsFV map[string]struct{}) ir.Core {
	switch n := expr.(type) {
	case *ir.Var:
		if repl, ok := subs[n.Name]; ok {
			return ir.Clone(repl)
		}
		return n
	case *ir.Lam:
		if _, shadowed := subs[n.Param]; shadowed {
			// Remove shadowed name from subs for the body.
			reduced := withoutKey(subs, n.Param)
			if len(reduced) == 0 {
				return n
			}
			return &ir.Lam{Param: n.Param, ParamType: n.ParamType, Body: substMany(n.Body, reduced, subsFV), FV: n.FV, Generated: n.Generated, S: n.S}
		}
		if _, captured := subsFV[n.Param]; captured {
			return n // would capture — bail out to preserve correctness
		}
		return &ir.Lam{Param: n.Param, ParamType: n.ParamType, Body: substMany(n.Body, subs, subsFV), FV: n.FV, Generated: n.Generated, S: n.S}
	case *ir.App:
		return &ir.App{Fun: substMany(n.Fun, subs, subsFV), Arg: substMany(n.Arg, subs, subsFV), S: n.S}
	case *ir.TyApp:
		return &ir.TyApp{Expr: substMany(n.Expr, subs, subsFV), TyArg: n.TyArg, S: n.S}
	case *ir.TyLam:
		return &ir.TyLam{TyParam: n.TyParam, Kind: n.Kind, Body: substMany(n.Body, subs, subsFV), S: n.S}
	case *ir.Con:
		args := make([]ir.Core, len(n.Args))
		for i, a := range n.Args {
			args[i] = substMany(a, subs, subsFV)
		}
		return &ir.Con{Name: n.Name, Args: args, S: n.S}
	case *ir.Case:
		alts := make([]ir.Alt, len(n.Alts))
		for i, alt := range n.Alts {
			// Check if any substitution name is bound by this pattern.
			active := subs
			for _, b := range alt.Pattern.Bindings() {
				if _, ok := active[b]; ok {
					active = withoutKey(active, b)
				}
			}
			if len(active) == 0 {
				alts[i] = alt
			} else {
				alts[i] = ir.Alt{Pattern: alt.Pattern, Body: substMany(alt.Body, active, subsFV), Generated: alt.Generated, S: alt.S}
			}
		}
		return &ir.Case{Scrutinee: substMany(n.Scrutinee, subs, subsFV), Alts: alts, S: n.S}
	case *ir.Fix:
		if _, shadowed := subs[n.Name]; shadowed {
			reduced := withoutKey(subs, n.Name)
			if len(reduced) == 0 {
				return n
			}
			return &ir.Fix{Name: n.Name, Body: substMany(n.Body, reduced, subsFV), S: n.S}
		}
		if _, captured := subsFV[n.Name]; captured {
			return n
		}
		return &ir.Fix{Name: n.Name, Body: substMany(n.Body, subs, subsFV), S: n.S}
	case *ir.Pure:
		return &ir.Pure{Expr: substMany(n.Expr, subs, subsFV), S: n.S}
	case *ir.Bind:
		comp := substMany(n.Comp, subs, subsFV)
		active := subs
		if _, shadowed := active[n.Var]; shadowed {
			active = withoutKey(active, n.Var)
		} else if _, captured := subsFV[n.Var]; captured {
			return &ir.Bind{Comp: comp, Var: n.Var, Discard: n.Discard, Body: n.Body, Generated: n.Generated, S: n.S}
		}
		if len(active) == 0 {
			return &ir.Bind{Comp: comp, Var: n.Var, Discard: n.Discard, Body: n.Body, Generated: n.Generated, S: n.S}
		}
		return &ir.Bind{Comp: comp, Var: n.Var, Discard: n.Discard, Body: substMany(n.Body, active, subsFV), Generated: n.Generated, S: n.S}
	case *ir.Thunk:
		return &ir.Thunk{Comp: substMany(n.Comp, subs, subsFV), FV: n.FV, S: n.S}
	case *ir.Force:
		return &ir.Force{Expr: substMany(n.Expr, subs, subsFV), S: n.S}
	case *ir.PrimOp:
		args := make([]ir.Core, len(n.Args))
		for i, a := range n.Args {
			args[i] = substMany(a, subs, subsFV)
		}
		return &ir.PrimOp{Name: n.Name, Arity: n.Arity, Effectful: n.Effectful, Args: args, S: n.S}
	case *ir.Lit:
		return n
	case *ir.Error:
		return n
	case *ir.RecordLit:
		fields := make([]ir.RecordField, len(n.Fields))
		for i, f := range n.Fields {
			fields[i] = ir.RecordField{Label: f.Label, Value: substMany(f.Value, subs, subsFV)}
		}
		return &ir.RecordLit{Fields: fields, S: n.S}
	case *ir.RecordProj:
		return &ir.RecordProj{Record: substMany(n.Record, subs, subsFV), Label: n.Label, S: n.S}
	case *ir.RecordUpdate:
		updates := make([]ir.RecordField, len(n.Updates))
		for i, f := range n.Updates {
			updates[i] = ir.RecordField{Label: f.Label, Value: substMany(f.Value, subs, subsFV)}
		}
		return &ir.RecordUpdate{Record: substMany(n.Record, subs, subsFV), Updates: updates, S: n.S}
	}
	panic(fmt.Sprintf("substMany: unhandled Core node %T", expr))
}

// withoutKey returns a copy of m without the given key.
func withoutKey(m map[string]ir.Core, key string) map[string]ir.Core {
	if len(m) <= 1 {
		return nil
	}
	reduced := make(map[string]ir.Core, len(m)-1)
	for k, v := range m {
		if k != key {
			reduced[k] = v
		}
	}
	return reduced
}
