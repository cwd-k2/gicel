// Decl effectful binding tests — CBPV top-level discipline: non-entry
// Computation-typed bindings are auto-thunked when unannotated, and
// explicit `:: Computation ...` annotations at non-entry position are
// rejected (the annotation expresses a deliberate "this should run
// directly" intent that non-entry bindings cannot honor).
// Does NOT cover: deferred.go, evidence.go.
package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

// TestAutoThunkBareComputationNonEntry: a non-entry top-level binding
// whose RHS infers to a bare Computation is silently thunked by the
// checker — it used to be a hard error (ErrEffectfulBinding) prior
// to the CBPV auto-coercion pass.
func TestAutoThunkBareComputationNonEntry(t *testing.T) {
	config := &CheckConfig{EntryPoint: "main"}
	checkSource(t, `
f := do { pure 1 }
main := 42
`, config)
}

// TestAutoThunkPolyComputation: after let-generalization the RHS
// has type `\a ...g. Computation ... a`; the auto-thunk walks under
// the forall chain and wraps the innermost CBPV node so the binding
// becomes `\a ...g. Thunk ... a`.
func TestAutoThunkPolyComputation(t *testing.T) {
	config := &CheckConfig{EntryPoint: "main"}
	checkSource(t, `
f := do { pure 1 }
main := 0
`, config)
}

func TestAllowEntryPointComputation(t *testing.T) {
	config := &CheckConfig{EntryPoint: "main"}
	checkSource(t, `main := do { pure 42 }`, config)
}

func TestAllowArrowReturningComputation(t *testing.T) {
	config := &CheckConfig{EntryPoint: "main"}
	checkSource(t, `
f := \x. do { pure x }
main := 42
`, config)
}

func TestAllowThunkType(t *testing.T) {
	config := &CheckConfig{EntryPoint: "main"}
	checkSource(t, `
t := thunk (do { pure 42 })
main := force t
`, config)
}

func TestNoCheckWithoutEntryPoint(t *testing.T) {
	// When EntryPoint is empty (check-only mode), bare Computation is allowed.
	checkSource(t, `f := do { pure 1 }`, nil)
}

// TestRejectAnnotatedBareComputation: an explicit Computation
// annotation at non-entry top level still errors — the annotation
// expresses intent the checker refuses to silently rewrite.
func TestRejectAnnotatedBareComputation(t *testing.T) {
	config := &CheckConfig{EntryPoint: "main"}
	checkSourceExpectCode(t, `
form Unit := { Unit: Unit; }
f :: Computation {} {} Unit
f := do { pure Unit }
main := Unit
`, config, diagnostic.ErrEffectfulBinding)
}

func TestAllowCustomEntryPoint(t *testing.T) {
	config := &CheckConfig{EntryPoint: "myMain"}
	checkSource(t, `myMain := do { pure 42 }`, config)
}

// TestAutoThunkMainWhenCustomEntry: when a custom entry point is set,
// `main` is just another non-entry binding and gets auto-thunked like
// any other unannotated Computation RHS.
func TestAutoThunkMainWhenCustomEntry(t *testing.T) {
	config := &CheckConfig{EntryPoint: "myMain"}
	checkSource(t, `
main := do { pure 1 }
myMain := 42
`, config)
}

func TestAllowMultiplePureBindings(t *testing.T) {
	config := &CheckConfig{EntryPoint: "main"}
	checkSource(t, `
x := 1
y := 2
main := (x, y)
`, config)
}
