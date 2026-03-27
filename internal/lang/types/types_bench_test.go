// Type operation benchmarks — Equal, SubstMany, FreeVars, TypeKey.
// Does NOT cover: evidence row operations (evidence_test.go), pretty printing.
package types

import (
	"testing"
)

// buildDeepArrowType builds Int -> (Int -> (... -> Int)) with n arrows.
func buildDeepArrowType(n int) Type {
	t := Type(Con("Int"))
	for i := 0; i < n; i++ {
		t = &TyArrow{From: Con("Int"), To: t}
	}
	return t
}

// buildDeepAppType builds ((..((F a0) a1) ..) aN) with n applications.
func buildDeepAppType(n int) Type {
	t := Type(Con("F"))
	for i := 0; i < n; i++ {
		t = &TyApp{Fun: t, Arg: &TyVar{Name: "a"}}
	}
	return t
}

// BenchmarkEqualDeepArrow benchmarks structural equality on deep arrow types.
func BenchmarkEqualDeepArrow(b *testing.B) {
	for _, depth := range []int{10, 50, 200} {
		ty := buildDeepArrowType(depth)
		b.Run(benchSize(depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				Equal(ty, ty)
			}
		})
	}
}

// BenchmarkEqualDeepApp benchmarks structural equality on deep application chains.
func BenchmarkEqualDeepApp(b *testing.B) {
	for _, depth := range []int{10, 50, 200} {
		ty := buildDeepAppType(depth)
		b.Run(benchSize(depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				Equal(ty, ty)
			}
		})
	}
}

// BenchmarkTypeKey benchmarks canonical key generation.
func BenchmarkTypeKey(b *testing.B) {
	for _, depth := range []int{10, 50, 200} {
		ty := buildDeepArrowType(depth)
		b.Run(benchSize(depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				TypeKey(ty)
			}
		})
	}
}

// BenchmarkFreeVars benchmarks free variable collection.
func BenchmarkFreeVars(b *testing.B) {
	// Build a type with many free variables: F a0 a1 ... aN
	buildFreeVarType := func(n int) Type {
		t := Type(Con("F"))
		for i := 0; i < n; i++ {
			t = &TyApp{Fun: t, Arg: &TyVar{Name: "a"}}
		}
		return t
	}
	for _, n := range []int{10, 50, 200} {
		ty := buildFreeVarType(n)
		b.Run(benchSize(n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				FreeVars(ty)
			}
		})
	}
}

// BenchmarkSubstMany benchmarks simultaneous substitution.
func BenchmarkSubstMany(b *testing.B) {
	// Build forall a. F a a a ... (body has n occurrences of a)
	buildBody := func(n int) Type {
		t := Type(Con("F"))
		for i := 0; i < n; i++ {
			t = &TyApp{Fun: t, Arg: &TyVar{Name: "a"}}
		}
		return t
	}
	subst := map[string]Type{"a": Con("Int")}
	for _, n := range []int{10, 50, 200} {
		ty := buildBody(n)
		b.Run(benchSize(n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				SubstMany(ty, subst)
			}
		})
	}
}

// BenchmarkSubstCapture benchmarks substitution with alpha-renaming (capture avoidance).
func BenchmarkSubstCapture(b *testing.B) {
	// forall a. F x a — substitute x -> a (triggers capture avoidance)
	body := &TyForall{
		Var:  "a",
		Kind: TypeOfTypes,
		Body: &TyApp{Fun: &TyApp{Fun: Con("F"), Arg: &TyVar{Name: "x"}}, Arg: &TyVar{Name: "a"}},
	}
	for i := 0; i < b.N; i++ {
		Subst(body, "x", &TyVar{Name: "a"})
	}
}

func benchSize(n int) string {
	switch {
	case n <= 10:
		return "10"
	case n <= 50:
		return "50"
	default:
		return "200"
	}
}
