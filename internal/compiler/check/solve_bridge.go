// solve_bridge.go — Boundary between check and solve packages.
// All cross-package interaction with the solver is consolidated here:
// solve.Env interface implementation, delegation methods, and type aliases.
package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Constraint type aliases ---

// Ct, CtClass, CtEq, CtFunEq, CtImplication are defined in the solve subpackage.
type Ct = solve.Ct
type CtClass = solve.CtClass
type CtEq = solve.CtEq
type CtFunEq = solve.CtFunEq
type CtImplication = solve.CtImplication

// InertSet is defined in the solve subpackage.
type InertSet = solve.InertSet

// Worklist is defined in the solve subpackage.
type Worklist = solve.Worklist

// --- solve.Env interface methods ---

func (ch *Checker) Zonk(t types.Type) types.Type         { return ch.unifier.Zonk(t) }
func (ch *Checker) Unify(a, b types.Type) error          { return ch.unifier.Unify(a, b) }
func (ch *Checker) SolverLevel() int                     { return ch.unifier.SolverLevel }
func (ch *Checker) SetSolverLevel(l int)                 { ch.unifier.SolverLevel = l }
func (ch *Checker) InstallGivenEq(id int, ty types.Type) { ch.unifier.InstallGivenEq(id, ty) }
func (ch *Checker) RemoveGivenEq(id int)                 { ch.unifier.RemoveGivenEq(id) }
func (ch *Checker) ScanContext(fn func(CtxEntry) bool)   { ch.ctx.Scan(fn) }
func (ch *Checker) AddCodedError(code diagnostic.Code, s span.Span, msg string) {
	ch.addCodedError(code, s, msg)
}
func (ch *Checker) ErrorCount() int                      { return ch.errors.Len() }
func (ch *Checker) TruncateErrors(n int)                 { ch.errors.Truncate(n) }
func (ch *Checker) ResetSolverSteps()                    { ch.budget.ResetSolverSteps() }
func (ch *Checker) SolverStep() error                    { return ch.budget.SolverStep() }
func (ch *Checker) EnterResolve() error                  { return ch.budget.EnterResolve() }
func (ch *Checker) LeaveResolve()                        { ch.budget.LeaveResolve() }
func (ch *Checker) CheckCancelled() bool                 { return ch.checkCancelled() }
func (ch *Checker) WithTrial(fn func() bool) bool        { return ch.withTrial(fn) }
func (ch *Checker) WithProbe(fn func() bool) bool        { return ch.withProbe(fn) }
func (ch *Checker) Fresh() int                           { return ch.fresh() }
func (ch *Checker) FreshMeta(k types.Type) *types.TyMeta { return ch.freshMeta(k) }
func (ch *Checker) InstancesForClass(name string) []*InstanceInfo {
	return ch.reg.InstancesForClass(name)
}
func (ch *Checker) LookupClass(name string) (*ClassInfo, bool) {
	return ch.reg.LookupClass(name)
}
func (ch *Checker) ClassFromDict(name string) (string, bool) {
	return ch.reg.ClassFromDict(name)
}
func (ch *Checker) ReduceTyFamily(name string, args []types.Type, s span.Span) (types.Type, bool) {
	return ch.reduceTyFamily(name, args, s)
}

// --- Delegation methods ---

// solveWanteds delegates to the solver's SolveWanteds.
func (ch *Checker) solveWanteds(shouldDefer func(string, []types.Type) bool) (map[string]ir.Core, []*CtClass) {
	return ch.solver.SolveWanteds(shouldDefer)
}

// emitClassConstraint records a class constraint by pushing it to the worklist.
func (ch *Checker) emitClassConstraint(placeholder string, entry types.ConstraintEntry, s span.Span) {
	ch.solver.EmitClassConstraint(placeholder, entry, s)
}

// emitEq emits a type equality constraint to the solver worklist.
// Origin provides semantic context for error reporting; nil = generic message.
func (ch *Checker) emitEq(lhs, rhs types.Type, s span.Span, origin *solve.CtOrigin) {
	ch.solver.Emit(&solve.CtEq{Lhs: lhs, Rhs: rhs, Origin: origin, S: s})
}

// emitGivenEq emits a given equality to the solver with priority processing.
// Given equalities are pushed to the front of the worklist so they are
// processed before wanted constraints. The unifier's InstallGivenEq is
// also called for Zonk transparency — this dual installation is the
// hybrid approach for Step 4; Step 7 will consolidate.
func (ch *Checker) emitGivenEq(lhs, rhs types.Type, s span.Span) {
	ch.solver.EmitGivenEq(&solve.CtEq{Lhs: lhs, Rhs: rhs, Flavor: solve.CtGiven, S: s})
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
	return ch.solver.CheckWithLocalScope(
		func(ty types.Type) ir.Core { return ch.check(expr, ty) },
		expected, skolemIDs,
	)
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
