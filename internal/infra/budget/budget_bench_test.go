// Budget overhead benchmarks — per-call cost of Step/Nest/Alloc on hot paths.
// Does NOT cover: correctness (budget_test.go).

package budget

import (
	"context"
	"testing"
)

func BenchmarkBudgetStep(b *testing.B) {
	bg := New(context.Background(), 0, 0) // unlimited
	for b.Loop() {
		_ = bg.Step()
	}
}

func BenchmarkBudgetStepCancel(b *testing.B) {
	ctx := b.Context()
	bg := New(ctx, 0, 0)
	for b.Loop() {
		_ = bg.Step()
	}
}

func BenchmarkBudgetEnterLeave(b *testing.B) {
	bg := New(context.Background(), 0, 0) // unlimited depth
	for b.Loop() {
		_ = bg.Enter()
		bg.Leave()
	}
}

func BenchmarkBudgetNestUnnest(b *testing.B) {
	bg := New(context.Background(), 0, 0)
	bg.SetNestingLimit(0) // unlimited
	for b.Loop() {
		_ = bg.Nest()
		bg.Unnest()
	}
}

func BenchmarkBudgetAlloc(b *testing.B) {
	bg := New(context.Background(), 0, 0)
	bg.SetAllocLimit(0) // unlimited
	for b.Loop() {
		_ = bg.Alloc(64)
	}
}
