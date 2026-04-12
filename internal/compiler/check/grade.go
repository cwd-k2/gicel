package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// gradeAlgebraKind returns the kind to use for grade algebra parameters.
// If "Mult" is registered as a promoted kind (via DataKinds), returns
// PromotedDataKind("Mult"); otherwise falls back to TypeOfTypes.
func gradeAlgebraKind(ch *Checker) types.Type {
	if k, ok := ch.reg.LookupPromotedKind("Mult"); ok {
		return k
	}
	return types.TypeOfTypes
}

// gradeAlgebraClassName is the name of the user-facing grade algebra class.
const gradeAlgebraClassName = "GradeAlgebra"

// resolvedGradeAlgebra holds the resolved join family name and drop value
// for a grade kind.
type resolvedGradeAlgebra struct {
	joinFamily    string     // name of the GradeJoin type family
	composeFamily string     // name of the GradeCompose type family
	dropValue     types.Type // the Drop element (promoted constructor, e.g. Zero)
	unitValue     types.Type // the Unit element (identity of Compose, e.g. Linear)
	valid         bool       // false if no GradeAlgebra instance found
}

// resolveGradeAlgebra looks up a GradeAlgebra instance for the given grade kind
// and extracts the associated type family names by reducing the associated types.
// Returns a result with valid=false if no GradeAlgebra instance is found;
// callers must check valid before using the algebra.
func (ch *Checker) resolveGradeAlgebra(gradeKind types.Type) resolvedGradeAlgebra {
	classInfo, hasClass := ch.reg.LookupClass(gradeAlgebraClassName)
	if hasClass {
		// Match grade kind against instance type args via the promoted kind
		// registry. GradeAlgebra takes a Kind-kinded parameter (g: Kind).
		// Instance: impl GradeAlgebra Mult := ...
		// Instance TypeArgs[0] = TyCon("Mult") at L0 (value-level type).
		// Grade kind = PromotedDataKind("Mult") at L1 (promoted kind).
		// The promotion relationship is established by DataKinds and stored
		// in the registry. We look up each instance type arg's promoted form
		// and compare structurally with the grade kind.
		instances := ch.reg.InstancesForClass(gradeAlgebraClassName)
		for _, inst := range instances {
			if len(inst.TypeArgs) == 0 {
				continue
			}
			con, ok := inst.TypeArgs[0].(*types.TyCon)
			if !ok {
				continue
			}
			promoted, hasPromoted := ch.reg.LookupPromotedKind(con.Name)
			if !hasPromoted {
				continue
			}
			if types.Equal(promoted, gradeKind) {
				result := ch.extractGradeAlgebra(classInfo, inst)
				result.valid = true
				return result
			}
		}
	}
	// No GradeAlgebra instance found. Grade enforcement not available.
	return resolvedGradeAlgebra{valid: false}
}

// extractGradeAlgebra extracts GradeJoin and GradeDrop from a matched instance
// by reducing the associated type families with the instance's type args.
func (ch *Checker) extractGradeAlgebra(classInfo *ClassInfo, inst *InstanceInfo) resolvedGradeAlgebra {
	var result resolvedGradeAlgebra
	for _, assocName := range classInfo.AssocTypes {
		if _, ok := ch.reg.LookupFamily(assocName); !ok {
			continue
		}
		// Reduce the associated type with the instance's type args.
		reduced, didReduce := ch.reduceTyFamily(assocName, inst.TypeArgs, inst.S)
		if !didReduce {
			continue
		}
		switch assocName {
		case "GradeJoin":
			if con, ok := reduced.(*types.TyCon); ok {
				result.joinFamily = con.Name
			} else {
				return resolvedGradeAlgebra{}
			}
		case "GradeCompose":
			if con, ok := reduced.(*types.TyCon); ok {
				result.composeFamily = con.Name
			} else {
				return resolvedGradeAlgebra{}
			}
		case "GradeDrop":
			result.dropValue = reduced
		case "GradeUnit":
			result.unitValue = reduced
		}
	}
	return result
}

