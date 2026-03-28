package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// declPhase identifies a declaration pipeline phase. Phases must execute
// in strictly ascending order; violating this invariant is a compiler bug.
type declPhase int

const (
	phaseRegisterTypes       declPhase = 1 // forms, aliases, families, cycle detection
	phaseRegisterClassLike   declPhase = 2 // class-like forms, impl headers, family reducers
	phaseCollectAnnotations  declPhase = 3 // type annotations (implicit forall)
	phaseCheckAssumptions    declPhase = 4 // host-provided assumptions
	phasePreregisterBindings declPhase = 5 // forward-declare annotated binding types
	phaseCheckInstances      declPhase = 6 // instance body elaboration + dict generation
	phaseCheckValues         declPhase = 7 // remaining value definitions
)

// declPipeline coordinates the multi-phase declaration checking process.
// Cross-phase state (annotations, instance headers) lives here rather than
// as loose locals, making the data flow between phases explicit.
//
// Phase ordering and dependencies:
//
//	Phase                       Depends on           Produces
//	─────────────────────────────────────────────────────────────────
//	RegisterTypes               (none)               typeKinds, aliases, families, data types
//	RegisterClassLike           RegisterTypes        classes, instances (headers), family reducers
//	CollectAnnotations          RegisterClassLike    annotations map
//	CheckAssumptions            CollectAnnotations   context vars (assumption bindings)
//	PreregisterBindings         RegisterClassLike    context vars (annotated binding types)
//	CheckInstances              PreregisterBindings  instance bodies, dict bindings
//	CheckValues                 CheckInstances       remaining value bindings
type declPipeline struct {
	ch            *Checker
	decls         []syntax.Decl
	prog          *ir.Program
	annotations   map[string]types.Type
	instances     []*InstanceInfo
	methodBodies  map[*InstanceInfo]map[string]syntax.Expr // instance → unevaluated method exprs (pipeline-local)
	formBodyCache map[*syntax.DeclForm]formBodyParts       // shared decomposition results
	currentPhase  declPhase                                // monotonically increasing phase tracker
}

// enterPhase asserts that the pipeline is transitioning to a later phase.
// Panics if the phase ordering invariant is violated.
func (p *declPipeline) enterPhase(phase declPhase) {
	if phase <= p.currentPhase {
		panic(fmt.Sprintf("internal: declaration phase %d entered after phase %d", phase, p.currentPhase))
	}
	p.currentPhase = phase
}

func (ch *Checker) checkDecls(decls []syntax.Decl) *ir.Program {
	p := &declPipeline{ch: ch, decls: decls, prog: &ir.Program{}, formBodyCache: make(map[*syntax.DeclForm]formBodyParts)}
	return p.run()
}

// checkDuplicateBindings detects duplicate value binding names and emits errors.
func (p *declPipeline) checkDuplicateBindings() {
	seen := make(map[string]bool)
	for _, d := range p.decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if seen[def.Name] {
				p.ch.addCodedError(diagnostic.ErrDuplicateDecl, def.S,
					fmt.Sprintf("duplicate binding: %s", def.Name))
			} else {
				seen[def.Name] = true
			}
		}
	}
}

// decomposeForm returns the decomposed body parts for a form declaration,
// caching the result to avoid repeated decomposition across pipeline phases.
func (p *declPipeline) decomposeForm(d *syntax.DeclForm) formBodyParts {
	if parts, ok := p.formBodyCache[d]; ok {
		return parts
	}
	parts := decomposeFormBody(d.Body)
	p.formBodyCache[d] = parts
	return parts
}

func (p *declPipeline) run() *ir.Program {
	p.checkDuplicateBindings()
	p.enterPhase(phaseRegisterTypes)
	p.registerTypes()
	p.enterPhase(phaseRegisterClassLike)
	p.registerClassLikeForms()
	if p.ch.checkCancelled() {
		return p.prog
	}
	p.enterPhase(phaseCollectAnnotations)
	p.collectAnnotations()
	p.enterPhase(phaseCheckAssumptions)
	p.checkAssumptions()
	p.enterPhase(phasePreregisterBindings)
	p.preregisterBindings()
	if p.ch.checkCancelled() {
		return p.prog
	}
	p.enterPhase(phaseCheckInstances)
	p.checkInstances()
	p.enterPhase(phaseCheckValues)
	p.checkValues()
	return p.prog
}

