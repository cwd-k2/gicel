// Package optimize provides Core IR optimization passes.
//
// INVARIANT: Optimization rules MUST NOT introduce new TyApp nodes.
// The label erasure pass (ir.EraseLabelArgsProgram) runs BEFORE optimization
// and converts Label-kinded TyApp nodes into App+Lit. If an optimization
// rule were to synthesize a new TyApp with a label argument, it would
// bypass erasure and cause a runtime type-application error.
package optimize

import (
	"context"

	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// optimize applies algebraic simplifications and registered rewrite rules.
// Runs up to maxPasses bottom-up passes, stopping early when a pass
// makes no changes (fixed-point detection). Checks the context between
// passes so that a timeout or cancellation aborts the optimization loop
// before runaway IR growth (e.g. recursive dictionary inlining) exhausts
// memory.
func optimize(ctx context.Context, c ir.Core, rules []func(ir.Core) ir.Core, ops *types.TypeOps) ir.Core {
	const maxPasses = 4
	for range maxPasses {
		if ctx.Err() != nil {
			break
		}
		rw := &rewriter{ctx: ctx, rules: rules, ops: ops}
		c = ir.TransformMut(c, rw.apply)
		if !rw.changed {
			break
		}
	}
	return c
}

// OptimizeProgram optimizes all bindings in a program.
// userBindings limits selective inlining of local bindings to the given
// names (nil = no local inlining). externalBindings supplies bindings
// from imported modules (e.g., Prelude transparent wrappers like $,
// fix, force) that are eligible for inlining at qualified call sites.
// The context is checked between optimization passes so that timeout or
// cancellation can abort before runaway IR growth exhausts memory.
func OptimizeProgram(ctx context.Context, prog *ir.Program, rules []func(ir.Core) ir.Core, userBindings map[string]bool, externalBindings []ExternalBinding, ops *types.TypeOps, externalDicts ...map[ir.VarKey]ExternalBinding) {
	candidates := collectInlineCandidates(prog, userBindings, externalBindings)
	allRules := rules
	if len(candidates) > 0 {
		allRules = append([]func(ir.Core) ir.Core{inlineRule(candidates)}, rules...)
	}
	// caseOfKnownDict: demand-driven dictionary inlining at Case sites.
	var dicts map[ir.VarKey]ExternalBinding
	if len(externalDicts) > 0 {
		dicts = externalDicts[0]
	}
	if len(dicts) > 0 || hasLocalDicts(prog) {
		allRules = append([]func(ir.Core) ir.Core{caseOfKnownDict(dicts, prog)}, allRules...)
	}
	for i, b := range prog.Bindings {
		prog.Bindings[i].Expr = optimize(ctx, b.Expr, allRules, ops)
	}
}

func hasLocalDicts(prog *ir.Program) bool {
	for _, b := range prog.Bindings {
		if b.Generated.IsGenerated() {
			return true
		}
	}
	return false
}

// caseOfKnownDict returns a rewrite rule that resolves Case expressions
// whose scrutinee is a known dictionary variable. The dictionary is
// inlined ONLY at the case site and immediately dissolved by
// caseOfKnownCtor, preventing the cascade bloat that would occur from
// general-purpose dictionary inlining.
func caseOfKnownDict(external map[ir.VarKey]ExternalBinding, prog *ir.Program) func(ir.Core) ir.Core {
	// Collect local dictionaries from the main program.
	// Skip recursive dictionaries (self-referential bindings) to prevent
	// infinite inlining expansion — e.g. `impl Functor Tree` where the
	// method body calls fmap recursively via the same dictionary.
	localDicts := make(map[ir.VarKey]ir.Core)
	for _, b := range prog.Bindings {
		if !b.Generated.IsGenerated() {
			continue
		}
		core := b.Expr
		for {
			switch n := core.(type) {
			case *ir.TyLam:
				core = n.Body
				continue
			case *ir.Lam:
				core = n.Body
				continue
			}
			break
		}
		switch core.(type) {
		case *ir.Con, *ir.App:
			bfv, bfvOverflow := ir.FreeVars(b.Expr)
			if _, selfRef := bfv[ir.LocalKey(b.Name)]; selfRef || bfvOverflow {
				continue // recursive dictionary — skip to prevent infinite expansion
			}
			localDicts[ir.LocalKey(b.Name)] = b.Expr
		}
	}

	return func(c ir.Core) ir.Core {
		cs, ok := c.(*ir.Case)
		if !ok {
			return c
		}
		v, ok := cs.Scrutinee.(*ir.Var)
		if !ok {
			return c
		}
		key := ir.VarKeyOf(v)
		var dictExpr ir.Core
		if eb, ok := external[key]; ok {
			dictExpr = eb.Expr
		} else if expr, ok := localDicts[key]; ok {
			dictExpr = expr
		}
		if dictExpr == nil {
			return c
		}
		scrutinee := ir.Clone(dictExpr)
		resolved := &ir.Case{Scrutinee: scrutinee, Alts: cs.Alts, S: cs.S}
		if result := caseOfKnownCtor(resolved); result != resolved {
			return result
		}
		// The checker emits constructor applications as zero-arg Con
		// wrapped in App/TyApp chains (Shape A), but caseOfKnownCtor
		// requires a saturated Con with Args (Shape B). Normalize
		// Shape A → Shape B by collecting App arguments into Con.Args.
		if con := normalizeConApp(scrutinee); con != nil {
			return caseOfKnownCtor(&ir.Case{Scrutinee: con, Alts: cs.Alts, S: cs.S})
		}
		return resolved
	}
}

// normalizeConApp converts an App/TyApp chain rooted at a zero-arg Con
// into a saturated Con with Args populated. This bridges the checker's
// constructor representation (Shape A) with what caseOfKnownCtor expects
// (Shape B).
//
//	App(TyApp(Con{C, Args:[]}, ty), a1) → Con{C, Args:[a1]}
func normalizeConApp(expr ir.Core) *ir.Con {
	var valueArgs []ir.Core
	cur := expr
	for {
		switch n := cur.(type) {
		case *ir.App:
			valueArgs = append(valueArgs, n.Arg)
			cur = n.Fun
			continue
		case *ir.TyApp:
			cur = n.Expr
			continue
		}
		break
	}
	con, ok := cur.(*ir.Con)
	if !ok || len(valueArgs) == 0 {
		return nil
	}
	// Reverse: spine was peeled outermost-first, but Con.Args is left-to-right.
	args := make([]ir.Core, len(valueArgs))
	for i, a := range valueArgs {
		args[len(valueArgs)-1-i] = a
	}
	return &ir.Con{Name: con.Name, Module: con.Module, Args: args, S: con.S}
}

// rewriter tracks whether any transformation fired during a pass.
type rewriter struct {
	ctx     context.Context
	rules   []func(ir.Core) ir.Core
	ops     *types.TypeOps
	changed bool
}

// apply runs algebraic simplifications and registered fusion rules on a
// single node, setting changed if any rule fires. Bails out immediately
// when the context is cancelled so that timeout aborts runaway expansion
// (e.g. recursive dictionary inlining) within a single TransformMut pass.
func (rw *rewriter) apply(c ir.Core) ir.Core {
	if rw.ctx.Err() != nil {
		return c
	}
	orig := c
	// Phase 1: algebraic simplifications (always active).
	c = betaReduce(c)
	c = tyAppBeta(c, rw.ops)
	c = appTyLamFloat(c)
	c = caseOfKnownCtor(c)
	c = caseOfKnownLit(c)
	c = bindPureElim(c)
	c = forceThunkElim(c)
	c = recordProjKnown(c)
	c = recordUpdateChain(c)
	c = primOpAbsorb(c)
	// Phase 3: case-of-case and bind-of-case (expose more simplification opportunities).
	c = caseOfCase(c)
	c = bindOfCase(c)
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
	argFV, overflow := ir.FreeVars(app.Arg)
	if overflow {
		return c
	}
	return substFV(lam.Body, lam.Param, app.Arg, fvNames(argFV))
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
		if pcon.Con != con.Name || pcon.Module != con.Module {
			continue
		}
		// Simultaneous substitution: collect all pattern→arg mappings,
		// recursively decomposing nested patterns (PRecord, PCon).
		subs := make(map[string]ir.Core)
		subsFV := make(map[string]struct{})
		for i, pat := range pcon.Args {
			if i >= len(con.Args) {
				break
			}
			collectPatSubs(pat, con.Args[i], subs, subsFV)
		}
		if len(subs) == 0 {
			return alt.Body
		}
		return substMany(alt.Body, subs, subsFV)
	}
	return c
}

