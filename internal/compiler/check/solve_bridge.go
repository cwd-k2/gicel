package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Constraint type aliases ---

// Ct, CtClass, CtFunEq, CtImplication are defined in the solve subpackage.
type Ct = solve.Ct
type CtClass = solve.CtClass
type CtFunEq = solve.CtFunEq
type CtImplication = solve.CtImplication

// InertSet is defined in the solve subpackage.
type InertSet = solve.InertSet

// Worklist is defined in the solve subpackage.
type Worklist = solve.Worklist

// --- Delegation methods ---

// solveWanteds delegates to the solver's SolveWanteds.
func (ch *Checker) solveWanteds(shouldDefer func(string, []types.Type) bool) (map[string]ir.Core, []*CtClass) {
	return ch.solver.SolveWanteds(shouldDefer)
}

// emitClassConstraint records a class constraint by pushing it to the worklist.
func (ch *Checker) emitClassConstraint(placeholder string, entry types.ConstraintEntry, s span.Span) {
	ch.solver.EmitClassConstraint(placeholder, entry, s)
}

// resolveDeferredConstraints discharges all worklist constraints eagerly.
func (ch *Checker) resolveDeferredConstraints(expr ir.Core) ir.Core {
	return ch.solver.ResolveDeferredConstraints(expr)
}

// resolveDeferredConstraintsDeferrable resolves worklist constraints but
// returns ambiguous plain-args constraints as residuals.
func (ch *Checker) resolveDeferredConstraintsDeferrable(expr ir.Core) (ir.Core, []*CtClass) {
	return ch.solver.ResolveDeferredConstraintsDeferrable(expr)
}

// tryResolveInstance attempts instance resolution without emitting errors.
func (ch *Checker) tryResolveInstance(className string, args []types.Type, s span.Span) (ir.Core, bool) {
	return ch.solver.TryResolveInstance(className, args, s)
}

// checkWithLocalScope checks expr against expected inside an implication scope.
func (ch *Checker) checkWithLocalScope(expr syntax.Expr, expected types.Type, skolemIDs map[int]string) ir.Core {
	return ch.solver.CheckWithLocalScope(expr, expected, skolemIDs)
}

// extractDictField delegates to the solver's method.
func (ch *Checker) extractDictField(classInfo *ClassInfo, dictExpr ir.Core, fieldIdx int, prefix string, s span.Span) ir.Core {
	return ch.solver.ExtractDictField(classInfo, dictExpr, fieldIdx, prefix, s)
}

// resolveInstance delegates to the solver's method.
func (ch *Checker) resolveInstance(className string, args []types.Type, s span.Span) ir.Core {
	return ch.solver.ResolveInstance(className, args, s)
}

// freshInstanceSubst delegates to the solver's method.
func (ch *Checker) freshInstanceSubst(inst *InstanceInfo) map[string]types.Type {
	return ch.solver.FreshInstanceSubst(inst)
}

// substitutePlaceholders replaces Var nodes matching placeholders.
func (ch *Checker) substitutePlaceholders(expr ir.Core, resolutions map[string]ir.Core) ir.Core {
	return solve.SubstitutePlaceholders(expr, resolutions)
}

// sliceHasMeta returns true if any type in the slice contains an unsolved TyMeta.
func sliceHasMeta(tys []types.Type) bool {
	return solve.SliceHasMeta(tys)
}
