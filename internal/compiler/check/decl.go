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

// classLikeResult holds instance headers and unevaluated method bodies
// produced by registerClassLikeForms (Phase 2), consumed by checkInstances (Phase 6).
// A zero value (nil fields) is valid when the superclass graph has cycles.
type classLikeResult struct {
	instances    []*InstanceInfo
	methodBodies map[*InstanceInfo]map[string]syntax.Expr
}

// declPipeline coordinates the multi-phase declaration checking process.
// Inter-phase data flows through explicit typed returns and parameters.
// Phases also mutate CheckState (ctx, reg, unifier) as side effects;
// these cross-cutting concerns are not threaded explicitly.
//
// Phase ordering and dependencies:
//
//	Phase                       Depends on           Produces
//	─────────────────────────────────────────────────────────────────
//	RegisterTypes               (none)               typeKinds, aliases, families, data types
//	RegisterClassLike           RegisterTypes        classLikeResult (instances, methodBodies)
//	CollectAnnotations          RegisterClassLike    annotations map
//	CheckAssumptions            CollectAnnotations   context vars (assumption bindings)
//	PreregisterBindings         CollectAnnotations   context vars (annotated binding types)
//	CheckInstances              PreregisterBindings  instance bodies, dict bindings
//	CheckValues                 CheckInstances       remaining value bindings
type declPipeline struct {
	ch            *Checker
	decls         []syntax.Decl
	prog          *ir.Program
	formBodyCache map[*syntax.DeclForm]formBodyParts // shared decomposition cache (Phases 1-2)
	currentPhase  declPhase                          // monotonically increasing phase tracker
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
				p.ch.addDiag(diagnostic.ErrDuplicateDecl, def.S,
					diagFmt{Format: "duplicate binding: %s", Args: []any{def.Name}})
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
	// Initialize phase-transient state; clear on return.
	p.ch.pipeState = &declPipeState{}
	defer func() { p.ch.pipeState = nil }()

	p.checkDuplicateBindings()

	p.enterPhase(phaseRegisterTypes)
	p.registerTypes()

	p.enterPhase(phaseRegisterClassLike)
	clr := p.registerClassLikeForms()
	if p.ch.checkCancelled() {
		return p.prog
	}

	p.enterPhase(phaseCollectAnnotations)
	annotations := p.collectAnnotations()

	p.enterPhase(phaseCheckAssumptions)
	p.checkAssumptions(annotations)

	p.enterPhase(phasePreregisterBindings)
	p.preregisterBindings(annotations)
	if p.ch.checkCancelled() {
		return p.prog
	}

	p.enterPhase(phaseCheckInstances)
	p.checkInstances(clr)

	p.enterPhase(phaseCheckValues)
	p.checkValues(annotations)

	// Refine Merge labels now that constraint resolution has solved row metas.
	// Previously caller convention; now structurally guaranteed.
	p.ch.refineMergeLabels()

	emitGradeAxiomViolations(
		collectGradeAxiomViolations(p.ch.reg, p.ch.scope.CurrentModule()),
		p.ch.errors,
	)

	return p.prog
}

// registerTypes handles form decls, type aliases, type families,
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

// registerClassLikeForms handles class-like form declarations,
// impl headers, type family reducer installation, and strict type name activation.
// Returns instance headers and method bodies for checkInstances.
func (p *declPipeline) registerClassLikeForms() classLikeResult {
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
		return classLikeResult{}
	}
	var clr classLikeResult
	clr.methodBodies = make(map[*InstanceInfo]map[string]syntax.Expr)
	for _, d := range p.decls {
		if impl, ok := d.(*syntax.DeclImpl); ok {
			info, methods := p.ch.processImplHeader(impl)
			if info != nil {
				clr.instances = append(clr.instances, info)
				clr.methodBodies[info] = methods
			}
		}
	}
	// Placed after class and impl headers because associated type families
	// are registered in class processing and equations are collected from impls.
	p.ch.installFamilyReducer()
	if p.ch.config.StrictTypeNames {
		p.ch.strictTypeNames = true
	}
	return clr
}

// collectAnnotations resolves type annotations.
// Free type variables are implicitly universally quantified.
// Returns the annotation map consumed by phaseCheckAssumptions,
// phasePreregisterBindings, and phaseCheckValues.
func (p *declPipeline) collectAnnotations() map[string]types.Type {
	annotations := make(map[string]types.Type)
	for _, d := range p.decls {
		if ann, ok := d.(*syntax.DeclTypeAnn); ok {
			ty := p.ch.resolveTypeExpr(ann.Type)
			annotations[ann.Name] = quantifyFreeVars(ty)
		}
	}
	return annotations
}

// checkAssumptions processes assumption declarations.
// These must be checked before instance bodies that may reference them.
func (p *declPipeline) checkAssumptions(annotations map[string]types.Type) {
	for _, d := range p.decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if isAssumptionDef(def) {
				p.ch.processValueDef(def, annotations, p.prog)
			}
		}
	}
}

