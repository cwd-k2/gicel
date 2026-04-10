package check

// Block expression and pure-bind elaboration — let-generalization,
// pure-bind desugaring, and block-expression inference.
// Does NOT cover: do-block elaboration (elaborate_do.go, elaborate_do_monadic.go).

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// localLetGen infers a binding expression and attempts let-generalization.
// Watermark-based: only metas born during this inference are quantified.
// If there are no unresolved constraints, or all unresolved constraints'
// metas appear in the result type, the binding is generalized. Otherwise
// constraints are left on the worklist for the enclosing scope to resolve.
func (ch *Checker) localLetGen(expr syntax.Expr) (types.Type, ir.Core) {
	watermark := ch.freshID
	savedWorklist := ch.solver.SaveWorklist()
	bindTy, bindCore := ch.infer(expr)

	// MonoLocalBinds: if inference produced class constraints whose class
	// has associated type families, skip SolveWanteds and generalization
	// entirely. Type family equations (e.g. Elem ?l = (Int, String)) live
	// in the inert set and would be destroyed by SolveWanteds' reset.
	// By leaving them in the current scope, the body's type checking can
	// solve the blocking meta (via Reactivate), and the outer
	// SolveWanteds processes the kicked-out equation correctly.
	newConstraints := ch.solver.SaveWorklist()
	if constraintsHaveAssocType(newConstraints, ch.reg) {
		ch.solver.RestoreWorklistAppend(savedWorklist)
		ch.solver.RestoreWorklistAppend(newConstraints)
		bindTy = ch.unifier.Zonk(bindTy)
		return bindTy, bindCore
	}
	ch.solver.RestoreWorklistAppend(newConstraints)

	// Normal path: resolve constraints and possibly generalize.
	bindCore, unresolved := ch.resolveDeferredConstraintsDeferrable(bindCore)
	bindTy = ch.unifier.Zonk(bindTy)
	if !ch.hasAmbiguousLocal(bindTy, unresolved, watermark) {
		bindTy, bindCore = ch.generalizeLocal(bindTy, bindCore, unresolved, watermark)
		// Generalization lifted constraints into qualified type; don't re-emit.
		unresolved = nil
	}
	// Unresolved constraints go back to the outer worklist.
	// They will be resolved once the enclosing context provides more type information.
	for _, uc := range unresolved {
		savedWorklist = append(savedWorklist, uc)
	}
	ch.solver.RestoreWorklistAppend(savedWorklist)
	return bindTy, bindCore
}

// constraintsHaveAssocType reports whether any class constraint in the
// list belongs to a class with associated type families.
func constraintsHaveAssocType(cts []solve.Ct, reg *Registry) bool {
	for _, ct := range cts {
		cc, ok := ct.(solve.CtClass)
		if !ok {
			continue
		}
		className := solve.CtClassHeadName(cc)
		if className == "" {
			continue
		}
		if ci, ok := reg.LookupClass(className); ok && len(ci.AssocTypes) > 0 {
			return true
		}
	}
	return false
}

// hasAmbiguousLocal checks whether any unresolved constraint has metas
// (born after watermark) that don't appear in the result type.
func (ch *Checker) hasAmbiguousLocal(ty types.Type, unresolved []*CtPlainClass, watermark int) bool {
	if len(unresolved) == 0 {
		return false
	}
	typeMetas := collectUnsolvedMetasAfter(watermark, ty)
	typeMetaIDs := make(map[int]bool, len(typeMetas))
	for _, m := range typeMetas {
		typeMetaIDs[m.id] = true
	}
	for _, uc := range unresolved {
		for _, cm := range collectUnsolvedMetasAfter(watermark, uc.Args...) {
			if !typeMetaIDs[cm.id] {
				return true
			}
		}
	}
	return false
}

// elaboratePureBind desugars x := e into App(Lam(x, rest), e).
// The binding is in scope for the duration of the rest callback.
// Caller must ensure st.Pat is a simple PatVar or PatWild.
func (ch *Checker) elaboratePureBind(st *syntax.StmtPureBind, rest func() ir.Core) ir.Core {
	name, _ := syntax.PatVarName(st.Pat)
	bindTy, bindCore := ch.localLetGen(st.Expr)
	ch.ctx.Push(&CtxVar{Name: name, Type: bindTy})
	restCore := rest()
	ch.ctx.Pop()
	return &ir.App{
		Fun: &ir.Lam{Param: name, Body: restCore, S: st.S},
		Arg: bindCore,
		S:   st.S,
	}
}

func (ch *Checker) inferBlock(e *syntax.ExprBlock) (types.Type, ir.Core) {
	type bindInfo struct {
		pat  syntax.Pattern
		ty   types.Type
		core ir.Core
		pr   *patternResult
		s    span.Span
	}
	binds := make([]bindInfo, len(e.Binds))
	for i, bind := range e.Binds {
		if name, ok := syntax.PatVarName(bind.Pat); ok {
			bindTy, bindCore := ch.localLetGen(bind.Expr)
			binds[i] = bindInfo{pat: bind.Pat, ty: bindTy, core: bindCore, s: bind.S}
			ch.ctx.Push(&CtxVar{Name: name, Type: bindTy})
		} else {
			bindTy, bindCore := ch.infer(bind.Expr)
			pr := ch.checkPattern(bind.Pat, bindTy)
			binds[i] = bindInfo{pat: bind.Pat, ty: bindTy, core: bindCore, pr: &pr, s: bind.S}
			for bname, bty := range pr.Bindings {
				ch.ctx.Push(&CtxVar{Name: bname, Type: bty})
			}
		}
	}

	if e.Body == nil {
		ch.addDiag(diagnostic.ErrEmptyDo, e.S, diagMsg("block must end with an expression"))
		for _, b := range binds {
			if b.pr != nil {
				for range b.pr.Bindings {
					ch.ctx.Pop()
				}
			} else {
				ch.ctx.Pop()
			}
		}
		return &types.TyError{S: e.S}, &ir.Lit{Value: nil, S: e.S}
	}
	resultTy, result := ch.infer(e.Body)

	for _, b := range binds {
		if b.pr != nil {
			for range b.pr.Bindings {
				ch.ctx.Pop()
			}
		} else {
			ch.ctx.Pop()
		}
	}

	for i := len(binds) - 1; i >= 0; i-- {
		b := binds[i]
		if b.pr != nil {
			result = &ir.Case{
				Scrutinee: b.core,
				Alts:      []ir.Alt{{Pattern: b.pr.Pattern, Body: result, S: b.s}},
				S:         b.s,
			}
		} else {
			name, _ := syntax.PatVarName(b.pat)
			lam := &ir.Lam{Param: name, Body: result, S: b.s}
			result = &ir.App{Fun: lam, Arg: b.core, S: b.s}
		}
	}

	return resultTy, result
}
