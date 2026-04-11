//go:build probe

// Session probe tests — session type encoding boundary tests, protocol fidelity.
// Does NOT cover: probe_effect_probe_test.go, probe_typesystem_probe_test.go.
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

form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
form Send := \s. { MkSend: Send s; }
form Recv := \s. { MkRecv: Recv s; }
form End := { MkEnd: End; }
form Offer := \(choices: Row). { MkOffer: Offer choices; }
form Choose := \(choices: Row). { MkChoose: Choose choices; }

type LUB :: Mult := \(m1: Mult) (m2: Mult). case (m1, m2) {
  (Linear, _) => Linear;
  (_, Linear) => Linear;
  (Affine, _) => Affine;
  (_, Affine) => Affine;
  (Unrestricted, Unrestricted) => Unrestricted
}

type DualRow :: Row := \(r: Row). MapRow Dual r

type Dual :: Type := \(s: Type). case s {
  Send s   => Recv (Dual s);
  Recv s   => Send (Dual s);
  Offer r  => Choose (DualRow r);
  Choose r => Offer (DualRow r);
  End      => End
}

send :: \s. Computation { ch: Send s @Linear } { ch: s @Linear } Int
send := assumption

recv :: \s. Computation { ch: Recv s @Linear } { ch: s @Linear } Int
recv := assumption

close :: Computation { ch: End @Linear } {} ()
close := assumption

chooseTag :: \(tag: Label) (choices: Row).
  Computation { ch: Choose choices @Linear } { ch: Lookup tag choices @Linear } ()
chooseTag := assumption
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