// gradeAxiomViolation records a detected axiom violation for deferred reporting.
type gradeAxiomViolation struct {
	kindName   string
	violations int
}

// collectGradeAxiomViolations checks GradeAlgebra axioms for all registered
// instances with closed (finite-domain) grade kinds in the current module.
// Returns violation records without touching the Checker's diagnostic state.
//
// This is a standalone function (not a Checker method) because the Go
// compiler's escape analysis is sensitive to addDiag call sites in Checker
// methods: even a conditional addDiag path causes the Checker to heap-escape,
// altering allocation patterns enough to perturb budget-sensitive module
// compilations. By collecting violations as data and deferring diagnostic
// emission to the caller, the axiom checker is invisible to escape analysis.
func collectGradeAxiomViolations(reg *Registry, currentMod string) []gradeAxiomViolation {
	classInfo, hasClass := reg.LookupClass(gradeAlgebraClassName)
	if !hasClass || classInfo == nil {
		return nil
	}
	instances := reg.InstancesForClass(gradeAlgebraClassName)
	var result []gradeAxiomViolation
	for _, inst := range instances {
		if inst.Module != currentMod {
			continue
		}
		if len(inst.TypeArgs) == 0 {
			continue
		}
		con, ok := inst.TypeArgs[0].(*types.TyCon)
		if !ok {
			continue
		}
		_, hasPromoted := reg.LookupPromotedKind(con.Name)
		if !hasPromoted {
			continue
		}
		algebra := extractGradeAlgebraFromRegistry(reg, classInfo, inst)
		if algebra.joinFamily == "" || algebra.dropValue == nil {
			continue
		}
		dt, dtOk := reg.LookupDataType(con.Name)
		if !dtOk {
			continue
		}
		fam, famOk := reg.LookupFamily(algebra.joinFamily)
		if !famOk || len(fam.Equations) == 0 {
			continue
		}
		if v := verifyGradeAxiomsForKind(dt, fam.Equations, algebra.dropValue); v > 0 {
			result = append(result, gradeAxiomViolation{kindName: con.Name, violations: v})
		}
	}
	return result
}

// emitGradeAxiomViolations reports collected violations as diagnostics.
// Takes *diagnostic.Errors directly (not *Checker) to avoid escape analysis
// interaction with the Checker receiver in the calling pipeline.
func emitGradeAxiomViolations(violations []gradeAxiomViolation, errs *diagnostic.Errors) {
	for _, v := range violations {
		errs.Add(&diagnostic.Error{
			Code:    diagnostic.ErrBadInstance,
			Phase:   diagnostic.PhaseCheck,
			Message: "GradeAlgebra axiom violation for " + v.kindName,
		})
	}
}

