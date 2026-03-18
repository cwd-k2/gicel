// Alias tests — cycle detection (direct, mutual), valid aliases.
// Does NOT cover: type family aliases (type_family_test.go).

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/errs"
)

func TestAliasCycleDirect(t *testing.T) {
	errMsg := checkSourceExpectCode(t, `type A := A`, nil, errs.ErrCyclicAlias)
	if !strings.Contains(errMsg, "A -> A") {
		t.Errorf("expected cycle path A -> A, got: %s", errMsg)
	}
}

func TestAliasCycleMutual(t *testing.T) {
	checkSourceExpectCode(t, "type A := B\ntype B := A", nil, errs.ErrCyclicAlias)
}

func TestAliasNoCycle(t *testing.T) {
	// Eff references Computation, which is a built-in — not an alias.
	source := `type Eff r a := Computation r r a
data Unit := Unit
main := pure Unit`
	checkSource(t, source, nil)
}
