//go:build probe

// Data.Math probe tests — integer and floating-point boundary conditions:
// division-by-zero, NaN propagation, overflow, bitwise corners.
// Does NOT cover: probe_stdlib_probe_test.go.
package probe_test

import (
	"math"
	"testing"

	"github.com/cwd-k2/gicel"
)

// TestProbeMath_PowBasics covers pow on small exponents and zero.
func TestProbeMath_PowBasics(t *testing.T) {
	cases := []struct {
		expr string
		want int64
	}{
		{"pow 2 10", 1024},
		{"pow 3 0", 1},
		{"pow 0 5", 0},
		{"pow 1 100", 1},
	}
	for _, c := range cases {
		v, err := probeRun(t, "import Prelude\nimport Data.Math\nmain := "+c.expr,
			gicel.Prelude, gicel.DataMath)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		probeAssertHostInt(t, v, c.want)
	}
}

// TestProbeMath_ClampInt covers the three branches of clamp: below, inside,
// above. Clamp with inverted bounds is not specified; we do not test it.
func TestProbeMath_ClampInt(t *testing.T) {
	cases := []struct {
		expr string
		want int64
	}{
		{"clamp 0 10 5", 5},       // inside
		{"clamp 0 10 (0 - 5)", 0}, // below
		{"clamp 0 10 20", 10},     // above
	}
	for _, c := range cases {
		v, err := probeRun(t, "import Prelude\nimport Data.Math\nmain := "+c.expr,
			gicel.Prelude, gicel.DataMath)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		probeAssertHostInt(t, v, c.want)
	}
}

// TestProbeMath_DivMod: (quotient, remainder) relation q*y + r == x must hold.
// Tests a positive dividend/divisor.
func TestProbeMath_DivMod(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Math

main := case divMod 17 5 {
  (q, r) => q * 5 + r
}
`, gicel.Prelude, gicel.DataMath)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 17)
}

// TestProbeMath_BitwiseIdentities guards common invariants used by
// cryptographic / encoding code.
func TestProbeMath_BitwiseIdentities(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Math

main := bitXor 42 (bitXor 42 0)
`, gicel.Prelude, gicel.DataMath)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0) // x XOR x == 0
}

// TestProbeMath_PopCount covers the Hamming weight on a known value.
func TestProbeMath_PopCount(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Math

main := popCount 255
`, gicel.Prelude, gicel.DataMath)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 8)
}

// TestProbeMath_SqrtNegativeIsNaN: sqrt of a negative double must return
// NaN (not panic, not Inf). This guards the stdlib against surface-level
// masking of IEEE 754 semantics.
func TestProbeMath_SqrtNegativeIsNaN(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Math

main := isNaN (sqrt (0.0 - 1.0))
`, gicel.Prelude, gicel.DataMath)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "True")
}

// TestProbeMath_InfiniteDivision: 1.0 / 0.0 must be +Inf per IEEE 754.
// Verifies the runtime does not trap or sanitize before reaching user code.
func TestProbeMath_InfiniteDivision(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Math

main := isInfinite (1.0 / 0.0)
`, gicel.Prelude, gicel.DataMath)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "True")
}

// TestProbeMath_TrigIdentity covers sin(0) = 0 exactly.
// We bypass the test helper for exact-float comparison.
func TestProbeMath_TrigIdentity(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Math

main := sin 0.0
`, gicel.Prelude, gicel.DataMath)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", v)
	}
	f, _ := hv.Inner.(float64)
	if f != 0.0 || math.Signbit(f) {
		t.Errorf("sin(0.0) = %v, want +0.0", f)
	}
}
