//go:build probe

// Type class method dispatch edge-case tests.
package probe_test

import (
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe D: Type class method dispatch
// ===================================================================

func TestProbeD_TypeClass_ShowInt(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := show 42
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertString(t, v, "42")
}

func TestProbeD_TypeClass_EqBool(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := True == False
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "False")
}

func TestProbeD_TypeClass_OrdInt(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := compare 1 2
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "LT")
}

func TestProbeD_TypeClass_FmapList(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := fmap (+ 1) (Cons 1 (Cons 2 (Cons 3 Nil)))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be [2, 3, 4]
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	pdAssertInt(t, con.Args[0], 2)
}
