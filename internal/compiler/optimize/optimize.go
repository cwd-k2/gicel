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
	"strconv"
	"sync/atomic"

	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// freshCounter provides globally unique suffixes for capture-avoiding
// renaming in substMany. Atomic to support concurrent optimization.
var freshCounter atomic.Uint64

func freshName(base string) string {
	n := freshCounter.Add(1)
	return base + "$r" + strconv.FormatUint(n, 10)
}

// optimize applies algebraic simplifications and registered rewrite rules.
// Runs up to maxPasses bottom-up passes, stopping early when a pass
// makes no changes (fixed-point detection).
func optimize(c ir.Core, rules []func(ir.Core) ir.Core) ir.Core {
	const maxPasses = 4
	for range maxPasses {
		rw := &rewriter{rules: rules}
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
func OptimizeProgram(prog *ir.Program, rules []func(ir.Core) ir.Core, userBindings map[string]bool, externalBindings []ExternalBinding, externalDicts ...map[string]ExternalBinding) {
	candidates := collectInlineCandidates(prog, userBindings, externalBindings)
	allRules := rules
	if len(candidates) > 0 {
		allRules = append([]func(ir.Core) ir.Core{inlineRule(candidates)}, rules...)
	}
	// caseOfKnownDict: demand-driven dictionary inlining at Case sites.
	var dicts map[string]ExternalBinding
	if len(externalDicts) > 0 {
		dicts = externalDicts[0]
	}
	if len(dicts) > 0 || hasLocalDicts(prog) {
		allRules = append([]func(ir.Core) ir.Core{caseOfKnownDict(dicts, prog)}, allRules...)
	}
	for i, b := range prog.Bindings {
		prog.Bindings[i].Expr = optimize(b.Expr, allRules)
	}
}

func hasLocalDicts(prog *ir.Program) bool {
	for _, b := range prog.Bindings {
		if b.Generated {
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
func caseOfKnownDict(external map[string]ExternalBinding, prog *ir.Program) func(ir.Core) ir.Core {
	// Collect local dictionaries from the main program.
	localDicts := make(map[string]ir.Core)
	for _, b := range prog.Bindings {
		if !b.Generated {
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
			localDicts[b.Name] = b.Expr
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
		key := ir.VarKey(v)
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

// renamePatternVar returns a pattern with all occurrences of oldName
// replaced by newName. Patterns are structurally simple (PVar, PCon with
// nested args, PRecord, PWild, PLit) so a direct recursive copy suffices.
func renamePatternVar(p ir.Pattern, oldName, newName string) ir.Pattern {
	switch pat := p.(type) {
	case *ir.PVar:
		if pat.Name == oldName {
			return &ir.PVar{Name: newName, Generated: pat.Generated, S: pat.S}
		}
		return pat
	case *ir.PCon:
		args := make([]ir.Pattern, len(pat.Args))
		changed := false
		for i, a := range pat.Args {
			args[i] = renamePatternVar(a, oldName, newName)
			if args[i] != a {
				changed = true
			}
		}
		if !changed {
			return pat
		}
		return &ir.PCon{Con: pat.Con, Module: pat.Module, Args: args, S: pat.S}
	case *ir.PRecord:
		fields := make([]ir.PRecordField, len(pat.Fields))
		changed := false
		for i, f := range pat.Fields {
			rp := renamePatternVar(f.Pattern, oldName, newName)
			fields[i] = ir.PRecordField{Label: f.Label, Pattern: rp}
			if rp != f.Pattern {
				changed = true
			}
		}
		if !changed {
			return pat
		}
		return &ir.PRecord{Fields: fields, S: pat.S}
	default:
		return p
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
	c = tyAppBeta(c)
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
			fv := ir.FreeVars(lit)
			return substFV(alt.Body, p.Name, lit, fv)
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
	var merged []ir.Field
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
		if repl, ok := subs[ir.VarKey(n)]; ok {
			return ir.Clone(repl)
		}
		return n
	case *ir.Lam:
		if _, shadowed := subs[n.Param]; shadowed {
			reduced := withoutKey(subs, n.Param)
			if len(reduced) == 0 {
				return n
			}
			return &ir.Lam{Param: n.Param, ParamType: n.ParamType, Body: substMany(n.Body, reduced, subsFV), Generated: n.Generated, S: n.S}
		}
		param := n.Param
		body := n.Body
		if _, captured := subsFV[param]; captured {
			fresh := freshName(param)
			body = substMany(body, map[string]ir.Core{param: &ir.Var{Name: fresh}}, map[string]struct{}{fresh: {}})
			param = fresh
		}
		return &ir.Lam{Param: param, ParamType: n.ParamType, Body: substMany(body, subs, subsFV), Generated: n.Generated, S: n.S}
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
		return &ir.Con{Name: n.Name, Module: n.Module, Args: args, S: n.S}
	case *ir.Case:
		alts := make([]ir.Alt, len(n.Alts))
		for i, alt := range n.Alts {
			active := subs
			pat := alt.Pattern
			for _, b := range pat.Bindings() {
				if _, ok := active[b]; ok {
					active = withoutKey(active, b)
				}
			}
			// Capture-avoiding: if a pattern binding collides with a
			// free variable in the substitution values, rename it to
			// prevent the pattern from capturing the outer reference.
			var rename map[string]ir.Core
			for _, b := range pat.Bindings() {
				if _, captured := subsFV[b]; captured {
					fresh := freshName(b)
					if rename == nil {
						rename = make(map[string]ir.Core)
					}
					rename[b] = &ir.Var{Name: fresh}
					pat = renamePatternVar(pat, b, fresh)
				}
			}
			body := alt.Body
			if rename != nil {
				renameFV := make(map[string]struct{})
				for _, v := range rename {
					renameFV[v.(*ir.Var).Name] = struct{}{}
				}
				body = substMany(body, rename, renameFV)
			}
			if len(active) == 0 && rename == nil {
				alts[i] = alt
			} else if len(active) == 0 {
				alts[i] = ir.Alt{Pattern: pat, Body: body, Generated: alt.Generated, S: alt.S}
			} else {
				alts[i] = ir.Alt{Pattern: pat, Body: substMany(body, active, subsFV), Generated: alt.Generated, S: alt.S}
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
		name := n.Name
		body := n.Body
		if _, captured := subsFV[name]; captured {
			fresh := freshName(name)
			body = substMany(body, map[string]ir.Core{name: &ir.Var{Name: fresh}}, map[string]struct{}{fresh: {}})
			name = fresh
		}
		return &ir.Fix{Name: name, Body: substMany(body, subs, subsFV), S: n.S}
	case *ir.Pure:
		return &ir.Pure{Expr: substMany(n.Expr, subs, subsFV), S: n.S}
	case *ir.Bind:
		comp := substMany(n.Comp, subs, subsFV)
		active := subs
		if _, shadowed := active[n.Var]; shadowed {
			active = withoutKey(active, n.Var)
		}
		bvar := n.Var
		body := n.Body
		if _, captured := subsFV[bvar]; captured {
			fresh := freshName(bvar)
			body = substMany(body, map[string]ir.Core{bvar: &ir.Var{Name: fresh}}, map[string]struct{}{fresh: {}})
			bvar = fresh
		}
		if len(active) == 0 {
			return &ir.Bind{Comp: comp, Var: bvar, Discard: n.Discard, Body: body, Generated: n.Generated, S: n.S}
		}
		return &ir.Bind{Comp: comp, Var: bvar, Discard: n.Discard, Body: substMany(body, active, subsFV), Generated: n.Generated, S: n.S}
	case *ir.Thunk:
		return &ir.Thunk{Comp: substMany(n.Comp, subs, subsFV), S: n.S}
	case *ir.Force:
		return &ir.Force{Expr: substMany(n.Expr, subs, subsFV), S: n.S}
	case *ir.Merge:
		return &ir.Merge{Left: substMany(n.Left, subs, subsFV), Right: substMany(n.Right, subs, subsFV), LeftLabels: n.LeftLabels, RightLabels: n.RightLabels, S: n.S}
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
		fields := make([]ir.Field, len(n.Fields))
		for i, f := range n.Fields {
			fields[i] = ir.Field{Label: f.Label, Value: substMany(f.Value, subs, subsFV)}
		}
		return &ir.RecordLit{Fields: fields, S: n.S}
	case *ir.RecordProj:
		return &ir.RecordProj{Record: substMany(n.Record, subs, subsFV), Label: n.Label, S: n.S}
	case *ir.RecordUpdate:
		updates := make([]ir.Field, len(n.Updates))
		for i, f := range n.Updates {
			updates[i] = ir.Field{Label: f.Label, Value: substMany(f.Value, subs, subsFV)}
		}
		return &ir.RecordUpdate{Record: substMany(n.Record, subs, subsFV), Updates: updates, S: n.S}
	}
	panic(fmt.Sprintf("substMany: unhandled Core node %T", expr))
}

// tyAppBeta reduces type-level beta redexes:
//
//	TyApp (TyLam @a body) ty  →  body[@a := ty]
//
// This is the type-level analogue of betaReduce (R2). It enables
// evidence specialization by peeling polymorphic wrappers off class
// method selectors, exposing the Lam/Case structure that R1 and R2
// can then reduce.
//
// Does NOT introduce new TyApp nodes (satisfies package INVARIANT).
func tyAppBeta(c ir.Core) ir.Core {
	ta, ok := c.(*ir.TyApp)
	if !ok {
		return c
	}
	tl, ok := ta.Expr.(*ir.TyLam)
	if !ok {
		return c
	}
	return substTyVarInCore(tl.Body, tl.TyParam, ta.TyArg)
}

// appTyLamFloat floats a value argument past a TyLam wrapper:
//
//	App (TyLam @a body) arg  →  TyLam @a (App body arg)
//
// This arises when a polymorphic class method selector receives its
// dictionary argument before all type arguments have been applied.
// The float pushes the value inward so that subsequent TyApp beta
// reductions can peel the TyLam layers.
func appTyLamFloat(c ir.Core) ir.Core {
	app, ok := c.(*ir.App)
	if !ok {
		return c
	}
	tl, ok := app.Fun.(*ir.TyLam)
	if !ok {
		return c
	}
	return &ir.TyLam{
		TyParam: tl.TyParam,
		Kind:    tl.Kind,
		Body:    &ir.App{Fun: tl.Body, Arg: app.Arg, S: app.S},
		S:       tl.S,
	}
}

// substTyVarInCore replaces all occurrences of a type variable in
// type-carrying positions (TyApp.TyArg, Lam.ParamType, TyLam.Kind)
// throughout a Core IR tree. This is the IR-level counterpart of
// types.Subst for type annotations embedded in Core.
func substTyVarInCore(c ir.Core, tyVar string, ty types.Type) ir.Core {
	st := func(t types.Type) types.Type {
		if t == nil {
			return nil
		}
		return types.Subst(t, tyVar, ty)
	}
	var walk func(ir.Core) ir.Core
	walk = func(c ir.Core) ir.Core {
		switch n := c.(type) {
		case *ir.TyApp:
			return &ir.TyApp{Expr: walk(n.Expr), TyArg: st(n.TyArg), S: n.S}
		case *ir.TyLam:
			if n.TyParam == tyVar {
				return c // shadowed — stop substituting
			}
			return &ir.TyLam{TyParam: n.TyParam, Kind: st(n.Kind), Body: walk(n.Body), S: n.S}
		case *ir.Lam:
			return &ir.Lam{Param: n.Param, ParamType: st(n.ParamType), Body: walk(n.Body), Generated: n.Generated, S: n.S}
		case *ir.App:
			return &ir.App{Fun: walk(n.Fun), Arg: walk(n.Arg), S: n.S}
		case *ir.Con:
			if len(n.Args) == 0 {
				return c
			}
			args := make([]ir.Core, len(n.Args))
			for i, a := range n.Args {
				args[i] = walk(a)
			}
			return &ir.Con{Name: n.Name, Module: n.Module, Args: args, S: n.S}
		case *ir.Case:
			alts := make([]ir.Alt, len(n.Alts))
			for i, a := range n.Alts {
				alts[i] = ir.Alt{Pattern: a.Pattern, Body: walk(a.Body), Generated: a.Generated, S: a.S}
			}
			return &ir.Case{Scrutinee: walk(n.Scrutinee), Alts: alts, S: n.S}
		case *ir.PrimOp:
			if len(n.Args) == 0 {
				return c
			}
			args := make([]ir.Core, len(n.Args))
			for i, a := range n.Args {
				args[i] = walk(a)
			}
			return &ir.PrimOp{Name: n.Name, Arity: n.Arity, Effectful: n.Effectful, Args: args, S: n.S}
		case *ir.Fix:
			return &ir.Fix{Name: n.Name, Body: walk(n.Body), S: n.S}
		case *ir.Bind:
			return &ir.Bind{Comp: walk(n.Comp), Var: n.Var, Discard: n.Discard, Body: walk(n.Body), Generated: n.Generated, S: n.S}
		case *ir.Pure:
			return &ir.Pure{Expr: walk(n.Expr), S: n.S}
		case *ir.Thunk:
			return &ir.Thunk{Comp: walk(n.Comp), S: n.S}
		case *ir.Force:
			return &ir.Force{Expr: walk(n.Expr), S: n.S}
		case *ir.Merge:
			return &ir.Merge{Left: walk(n.Left), Right: walk(n.Right), LeftLabels: n.LeftLabels, RightLabels: n.RightLabels, S: n.S}
		case *ir.Var, *ir.Lit, *ir.Error:
			return c
		case *ir.RecordLit:
			fields := make([]ir.Field, len(n.Fields))
			for i, f := range n.Fields {
				fields[i] = ir.Field{Label: f.Label, Value: walk(f.Value)}
			}
			return &ir.RecordLit{Fields: fields, S: n.S}
		case *ir.RecordProj:
			return &ir.RecordProj{Record: walk(n.Record), Label: n.Label, S: n.S}
		case *ir.RecordUpdate:
			updates := make([]ir.Field, len(n.Updates))
			for i, f := range n.Updates {
				updates[i] = ir.Field{Label: f.Label, Value: walk(f.Value)}
			}
			return &ir.RecordUpdate{Record: walk(n.Record), Updates: updates, S: n.S}
		}
		return c
	}
	return walk(c)
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
	return &ir.PrimOp{Name: po.Name, Arity: po.Arity, Effectful: po.Effectful, Args: args, S: po.S}
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
