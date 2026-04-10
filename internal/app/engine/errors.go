package engine

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// Diagnostic is a single structured error from compilation.
type Diagnostic struct {
	Code    int
	Phase   string // "lex", "parse", or "check"
	Line    int
	Col     int
	Message string
	Hints   []DiagnosticHint // secondary annotations (may be nil)
}

// DiagnosticHint is a secondary annotation on a compilation diagnostic.
type DiagnosticHint struct {
	Line    int
	Col     int
	Message string
}

// CompileError wraps compilation errors (lex, parse, or type check).
// Use Error() for a formatted message or Diagnostics() for structured access.
type CompileError struct {
	errs *diagnostic.Errors
}

func (e *CompileError) Error() string {
	return e.errs.Format()
}

// Diagnostics returns structured diagnostics for programmatic access.
// Errors without source location (e.g. context cancellation) have Line=0, Col=0.
func (e *CompileError) Diagnostics() []Diagnostic {
	diags := make([]Diagnostic, len(e.errs.Errs))
	for i, err := range e.errs.Errs {
		d := Diagnostic{
			Code:    int(err.Code),
			Phase:   err.Phase.String(),
			Message: err.Message,
		}
		if err.Span != (span.Span{}) {
			d.Line, d.Col = e.errs.Source.Location(err.Span.Start)
		}
		if len(err.Hints) > 0 {
			d.Hints = make([]DiagnosticHint, len(err.Hints))
			for j, h := range err.Hints {
				dh := DiagnosticHint{Message: h.Message}
				if h.Span != (span.Span{}) {
					dh.Line, dh.Col = e.errs.Source.Location(h.Span.Start)
				}
				d.Hints[j] = dh
			}
		}
		diags[i] = d
	}
	return diags
}