// preregisterBindings pre-registers annotated non-assumption bindings.
// Only the type is registered; bodies are checked in checkValues.
// This allows instance methods to reference these bindings, matching
// the open-scope semantics of Wadler & Blott type classes.
func (p *declPipeline) preregisterBindings(annotations map[string]types.Type) {
	for _, d := range p.decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if isAssumptionDef(def) {
				continue
			}
			if annTy, hasAnn := annotations[def.Name]; hasAnn {
				p.ch.ctx.Push(&CtxVar{Name: def.Name, Type: annTy, Module: p.ch.scope.CurrentModule()})
			}
		}
	}
}

// checkInstances type-checks instance bodies and generates dict bindings.
func (p *declPipeline) checkInstances(clr classLikeResult) {
	for _, inst := range clr.instances {
		p.ch.processInstanceBody(inst, clr.methodBodies[inst], p.prog)
	}
}

// checkValues processes remaining (non-assumption) value definitions.
func (p *declPipeline) checkValues(annotations map[string]types.Type) {
	for _, d := range p.decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			if isAssumptionDef(def) {
				continue
			}
			p.ch.processValueDef(def, annotations, p.prog)
		}
	}
}

// isAssumptionDef reports whether a value definition is an assumption declaration.
func isAssumptionDef(def *syntax.DeclValueDef) bool {
	return def.IsAssumption
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
	if ch.config.HoverRecorder != nil {
		var kind types.Type = types.TypeOfTypes
		for i := len(paramKinds) - 1; i >= 0; i-- {
			kind = types.MkArrow(paramKinds[i], kind)
		}
		ch.config.HoverRecorder.RecordDecl(d.S, DeclAlias, d.Name, kind)
	}
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

	// General form: unwrap application chain (non-tuple patterns).
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
	ch.pipeState.currentBinding = d.Name
	defer func() { ch.pipeState.currentBinding = "" }()
	annTy, hasAnn := annotations[d.Name]

	// Check if it's an assumption.
	if isAssumptionDef(d) {
		if ch.config.DenyAssumptions {
			ch.addDiag(diagnostic.ErrAssumption, d.S, diagMsg("assumption declarations are not allowed (host-only feature)"))
			return
		}
		// Try AST annotation first, then config assumptions.
		aTy := annTy
		if !hasAnn {
			if ch.config.Assumptions != nil {
				aTy, hasAnn = ch.config.Assumptions[d.Name]
			}
		}
		if !hasAnn {
			ch.addDiag(diagnostic.ErrAssumption, d.S, diagFmt{Format: "assumption %s requires a type annotation", Args: []any{d.Name}})
			return
		}
		// Note: assumptions without a corresponding RegisterPrim are caught at
		// runtime with "missing primitive" error. Compile-time validation is not
		// feasible because stdlib modules use RegisterPrim (not DeclareAssumption).
		ch.ctx.Push(&CtxVar{Name: d.Name, Type: aTy, Module: ch.scope.CurrentModule()})
		prog.Bindings = append(prog.Bindings, ir.Binding{
			Name: d.Name,
			Type: aTy,
			Expr: &ir.PrimOp{Name: d.Name, Arity: typeArity(aTy), IsEffectful: isComputationType(aTy), S: d.S},
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
		// CBPV auto-force at the entry point: a Thunk-typed entry
		// binding (e.g. `main := pipeline` where pipeline is Thunk)
		// is silently unfolded into the underlying Computation so the
		// runtime can drive it as the program. Non-entry bindings stay
		// as Thunks because the existing bare-Computation rejection
		// below enforces the "non-entry bindings are values" discipline.
		if ch.config.EntryPoint != "" && d.Name == ch.config.EntryPoint {
			ty, coreExpr = ch.autoForceIfThunk(ty, coreExpr, d.S)
		}
	}

	// Resolve deferred constraints now that metas are solved.
	// For unannotated bindings, defer constraints on unsolved metas
	// so they can be lifted into qualified types by generalization.
	var unresolvedConstraints []*CtPlainClass
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
		ty, coreExpr = ch.generalizeConstrained(ty, coreExpr, unresolvedConstraints, 0)
	}

	// Non-entry top-level bindings can only be values. A Computation
	// RHS at that position is only meaningful when suspended as a
	// Thunk, so the checker auto-thunks it silently — matching the
	// CBPV coercion that fires at function arguments, do bindings,
	// and the entry-point binding. An explicit `Computation` type
	// annotation still errors, because the annotation expresses
	// deliberate intent that the checker should not silently rewrite.
	if ch.config.EntryPoint != "" && d.Name != ch.config.EntryPoint && isBareComputationType(ty) {
		if hasAnn {
			ch.addDiag(diagnostic.ErrEffectfulBinding, d.S,
				diagFmt{Format: "top-level binding %s is annotated as bare Computation; non-entry bindings must be values — annotate as Thunk, drop the annotation to let the checker auto-thunk, or move the body into a function", Args: []any{d.Name}})
		} else {
			ty, coreExpr = ch.autoThunkComputation(ty, coreExpr, d.S)
		}
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
