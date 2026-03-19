package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// declPipeline coordinates the multi-phase declaration checking process.
// Cross-phase state (annotations, instance headers) lives here rather than
// as loose locals, making the data flow between phases explicit.
type declPipeline struct {
	ch          *Checker
	decls       []syntax.Decl
	prog        *core.Program
	annotations map[string]types.Type
	instances   []*InstanceInfo
}

func (ch *Checker) checkDecls(decls []syntax.Decl) *core.Program {
	p := &declPipeline{ch: ch, decls: decls, prog: &core.Program{}}
	return p.run()
}

func (p *declPipeline) run() *core.Program {
	p.registerTypes()
	p.registerClasses()
	if p.ch.checkCancelled() {
		return p.prog
	}
	p.collectAnnotations()
	p.checkAssumptions()
	p.preregisterBindings()
	if p.ch.checkCancelled() {
		return p.prog
	}
	p.checkInstances()
	p.checkValues()
	return p.prog
}

// registerTypes handles phases 1–3.5: data decls, type aliases, type families,
// cyclic alias detection, and alias expander installation.
func (p *declPipeline) registerTypes() {
	for _, d := range p.decls {
		if data, ok := d.(*syntax.DeclData); ok {
			p.ch.processDataDecl(data, p.prog)
		}
	}
	for _, d := range p.decls {
		if alias, ok := d.(*syntax.DeclTypeAlias); ok {
			p.ch.processTypeAlias(alias)
		}
	}
	for _, d := range p.decls {
		if tf, ok := d.(*syntax.DeclTypeFamily); ok {
			p.ch.processTypeFamily(tf)
		}
	}
	hasCyclicAlias := p.ch.validateAliasGraph()
	if !hasCyclicAlias {
		p.ch.installAliasExpander()
	}
}

// registerClasses handles phases 4–5.6: class declarations, instance headers,
// type family reducer installation, and strict type name activation.
func (p *declPipeline) registerClasses() {
	for _, d := range p.decls {
		if cls, ok := d.(*syntax.DeclClass); ok {
			p.ch.processClassDecl(cls, p.prog)
		}
	}
	for _, d := range p.decls {
		if inst, ok := d.(*syntax.DeclInstance); ok {
			info := p.ch.processInstanceHeader(inst)
			if info != nil {
				p.instances = append(p.instances, info)
			}
		}
	}
	// Placed after class and instance headers because associated type families
	// are registered in class processing and equations are collected from instances.
	p.ch.installFamilyReducer()
	if p.ch.config.StrictTypeNames {
		p.ch.strictTypeNames = true
	}
}

// collectAnnotations resolves type annotations (phase 6).
// Free type variables are implicitly universally quantified.
func (p *declPipeline) collectAnnotations() {
	p.annotations = make(map[string]types.Type)
	for _, d := range p.decls {
		if ann, ok := d.(*syntax.DeclTypeAnn); ok {
			ty := p.ch.resolveTypeExpr(ann.Type)
			p.annotations[ann.Name] = quantifyFreeVars(ty)
		}
	}
}

// checkAssumptions processes assumption declarations (phase 7).
// These must be checked before instance bodies that may reference them.
func (p *declPipeline) checkAssumptions() {
	for _, d := range p.decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if v, ok := def.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
				p.ch.processValueDef(def, p.annotations, p.prog)
			}
		}
	}
}

// preregisterBindings pre-registers annotated non-assumption bindings (phase 7.5).
// Only the type is registered; bodies are checked in checkValues.
// This allows instance methods to reference these bindings, matching
// the open-scope semantics of Wadler & Blott type classes.
func (p *declPipeline) preregisterBindings() {
	for _, d := range p.decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if v, ok := def.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
				continue
			}
			if annTy, hasAnn := p.annotations[def.Name]; hasAnn {
				p.ch.ctx.Push(&CtxVar{Name: def.Name, Type: annTy, Module: p.ch.scope.currentModule})
			}
		}
	}
}

// checkInstances type-checks instance bodies and generates dict bindings (phase 8).
func (p *declPipeline) checkInstances() {
	for _, inst := range p.instances {
		p.ch.processInstanceBody(inst, p.prog)
	}
}

// checkValues processes remaining (non-assumption) value definitions (phase 9).
func (p *declPipeline) checkValues() {
	for _, d := range p.decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if v, ok := def.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
				continue
			}
			p.ch.processValueDef(def, p.annotations, p.prog)
		}
	}
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
	if ch.checkCancelled() {
		return
	}
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

	// Resolve deferred constraints now that metas are solved.
	// For unannotated bindings, defer constraints on unsolved metas
	// so they can be lifted into qualified types by generalization.
	var unresolvedConstraints []*CtClass
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

	// Reject bare Computation types in non-entry top-level bindings.
	// In CBPV, top-level bindings should be values; computations must be
	// wrapped with 'thunk' to suspend them.
	if ch.config.EntryPoint != "" && d.Name != ch.config.EntryPoint && isBareComputationType(ty) {
		ch.addCodedError(errs.ErrEffectfulBinding, d.S,
			fmt.Sprintf("top-level binding %s has bare Computation type; "+
				"wrap with 'thunk' to suspend, or make it a function parameter",
				d.Name))
	}

	ch.ctx.Push(&CtxVar{Name: d.Name, Type: ty, Module: ch.scope.currentModule})
	prog.Bindings = append(prog.Bindings, core.Binding{
		Name: d.Name,
		Type: ty,
		Expr: coreExpr,
		S:    d.S,
	})
}