// registerTypes handles phases 1–3.5: form decls, type aliases, type families,
// cyclic alias detection, and alias expander installation.
func (p *declPipeline) registerTypes() {
	for _, d := range p.decls {
		if form, ok := d.(*syntax.DeclForm); ok {
			parts := p.decomposeForm(form)
			if !isClassLikeForm(parts) {
				p.ch.processFormDeclParts(form, parts, p.prog)
			}
		}
	}
	for _, d := range p.decls {
		if alias, ok := d.(*syntax.DeclTypeAlias); ok {
			if isTypeFamilyBody(alias.Body) {
				p.ch.processTypeFamilyFromAlias(alias)
			} else {
				p.ch.processTypeAlias(alias)
			}
		}
	}
	hasCyclicAlias := p.ch.validateAliasGraph()
	if !hasCyclicAlias {
		p.ch.installAliasExpander()
	}
}

// registerClassLikeForms handles phases 4–5.6: class-like form declarations,
// impl headers, type family reducer installation, and strict type name activation.
func (p *declPipeline) registerClassLikeForms() {
	// Process class-like form declarations (forms with all-lowercase fields).
	for _, d := range p.decls {
		if form, ok := d.(*syntax.DeclForm); ok {
			parts := p.decomposeForm(form)
			if isClassLikeForm(parts) {
				p.ch.processClassLikeForm(form, parts, p.prog)
			}
		}
	}
	// Detect cyclic superclass constraints before processing instances.
	// Cycles (A requires B, B requires A) would cause infinite loops or
	// uninitialized forward references during dictionary construction.
	if p.ch.validateSuperclassGraph() {
		return
	}
	p.methodBodies = make(map[*InstanceInfo]map[string]syntax.Expr)
	for _, d := range p.decls {
		if impl, ok := d.(*syntax.DeclImpl); ok {
			info, methods := p.ch.processImplHeader(impl)
			if info != nil {
				p.instances = append(p.instances, info)
				p.methodBodies[info] = methods
			}
		}
	}
	// Placed after class and impl headers because associated type families
	// are registered in class processing and equations are collected from impls.
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
			if isAssumptionDef(def) {
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
			if isAssumptionDef(def) {
				continue
			}
			if annTy, hasAnn := p.annotations[def.Name]; hasAnn {
				p.ch.ctx.Push(&CtxVar{Name: def.Name, Type: annTy, Module: p.ch.scope.CurrentModule()})
			}
		}
	}
}

// checkInstances type-checks instance bodies and generates dict bindings (phase 8).
func (p *declPipeline) checkInstances() {
	for _, inst := range p.instances {
		p.ch.processInstanceBody(inst, p.methodBodies[inst], p.prog)
	}
}

// checkValues processes remaining (non-assumption) value definitions (phase 9).
func (p *declPipeline) checkValues() {
	for _, d := range p.decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if isAssumptionDef(def) {
				continue
			}
			p.ch.processValueDef(def, p.annotations, p.prog)
		}
	}
}

// isAssumptionDef reports whether a value definition is an assumption declaration
// (i.e., its body is the bare identifier "assumption").
func isAssumptionDef(def *syntax.DeclValueDef) bool {
	v, ok := def.Expr.(*syntax.ExprVar)
	return ok && v.Name == "assumption"
}

func (ch *Checker) processTypeAlias(d *syntax.DeclTypeAlias) {
	parts := decomposeTypeAliasBody(d.Body)
	var params []string
	var paramKinds []types.Type
	for _, p := range parts.Params {
		params = append(params, p.Name)
		paramKinds = append(paramKinds, ch.resolveKindExpr(p.Kind))
	}
	body := ch.resolveTypeExpr(parts.Body)
	ch.reg.RegisterAlias(d.Name, &AliasInfo{Params: params, ParamKinds: paramKinds, Body: body})
}

// processTypeFamilyFromAlias handles a DeclTypeAlias whose body contains a
// type-level case, indicating a closed type family. Extracts the case
// alternatives and delegates directly to processTypeFamilyDecl.
func (ch *Checker) processTypeFamilyFromAlias(d *syntax.DeclTypeAlias) {
	parts := decomposeTypeAliasBody(d.Body)

	caseExpr, ok := parts.Body.(*syntax.TyExprCase)
	if !ok {
		return
	}

	ch.processTypeFamilyDecl(d.Name, parts.Params, d.KindAnn, caseExpr.Alts, d.S)
}

