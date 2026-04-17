package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Structured diagnostic detail types. Each implements
// diagnostic.DetailFormatter. The checker constructs these instead of
// formatting message strings inline, separating diagnostic content
// from type-checking logic.

// diagMsg is a simple static or preformatted message.
type diagMsg string

func (d diagMsg) DiagnosticMessage() string { return string(d) }

// diagUnknown reports an unknown/unbound name.
type diagUnknown struct {
	Kind string // "variable", "constructor", "qualifier", "type", "class", "module", "operator"
	Name string
}

func (d diagUnknown) DiagnosticMessage() string { return "unknown " + d.Kind + ": " + d.Name }

// diagWithErr appends an error's message to a context string.
type diagWithErr struct {
	Context string
	Err     error
}

func (d diagWithErr) DiagnosticMessage() string { return d.Context + ": " + d.Err.Error() }

// diagWithType appends a pretty-printed type to a context string.
type diagWithType struct {
	Context string
	Type    types.Type
	TypeOps *types.TypeOps
}

func (d diagWithType) DiagnosticMessage() string {
	return d.Context + d.TypeOps.Pretty(d.Type)
}

// diagExpectGot reports "expected X, got Y" with pretty-printed types.
type diagExpectGot struct {
	Context  string
	Expected types.Type
	Got      types.Type
	TypeOps  *types.TypeOps
}

func (d diagExpectGot) DiagnosticMessage() string {
	return d.Context + ": expected " + d.TypeOps.Pretty(d.Expected) + ", got " + d.TypeOps.Pretty(d.Got)
}

// diagFmt uses fmt.Sprintf for complex messages.
type diagFmt struct {
	Format string
	Args   []any
}

func (d diagFmt) DiagnosticMessage() string { return fmt.Sprintf(d.Format, d.Args...) }

// diagLabel reports a label-related issue.
type diagLabel struct {
	Label   string
	Context string // "duplicate label %q in record type", etc.
}

func (d diagLabel) DiagnosticMessage() string {
	return fmt.Sprintf(d.Context, d.Label)
}

// --- Helper methods on CheckState ---

func (s *CheckState) addDiag(code diagnostic.Code, sp span.Span, detail diagnostic.DetailFormatter) {
	// Once a nesting limit error has been reported, suppress secondary
	// diagnostics (type mismatches, missing instances) that cascade from
	// the TyError placeholder returned at the limit boundary.
	if s.nestingReported && code != diagnostic.ErrNestingLimit {
		return
	}
	s.errors.Add(&diagnostic.Error{
		Code:    code,
		Phase:   diagnostic.PhaseCheck,
		Span:    sp,
		Message: detail.DiagnosticMessage(),
		Detail:  detail,
	})
}

func (s *CheckState) addDiagHints(code diagnostic.Code, sp span.Span, detail diagnostic.DetailFormatter, hints []diagnostic.Hint) {
	if s.nestingReported && code != diagnostic.ErrNestingLimit {
		return
	}
	s.errors.Add(&diagnostic.Error{
		Code:    code,
		Phase:   diagnostic.PhaseCheck,
		Span:    sp,
		Message: detail.DiagnosticMessage(),
		Detail:  detail,
		Hints:   hints,
	})
}

// --- Unification error mapping ---

// unifyErrorCode maps a UnifyError to the corresponding diagnostic.Code.
// Returns ErrTypeMismatch for non-UnifyError or general mismatch.
func unifyErrorCode(err error) diagnostic.Code {
	ue, ok := err.(unify.UnifyError)
	if !ok {
		return diagnostic.ErrTypeMismatch
	}
	switch ue.Kind() {
	case unify.UnifyOccursCheck:
		return diagnostic.ErrOccursCheck
	case unify.UnifyDupLabel:
		return diagnostic.ErrDuplicateLabel
	case unify.UnifyRowMismatch:
		return diagnostic.ErrRowMismatch
	case unify.UnifySkolemRigid:
		return diagnostic.ErrSkolemRigid
	case unify.UnifyUntouchable:
		return diagnostic.ErrUntouchable
	default:
		return diagnostic.ErrTypeMismatch
	}
}

// addUnifyError maps a unification error to the appropriate structured error code.
func (s *CheckState) addUnifyError(err error, sp span.Span, ctx string) {
	if _, ok := err.(*unify.MismatchError); ok {
		s.addDiag(unifyErrorCode(err), sp, diagMsg(ctx))
		return
	}
	s.addDiag(unifyErrorCode(err), sp, diagWithErr{Context: ctx, Err: err})
}

// addSemanticUnifyError reports a unification failure with a semantic error code.
// For simple mismatches, the semantic code is used. For specific failures
// (occurs check, skolem rigidity, etc.), the underlying error overrides.
func (s *CheckState) addSemanticUnifyError(semanticCode diagnostic.Code, err error, sp span.Span, ctx string) {
	code := unifyErrorCode(err)
	if code == diagnostic.ErrTypeMismatch {
		s.addDiag(semanticCode, sp, diagMsg(ctx))
		return
	}
	s.addDiag(code, sp, diagWithErr{Context: ctx, Err: err})
}
