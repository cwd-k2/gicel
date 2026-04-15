package span

// Pos is a byte offset into the source.
type Pos int

// Span is a half-open byte range [Start, End) in the source.
// Value type: embeds cheaply in AST, Core, and Type nodes.
// The zero value represents "no source location."
type Span struct {
	Start Pos
	End   Pos
}

// IsZero reports whether the span is the zero value (no source location).
func (s Span) IsZero() bool { return s == Span{} }

// Source maps byte offsets to line/column for diagnostics.
// Shared (one per parse unit), referenced by pointer.
type Source struct {
	Name  string // file name or "<input>"
	Text  string // full source text
	Lines []Pos  // byte offset of each line start
}

// NewSource builds the line offset table from the source text.
func NewSource(name, text string) *Source {
	lines := []Pos{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			lines = append(lines, Pos(i+1))
		}
	}
	return &Source{Name: name, Text: text, Lines: lines}
}

// Location returns the 1-based line and column for a byte position.
func (s *Source) Location(p Pos) (line, col int) {
	lo, hi := 0, len(s.Lines)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if s.Lines[mid] <= p {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	// lo-1 is the index of the line containing p.
	line = lo // 1-based
	col = int(p-s.Lines[lo-1]) + 1
	return
}

// Excerpt returns the source text for a Span.
func (s *Source) Excerpt(sp Span) string {
	start, end := int(sp.Start), int(sp.End)
	if start < 0 {
		start = 0
	}
	if end > len(s.Text) {
		end = len(s.Text)
	}
	if start >= end {
		return ""
	}
	return s.Text[start:end]
}
