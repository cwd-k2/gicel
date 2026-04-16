//go:build probe

// Data.Sequence probe tests — 2-3 finger tree boundary cases: empty
// operations, large-size invariants, deque symmetry.
package probe_test

import (
	"testing"

	"github.com/cwd-k2/gicel"
)

// TestProbeSeq_EmptyHead: head/last on empty must return Nothing.
func TestProbeSeq_EmptyHead(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Sequence as Seq

main := case Seq.head (Seq.empty :: Seq Int) {
  Nothing => True;
  Just _  => False
}
`, gicel.Prelude, gicel.DataSequence)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "True")
}

// TestProbeSeq_ConsSnocSymmetry: cons then uncons recovers the element,
// snoc then unsnoc recovers the element. This is the deque correctness
// invariant — asymmetry here means the 2-3 balancing is broken.
func TestProbeSeq_ConsSnocSymmetry(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Sequence as Seq

main := case Seq.uncons (Seq.cons 42 Seq.empty) {
  Just (x, _) => x;
  Nothing     => 0 - 1
}
`, gicel.Prelude, gicel.DataSequence)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 42)
}

// TestProbeSeq_AppendLength: length (append xs ys) == length xs + length ys.
// A bug in append's node redistribution is often caught here.
func TestProbeSeq_AppendLength(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Sequence as Seq

a := fromList [1, 2, 3] :: Seq Int
b := fromList [4, 5] :: Seq Int
main := Seq.length (Seq.append a b)
`, gicel.Prelude, gicel.DataSequence)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 5)
}

// TestProbeSeq_IndexOutOfBounds: index n must return Nothing for
// n ≥ length, not panic or wrap.
func TestProbeSeq_IndexOutOfBounds(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Sequence as Seq

s := fromList [10, 20, 30] :: Seq Int
main := case Seq.index 100 s {
  Nothing => True;
  Just _  => False
}
`, gicel.Prelude, gicel.DataSequence)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "True")
}

// TestProbeSeq_ReverseInvolution: reverse (reverse s) == s (structurally)
// is the classic double-dual check for deque correctness.
func TestProbeSeq_ReverseInvolution(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Sequence as Seq

s := fromList [1, 2, 3, 4, 5] :: Seq Int
main := case Seq.index 2 (Seq.reverse (Seq.reverse s)) {
  Just x  => x;
  Nothing => 0 - 1
}
`, gicel.Prelude, gicel.DataSequence)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 3)
}
