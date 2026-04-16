//go:build probe

// Data.Stream probe tests — boundary conditions for lazy streams:
// infinite iteration under take, negative arguments, and filter+iterate composition.
// Does NOT cover: probe_stdlib_collection_probe_test.go, probe_stdlib_probe_test.go.
package probe_test

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// TestProbeStream_TakeFromInfinite verifies that `take n (iterate f x)` is
// productive under the engine's default budget — without laziness, this
// would hang or exhaust the step budget.
func TestProbeStream_TakeFromInfinite(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Stream as S

main := S.take 5 (S.iterate (\x. x + 1) 0)
`, gicel.Prelude, gicel.DataStream)
	if err != nil {
		t.Fatal(err)
	}
	// [0, 1, 2, 3, 4] — verify structural shape via PrettyValue.
	got := gicel.PrettyValue(v)
	for _, n := range []string{"0", "1", "2", "3", "4"} {
		if !strings.Contains(got, n) {
			t.Errorf("expected %q in result, got %q", n, got)
		}
	}
}

// TestProbeStream_TakeZero: n=0 must yield Nil, not crash.
func TestProbeStream_TakeZero(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Stream as S

main := S.take 0 (S.iterate (\x. x + 1) 0)
`, gicel.Prelude, gicel.DataStream)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nil")
}

// TestProbeStream_TakeNegative: n<0 is undefined by spec, but must not
// panic or hang. We accept either Nil or any non-panicking result.
//
// Note: GICEL parses (-3) as an operator section, not a negative literal,
// so we produce the negative value via subtraction.
func TestProbeStream_TakeNegative(t *testing.T) {
	_, err := probeRun(t, `
import Prelude
import Data.Stream as S

main := S.take (0 - 3) (S.iterate (\x. x + 1) 0)
`, gicel.Prelude, gicel.DataStream)
	if err != nil {
		t.Fatalf("negative take must not error, got %v", err)
	}
}

// TestProbeStream_FilterProductive: filter on infinite stream followed by
// take exercises the demand-driven evaluator — no prefix may be forced
// eagerly. If filter were strict this would diverge.
func TestProbeStream_FilterProductive(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Stream as S

main := S.take 3 (S.filter (\x. mod x 2 == 0) (S.iterate (\x. x + 1) 0))
`, gicel.Prelude, gicel.DataStream)
	if err != nil {
		t.Fatal(err)
	}
	got := gicel.PrettyValue(v)
	// First three evens: 0, 2, 4.
	for _, n := range []string{"0", "2", "4"} {
		if !strings.Contains(got, n) {
			t.Errorf("expected even %q in result, got %q", n, got)
		}
	}
}

// TestProbeStream_DropThenTake: drop must also be lazy; dropping from an
// infinite stream then taking must terminate.
func TestProbeStream_DropThenTake(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Stream as S

main := S.take 2 (S.drop 10 (S.iterate (\x. x + 1) 0))
`, gicel.Prelude, gicel.DataStream)
	if err != nil {
		t.Fatal(err)
	}
	got := gicel.PrettyValue(v)
	// [10, 11]
	if !strings.Contains(got, "10") || !strings.Contains(got, "11") {
		t.Errorf("expected 10 and 11, got %q", got)
	}
}

// TestProbeStream_HeadEmpty: head of LNil must return Nothing, not crash.
// Uses open import (not qualified) so the LNil constructor is in scope
// without a module prefix; type-level introspection resolves Stream Int
// from the surrounding context.
func TestProbeStream_HeadEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Stream

main := head LNil
`, gicel.Prelude, gicel.DataStream)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nothing")
}
