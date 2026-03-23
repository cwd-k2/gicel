// Decl effectful binding tests — bare Computation rejection at top level.
// Does NOT cover: deferred.go, evidence.go.
package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

func TestRejectBareComputationNonEntry(t *testing.T) {
	config := &CheckConfig{EntryPoint: "main"}
	checkSourceExpectCode(t, `
f := do { pure 1 }
main := 42
`, config, diagnostic.ErrEffectfulBinding)
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

func TestRejectMainWhenCustomEntry(t *testing.T) {
	config := &CheckConfig{EntryPoint: "myMain"}
	checkSourceExpectCode(t, `
main := do { pure 1 }
myMain := 42
`, config, diagnostic.ErrEffectfulBinding)
}

func TestRejectGeneralizedBareComputation(t *testing.T) {
	// Let-generalized: f := do { pure 1 } gets type \a. Computation a a Int
	// After generalization, still a bare Computation.
	config := &CheckConfig{EntryPoint: "main"}
	checkSourceExpectCode(t, `
f := do { pure 1 }
main := 0
`, config, diagnostic.ErrEffectfulBinding)
}

func TestAllowMultiplePureBindings(t *testing.T) {
	config := &CheckConfig{EntryPoint: "main"}
	checkSource(t, `
x := 1
y := 2
main := (x, y)
`, config)
}
