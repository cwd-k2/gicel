// Unifier benchmarks — Unify, level unify, row unify, snapshot/restore.
// Does NOT cover: Zonk (check_bench_test.go has ZonkDeepChain).
package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// buildDeepArrow builds a -> (a -> (a -> ... -> a)) with n arrows.
func buildDeepArrow(n int) types.Type {
	var t types.Type = types.Con("Int")
	for i := 0; i < n; i++ {
		t = &types.TyArrow{From: types.Con("Int"), To: t}
	}
	return t
}

// buildDeepApp builds (((...(F a) a) a) ... a) with n applications.
func buildDeepApp(n int) types.Type {
	t := types.Type(types.Con("F"))
	for i := 0; i < n; i++ {
		t = &types.TyApp{Fun: t, Arg: types.Con("Int")}
	}
	return t
}

// BenchmarkUnifyIdentical benchmarks unifying two identical deep arrow types.
// Measures the best-case path through the Unify switch (structural match, no solving).
func BenchmarkUnifyIdentical(b *testing.B) {
	for _, depth := range []int{10, 50, 200} {
		ty := buildDeepArrow(depth)
		b.Run(depthName(depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				u := NewUnifier()
				_ = u.Unify(ty, ty)
			}
		})
	}
}

// BenchmarkUnifyMetaSolve benchmarks solving n meta variables against concrete types.
// Measures solveMeta path + trail writes.
func BenchmarkUnifyMetaSolve(b *testing.B) {
	for _, n := range []int{10, 50, 200} {
		b.Run(depthName(n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				u := NewUnifier()
				for j := 0; j < n; j++ {
					m := &types.TyMeta{ID: j, Kind: types.TypeOfTypes}
					_ = u.Unify(m, types.Con("Int"))
				}
			}
		})
	}
}

// BenchmarkUnifyDeepApp benchmarks unifying two deep TyApp chains.
// Exercises the TyApp case with recursive descent.
func BenchmarkUnifyDeepApp(b *testing.B) {
	for _, depth := range []int{10, 50, 200} {
		ty := buildDeepApp(depth)
		b.Run(depthName(depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				u := NewUnifier()
				_ = u.Unify(ty, ty)
			}
		})
	}
}

// BenchmarkSnapshotRestore benchmarks snapshot/restore overhead.
// Measures trail allocation and restoration for speculative unification.
func BenchmarkSnapshotRestore(b *testing.B) {
	for _, n := range []int{10, 50, 200} {
		b.Run(depthName(n), func(b *testing.B) {
			u := NewUnifier()
			for i := 0; i < b.N; i++ {
				snap := u.Snapshot()
				for j := 0; j < n; j++ {
					m := &types.TyMeta{ID: j + i*n, Kind: types.TypeOfTypes}
					_ = u.Unify(m, types.Con("Int"))
				}
				u.Restore(snap)
			}
		})
	}
}

// BenchmarkUnifyLevelLit benchmarks level unification for TyCon pairs.
func BenchmarkUnifyLevelLit(b *testing.B) {
	a := &types.TyCon{Name: "Type", Level: types.L1}
	bTy := &types.TyCon{Name: "Type", Level: types.L1}
	for i := 0; i < b.N; i++ {
		u := NewUnifier()
		_ = u.Unify(a, bTy)
	}
}

func depthName(n int) string {
	switch {
	case n <= 10:
		return "10"
	case n <= 50:
		return "50"
	default:
		return "200"
	}
}
