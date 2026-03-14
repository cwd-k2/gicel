package gicel

import "github.com/cwd-k2/gicel/internal/errs"

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
	errors *errs.Errors
}

func (e *CompileError) Error() string {
	return e.errors.Format()
}

// Diagnostics returns structured diagnostics for programmatic access.
func (e *CompileError) Diagnostics() []Diagnostic {
	diags := make([]Diagnostic, len(e.errors.Errs))
	for i, err := range e.errors.Errs {
		line, col := e.errors.Source.Location(err.Span.Start)
		diags[i] = Diagnostic{
			Code:    int(err.Code),
			Phase:   err.Phase.String(),
			Line:    line,
			Col:     col,
			Message: err.Message,
		}
	}
	return diags
}
