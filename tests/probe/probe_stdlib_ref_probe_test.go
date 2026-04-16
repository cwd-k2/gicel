//go:build probe

// Effect.Ref probe tests — mutable reference semantics: read-after-write
// consistency, modify composability, named-capability variants.
package probe_test

import (
	"testing"

	"github.com/cwd-k2/gicel"
)

// TestProbeRef_ReadAfterWrite: write then read must observe the fresh value.
// This is the most basic correctness property; a broken capability-threading
// implementation would break here first.
func TestProbeRef_ReadAfterWrite(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Effect.Ref as R

main := do {
  r <- R.new 10;
  R.write 42 r;
  R.read r
}
`, gicel.Prelude, gicel.EffectRef)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 42)
}

// TestProbeRef_Modify: modify = read, apply f, write. The semantics must
// compose — verifying by applying modify twice with a known function.
func TestProbeRef_Modify(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Effect.Ref as R

main := do {
  r <- R.new 0;
  R.modify (\x. x + 1) r;
  R.modify (\x. x * 10) r;
  R.read r
}
`, gicel.Prelude, gicel.EffectRef)
	if err != nil {
		t.Fatal(err)
	}
	// ((0 + 1) * 10) = 10
	probeAssertHostInt(t, v, 10)
}

// TestProbeRef_Independence: two separate refs must not alias. Writing to
// one must not affect the other's observed value.
func TestProbeRef_Independence(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Effect.Ref as R

main := do {
  a <- R.new 1;
  b <- R.new 2;
  R.write 100 a;
  va <- R.read a;
  vb <- R.read b;
  pure (va + vb)
}
`, gicel.Prelude, gicel.EffectRef)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 102)
}

// TestProbeRef_StringValue: non-trivial payloads (here a string) must
// survive read/write without coercion.
func TestProbeRef_StringValue(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Effect.Ref as R

main := do {
  r <- R.new "hello";
  R.write "world" r;
  R.read r
}
`, gicel.Prelude, gicel.EffectRef)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "world")
}
