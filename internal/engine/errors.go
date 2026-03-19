package engine

import (
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
)

// Diagnostic is a single structured error from compilation.
type Diagnostic struct {
	Code    int
	Phase   string // "lex", "parse", or "check"
	Line    int
	Col     int
	Message string
}

// CompileError wraps compilation errors (lex, parse, or type check).
// Use Error() for a formatted message or Diagnostics() for structured access.
type CompileError struct {
	Errors *errs.Errors
}

func (e *CompileError) Error() string {
	return e.Errors.Format()
}

// Diagnostics returns structured diagnostics for programmatic access.
// Errors without source location (e.g. context cancellation) have Line=0, Col=0.
func (e *CompileError) Diagnostics() []Diagnostic {
	diags := make([]Diagnostic, len(e.Errors.Errs))
	for i, err := range e.Errors.Errs {
		d := Diagnostic{
			Code:    int(err.Code),
			Phase:   err.Phase.String(),
			Message: err.Message,
		}
		if err.Span != (span.Span{}) {
			d.Line, d.Col = e.Errors.Source.Location(err.Span.Start)
		}
		diags[i] = d
	}
	return diags
}