// collectPatSubs recursively walks a pattern and a corresponding value,
// collecting name→value substitutions for all PVar leaves.
func collectPatSubs(pat ir.Pattern, val ir.Core, subs map[string]ir.Core, subsFV map[string]struct{}) {
	switch p := pat.(type) {
	case *ir.PVar:
		subs[p.Name] = val
		valFV, _ := ir.FreeVars(val)
		for k := range valFV {
			subsFV[k.Name] = struct{}{}
		}
	case *ir.PRecord:
		// For each field in the record pattern, project the corresponding
		// field from the value and recurse.
		for _, f := range p.Fields {
			var fieldVal ir.Core
			// If the value is a known RecordLit, extract the field directly.
			if rec, ok := val.(*ir.RecordLit); ok {
				for _, rf := range rec.Fields {
					if rf.Label == f.Label {
						fieldVal = rf.Value
						break
					}
				}
			}
			if fieldVal == nil {
				fieldVal = &ir.RecordProj{Record: val, Label: f.Label, S: val.Span()}
			}
			collectPatSubs(f.Pattern, fieldVal, subs, subsFV)
		}
	case *ir.PCon:
		// Nested constructor pattern: only decompose if the value is
		// also a known constructor with the same tag.
		if con, ok := val.(*ir.Con); ok && con.Name == p.Con && con.Module == p.Module {
			for i, sub := range p.Args {
				if i < len(con.Args) {
					collectPatSubs(sub, con.Args[i], subs, subsFV)
				}
			}
		}
	case *ir.PWild:
		// nothing to bind
	}
}

