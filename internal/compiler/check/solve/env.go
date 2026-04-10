package solve

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Env provides the solver with access to checker infrastructure.
// Defined by the consumer (check package); implemented by *Checker.
//
// Composed of five role-specific sub-interfaces:
//   - UnifierEnv:     unification state and trial/probe scopes
//   - RegistryEnv:    type class registry and context lookup
//   - BudgetEnv:      step limits, depth limits, cancellation
//   - DiagnosticEnv:  error reporting and fresh generation
//   - FamilyEnv:      type family reduction
type Env interface {
	UnifierEnv
	RegistryEnv
	BudgetEnv
	DiagnosticEnv
	FamilyEnv
}

// UnifierEnv groups operations on the unification state: zonking, solving,
// level management, given equalities, and speculative scopes.
type UnifierEnv interface {
	Zonk(types.Type) types.Type
	Unify(a, b types.Type) error
	SolverLevel() int
	SetSolverLevel(int)
	InstallGivenEq(skolemID int, ty types.Type)
	RemoveGivenEq(skolemID int)
	WithTrial(fn func() bool) bool
	WithProbe(fn func() bool) bool
}

// RegistryEnv groups read-only queries against the type class registry
// and the current checker context for evidence lookup.
type RegistryEnv interface {
	InstancesForClass(string) []*env.InstanceInfo
	LookupClass(string) (*env.ClassInfo, bool)
	ClassFromDict(string) (string, bool)
	ScanContext(func(env.CtxEntry) bool)
	LookupDictVar(className string) []*env.CtxVar
	LookupEvidence(className string) []*env.CtxEvidence
	DictVarClasses() []string
}

// BudgetEnv groups resource-budget operations that enforce termination
// and cancellation.
type BudgetEnv interface {
	ResetSolverSteps()
	SolverStep() error
	EnterResolve() error
	LeaveResolve()
	CheckCancelled() bool
}

// DiagnosticEnv groups error reporting and fresh name/meta generation.
type DiagnosticEnv interface {
	AddCodedError(diagnostic.Code, span.Span, string)
	ErrorCount() int
	TruncateErrors(int)
	Fresh() int
	FreshMeta(types.Type) *types.TyMeta
}

// FamilyEnv groups type family reduction.
type FamilyEnv interface {
	ReduceTyFamily(name string, args []types.Type, s span.Span) (types.Type, bool)
}
