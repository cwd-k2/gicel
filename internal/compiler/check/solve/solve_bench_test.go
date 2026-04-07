// Solver data structure benchmarks — worklist, inert set, kick-out.
// Does NOT cover: full SolveWanteds (requires Env mock), instance resolution.
package solve

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// BenchmarkWorklistPushPop benchmarks the worklist push/pop cycle.
func BenchmarkWorklistPushPop(b *testing.B) {
	for _, n := range []int{10, 100, 500} {
		b.Run(sizeName(n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				var w Worklist
				for range n {
					w.Push(&CtClass{
						ClassName: "Eq",
						Args:      []types.Type{types.Con("Int")},
						S:         span.Span{},
					})
				}
				for {
					_, ok := w.Pop()
					if !ok {
						break
					}
				}
			}
		})
	}
}

// BenchmarkInertSetInsertKickOut benchmarks inserting CtFunEq constraints
// then kicking out those matching a solved meta.
func BenchmarkInertSetInsertKickOut(b *testing.B) {
	for _, n := range []int{10, 50, 200} {
		b.Run(sizeName(n), func(b *testing.B) {
			meta := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
			for i := 0; i < b.N; i++ {
				is := &InertSet{}
				for j := range n {
					is.InsertFunEq(&CtFunEq{
						FamilyName: "F",
						Args:       []types.Type{meta, types.Con("Int")},
						ResultMeta: &types.TyMeta{ID: 1000 + j, Kind: types.TypeOfTypes},
						BlockingOn: []int{1},
						S:          span.Span{},
					})
				}
				is.KickOut(1)
			}
		})
	}
}

// BenchmarkInertSetScopeEnterLeave benchmarks scope enter/leave with constraints.
func BenchmarkInertSetScopeEnterLeave(b *testing.B) {
	for b.Loop() {
		is := &InertSet{}
		is.InsertFunEq(&CtFunEq{FamilyName: "F", Args: []types.Type{types.Con("Int")}, ResultMeta: &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}, BlockingOn: []int{1}})
		is.EnterScope()
		for j := range 20 {
			is.InsertFunEq(&CtFunEq{FamilyName: "G", Args: []types.Type{types.Con("Int")}, ResultMeta: &types.TyMeta{ID: 100 + j, Kind: types.TypeOfTypes}, BlockingOn: []int{100 + j}})
		}
		is.LeaveScope()
	}
}

// BenchmarkKickOutMentioningSkolem benchmarks skolem-based kick-out.
func BenchmarkKickOutMentioningSkolem(b *testing.B) {
	for _, n := range []int{10, 50, 200} {
		b.Run(sizeName(n), func(b *testing.B) {
			skolem := &types.TySkolem{ID: 99, Name: "sk", Kind: types.TypeOfTypes}
			for i := 0; i < b.N; i++ {
				is := &InertSet{}
				for j := range n {
					meta := &types.TyMeta{ID: j, Kind: types.TypeOfTypes}
					is.InsertEq(&CtEq{
						Lhs:    meta,
						Rhs:    &types.TyApp{Fun: types.Con("F"), Arg: skolem},
						Flavor: CtWanted,
						S:      span.Span{},
					}, []int{j})
				}
				is.KickOutMentioningSkolem(99)
			}
		})
	}
}

func sizeName(n int) string {
	switch {
	case n <= 10:
		return "10"
	case n <= 50:
		return "50"
	case n <= 100:
		return "100"
	case n <= 200:
		return "200"
	default:
		return "500"
	}
}
