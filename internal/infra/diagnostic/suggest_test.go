package diagnostic

import (
	"testing"
)

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "b", 1},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "abcd", 1},
		{"abc", "ab", 1},
		{"kitten", "sitting", 3},
		{"map", "mpa", 2},
		{"True", "tru", 2},
		{"Just", "Juxt", 1},
	}
	for _, c := range cases {
		got := levenshtein(c.a, c.b)
		if got != c.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSuggest(t *testing.T) {
	candidates := []string{"map", "filter", "foldl", "foldr", "flip", "fmap", "max", "min"}

	tests := []struct {
		target    string
		threshold int
		want      []string
	}{
		{"mpa", 2, []string{"map", "max", "min"}},
		{"map", 2, []string{"fmap", "max", "min"}},
		{"fild", 2, []string{"foldl", "foldr"}},
		{"xyz", 2, nil},
		{"filtr", 2, []string{"filter", "foldr"}},
	}
	for _, tt := range tests {
		got := Suggest(tt.target, candidates, tt.threshold, 3)
		if len(got) != len(tt.want) {
			t.Errorf("Suggest(%q) = %v, want %v", tt.target, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("Suggest(%q)[%d] = %q, want %q", tt.target, i, got[i], tt.want[i])
			}
		}
	}
}
