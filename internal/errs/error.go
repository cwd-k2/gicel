package errs

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/span"
)

// Code is a numeric error identifier (E0001–E9999).
type Code int

// --- Error codes ---
// Lex errors (E0001–E0099)
const (
	ErrUnexpectedChar  Code = 1 // unexpected character in source
	ErrBadEscape       Code = 3 // unknown escape sequence
	ErrUnterminatedLit Code = 4 // unterminated literal
	ErrEmptyRuneLit    Code = 5 // empty rune literal
	ErrReservedInOp    Code = 6 // reserved symbol inside operator
)

// Parse errors (E0100–E0199)
const (
	ErrParseSyntax      Code = 100 // general syntax error (fallback)
	ErrUnexpectedToken  Code = 101 // unexpected token
	ErrUnclosedDelim    Code = 102 // unclosed delimiter (paren/brace/bracket)
	ErrMissingBody      Code = 103 // missing expression body
	ErrInvalidOperator  Code = 104 // invalid operator in declaration
	ErrExpectedType     Code = 105 // expected type expression
	ErrInvalidPattern   Code = 106 // invalid pattern
	ErrImportSyntax     Code = 107 // import syntax error
	ErrClassSyntax      Code = 108 // class/instance syntax error
	ErrFixitySyntax     Code = 109 // fixity declaration error
	ErrParserLimit      Code = 110 // recursion/step limit in parser
)

// Type check errors (E0200–E0299)
const (
	ErrTypeMismatch        Code = 200 // type mismatch in unification
	ErrUnboundVar          Code = 201 // unbound variable
	ErrUnboundCon          Code = 202 // unknown constructor
	ErrNonExhaustive       Code = 203 // non-exhaustive pattern match
	ErrBadApplication      Code = 204 // application of non-function type
	ErrBadComputation      Code = 205 // expected computation type
	ErrBadThunk            Code = 206 // thunk/force type error
	ErrSpecialForm         Code = 207 // misuse of special form (pure/bind/thunk/force)
	ErrAssumption          Code = 208 // assumption declaration error
	ErrCyclicAlias         Code = 209 // cyclic type alias
	ErrDuplicateLabel      Code = 210 // duplicate label in row
	ErrRowMismatch         Code = 211 // row unification failure
	ErrOccursCheck         Code = 212 // infinite type (occurs check)
	ErrBadTypeApp          Code = 213 // type application error
	ErrEmptyDo             Code = 214 // empty do block
	ErrBadDoEnding         Code = 215 // do block doesn't end with expression
	ErrNoInstance          Code = 220 // no matching type class instance
	ErrOverlap             Code = 221 // overlapping instances
	ErrBadClass            Code = 222 // invalid class declaration
	ErrBadInstance         Code = 223 // invalid instance declaration
	ErrMissingMethod       Code = 224 // missing method in instance
	ErrRedundantPattern    Code = 225 // redundant pattern in case expression
	ErrImport              Code = 230 // module import error
	ErrSkolemEscape        Code = 240 // existential type variable escapes scope
	ErrSkolemRigid         Code = 241 // cannot unify rigid (skolem) type variable
	ErrKindMismatch        Code = 250 // kind mismatch in type application
	ErrResolutionDepth     Code = 260 // instance resolution depth limit exceeded
	ErrNestingLimit        Code = 261 // structural nesting depth limit exceeded
	ErrAliasExpansion      Code = 270 // alias expansion depth limit exceeded
	ErrDuplicateDecl       Code = 280 // duplicate type-level declaration
	ErrTypeFamilyEquation  Code = 281 // invalid type family equation
	ErrInjectivity         Code = 282 // type family injectivity violation
	ErrTypeFamilyReduction Code = 283 // type family reduction error
	ErrMultiplicity        Code = 290 // multiplicity constraint violation
	ErrEffectfulBinding    Code = 291 // bare Computation type in non-entry top-level binding
	ErrCancelled           Code = 299 // compilation cancelled (context deadline/cancellation)
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

// MaxErrors is the maximum number of errors collected before truncation.
const MaxErrors = 50

// Errors collects multiple diagnostics for a single source.
type Errors struct {
	Source   *span.Source
	Errs     []*Error
	Overflow int // count of errors dropped after MaxErrors
}

// Add appends an error to the collection.
func (es *Errors) Add(e *Error) {
	if len(es.Errs) >= MaxErrors {
		es.Overflow++
		return
	}
	es.Errs = append(es.Errs, e)
}

// Capped reports whether the error cap has been reached.
// When true, subsequent Add calls are silently dropped.
func (es *Errors) Capped() bool { return len(es.Errs) >= MaxErrors }

// Len returns the current number of collected errors.
func (es *Errors) Len() int { return len(es.Errs) }

// Truncate discards errors beyond position n, restoring the error list
// to a previously saved length. Used by speculative parsing to discard
// phantom errors from backtracked attempts.
func (es *Errors) Truncate(n int) {
	if n < len(es.Errs) {
		es.Errs = es.Errs[:n]
	}
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
	if es.Overflow > 0 {
		fmt.Fprintf(&b, "\n... and %d more errors (truncated)\n", es.Overflow)
	}
	return b.String()
}

func (es *Errors) formatOne(b *strings.Builder, e *Error) {
	fmt.Fprintf(b, "error[E%04d]: %s\n", e.Code, e.Message)
	if e.Span != (span.Span{}) {
		line, col := es.Source.Location(e.Span.Start)
		fmt.Fprintf(b, " --> %s:%d:%d\n", es.Source.Name, line, col)
		es.renderSpan(b, e.Span)
	}

	for _, h := range e.Hints {
		if h.Span != (span.Span{}) {
			hLine, hCol := es.Source.Location(h.Span.Start)
			fmt.Fprintf(b, " --> %s:%d:%d\n", es.Source.Name, hLine, hCol)
		}
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