func TestProbeS_CompliantPingPong(t *testing.T) {
	errs := sessionCheck(t, `
main :: Computation { ch: Send (Recv End) @Linear } {} Int
main := do { send; response <- recv; close; pure response }
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

func TestProbeS_CompliantResponder(t *testing.T) {
	errs := sessionCheck(t, `
main :: Computation { ch: Recv (Send End) @Linear } {} Int
main := do { request <- recv; send; close; pure request }
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

func TestProbeS_CompliantMultiStep(t *testing.T) {
	errs := sessionCheck(t, `
main :: Computation { ch: Send (Send (Recv End)) @Linear } {} Int
main := do { send; send; result <- recv; close; pure result }
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

func TestProbeS_CompliantCloseOnly(t *testing.T) {
	errs := sessionCheck(t, `
main :: Computation { ch: End @Linear } {} ()
main := do { close }
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

// --- Protocol violations (must be rejected) ---

func TestProbeS_ViolationReversedOrder(t *testing.T) {
	// Recv before Send — protocol says Send first.
	errs := sessionCheck(t, `
main :: Computation { ch: Send (Recv End) @Linear } {} Int
main := do { x <- recv; send; close; pure x }
`)
	if errs == "" {
		t.Fatal("expected type error for reversed send/recv order")
	}
}

func TestProbeS_ViolationSkipStep(t *testing.T) {
	// Close immediately — protocol requires Send then Recv first.
	errs := sessionCheck(t, `
main :: Computation { ch: Send (Recv End) @Linear } {} ()
main := do { close }
`)
	if errs == "" {
		t.Fatal("expected type error for skipping protocol steps")
	}
}

func TestProbeS_ViolationIncomplete(t *testing.T) {
	// Send but never close — session not completed.
	errs := sessionCheck(t, `
main :: Computation { ch: Send End @Linear } {} Int
main := do { send }
`)
	if errs == "" {
		t.Fatal("expected type error for incomplete session (not closed)")
	}
}

func TestProbeS_ViolationDoubleClose(t *testing.T) {
	// Close twice — channel already consumed.
	errs := sessionCheck(t, `
main :: Computation { ch: End @Linear } {} ()
main := do { close; close }
`)
	if errs == "" {
		t.Fatal("expected type error for double close")
	}
}

func TestProbeS_ViolationExtraRecv(t *testing.T) {
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

func TestProbeS_DualInvolution(t *testing.T) {
	// Dual (Dual (Send (Recv End))) should reduce to Send (Recv End).
	// We test this by checking that a value annotated with the double-dual type
	// unifies with the original type.
	errs := sessionCheck(t, `
type DualDual :: Type := \(s: Type). case s { s => Dual (Dual s) }

-- If Dual(Dual(Send (Recv End))) = Send (Recv End),
-- then these types should unify.
id :: \s. Computation { ch: s @Linear } { ch: s @Linear } ()
id := assumption

main :: Computation { ch: DualDual (Send (Recv End)) @Linear } { ch: Send (Recv End) @Linear } ()
main := do { id }
`)
	if errs != "" {
		t.Fatalf("Dual involution failed: %s", errs)
	}
}

// --- Leak detection ---

func TestProbeS_LeakLinear(t *testing.T) {
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

func TestProbeS_ErrorMentionsPreState(t *testing.T) {
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

// ===================================================================
// Probe: Choose (internal choice) — type safety for branch selection.
// ===================================================================

func TestProbeS_ChooseBalance(t *testing.T) {
	// Choose "balance" branch, then recv, then close.
	errs := sessionCheck(t, `
type ATM := Choose { balance: Recv End, deposit: Send (Recv End), quit: End }

main :: Computation { ch: ATM @Linear } {} Int
main := do {
  chooseTag @#balance;
  n <- recv;
  close;
  pure n
}
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

func TestProbeS_ChooseDeposit(t *testing.T) {
	// Choose "deposit" branch, then send, recv, close.
	errs := sessionCheck(t, `
type ATM := Choose { balance: Recv End, deposit: Send (Recv End), quit: End }

main :: Computation { ch: ATM @Linear } {} Int
main := do {
  chooseTag @#deposit;
  send;
  newBal <- recv;
  close;
  pure newBal
}
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

func TestProbeS_ChooseQuit(t *testing.T) {
	// Choose "quit" branch, then close.
	errs := sessionCheck(t, `
type ATM := Choose { balance: Recv End, deposit: Send (Recv End), quit: End }

main :: Computation { ch: ATM @Linear } {} ()
main := do {
  chooseTag @#quit;
  close
}
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

func TestProbeS_ChooseWrongOp(t *testing.T) {
	// Choose "balance" (Recv End) but then try to send — must fail.
	errs := sessionCheck(t, `
type ATM := Choose { balance: Recv End, quit: End }

main :: Computation { ch: ATM @Linear } {} Int
main := do {
  chooseTag @#balance;
  send;
  close;
  pure 0
}
`)
	if errs == "" {
		t.Fatal("expected type error for wrong operation after choose")
	}
}

func TestProbeS_ChooseNonexistentLabel(t *testing.T) {
	// Choose a label not present in the protocol — must fail.
	errs := sessionCheck(t, `
type ATM := Choose { balance: Recv End, quit: End }

main :: Computation { ch: ATM @Linear } {} ()
main := do {
  chooseTag @#withdraw;
  close
}
`)
	if errs == "" {
		t.Fatal("expected type error for nonexistent label")
	}
	if !strings.Contains(errs, "Lookup") {
		t.Logf("error does not mention Lookup: %s", errs)
	}
}

func TestProbeS_ChooseLinearLeak(t *testing.T) {
	// Choose a branch but never close — linear capability leak.
	errs := sessionCheck(t, `
type ATM := Choose { balance: Recv End, quit: End }

main :: Computation { ch: ATM @Linear } {} ()
main := do {
  chooseTag @#quit;
  pure ()
}
`)
	if errs == "" {
		t.Fatal("expected type error for linear capability leak after choose")
	}
}

func TestProbeS_DualChooseOffer(t *testing.T) {
	// Dual (Choose { ping: Recv End, quit: End })
	//   = Offer (MapRow Dual { ping: Recv End, quit: End })
	//   = Offer { ping: Send End, quit: End }
	errs := sessionCheck(t, `
type DualRow :: Row := \(r: Row). MapRow Dual r

type Proto := Choose { ping: Recv End, quit: End }

id :: \s. Computation { ch: s @Linear } { ch: s @Linear } ()
id := assumption

main :: Computation { ch: Dual Proto @Linear } { ch: Offer { ping: Send End, quit: End } @Linear } ()
main := do { id }
`)
	if errs != "" {
		t.Fatalf("Dual Choose→Offer (deep) failed: %s", errs)
	}
}

// ===================================================================
// Probe: Variant + receiveAt — external choice via case elimination.
// ===================================================================

func TestProbeS_VariantReceiveSingleBranch(t *testing.T) {
	errs := sessionCheck(t, `
receiveAt :: \(l: Label) (choices: Row) (s: Type) r. Computation { l: Offer choices | r } { l: s | r } (Variant choices s)
receiveAt := assumption

closeNoGrade :: Computation { ch: End } {} ()
closeNoGrade := assumption

main :: Computation { ch: Offer { a: End } } {} ()
main := do {
  tag <- receiveAt @#ch;
  case tag { #a => do { closeNoGrade } }
}
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

func TestProbeS_VariantReceiveMultiBranch(t *testing.T) {
	errs := sessionCheck(t, `
receiveAt :: \(l: Label) (choices: Row) (s: Type) r. Computation { l: Offer choices | r } { l: s | r } (Variant choices s)
receiveAt := assumption

sendNG :: \s. Computation { ch: Send s } { ch: s } Int
sendNG := assumption

closeNG :: Computation { ch: End } {} ()
closeNG := assumption

type Proto := Offer { ping: Send End, quit: End }

main :: Computation { ch: Proto } {} Int
main := do {
  tag <- receiveAt @#ch;
  case tag {
    #ping => do { sendNG; closeNG; pure 42 };
    #quit => do { closeNG; pure 0 }
  }
}
`)
	if errs != "" {
		t.Fatalf("expected no error, got: %s", errs)
	}
}

func TestProbeS_VariantWrongOpInBranch(t *testing.T) {
	errs := sessionCheck(t, `
receiveAt :: \(l: Label) (choices: Row) (s: Type) r. Computation { l: Offer choices | r } { l: s | r } (Variant choices s)
receiveAt := assumption

sendNG :: \s. Computation { ch: Send s } { ch: s } Int
sendNG := assumption

recvNG :: \s. Computation { ch: Recv s } { ch: s } Int
recvNG := assumption

closeNG :: Computation { ch: End } {} ()
closeNG := assumption

main :: Computation { ch: Offer { ping: Send End, quit: End } } {} Int
main := do {
  tag <- receiveAt @#ch;
  case tag {
    #ping => do { recvNG; closeNG; pure 42 };
    #quit => do { closeNG; pure 0 }
  }
}
`)
	if errs == "" {
		t.Fatal("expected type error for wrong operation in Variant branch")
	}
}

func TestProbeS_VariantMissingBranch(t *testing.T) {
	errs := sessionCheck(t, `
receiveAt :: \(l: Label) (choices: Row) (s: Type) r. Computation { l: Offer choices | r } { l: s | r } (Variant choices s)
receiveAt := assumption

closeNG :: Computation { ch: End } {} ()
closeNG := assumption

main :: Computation { ch: Offer { ping: End, quit: End } } {} ()
main := do {
  tag <- receiveAt @#ch;
  case tag { #ping => do { closeNG } }
}
`)
	if errs == "" {
		t.Fatal("expected non-exhaustive error for missing Variant branch")
	}
}

func TestProbeS_DualOfferChoose(t *testing.T) {
	// Dual (Offer { a: Send End, b: End })
	//   = Choose (MapRow Dual { a: Send End, b: End })
	//   = Choose { a: Recv End, b: End }
	errs := sessionCheck(t, `
type DualRow :: Row := \(r: Row). MapRow Dual r

type Proto := Offer { a: Send End, b: End }

id :: \s. Computation { ch: s @Linear } { ch: s @Linear } ()
id := assumption

main :: Computation { ch: Dual Proto @Linear } { ch: Choose { a: Recv End, b: End } @Linear } ()
main := do { id }
`)
	if errs != "" {
		t.Fatalf("Dual Offer→Choose (deep) failed: %s", errs)
	}
}
