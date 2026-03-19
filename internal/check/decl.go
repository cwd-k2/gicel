package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

func (ch *Checker) checkDecls(decls []syntax.Decl) *core.Program {
	prog := &core.Program{}

	// 1. Process data declarations (register constructors first).
	for _, d := range decls {
		if data, ok := d.(*syntax.DeclData); ok {
			ch.processDataDecl(data, prog)
		}
	}

	// 2. Process type aliases.
	for _, d := range decls {
		if alias, ok := d.(*syntax.DeclTypeAlias); ok {
			ch.processTypeAlias(alias)
		}
	}

	// 2.5. Process type family declarations.
	for _, d := range decls {
		if tf, ok := d.(*syntax.DeclTypeFamily); ok {
			ch.processTypeFamily(tf)
		}
	}

	// 3. Detect cyclic aliases.
	hasCyclicAlias := ch.validateAliasGraph()

	// 3.5. Install alias expander in unifier for transparent alias handling.
	// Skip installation if cyclic aliases were found to prevent infinite expansion.
	if !hasCyclicAlias {
		ch.installAliasExpander()
	}

	// 4. Process class declarations (generates dict types + selectors).
	for _, d := range decls {
		if cls, ok := d.(*syntax.DeclClass); ok {
			ch.processClassDecl(cls, prog)
		}
	}

	// 5. Process instance headers (validates, registers).
	var instanceDecls []*InstanceInfo
	for _, d := range decls {
		if inst, ok := d.(*syntax.DeclInstance); ok {
			info := ch.processInstanceHeader(inst)
			if info != nil {
				instanceDecls = append(instanceDecls, info)
			}
		}
	}

	// 5.5. Install type family reducer in unifier.
	// Placed after class (phase 4) and instance headers (phase 5) because
	// associated type families are registered in class processing and their
	// equations are collected from instances.
	ch.installFamilyReducer()

	// 5.6. Enable strict type name validation now that all declarations are registered.
	if ch.config.StrictTypeNames {
		ch.strictTypeNames = true
	}

	// 6. Collect type annotations.
	// Free type variables are implicitly universally quantified (implicit forall).
	annotations := make(map[string]types.Type)
	for _, d := range decls {
		if ann, ok := d.(*syntax.DeclTypeAnn); ok {
			ty := ch.resolveTypeExpr(ann.Type)
			annotations[ann.Name] = quantifyFreeVars(ty)
		}
	}

	// 7. Process assumption declarations first (needed by instance bodies).
	for _, d := range decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if v, ok := def.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
				ch.processValueDef(def, annotations, prog)
			}
		}
	}

	// 7.5. Pre-register annotated non-assumption bindings into the context.
	// Only the type is registered; bodies are checked in phase 9.
	// This allows instance methods (phase 8) to reference these bindings,
	// matching the open-scope semantics of Wadler & Blott type classes.
	for _, d := range decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if v, ok := def.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
				continue
			}
			if annTy, hasAnn := annotations[def.Name]; hasAnn {
				ch.ctx.Push(&CtxVar{Name: def.Name, Type: annTy, Module: ch.scope.currentModule})
			}
		}
	}

	// 8. Process instance bodies (type-checks methods, generates dict bindings).
	for _, inst := range instanceDecls {
		ch.processInstanceBody(inst, prog)
	}

	// 9. Process remaining value definitions (non-assumption).
	for _, d := range decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if v, ok := def.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
				continue // already processed
			}
			ch.processValueDef(def, annotations, prog)
		}
	}

	return prog
}

func (ch *Checker) processTypeAlias(d *syntax.DeclTypeAlias) {
	var params []string
	var paramKinds []types.Kind
	for _, p := range d.Params {
		params = append(params, p.Name)
		paramKinds = append(paramKinds, ch.resolveKindExpr(p.Kind))
	}
	body := ch.resolveTypeExpr(d.Body)
	ch.reg.aliases[d.Name] = &AliasInfo{Params: params, ParamKinds: paramKinds, Body: body}
}

func (ch *Checker) processValueDef(d *syntax.DeclValueDef, annotations map[string]types.Type, prog *core.Program) {
	annTy, hasAnn := annotations[d.Name]

	// Check if it's an assumption.
	if v, ok := d.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
		// Try AST annotation first, then config assumptions.
		aTy := annTy
		if !hasAnn {
			if ch.config.Assumptions != nil {
				aTy, hasAnn = ch.config.Assumptions[d.Name]
			}
		}
		if !hasAnn {
			ch.addCodedError(errs.ErrAssumption, d.S, fmt.Sprintf("assumption %s requires a type annotation", d.Name))
			return
		}
		// Note: assumptions without a corresponding RegisterPrim are caught at
		// runtime with "missing primitive" error. Compile-time validation is not
		// feasible because stdlib modules use RegisterPrim (not DeclareAssumption).
		ch.ctx.Push(&CtxVar{Name: d.Name, Type: aTy, Module: ch.scope.currentModule})
		prog.Bindings = append(prog.Bindings, core.Binding{
			Name: d.Name,
			Type: aTy,
			Expr: &core.PrimOp{Name: d.Name, Arity: typeArity(aTy), Effectful: isComputationType(aTy), S: d.S},
			S:    d.S,
		})
		return
	}

	var coreExpr core.Core
	var ty types.Type
	if hasAnn {
		coreExpr = ch.check(d.Expr, annTy)
		ty = annTy
	} else {
		ty, coreExpr = ch.infer(d.Expr)
	}

	// Process stuck type family re-activations: metas solved during
	// type checking may have unblocked previously stuck family applications.
	ch.ProcessRework()

	// Resolve deferred constraints now that metas are solved.
	// For unannotated bindings, defer constraints on unsolved metas
	// so they can be lifted into qualified types by generalization.
	var unresolvedConstraints []deferredConstraint
	if hasAnn {
		coreExpr = ch.resolveDeferredConstraints(coreExpr)
	} else {
		coreExpr, unresolvedConstraints = ch.resolveDeferredConstraintsDeferrable(coreExpr)
	}

	// Zonk the type.
	ty = ch.unifier.Zonk(ty)

	// Let-generalization: for unannotated bindings, replace unsolved
	// metavariables with universally quantified type variables,
	// and lift unresolved constraints into qualified types.
	if !hasAnn {
		ty, coreExpr = ch.generalizeConstrained(ty, coreExpr, unresolvedConstraints)
	}

	ch.ctx.Push(&CtxVar{Name: d.Name, Type: ty, Module: ch.scope.currentModule})
	prog.Bindings = append(prog.Bindings, core.Binding{
		Name: d.Name,
		Type: ty,
		Expr: coreExpr,
		S:    d.S,
	})
}