// R1b: Case (Lit v) alts  →  matching alt (literal scrutinee)
func caseOfKnownLit(c ir.Core) ir.Core {
	cs, ok := c.(*ir.Case)
	if !ok {
		return c
	}
	lit, ok := cs.Scrutinee.(*ir.Lit)
	if !ok {
		return c
	}
	var wildcard *ir.Alt
	for i := range cs.Alts {
		alt := &cs.Alts[i]
		switch p := alt.Pattern.(type) {
		case *ir.PLit:
			if p.Value == lit.Value {
				return alt.Body
			}
		case *ir.PWild:
			wildcard = alt
		case *ir.PVar:
			// A PVar binds the literal: substitute it.
			litFV, litOverflow := ir.FreeVars(lit)
			if litOverflow {
				return c
			}
			return substFV(alt.Body, p.Name, lit, fvNames(litFV))
		}
	}
	if wildcard != nil {
		return wildcard.Body
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
	exprFV, overflow := ir.FreeVars(pure.Expr)
	if overflow {
		return c
	}
	return substFV(bind.Body, bind.Var, pure.Expr, fvNames(exprFV))
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
	var merged []ir.Field
	for _, u := range inner.Updates {
		if !outerLabels[u.Label] {
			merged = append(merged, u)
		}
	}
	merged = append(merged, outer.Updates...)
	return &ir.RecordUpdate{Record: inner.Record, Updates: merged, S: outer.Span()}
}

// primOpAbsorb collects App-applied arguments into a PrimOp's Args:
//
//	App (PrimOp name arity [a₁..aₙ]) arg  →  PrimOp name arity [a₁..aₙ, arg]
//
// when n < arity. This normalizes the IR so that fusion rules, which
// pattern-match on fully-saturated PrimOp nodes, can fire.
func primOpAbsorb(c ir.Core) ir.Core {
	app, ok := c.(*ir.App)
	if !ok {
		return c
	}
	// Peel TyApp wrappers around the PrimOp. After dictionary method
	// extraction, the checker's type annotations produce TyApp(PrimOp, ty)
	// chains that are erased at runtime but block PrimOp saturation.
	fun := app.Fun
	for {
		if ta, ok := fun.(*ir.TyApp); ok {
			fun = ta.Expr
			continue
		}
		break
	}
	po, ok := fun.(*ir.PrimOp)
	if !ok {
		return c
	}
	if len(po.Args) >= po.Arity {
		return c // already saturated or over-saturated
	}
	args := make([]ir.Core, len(po.Args)+1)
	copy(args, po.Args)
	args[len(po.Args)] = app.Arg
	return &ir.PrimOp{Name: po.Name, Arity: po.Arity, IsEffectful: po.IsEffectful, Args: args, S: app.S}
}
