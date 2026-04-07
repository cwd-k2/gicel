// VM dispatch microbenchmarks — directly measure the PrimImpl call paths
// (callPrim with defer/recover vs callTrustedPrim without). The diff between
// the two is the pure cost of the panic-recovery defer.
//
// Why this matters: callPrim and callTrustedPrim share the same hot-path
// resolution (Step 5 cache) but currently differ in trust handling. The
// asymmetry is historical, not principled. B2 weighs unifying them; this
// bench measures the per-call defer cost so the decision is grounded in
// numbers rather than intuition.
//
// Does NOT cover: actual PrimImpl bodies, observer overhead, error paths.

package vm

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// trivialPrim is a minimal PrimImpl: returns its first argument unchanged.
// Has no work to dilute the dispatch cost — exactly what we want to isolate.
func trivialPrim(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return args[0], ce, nil
}

// newDispatchBenchVM constructs a minimal VM suitable for invoking
// callPrim / callTrustedPrim directly. No globals, no proto stack — the
// bench loops below the bytecode dispatch layer.
func newDispatchBenchVM(b *testing.B) *VM {
	b.Helper()
	bg := budget.New(context.Background(), 1<<30, 1<<20)
	bg.SetNestingLimit(1024)
	bg.SetAllocLimit(1 << 30)
	return NewVM(VMConfig{
		Globals:     nil,
		GlobalSlots: map[string]int{},
		Prims:       eval.NewPrimRegistry(),
		Budget:      bg,
		Ctx:         context.Background(),
	})
}

// BenchmarkCallTrustedPrim measures the trusted dispatch path: a direct
// PrimImpl invocation with no defer/recover, used by OpPrim and the
// applyN/applyPrim saturated fast paths.
func BenchmarkCallTrustedPrim(b *testing.B) {
	vm := newDispatchBenchVM(b)
	args := []eval.Value{eval.IntVal(1)}
	ce := eval.EmptyCapEnv()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, err := vm.callTrustedPrim(trivialPrim, ce, args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCallPrim measures the recover-wrapped dispatch path: identical
// to callTrustedPrim except for the defer/recover, used by host-callback
// re-entry (applyForPrim, applyNForPrim, forceEffectful) and the slow
// applyPrim/applyN PrimVal branches.
//
// The diff vs BenchmarkCallTrustedPrim is the pure defer cost — the only
// per-call overhead the trust asymmetry imposes.
func BenchmarkCallPrim(b *testing.B) {
	vm := newDispatchBenchVM(b)
	args := []eval.Value{eval.IntVal(1)}
	ce := eval.EmptyCapEnv()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, err := vm.callPrim(trivialPrim, ce, args)
		if err != nil {
			b.Fatal(err)
		}
	}
}
