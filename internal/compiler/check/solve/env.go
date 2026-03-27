package solve

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Env provides the solver with access to checker infrastructure.
// Defined by the consumer (check package); implemented by *Checker.
type Env interface {
	// Unifier
	Zonk(types.Type) types.Type
	Unify(a, b types.Type) error
	SolverLevel() int
	SetSolverLevel(int)
	InstallGivenEq(skolemID int, ty types.Type)
	RemoveGivenEq(skolemID int)

	// Registry reads
	InstancesForClass(string) []*env.InstanceInfo
	LookupClass(string) (*env.ClassInfo, bool)
	ClassFromDict(string) (string, bool)

	// Context scan
	ScanContext(func(env.CtxEntry) bool)
	LookupDictVar(className string) []*env.CtxVar
	LookupEvidence(className string) []*env.CtxEvidence

	// Budget
	ResetSolverSteps()
	SolverStep() error
	EnterResolve() error
	LeaveResolve()

	// Diagnostics
	AddCodedError(diagnostic.Code, span.Span, string)
	ErrorCount() int
	TruncateErrors(int)

	// Fresh generation
	Fresh() int
	FreshMeta(types.Type) *types.TyMeta

	// Trial/probe unification scopes
	WithTrial(fn func() bool) bool
	WithProbe(fn func() bool) bool

	// Cancellation
	CheckCancelled() bool

	// Type family reduction
	ReduceTyFamily(name string, args []types.Type, s span.Span) (types.Type, bool)
}
