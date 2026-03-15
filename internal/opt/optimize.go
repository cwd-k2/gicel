// Package opt provides Core IR optimization passes.
package opt

import "github.com/cwd-k2/gicel/internal/core"

// Optimize applies simplification rules to a Core expression.
// Runs multiple bottom-up passes until no further reductions apply.
func Optimize(c core.Core) core.Core {
	const maxPasses = 4
	for range maxPasses {
		prev := c
		c = core.Transform(c, simplify)
		if coreIdentical(prev, c) {
			break
		}
	}
	return c
}

// OptimizeProgram optimizes all bindings in a program.
func OptimizeProgram(prog *core.Program) {
	for i, b := range prog.Bindings {
		prog.Bindings[i].Expr = Optimize(b.Expr)
	}
}

// simplify is the bottom-up rewrite function applied at each node.
func simplify(c core.Core) core.Core {
	c = betaReduce(c)
	c = caseOfKnownCtor(c)
	c = bindPureElim(c)
	c = forceThunkElim(c)
	c = recordProjKnown(c)
	c = recordUpdateChain(c)
	c = sliceFusion(c)
	c = packedRoundtrip(c)
	return c
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

// R10/R11: Slice PrimOp fusion.
func sliceFusion(c core.Core) core.Core {
	po, ok := c.(*core.PrimOp)
	if !ok {
		return c
	}
	switch po.Name {
	case "_sliceMap":
		// R10: _sliceMap f (_sliceMap g xs) → _sliceMap (\$x -> f (g $x)) xs
		if len(po.Args) == 2 {
			if inner, ok := po.Args[1].(*core.PrimOp); ok && inner.Name == "_sliceMap" && len(inner.Args) == 2 {
				f, g, xs := po.Args[0], inner.Args[0], inner.Args[1]
				composed := makeCompose(f, g)
				return &core.PrimOp{Name: "_sliceMap", Arity: 2, Args: []core.Core{composed, xs}, S: po.S}
			}
		}
	case "_sliceFoldr":
		// R11: _sliceFoldr k z (_sliceMap f xs) → _sliceFoldr (\$x $acc -> k (f $x) $acc) z xs
		if len(po.Args) == 3 {
			if inner, ok := po.Args[2].(*core.PrimOp); ok && inner.Name == "_sliceMap" && len(inner.Args) == 2 {
				k, z, f, xs := po.Args[0], po.Args[1], inner.Args[0], inner.Args[1]
				fused := makeFoldMapFusion(k, f)
				return &core.PrimOp{Name: "_sliceFoldr", Arity: 3, Args: []core.Core{fused, z, xs}, S: po.S}
			}
		}
	}
	return c
}

// R12/R13: Packed roundtrip elimination.
func packedRoundtrip(c core.Core) core.Core {
	po, ok := c.(*core.PrimOp)
	if !ok {
		return c
	}
	if len(po.Args) != 1 {
		return c
	}
	inner, ok := po.Args[0].(*core.PrimOp)
	if !ok || len(inner.Args) != 1 {
		return c
	}
	// Check known roundtrip pairs: pack (unpack x) = x
	switch {
	case po.Name == "_fromRunes" && inner.Name == "_toRunes":
		return inner.Args[0] // R13
	case po.Name == "_sliceToList" && inner.Name == "_sliceFromList":
		return inner.Args[0] // R12
	}
	return c
}

// makeCompose builds \$x -> f (g $x).
func makeCompose(f, g core.Core) core.Core {
	x := "$opt_x"
	return &core.Lam{
		Param: x,
		Body: &core.App{
			Fun: f,
			Arg: &core.App{Fun: g, Arg: &core.Var{Name: x}},
		},
	}
}

// makeFoldMapFusion builds \$x $acc -> k (f $x) $acc.
func makeFoldMapFusion(k, f core.Core) core.Core {
	x, acc := "$opt_x", "$opt_acc"
	return &core.Lam{
		Param: x,
		Body: &core.Lam{
			Param: acc,
			Body: &core.App{
				Fun: &core.App{
					Fun: k,
					Arg: &core.App{Fun: f, Arg: &core.Var{Name: x}},
				},
				Arg: &core.Var{Name: acc},
			},
		},
	}
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
	return expr
}

// binds checks if a pattern binds the given name.
func binds(p core.Pattern, name string) bool {
	for _, b := range p.Bindings() {
		if b == name {
			return true
		}
	}
	return false
}

// coreIdentical performs a fast pointer-equality-first structural check.
func coreIdentical(a, b core.Core) bool {
	if a == b {
		return true
	}
	// Different pointers after Transform means something changed.
	return false
}
