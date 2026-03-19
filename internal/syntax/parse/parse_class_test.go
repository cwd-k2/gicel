// Class declaration tests — invalid head recovery.
// Does NOT cover: valid class declarations (parse_test.go, declaration_probe_test.go).
package parse

import (
	"strings"
	"testing"
)

// TestParseClassInvalidHead verifies that a non-variable class type parameter
// (e.g. a type constructor application) produces a parse error rather than
// panicking on an unchecked type assertion.
func TestParseClassInvalidHead(t *testing.T) {
	src := `class Foo (Maybe a) { m :: Int }`
	_, es := parse(src)
	if !es.HasErrors() {
		t.Fatal("expected parse error for non-variable class type parameter, got none")
	}
	formatted := es.Format()
	if !strings.Contains(formatted, "type variable") && !strings.Contains(formatted, "expected") {
		t.Errorf("error message should mention type variable or expected, got: %s", formatted)
	}
}
