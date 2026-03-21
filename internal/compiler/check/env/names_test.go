// Name classification tests — IsPrivateName, IsOperatorName.
// Does NOT cover: usage in import/export logic (import_collision_test.go, export_test.go).

package env

import "testing"

func TestIsPrivateName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// Private: underscore prefix.
		{"_hidden", true},
		{"_", true},
		{"_X", true},

		// Private: compiler-generated ($-containing identifier).
		{"$d", true},
		{"$dict", true},
		{"x$y", true},
		{"Eq$Dict", true},

		// Not private: regular names.
		{"foo", false},
		{"Foo", false},
		{"x", false},
		{"FooBar123", false},

		// Not private: operators (even if containing $).
		{"<$>", false},
		{"$", false},
		{"+>", false},
		{">>", false},
		{"<*>", false},

		// Not private: empty string.
		{"", false},
	}
	for _, tt := range tests {
		if got := IsPrivateName(tt.name); got != tt.want {
			t.Errorf("IsPrivateName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsOperatorName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// Operators: all symbol characters.
		{"+", true},
		{"<$>", true},
		{">>", true},
		{"<*>", true},
		{"$", true},
		{"+>", true},
		{"=:=", true},

		// Not operators: contain alphanumeric or underscore.
		{"foo", false},
		{"a+b", false},
		{"_x", false},
		{"X", false},
		{"a1", false},

		// Edge case: empty string (vacuously true).
		{"", true},
	}
	for _, tt := range tests {
		if got := IsOperatorName(tt.name); got != tt.want {
			t.Errorf("IsOperatorName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
