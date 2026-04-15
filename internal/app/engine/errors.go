package engine

import (
	"fmt"
	"runtime"
	"strings"

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

// recoverAsError installs a deferred recovery that converts application-level
// panics to *InternalPanicError. Runtime panics that indicate process-level
// corruption (concurrent map access, stack overflow) are re-panicked —
// continuing execution after such failures is unsafe.
func recoverAsError(errp *error) {
	r := recover()
	if r == nil {
		return
	}
	// runtime.Error covers nil deref, index out of range, etc. — these are
	// application bugs that the host can meaningfully report and recover from.
	// However, certain runtime panics indicate process corruption where
	// recovery is unsafe. Go surfaces these as plain strings, not runtime.Error.
	if s, ok := r.(string); ok {
		if strings.Contains(s, "concurrent") {
			panic(r) // concurrent map access — process state is corrupted
		}
	}
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	*errp = &InternalPanicError{Value: r, Stack: buf[:n]}
}

// panicToErrors converts a recovered panic into a *diagnostic.Errors
// for use in Analyze (which returns AnalysisResult, not error).
// The stack trace is included in the diagnostic message so that
// embedded callers can log the full context for bug reports.
func panicToErrors(r any, stack []byte) *diagnostic.Errors {
	errs := &diagnostic.Errors{}
	errs.Add(&diagnostic.Error{
		Code:    0, // internal panic — no user-facing error code
		Phase:   diagnostic.PhaseCheck,
		Message: fmt.Sprintf("internal panic: %v\n%s", r, stack),
	})
	return errs
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
