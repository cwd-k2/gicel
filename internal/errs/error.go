package errs

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gomputation/internal/span"
)

// Code is a numeric error identifier (E0001–E9999).
type Code int

// --- Error codes ---
// Lex errors (E0001–E0099)
const (
	ErrUnexpectedChar  Code = 1 // unexpected character in source
	ErrUnterminatedStr Code = 2 // unterminated string literal
	ErrBadEscape       Code = 3 // unknown escape sequence
	ErrUnterminatedLit Code = 4 // unterminated literal
	ErrEmptyRuneLit    Code = 5 // empty rune literal
)

// Parse errors (E0100–E0199)
const (
	ErrParseSyntax Code = 100 // general syntax error
)

// Type check errors (E0200–E0299)
const (
	ErrTypeMismatch   Code = 200 // type mismatch in unification
	ErrUnboundVar     Code = 201 // unbound variable
	ErrUnboundCon     Code = 202 // unknown constructor
	ErrNonExhaustive  Code = 203 // non-exhaustive pattern match
	ErrBadApplication Code = 204 // application of non-function type
	ErrBadComputation Code = 205 // expected computation type
	ErrBadThunk       Code = 206 // thunk/force type error
	ErrSpecialForm    Code = 207 // misuse of special form (pure/bind/thunk/force)
	ErrAssumption     Code = 208 // assumption declaration error
	ErrCyclicAlias    Code = 209 // cyclic type alias
	ErrDuplicateLabel Code = 210 // duplicate label in row
	ErrRowMismatch    Code = 211 // row unification failure
	ErrOccursCheck    Code = 212 // infinite type (occurs check)
	ErrBadTypeApp     Code = 213 // type application error
	ErrEmptyDo        Code = 214 // empty do block
	ErrBadDoEnding    Code = 215 // do block doesn't end with expression
	ErrNoInstance     Code = 220 // no matching type class instance
	ErrOverlap        Code = 221 // overlapping instances
	ErrBadClass       Code = 222 // invalid class declaration
	ErrBadInstance    Code = 223 // invalid instance declaration
	ErrMissingMethod  Code = 224 // missing method in instance
	ErrImport         Code = 230 // module import error
	ErrSkolemEscape   Code = 240 // existential type variable escapes scope
	ErrSkolemRigid    Code = 241 // cannot unify rigid (skolem) type variable
)

// Phase indicates which compiler stage produced the error.
type Phase int

const (
	PhaseLex Phase = iota
	PhaseParse
	PhaseCheck
	PhaseEval
)

// String returns a human-readable phase name.
func (p Phase) String() string {
	switch p {
	case PhaseLex:
		return "lex"
	case PhaseParse:
		return "parse"
	case PhaseCheck:
		return "check"
	case PhaseEval:
		return "eval"
	default:
		return "unknown"
	}
}

// Error is a structured diagnostic.
type Error struct {
	Code    Code
	Phase   Phase
	Span    span.Span
	Message string
	Hints   []Hint
}

func (e *Error) Error() string {
	return fmt.Sprintf("E%04d: %s", e.Code, e.Message)
}

// Hint is a secondary annotation attached to a span.
type Hint struct {
	Span    span.Span
	Message string
}

// Errors collects multiple diagnostics for a single source.
type Errors struct {
	Source *span.Source
	Errs   []*Error
}

// Add appends an error to the collection.
func (es *Errors) Add(e *Error) {
	es.Errs = append(es.Errs, e)
}

// HasErrors returns true if any errors have been collected.
func (es *Errors) HasErrors() bool {
	return len(es.Errs) > 0
}

// Format renders all errors for human consumption.
func (es *Errors) Format() string {
	if !es.HasErrors() {
		return ""
	}

	var b strings.Builder
	for i, e := range es.Errs {
		if i > 0 {
			b.WriteByte('\n')
		}
		es.formatOne(&b, e)
	}
	return b.String()
}

func (es *Errors) formatOne(b *strings.Builder, e *Error) {
	line, col := es.Source.Location(e.Span.Start)
	// error[E0001]: message
	//  --> file.gmp:1:5
	fmt.Fprintf(b, "error[E%04d]: %s\n", e.Code, e.Message)
	fmt.Fprintf(b, " --> %s:%d:%d\n", es.Source.Name, line, col)

	// Source line with underline
	es.renderSpan(b, e.Span)

	for _, h := range e.Hints {
		hLine, hCol := es.Source.Location(h.Span.Start)
		fmt.Fprintf(b, " --> %s:%d:%d\n", es.Source.Name, hLine, hCol)
		fmt.Fprintf(b, "   = hint: %s\n", h.Message)
	}
}

func (es *Errors) renderSpan(b *strings.Builder, sp span.Span) {
	line, col := es.Source.Location(sp.Start)
	lineIdx := line - 1

	// Extract the source line text.
	var lineText string
	if lineIdx < len(es.Source.Lines) {
		start := int(es.Source.Lines[lineIdx])
		end := len(es.Source.Text)
		if lineIdx+1 < len(es.Source.Lines) {
			end = int(es.Source.Lines[lineIdx+1])
		}
		lineText = strings.TrimRight(es.Source.Text[start:end], "\n")
	}

	width := fmt.Sprintf("%d", line)
	fmt.Fprintf(b, " %s |\n", strings.Repeat(" ", len(width)))
	fmt.Fprintf(b, " %d | %s\n", line, lineText)
	fmt.Fprintf(b, " %s | %s%s\n",
		strings.Repeat(" ", len(width)),
		strings.Repeat(" ", col-1),
		underline(sp))
}

func underline(sp span.Span) string {
	length := int(sp.End - sp.Start)
	if length <= 0 {
		return "^"
	}
	return strings.Repeat("^", length)
}
