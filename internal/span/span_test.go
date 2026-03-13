package span

import "testing"

func TestNewSourceLineOffsets(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		lines []Pos
	}{
		{"empty", "", []Pos{0}},
		{"no newline", "hello", []Pos{0}},
		{"one line", "hello\n", []Pos{0, 6}},
		{"two lines", "hello\nworld\n", []Pos{0, 6, 12}},
		{"trailing no newline", "hello\nworld", []Pos{0, 6}},
		{"empty lines", "\n\n\n", []Pos{0, 1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := NewSource("test", tt.text)
			if len(src.Lines) != len(tt.lines) {
				t.Fatalf("Lines count: got %d, want %d", len(src.Lines), len(tt.lines))
			}
			for i, got := range src.Lines {
				if got != tt.lines[i] {
					t.Errorf("Lines[%d]: got %d, want %d", i, got, tt.lines[i])
				}
			}
		})
	}
}

func TestLocation(t *testing.T) {
	// "abc\nde\nfghij\n"
	//  0123 456 789...
	src := NewSource("test", "abc\nde\nfghij\n")

	tests := []struct {
		pos      Pos
		wantLine int
		wantCol  int
	}{
		{0, 1, 1},  // 'a'
		{2, 1, 3},  // 'c'
		{3, 1, 4},  // '\n'
		{4, 2, 1},  // 'd'
		{6, 2, 3},  // '\n'
		{7, 3, 1},  // 'f'
		{11, 3, 5}, // 'j'
		{12, 3, 6}, // '\n'
	}
	for _, tt := range tests {
		line, col := src.Location(tt.pos)
		if line != tt.wantLine || col != tt.wantCol {
			t.Errorf("Location(%d): got (%d,%d), want (%d,%d)",
				tt.pos, line, col, tt.wantLine, tt.wantCol)
		}
	}
}

func TestLocationSingleLine(t *testing.T) {
	src := NewSource("test", "hello")
	line, col := src.Location(0)
	if line != 1 || col != 1 {
		t.Errorf("got (%d,%d), want (1,1)", line, col)
	}
	line, col = src.Location(4)
	if line != 1 || col != 5 {
		t.Errorf("got (%d,%d), want (1,5)", line, col)
	}
}

func TestLocationUnicode(t *testing.T) {
	// "αβ\nγ" — α=2 bytes, β=2 bytes, \n=1, γ=2
	src := NewSource("test", "αβ\nγ")
	line, col := src.Location(0)
	if line != 1 || col != 1 {
		t.Errorf("α: got (%d,%d), want (1,1)", line, col)
	}
	// γ starts at byte offset 5
	line, col = src.Location(5)
	if line != 2 || col != 1 {
		t.Errorf("γ: got (%d,%d), want (2,1)", line, col)
	}
}

func TestExcerpt(t *testing.T) {
	src := NewSource("test", "hello world")

	tests := []struct {
		name string
		span Span
		want string
	}{
		{"full", Span{0, 11}, "hello world"},
		{"partial", Span{6, 11}, "world"},
		{"empty span", Span{3, 3}, ""},
		{"clamp start", Span{-1, 5}, "hello"},
		{"clamp end", Span{6, 100}, "world"},
		{"both clamp", Span{-1, 100}, "hello world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := src.Excerpt(tt.span)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
