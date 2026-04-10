package check

import (
	"fmt"

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

func (d diagUnknown) DiagnosticMessage() string { return d.Kind + ": " + d.Name }

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
}

func (d diagWithType) DiagnosticMessage() string {
	return d.Context + types.Pretty(d.Type)
}

// diagExpectGot reports "expected X, got Y" with pretty-printed types.
type diagExpectGot struct {
	Context  string
	Expected types.Type
	Got      types.Type
}

func (d diagExpectGot) DiagnosticMessage() string {
	return d.Context + ": expected " + types.Pretty(d.Expected) + ", got " + types.Pretty(d.Got)
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
	s.errors.Add(&diagnostic.Error{
		Code:    code,
		Phase:   diagnostic.PhaseCheck,
		Span:    sp,
		Message: detail.DiagnosticMessage(),
		Detail:  detail,
	})
}

func (s *CheckState) addDiagHints(code diagnostic.Code, sp span.Span, detail diagnostic.DetailFormatter, hints []diagnostic.Hint) {
	s.errors.Add(&diagnostic.Error{
		Code:    code,
		Phase:   diagnostic.PhaseCheck,
		Span:    sp,
		Message: detail.DiagnosticMessage(),
		Detail:  detail,
		Hints:   hints,
	})
}
