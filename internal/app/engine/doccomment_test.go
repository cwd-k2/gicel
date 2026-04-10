package engine

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

func TestExtractDocComment_Basic(t *testing.T) {
	source := "-- Hello world.\nfoo := 42\n"
	doc := ExtractDocComment(source, span.Pos(16)) // 'f' of foo
	if doc != "Hello world." {
		t.Fatalf("expected 'Hello world.', got %q", doc)
	}
}

func TestExtractDocComment_MultiLine(t *testing.T) {
	source := "-- Line one.\n-- Line two.\nfoo := 42\n"
	doc := ExtractDocComment(source, span.Pos(26)) // 'f' of foo
	want := "Line one.\nLine two."
	if doc != want {
		t.Fatalf("expected %q, got %q", want, doc)
	}
}

func TestExtractDocComment_EmptyLineSeparator(t *testing.T) {
	source := "-- Unrelated.\n\n-- Doc for foo.\nfoo := 42\n"
	doc := ExtractDocComment(source, span.Pos(31)) // 'f' of foo
	if doc != "Doc for foo." {
		t.Fatalf("expected 'Doc for foo.', got %q", doc)
	}
}

func TestExtractDocComment_NoComment(t *testing.T) {
	source := "foo := 42\n"
	doc := ExtractDocComment(source, span.Pos(0))
	if doc != "" {
		t.Fatalf("expected empty, got %q", doc)
	}
}

func TestExtractDocComment_FirstLine(t *testing.T) {
	source := "-- Doc.\nfoo := 42\n"
	doc := ExtractDocComment(source, span.Pos(8))
	if doc != "Doc." {
		t.Fatalf("expected 'Doc.', got %q", doc)
	}
}

func TestExtractDocComment_DashDashNoSpace(t *testing.T) {
	source := "--No space prefix.\nfoo := 42\n"
	doc := ExtractDocComment(source, span.Pos(19))
	if doc != "No space prefix." {
		t.Fatalf("expected 'No space prefix.', got %q", doc)
	}
}
