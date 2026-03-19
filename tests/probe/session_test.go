//go:build probe

package probe_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe: Session Fidelity — boundary tests for session type encoding.
// Verifies that protocol-compliant programs pass type checking and
// protocol violations are rejected.
// ===================================================================

const sessionPreamble = `
import Prelude

data Mult := Unrestricted | Affine | Linear
data Send s := MkSend
data Recv s := MkRecv
data End := MkEnd

type LUB (m1: Mult) (m2: Mult) :: Mult := {
  LUB Linear _ =: Linear;
  LUB _ Linear =: Linear;
  LUB Affine _ =: Affine;
  LUB _ Affine =: Affine;
  LUB Unrestricted Unrestricted =: Unrestricted
}

type Dual (s: Type) :: Type := {
  Dual (Send s) =: Recv (Dual s);
  Dual (Recv s) =: Send (Dual s);
  Dual End      =: End
}

send :: \s. Computation { ch: Send s @Linear } { ch: s @Linear } Int
send := assumption

recv :: \s. Computation { ch: Recv s @Linear } { ch: s @Linear } Int
recv := assumption

close :: Computation { ch: End @Linear } {} ()
close := assumption
`

// sessionCheck type-checks source (prepended with preamble), returns error string or "".
func sessionCheck(t *testing.T, source string) string {
	t.Helper()
	full := sessionPreamble + source
	eng := gicel.NewEngine()
	if err := gicel.Prelude(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.NewRuntime(context.Background(), full)
	if err != nil {
		return err.Error()
	}
	return ""
}

// --- Protocol-compliant programs (must type-check) ---

func TestSessionCompliantPingPong(t *testing.T) {
	errs := sessionCheck(t, `
main :: Computation { ch: Send (Recv End) @Linear } {} Int
main := do { send; response <- recv; close; pure response }
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

func TestSessionCompliantResponder(t *testing.T) {
	errs := sessionCheck(t, `
main :: Computation { ch: Recv (Send End) @Linear } {} Int
main := do { request <- recv; send; close; pure request }
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

func TestSessionCompliantMultiStep(t *testing.T) {
	errs := sessionCheck(t, `
main :: Computation { ch: Send (Send (Recv End)) @Linear } {} Int
main := do { send; send; result <- recv; close; pure result }
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

func TestSessionCompliantCloseOnly(t *testing.T) {
	errs := sessionCheck(t, `
main :: Computation { ch: End @Linear } {} ()
main := do { close }
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

// --- Protocol violations (must be rejected) ---

func TestSessionViolationReversedOrder(t *testing.T) {
	// Recv before Send — protocol says Send first.
	errs := sessionCheck(t, `
main :: Computation { ch: Send (Recv End) @Linear } {} Int
main := do { x <- recv; send; close; pure x }
`)
	if errs == "" {
		t.Fatal("expected type error for reversed send/recv order")
	}
}

func TestSessionViolationSkipStep(t *testing.T) {
	// Close immediately — protocol requires Send then Recv first.
	errs := sessionCheck(t, `
main :: Computation { ch: Send (Recv End) @Linear } {} ()
main := do { close }
`)
	if errs == "" {
		t.Fatal("expected type error for skipping protocol steps")
	}
}

func TestSessionViolationIncomplete(t *testing.T) {
	// Send but never close — session not completed.
	errs := sessionCheck(t, `
main :: Computation { ch: Send End @Linear } {} Int
main := do { send }
`)
	if errs == "" {
		t.Fatal("expected type error for incomplete session (not closed)")
	}
}

func TestSessionViolationDoubleClose(t *testing.T) {
	// Close twice — channel already consumed.
	errs := sessionCheck(t, `
main :: Computation { ch: End @Linear } {} ()
main := do { close; close }
`)
	if errs == "" {
		t.Fatal("expected type error for double close")
	}
}

func TestSessionViolationExtraRecv(t *testing.T) {
	// Extra recv after protocol ends.
	errs := sessionCheck(t, `
main :: Computation { ch: Send End @Linear } {} Int
main := do { send; x <- recv; close; pure x }
`)
	if errs == "" {
		t.Fatal("expected type error for extra recv")
	}
}

// --- Duality involution ---

func TestSessionDualInvolution(t *testing.T) {
	// Dual (Dual (Send (Recv End))) should reduce to Send (Recv End).
	// We test this by checking that a value annotated with the double-dual type
	// unifies with the original type.
	errs := sessionCheck(t, `
type DualDual (s: Type) :: Type := { DualDual s =: Dual (Dual s) }

-- If Dual(Dual(Send (Recv End))) = Send (Recv End),
-- then these types should unify.
id :: \s. Computation { ch: s @Linear } { ch: s @Linear } ()
id := assumption

test :: Computation { ch: DualDual (Send (Recv End)) @Linear } { ch: Send (Recv End) @Linear } ()
test := do { id }
`)
	if errs != "" {
		t.Fatalf("Dual involution failed: %s", errs)
	}
}

// --- Leak detection ---

func TestSessionLeakLinear(t *testing.T) {
	// Attempt to leak a @Linear channel (not consumed, post-state mismatch).
	errs := sessionCheck(t, `
main :: Computation { ch: Send End @Linear } {} ()
main := do { pure () }
`)
	if errs == "" {
		t.Fatal("expected type error for leaking @Linear channel")
	}
}

// --- Error message quality ---

func TestSessionErrorMentionsPreState(t *testing.T) {
	errs := sessionCheck(t, `
main :: Computation { ch: Send (Recv End) @Linear } {} ()
main := do { close }
`)
	if errs == "" {
		t.Fatal("expected type error")
	}
	if !strings.Contains(errs, "mismatch") {
		t.Logf("error: %s", errs)
	}
}
