// Core IR substitution — term-level and type-level.
//
// substMany: capture-avoiding simultaneous term substitution.
// substTyVarInCore: type variable substitution in type-carrying IR positions.
//
// Neither function uses ir.Transform because both require pre-order
// control that the bottom-up Transform does not provide:
//   - substMany needs binder-aware capture avoidance (shadow + rename).
//   - substTyVarInCore must skip subtrees under shadowing TyLam nodes;
//     types.Subst does not know about Core-level TyLam binders, so a
//     post-order Transform would incorrectly substitute inside shadowed bodies.
//
// IR tree depth is bounded by --max-nesting (default 512), so the recursive
// traversals here do not need an explicit depth guard beyond Go's stack limit.
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
//
// Namespace: "$r" suffix. Other fresh-name generators use different
// namespaces to avoid collisions:
//   - types/subst.go: "$" suffix (type-level substitution)
//   - check/checker.go: "_" suffix (per-checker, non-global)
var freshCounter atomic.Uint64

func freshName(base string) string {
	n := freshCounter.Add(1)
	return base + "$r" + strconv.FormatUint(n, 10)
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

// fvNames extracts bare variable names from a VarKey-keyed free variable set.
// Used at the boundary between ir.FreeVars (which returns map[VarKey]struct{})
// and the capture-avoiding substitution engine (which uses bare names for
// binder collision checks).
func fvNames(fv map[ir.VarKey]struct{}) map[string]struct{} {
	if len(fv) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(fv))
	for k := range fv {
		m[k.Name] = struct{}{}
	}
	return m
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
		// Only substitute local (unqualified) variables. Qualified
		// module references are distinct from local binders and must
		// not be captured by bare-name substitution.
		if ir.VarKeyOf(n).IsUnqualified() {
			if repl, ok := subs[n.Name]; ok {
				return ir.Clone(repl)
			}
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
	case *ir.VariantLit:
		newValue := substMany(n.Value, subs, subsFV)
		if newValue == n.Value {
			return n
		}
		return &ir.VariantLit{Tag: n.Tag, Value: newValue, S: n.S}
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
//
// Cannot use ir.Transform: Transform is bottom-up (post-order) and
// would recurse into shadowed TyLam bodies before the visitor can
// skip them. types.Subst does not know about Core-level TyLam
// binders, so it would incorrectly substitute locally-bound type
// variables. The pre-order shadow check (TyLam.TyParam == tyVar →
// return unchanged) is essential for correctness.
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
			newExpr := walk(n.Expr)
			newTyArg := st(n.TyArg)
			if newExpr == n.Expr && newTyArg == n.TyArg {
				return c
			}
			return &ir.TyApp{Expr: newExpr, TyArg: newTyArg, S: n.S}
		case *ir.TyLam:
			if n.TyParam == tyVar {
				return c // shadowed — stop substituting
			}
			newKind := st(n.Kind)
			newBody := walk(n.Body)
			if newKind == n.Kind && newBody == n.Body {
				return c
			}
			return &ir.TyLam{TyParam: n.TyParam, Kind: newKind, Body: newBody, S: n.S}
		case *ir.Lam:
			newPT := st(n.ParamType)
			newBody := walk(n.Body)
			if newPT == n.ParamType && newBody == n.Body {
				return c
			}
			return &ir.Lam{Param: n.Param, ParamType: newPT, Body: newBody, Generated: n.Generated, S: n.S}
		case *ir.App:
			newFun := walk(n.Fun)
			newArg := walk(n.Arg)
			if newFun == n.Fun && newArg == n.Arg {
				return c
			}
			return &ir.App{Fun: newFun, Arg: newArg, S: n.S}
		case *ir.Con:
			if len(n.Args) == 0 {
				return c
			}
			args := make([]ir.Core, len(n.Args))
			changed := false
			for i, a := range n.Args {
				args[i] = walk(a)
				if args[i] != a {
					changed = true
				}
			}
			if !changed {
				return c
			}
			return &ir.Con{Name: n.Name, Module: n.Module, Args: args, S: n.S}
		case *ir.Case:
			newScrutinee := walk(n.Scrutinee)
			changed := newScrutinee != n.Scrutinee
			alts := make([]ir.Alt, len(n.Alts))
			for i, a := range n.Alts {
				newBody := walk(a.Body)
				if newBody != a.Body {
					changed = true
				}
				alts[i] = ir.Alt{Pattern: a.Pattern, Body: newBody, Generated: a.Generated, S: a.S}
			}
			if !changed {
				return c
			}
			return &ir.Case{Scrutinee: newScrutinee, Alts: alts, S: n.S}
		case *ir.PrimOp:
			if len(n.Args) == 0 {
				return c
			}
			args := make([]ir.Core, len(n.Args))
			changed := false
			for i, a := range n.Args {
				args[i] = walk(a)
				if args[i] != a {
					changed = true
				}
			}
			if !changed {
				return c
			}
			return &ir.PrimOp{Name: n.Name, Arity: n.Arity, Effectful: n.Effectful, Args: args, S: n.S}
		case *ir.Fix:
			newBody := walk(n.Body)
			if newBody == n.Body {
				return c
			}
			return &ir.Fix{Name: n.Name, Body: newBody, S: n.S}
		case *ir.Bind:
			newComp := walk(n.Comp)
			newBody := walk(n.Body)
			if newComp == n.Comp && newBody == n.Body {
				return c
			}
			return &ir.Bind{Comp: newComp, Var: n.Var, Discard: n.Discard, Body: newBody, Generated: n.Generated, S: n.S}
		case *ir.Pure:
			newExpr := walk(n.Expr)
			if newExpr == n.Expr {
				return c
			}
			return &ir.Pure{Expr: newExpr, S: n.S}
		case *ir.Thunk:
			newComp := walk(n.Comp)
			if newComp == n.Comp {
				return c
			}
			return &ir.Thunk{Comp: newComp, S: n.S}
		case *ir.Force:
			newExpr := walk(n.Expr)
			if newExpr == n.Expr {
				return c
			}
			return &ir.Force{Expr: newExpr, S: n.S}
		case *ir.Merge:
			newLeft := walk(n.Left)
			newRight := walk(n.Right)
			if newLeft == n.Left && newRight == n.Right {
				return c
			}
			return &ir.Merge{Left: newLeft, Right: newRight, LeftLabels: n.LeftLabels, RightLabels: n.RightLabels, S: n.S}
		case *ir.Var, *ir.Lit, *ir.Error:
			return c
		case *ir.RecordLit:
			fields := make([]ir.Field, len(n.Fields))
			changed := false
			for i, f := range n.Fields {
				newVal := walk(f.Value)
				if newVal != f.Value {
					changed = true
				}
				fields[i] = ir.Field{Label: f.Label, Value: newVal}
			}
			if !changed {
				return c
			}
			return &ir.RecordLit{Fields: fields, S: n.S}
		case *ir.RecordProj:
			newRecord := walk(n.Record)
			if newRecord == n.Record {
				return c
			}
			return &ir.RecordProj{Record: newRecord, Label: n.Label, S: n.S}
		case *ir.RecordUpdate:
			newRecord := walk(n.Record)
			changed := newRecord != n.Record
			updates := make([]ir.Field, len(n.Updates))
			for i, f := range n.Updates {
				newVal := walk(f.Value)
				if newVal != f.Value {
					changed = true
				}
				updates[i] = ir.Field{Label: f.Label, Value: newVal}
			}
			if !changed {
				return c
			}
			return &ir.RecordUpdate{Record: newRecord, Updates: updates, S: n.S}
		case *ir.VariantLit:
			newValue := walk(n.Value)
			if newValue == n.Value {
				return c
			}
			return &ir.VariantLit{Tag: n.Tag, Value: newValue, S: n.S}
		default:
			panic(fmt.Sprintf("substTyVarInCore: unhandled Core node %T", c))
		}
		panic("unreachable") // default always panics
	}
	return walk(c)
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
