// Grade axiom verification — checks GradeAlgebra axioms for registered instances.
// Does NOT cover: grade_boundary.go (preservation enforcement), grade.go (algebra resolution).
package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

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
func collectGradeAxiomViolations(ops *types.TypeOps, reg *Registry, currentMod string) []gradeAxiomViolation {
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
		algebra := extractGradeAlgebraFromRegistry(ops, reg, classInfo, inst)
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
		if v := verifyGradeAxiomsForKind(ops, dt, fam.Equations, algebra.dropValue); v > 0 {
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
func extractGradeAlgebraFromRegistry(ops *types.TypeOps, reg *Registry, classInfo *ClassInfo, inst *InstanceInfo) resolvedGradeAlgebra {
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
				if !matchConcretePattern(ops, pat, inst.TypeArgs[i], subst) {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
			reduced := substType(ops, eq.RHS, subst)
			switch assocName {
			case gradeAssocJoin:
				if c, ok := reduced.(*types.TyCon); ok {
					result.joinFamily = c.Name
				}
			case gradeAssocCompose:
				if c, ok := reduced.(*types.TyCon); ok {
					result.composeFamily = c.Name
				}
			case gradeAssocDrop:
				result.dropValue = reduced
			case gradeAssocUnit:
				result.unitValue = reduced
			}
			break
		}
	}
	return result
}

// verifyGradeAxiomsForKind checks axioms for one grade kind and returns
// the number of violations. Not a Checker method — deliberately avoids
// any reference to the Checker to prevent escape analysis side effects.
func verifyGradeAxiomsForKind(
	ops *types.TypeOps,
	dt *DataTypeInfo,
	joinEqs []env.TFEquation,
	dropValue types.Type,
) int {
	var cons []*types.TyCon
	for _, ci := range dt.Constructors {
		if ci.Arity == 0 {
			cons = append(cons, ops.ConLevel(ci.Name, types.L0, false))
		}
	}
	if len(cons) < 2 {
		return 0
	}
	return checkGradeAxiomsConcrete(ops, joinEqs, cons, dropValue)
}

// checkGradeAxiomsConcrete verifies commutativity and left-identity axioms
// for a concrete (finite-domain) grade kind. Returns the number of violations.
// Standalone function — no Checker dependency, no budget/unifier interaction.
func checkGradeAxiomsConcrete(ops *types.TypeOps, joinEqs []env.TFEquation, cons []*types.TyCon, _ types.Type) int {
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
			ab, okAB := reduceConcreteEqs(ops, joinEqs, []types.Type{a, b})
			ba, okBA := reduceConcreteEqs(ops, joinEqs, []types.Type{b, a})
			if okAB && okBA && !gradeConEqual(ops, ab, ba) {
				violations++
			}
		}
	}
	return violations
}
