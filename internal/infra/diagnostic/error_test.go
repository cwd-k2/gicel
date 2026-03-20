package diagnostic

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

func TestErrorString(t *testing.T) {
	e := &Error{Code: 1, Phase: PhaseLex, Message: "unexpected character"}
	got := e.Error()
	want := "E0001: unexpected character"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPhaseString(t *testing.T) {
	tests := []struct {
		phase Phase
		want  string
	}{
		{PhaseLex, "lex"},
		{PhaseParse, "parse"},
		{PhaseCheck, "check"},
		{PhaseEval, "eval"},
		{Phase(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.phase.String(); got != tt.want {
			t.Errorf("Phase(%d).String() = %q, want %q", tt.phase, got, tt.want)
		}
	}
}

func TestErrorsEmpty(t *testing.T) {
	src := span.NewSource("test", "")
	es := &Errors{Source: src}
	if es.HasErrors() {
		t.Error("expected no errors")
	}
	if got := es.Format(); got != "" {
		t.Errorf("expected empty format, got %q", got)
	}
}

func TestFormatSingleError(t *testing.T) {
	// "let x = 42\n"
	src := span.NewSource("test.gicel", "let x = 42\n")
	es := &Errors{Source: src}
	es.Add(&Error{
		Code:    1,
		Phase:   PhaseParse,
		Span:    span.Span{Start: 0, End: 3}, // "let"
		Message: "unexpected keyword",
	})

	got := es.Format()

	// Check key fragments.
	assertContains(t, got, "error[E0001]")
	assertContains(t, got, "unexpected keyword")
	assertContains(t, got, "test.gicel:1:1")
	assertContains(t, got, "let x = 42")
	assertContains(t, got, "^^^") // underline for 3 bytes
}

func TestFormatWithHint(t *testing.T) {
	src := span.NewSource("test.gicel", "f x y\n")
	es := &Errors{Source: src}
	es.Add(&Error{
		Code:    2,
		Phase:   PhaseCheck,
		Span:    span.Span{Start: 2, End: 3}, // "x"
		Message: "type mismatch",
		Hints: []Hint{
			{Span: span.Span{Start: 0, End: 1}, Message: "expected Int"},
		},
	})

	got := es.Format()

	assertContains(t, got, "error[E0002]")
	assertContains(t, got, "type mismatch")
	assertContains(t, got, "test.gicel:1:3")
	assertContains(t, got, "hint: expected Int")
}

func TestFormatMultipleErrors(t *testing.T) {
	src := span.NewSource("test.gicel", "a b c\n")
	es := &Errors{Source: src}
	es.Add(&Error{Code: 1, Phase: PhaseLex, Span: span.Span{Start: 0, End: 1}, Message: "err one"})
	es.Add(&Error{Code: 2, Phase: PhaseLex, Span: span.Span{Start: 4, End: 5}, Message: "err two"})

	got := es.Format()

	assertContains(t, got, "E0001")
	assertContains(t, got, "err one")
	assertContains(t, got, "E0002")
	assertContains(t, got, "err two")
}

func TestErrorsOverflow(t *testing.T) {
	src := span.NewSource("test", "x")
	es := &Errors{Source: src}
	for i := range MaxErrors + 5 {
		es.Add(&Error{Code: 1, Phase: PhaseLex, Span: span.Span{}, Message: fmt.Sprintf("err %d", i)})
	}
	if len(es.Errs) != MaxErrors {
		t.Errorf("expected %d errors, got %d", MaxErrors, len(es.Errs))
	}
	if es.Overflow != 5 {
		t.Errorf("expected overflow=5, got %d", es.Overflow)
	}
	assertContains(t, es.Format(), "more errors")
}

func TestFormatZeroWidthSpan(t *testing.T) {
	src := span.NewSource("test", "hello\n")
	es := &Errors{Source: src}
	es.Add(&Error{Code: 1, Phase: PhaseLex, Span: span.Span{Start: 3, End: 3}, Message: "at point"})
	got := es.Format()
	assertContains(t, got, "^") // single caret for zero-width
}

func TestFormatMultiLineSpan(t *testing.T) {
	src := span.NewSource("test", "line1\nline2\n")
	es := &Errors{Source: src}
	// Error at start of "line2" (offset 6)
	es.Add(&Error{Code: 1, Phase: PhaseLex, Span: span.Span{Start: 6, End: 11}, Message: "on line 2"})
	got := es.Format()
	assertContains(t, got, "2:1")
	assertContains(t, got, "line2")
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("output missing %q:\n%s", substr, s)
	}
}
