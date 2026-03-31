//go:build probe

// Do-block pattern bind probe tests — tuple/record/wildcard/nested patterns in do and block binds.
// Does NOT cover: monadic state threading (bidir_case_probe_test.go), exhaustiveness (exhaust_test.go).

package check

import (
	"testing"
)

// TestProbeF_PatternBindTupleInDo — monadic bind with tuple pattern.
func TestProbeF_PatternBindTupleInDo(t *testing.T) {
	source := `main := do { (a, b) <- pure (1, 2); pure a }`
	checkSource(t, source, nil)
}

// TestProbeF_PatternPureBindTupleInDo — pure bind with tuple pattern inside do.
func TestProbeF_PatternPureBindTupleInDo(t *testing.T) {
	source := `main := do { (x, y) := (3, 4); pure x }`
	checkSource(t, source, nil)
}

// TestProbeF_PatternBindInBlock — tuple pattern bind in block syntax.
func TestProbeF_PatternBindInBlock(t *testing.T) {
	source := `main := { (a, b) := (1, 2); a }`
	checkSource(t, source, nil)
}

// TestProbeF_PatternBindNestedTuple — nested tuple pattern in block.
func TestProbeF_PatternBindNestedTuple(t *testing.T) {
	source := `main := { ((a, b), c) := ((1, 2), 3); a }`
	checkSource(t, source, nil)
}

// TestProbeF_PatternBindWildcard — wildcard in tuple pattern bind.
func TestProbeF_PatternBindWildcard(t *testing.T) {
	source := `main := { (_, b) := (1, 2); b }`
	checkSource(t, source, nil)
}

// TestProbeF_PatternBindTriple — triple tuple pattern bind in do.
func TestProbeF_PatternBindTriple(t *testing.T) {
	source := `main := do { (a, b, c) <- pure (1, 2, 3); pure b }`
	checkSource(t, source, nil)
}

// TestProbeF_PatternBindWithIfThenElse — tuple bind followed by if-then-else.
func TestProbeF_PatternBindWithIfThenElse(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
main := { (a, b) := (1, 2);
  if True
     then a
     else b
}`
	checkSource(t, source, nil)
}