// countTupleArity returns the number of elements if the type expression is a tuple,
// or 1 if it's a regular type.
func countTupleArity(t syntax.TypeExpr) int {
	// Tuple pattern: (A, B) parses as TyExprApp(TyExprCon("Record"), TyExprRow{Fields: [_1: A, _2: B]})
	if app, ok := t.(*syntax.TyExprApp); ok {
		if con, ok := app.Fun.(*syntax.TyExprCon); ok && con.Name == types.TyConRecord {
			if row, ok := app.Arg.(*syntax.TyExprRow); ok && len(row.Fields) > 0 {
				return len(row.Fields)
			}
		}
	}
	return 1
}

// extractTFPatterns extracts type family equation patterns from a case alternative pattern.
// For single-param families, returns [pattern].
// For multi-param: unwraps tuple patterns (Record {_1: P1, _2: P2}) or application chains.
func extractTFPatterns(pat syntax.TypeExpr, numParams int) []syntax.TypeExpr {
	if numParams <= 1 {
		return []syntax.TypeExpr{pat}
	}

	// Check for tuple pattern: (P1, P2, ...) = TyExprApp(Record, TyExprRow{Fields: ...})
	if app, ok := pat.(*syntax.TyExprApp); ok {
		if con, ok := app.Fun.(*syntax.TyExprCon); ok && con.Name == types.TyConRecord {
			if row, ok := app.Arg.(*syntax.TyExprRow); ok && len(row.Fields) >= numParams {
				result := make([]syntax.TypeExpr, len(row.Fields))
				for i, f := range row.Fields {
					result[i] = f.Type
				}
				return result
			}
		}
	}

	// Legacy format: unwrap application chain.
	var result []syntax.TypeExpr
	t := pat
	for {
		if app, ok := t.(*syntax.TyExprApp); ok {
			result = append([]syntax.TypeExpr{app.Arg}, result...)
			t = app.Fun
		} else {
			result = append([]syntax.TypeExpr{t}, result...)
			break
		}
	}
	return result
}

func (ch *Checker) processValueDef(d *syntax.DeclValueDef, annotations map[string]types.Type, prog *ir.Program) {
	if ch.checkCancelled() {
		return
	}
	annTy, hasAnn := annotations[d.Name]

	// Check if it's an assumption.
	if isAssumptionDef(d) {
		// Try AST annotation first, then config assumptions.
		aTy := annTy
		if !hasAnn {
			if ch.config.Assumptions != nil {
				aTy, hasAnn = ch.config.Assumptions[d.Name]
			}
		}
		if !hasAnn {
			ch.addCodedError(diagnostic.ErrAssumption, d.S, fmt.Sprintf("assumption %s requires a type annotation", d.Name))
			return
		}
		// Note: assumptions without a corresponding RegisterPrim are caught at
		// runtime with "missing primitive" error. Compile-time validation is not
		// feasible because stdlib modules use RegisterPrim (not DeclareAssumption).
		ch.ctx.Push(&CtxVar{Name: d.Name, Type: aTy, Module: ch.scope.CurrentModule()})
		prog.Bindings = append(prog.Bindings, ir.Binding{
			Name: d.Name,
			Type: aTy,
			Expr: &ir.PrimOp{Name: d.Name, Arity: typeArity(aTy), Effectful: isComputationType(aTy), S: d.S},
			S:    d.S,
		})
		return
	}

	var coreExpr ir.Core
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
		ch.addCodedError(diagnostic.ErrEffectfulBinding, d.S,
			fmt.Sprintf("top-level binding %s has bare Computation type; "+
				"wrap with 'thunk' to suspend, or make it a function parameter",
				d.Name))
	}

	// Annotated bindings were pre-registered in preregisterBindings (phase 7.5).
	// Only push for unannotated or assumption bindings to avoid double-push.
	if !hasAnn {
		ch.ctx.Push(&CtxVar{Name: d.Name, Type: ty, Module: ch.scope.CurrentModule()})
	}
	prog.Bindings = append(prog.Bindings, ir.Binding{
		Name: d.Name,
		Type: ty,
		Expr: coreExpr,
		S:    d.S,
	})
}