// extractGradeAlgebraFromRegistry extracts GradeAlgebra fields without
// touching any Checker state. Uses standalone pattern matching.
func extractGradeAlgebraFromRegistry(reg *Registry, classInfo *ClassInfo, inst *InstanceInfo) resolvedGradeAlgebra {
	var result resolvedGradeAlgebra
	for _, assocName := range classInfo.AssocTypes {
		fam, ok := reg.LookupFamily(assocName)
		if !ok {
			continue
		}
		for _, eq := range fam.Equations {
			if len(eq.Patterns) != len(inst.TypeArgs) {
				continue
			}
			subst := make(map[string]types.Type)
			matched := true
			for i, pat := range eq.Patterns {
				if !matchConcretePattern(pat, inst.TypeArgs[i], subst) {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
			reduced := substType(eq.RHS, subst)
			switch assocName {
			case "GradeJoin":
				if c, ok := reduced.(*types.TyCon); ok {
					result.joinFamily = c.Name
				}
			case "GradeCompose":
				if c, ok := reduced.(*types.TyCon); ok {
					result.composeFamily = c.Name
				}
			case "GradeDrop":
				result.dropValue = reduced
			case "GradeUnit":
				result.unitValue = reduced
			}
			break
		}
	}
	return result
}

// verifyGradeAxiomsFor checks axioms for one grade kind by brute-force
// enumeration of all constructor pairs. Uses family/reduce directly (not
// ch.reduceTyFamily) to avoid polluting the checker's unifier/budget state
// during cached module compilation.
// verifyGradeAxiomsForKind checks axioms for one grade kind and returns
// the number of violations. Not a Checker method — deliberately avoids
// any reference to the Checker to prevent escape analysis side effects.
func verifyGradeAxiomsForKind(
	dt *DataTypeInfo,
	joinEqs []env.TFEquation,
	dropValue types.Type,
) int {
	var cons []*types.TyCon
	for _, ci := range dt.Constructors {
		if ci.Arity == 0 {
			cons = append(cons, &types.TyCon{Name: ci.Name, Level: types.L0})
		}
	}
	if len(cons) < 2 {
		return 0
	}
	return checkGradeAxiomsConcrete(joinEqs, cons, dropValue)
}

// checkGradeAxiomsConcrete verifies commutativity and left-identity axioms
// for a concrete (finite-domain) grade kind. Returns the number of violations.
// Standalone function — no Checker dependency, no budget/unifier interaction.
func checkGradeAxiomsConcrete(joinEqs []env.TFEquation, cons []*types.TyCon, _ types.Type) int {
	violations := 0
	// Commutativity: Join(a, b) = Join(b, a)
	//
	// Note: left-identity (Join(Drop, a) = a for all a) is NOT checked.
	// GradeDrop is the "zero usage" element, but it is not the identity
	// of GradeJoin in a resource-consumption lattice. For example,
	// MultJoin(Zero, Linear) = Affine — this is correct semantics
	// (0 + 1 = at-most-1), not an axiom violation. The condition
	// Join(Drop, grade) = grade is used per-field in checkGradeBoundary
	// as a preservation test, not as a universal algebraic identity.
	for i, a := range cons {
		for j := i + 1; j < len(cons); j++ {
			b := cons[j]
			ab, okAB := reduceConcreteEqs(joinEqs, []types.Type{a, b})
			ba, okBA := reduceConcreteEqs(joinEqs, []types.Type{b, a})
			if okAB && okBA && !gradeConEqual(ab, ba) {
				violations++
			}
		}
	}
	return violations
}

// gradeContainsMeta reports whether ty contains any unsolved metavariable.
func gradeContainsMeta(ty types.Type) bool {
	return types.AnyType(ty, func(t types.Type) bool {
		_, ok := t.(*types.TyMeta)
		return ok
	})
}

// --- Grade boundary check ---
//
// Grade verification operates at two levels with distinct responsibilities:
//
// 1. Structural (unifier): row_unify.go ensures both sides of a unification
//    agree on the same grade value. This is pure shape matching — it resolves
//    grade metavariables but does not interpret them as algebraic elements.
//
// 2. Algebraic (this file): checkGradeBoundary verifies that resolved grades
//    satisfy GradeAlgebra laws (e.g., Join(Drop, g) = g for preservation).
//    This layer runs after unification and uses type family reduction.
//
// The two layers are complementary: (1) determines *what* the grade is,
// (2) determines whether it *permits* the operation.

// checkGradeBoundary verifies that capability fields with grade annotations
// respect their grades across the computation boundary.
//
// For each graded field in pre: if the field appears in post with the same type
// (i.e., was preserved unchanged), the grade must permit preservation.
// A field that was consumed (absent from post) or transitioned (type changed)
// is always valid regardless of grade.
//
// Two enforcement paths:
//   - Concrete grade (no metas): fast path via gradeCanPreserveDynamic with immediate error.
//   - Grade containing metas: emit CtFunEq constraint "GradeJoin(Drop, grade) ~ grade"
//     so the solver can re-check once the meta is solved.
//
// If no GradeAlgebra instance is found for the grade kind, grade enforcement
// is skipped — the field is treated as unrestricted.
func (ch *Checker) checkGradeBoundary(comp *types.TyCBPV, s span.Span) {
	preFields := extractCapFields(ch, comp.Pre)
	if len(preFields) == 0 {
		return
	}

	postFields := extractCapFields(ch, comp.Post)

	for _, f := range preFields {
		if len(f.Grades) == 0 {
			continue // no grade constraints (unrestricted)
		}

		postTy := types.RowFieldType(postFields, f.Label)
		if postTy == nil {
			continue // consumed: field not in post → OK
		}

		preTy := ch.unifier.Zonk(f.Type)
		postTy = ch.unifier.Zonk(postTy)

		if !types.Equal(preTy, postTy) {
			continue // transitioned: type changed → OK
		}

		// Field preserved unchanged. Check each grade allows preservation.
		for _, grade := range f.Grades {
			grade = ch.unifier.Zonk(grade)
			gk := ch.kindOfType(grade)
			if gk == nil {
				gk = gradeAlgebraKind(ch)
			}

			// Verify that GradeAlgebra exists for the grade kind.
			// Without an algebra, Join/Compose/Drop are undefined and
			// grade enforcement cannot operate — the annotation is
			// semantically vacuous. Report this explicitly rather than
			// silently treating the field as unrestricted.
			algebra := ch.resolveGradeAlgebra(gk)
			if !algebra.valid {
				ch.addDiag(diagnostic.ErrMultiplicity, s,
					diagFmt{Format: "grade @%s on capability %q requires impl %s %s",
						Args: []any{types.Pretty(grade), f.Label, gradeAlgebraClassName, types.Pretty(gk)}})
				continue
			}

			if gradeContainsMeta(grade) {
				// Deferred path: emit CtFunEq for Join(Drop, grade) ~ grade.
				// When the meta is solved, the solver re-processes the constraint
				// and the family reduces to a concrete result for unification.
				ch.emitGradePreserveConstraint(grade, gk, s)
				continue
			}

			// Fast path: concrete grade, check immediately.
			if !ch.gradeCanPreserveDynamic(grade, gk) {
				ch.addDiag(diagnostic.ErrMultiplicity, s,
					diagFmt{Format: "@%s capability %q must be consumed (type unchanged across computation boundary)",
						Args: []any{types.Pretty(grade), f.Label}})
			}
		}
	}
}

// gradeCanPreserveDynamic checks whether a field with the given grade can be
// preserved unchanged across a computation boundary, using the resolved grade algebra.
func (ch *Checker) gradeCanPreserveDynamic(grade types.Type, gradeKind types.Type) bool {
	algebra := ch.resolveGradeAlgebra(gradeKind)
	if !algebra.valid {
		return true // no grade algebra → treat as unrestricted
	}
	joined, ok := ch.reduceTyFamily(algebra.joinFamily, []types.Type{algebra.dropValue, grade}, span.Span{})
	if !ok {
		// Family reduction stuck (e.g., unsolved meta in args).
		// Assume OK; will be checked when the meta solves.
		return true
	}
	return types.Equal(joined, grade)
}

// emitGradePreserveConstraint emits a CtFunEq constraint encoding the
// preservation check: Join(Drop, grade) ~ grade.
//
// If the grade is e.g. a metavariable ?m, this constraint says:
// "when ?m is solved, Join(Drop, ?m) must equal ?m" — which is the
// algebraic definition of grade preservation.
func (ch *Checker) emitGradePreserveConstraint(grade types.Type, gradeKind types.Type, s span.Span) {
	algebra := ch.resolveGradeAlgebra(gradeKind)
	if !algebra.valid {
		return // no grade algebra → skip constraint emission
	}
	args := []types.Type{algebra.dropValue, grade}

	resultMeta := ch.freshMeta(gradeKind)
	blocking := ch.unifier.CollectBlockingMetas(args)
	if len(blocking) == 0 {
		// Invariant: gradeContainsMeta was true, so CollectBlockingMetas should
		// find at least one meta. If not, the meta was zonked between the check
		// and here. Fall back to the concrete fast path.
		if !ch.gradeCanPreserveDynamic(ch.unifier.Zonk(grade), gradeKind) {
			ch.addDiag(diagnostic.ErrMultiplicity, s,
				diagFmt{Format: "@%s capability must be consumed (type unchanged across computation boundary)", Args: []any{types.Pretty(grade)}})
		}
		return
	}

	ct := &CtFunEq{
		FamilyName: algebra.joinFamily,
		Args:       args,
		ResultMeta: resultMeta,
		BlockingOn: blocking,
		OnFailure: func(errSpan span.Span, expected, actual types.Type) {
			ch.addDiag(diagnostic.ErrMultiplicity, errSpan,
				diagFmt{Format: "@%s capability must be consumed (grade preservation violation: expected %s, got %s)", Args: []any{types.Pretty(grade), types.Pretty(expected), types.Pretty(actual)}})
		},
		S: s,
	}
	ch.registerStuckFunEq(ct)

	// When the family reduces, resultMeta will be unified with Join(Zero, grade).
	// Unify resultMeta ~ grade so that preservation is enforced: the result
	// of Join(Zero, grade) must equal grade itself.
	ch.emitEq(resultMeta, grade, s, nil)
}

// resolveGradeDrop returns the GradeDrop value for the default grade algebra,
// or nil if no grade algebra is available.
func (ch *Checker) resolveGradeDrop() types.Type {
	gk := gradeAlgebraKind(ch)
	algebra := ch.resolveGradeAlgebra(gk)
	if !algebra.valid {
		return nil
	}
	return algebra.dropValue
}

// extractCompGrade extracts the Grade from a TyCBPV, or nil if ungraded.
func (ch *Checker) extractCompGrade(ty types.Type) types.Type {
	ty = ch.unifier.Zonk(ty)
	if comp, ok := ty.(*types.TyCBPV); ok {
		return comp.Grade
	}
	return nil
}

// composeGrades computes GradeCompose(g1, g2), or nil if either is nil.
func (ch *Checker) composeGrades(g1, g2 types.Type) types.Type {
	if g1 == nil || g2 == nil {
		return nil
	}
	gk := gradeAlgebraKind(ch)
	algebra := ch.resolveGradeAlgebra(gk)
	if !algebra.valid || algebra.composeFamily == "" {
		return nil
	}
	composed, ok := ch.reduceTyFamily(algebra.composeFamily, []types.Type{g1, g2}, span.Span{})
	if !ok {
		// Family reduction stuck — return a TyFamilyApp to defer.
		return &types.TyFamilyApp{Name: algebra.composeFamily, Args: []types.Type{g1, g2}}
	}
	return composed
}

// extractCapFields returns the capability fields from a zonked row type, or nil.
func extractCapFields(ch *Checker, ty types.Type) []types.RowField {
	ty = ch.unifier.Zonk(ty)
	ev, ok := ty.(*types.TyEvidenceRow)
	if !ok {
		return nil
	}
	cap, ok := ev.Entries.(*types.CapabilityEntries)
	if !ok {
		return nil
	}
	return cap.Fields
}

// joinGrades computes the grade join of two annotated capability fields.
// Uses the GradeJoin associated type family from GradeAlgebra when available;
// falls back to unification.
func (ch *Checker) joinGrades(result *types.RowField, other []types.Type, s span.Span) {
	if len(result.Grades) == 0 && len(other) == 0 {
		return
	}

	// One side annotated, other unrestricted → take the annotation (more restrictive).
	if len(result.Grades) == 0 && len(other) > 0 {
		result.Grades = other
		return
	}
	if len(result.Grades) > 0 && len(other) == 0 {
		return // keep result grades
	}

	// Both annotated: grade counts must match.
	if len(result.Grades) != len(other) {
		ch.addDiag(diagnostic.ErrTypeMismatch, s,
			diagFmt{Format: "grade count mismatch for %s: %d vs %d",
				Args: []any{result.Label, len(result.Grades), len(other)}})
		return
	}

	// Resolve the grade algebra to get the join family name.
	gk := gradeAlgebraKind(ch)
	algebra := ch.resolveGradeAlgebra(gk)

	for i := range result.Grades {
		a := ch.unifier.Zonk(result.Grades[i])
		b := ch.unifier.Zonk(other[i])

		// Try GradeJoin family reduction.
		if algebra.valid && algebra.joinFamily != "" {
			joinResult, ok := ch.reduceTyFamily(algebra.joinFamily, []types.Type{a, b}, s)
			if ok {
				result.Grades[i] = joinResult
				continue
			}
			// Stuck: emit CtFunEq for deferred join reduction.
			args := []types.Type{a, b}
			blocking := ch.unifier.CollectBlockingMetas(args)
			if len(blocking) > 0 {
				resultMeta := ch.freshMeta(gk)
				ct := &CtFunEq{
					FamilyName: algebra.joinFamily,
					Args:       args,
					ResultMeta: resultMeta,
					BlockingOn: blocking,
					S:          s,
				}
				ch.registerStuckFunEq(ct)
				result.Grades[i] = resultMeta
				continue
			}
		}
		// No GradeAlgebra or no blocking metas: fall back to equality constraint.
		ch.emitEq(a, b, s, nil)
	}
}

// reduceConcreteEqs matches type family equations against concrete
// (meta-free) args. Pure function — no side effects.
func reduceConcreteEqs(eqs []env.TFEquation, args []types.Type) (types.Type, bool) {
	for _, eq := range eqs {
		if len(eq.Patterns) != len(args) {
			continue
		}
		subst := make(map[string]types.Type)
		matched := true
		for i, pat := range eq.Patterns {
			if !matchConcretePattern(pat, args[i], subst) {
				matched = false
				break
			}
		}
		if matched {
			return substType(eq.RHS, subst), true
		}
	}
	return nil, false
}

// gradeConEqual compares two grade constructor values by name only.
// Grade constructors in axiom verification come from two sources with
// potentially different Level fields (equation RHS vs freshly constructed).
// Since grade constructors are always nullary promoted data constructors,
// name equality is sufficient and correct.
func gradeConEqual(a, b types.Type) bool {
	ac, ok1 := a.(*types.TyCon)
	bc, ok2 := b.(*types.TyCon)
	if ok1 && ok2 {
		return ac.Name == bc.Name
	}
	return types.Equal(a, b)
}

// matchConcretePattern matches a TF equation pattern against a concrete
// (meta-free) argument. Handles TyVar (pattern variable), TyCon
// (constructor literal), and TyApp (type application, e.g. tuple patterns).
func matchConcretePattern(pat, arg types.Type, subst map[string]types.Type) bool {
	switch p := pat.(type) {
	case *types.TyVar:
		if p.Name == "_" {
			return true
		}
		if existing, ok := subst[p.Name]; ok {
			return types.Equal(existing, arg)
		}
		subst[p.Name] = arg
		return true
	case *types.TyCon:
		c, ok := arg.(*types.TyCon)
		return ok && p.Name == c.Name
	case *types.TyApp:
		a, ok := arg.(*types.TyApp)
		if !ok {
			return false
		}
		return matchConcretePattern(p.Fun, a.Fun, subst) &&
			matchConcretePattern(p.Arg, a.Arg, subst)
	default:
		return types.Equal(pat, arg)
	}
}

// substType substitutes pattern variables in a type family RHS.
func substType(t types.Type, subst map[string]types.Type) types.Type {
	switch x := t.(type) {
	case *types.TyVar:
		if s, ok := subst[x.Name]; ok {
			return s
		}
		return t
	case *types.TyApp:
		newFun := substType(x.Fun, subst)
		newArg := substType(x.Arg, subst)
		if newFun == x.Fun && newArg == x.Arg {
			return t
		}
		return &types.TyApp{Fun: newFun, Arg: newArg, IsGrade: x.IsGrade}
	default:
		return t
	}
}
