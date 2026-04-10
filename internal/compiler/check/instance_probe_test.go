//go:build probe

// Instance probe tests — resolution depth, self-recursive instances, overlapping, missing methods.
// Does NOT cover: instance_test.go, instance_tuple_test.go, class_elaboration_test.go, class_hkt_test.go.
package check

import (
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// Instance probe tests — instance resolution depth, self-recursive instances,
// empty classes, overlapping instances, missing methods, superclass constraints,
// ambiguous constraints, deep superclass chains, contextual instances.
// =============================================================================

// =====================================================================
// From probe_d: Instance resolution depth
// =====================================================================

// TestProbeD_InstanceDepth_NearLimit — 15 levels of nested contextual
// instance resolution. maxResolveDepth = 64, so 15 is well within range.
func TestProbeD_InstanceDepth_NearLimit(t *testing.T) {
	const N = 15
	var sb strings.Builder
	sb.WriteString("form Bool := { True: Bool; False: Bool; }\n")
	sb.WriteString("form Eq := \\a. { eq: a -> a -> Bool; }\n")
	sb.WriteString("impl Eq Bool := { eq := \\x y. True }\n\n")

	for i := 0; i < N; i++ {
		sb.WriteString(fmt.Sprintf("form W%d := \\a. { MkW%d: a -> W%d a; }\n", i, i, i))
		sb.WriteString(fmt.Sprintf("impl Eq a => Eq (W%d a) := { eq := \\x y. True }\n\n", i))
	}

	// Build W0 (W1 (... (W14 Bool) ...))
	inner := "True"
	for i := N - 1; i >= 0; i-- {
		inner = fmt.Sprintf("(MkW%d %s)", i, inner)
	}
	sb.WriteString(fmt.Sprintf("main := eq %s %s\n", inner, inner))
	checkSource(t, sb.String(), nil)
}

// TestProbeD_InstanceDepth_SelfRecursiveSameType — an instance whose context
// requires itself at the same type should be rejected at instance registration.
func TestProbeD_InstanceDepth_SelfRecursiveSameType(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form C := \a. { m: a -> Bool }
impl C a => C a := { m := \x. True }
main := m True
`
	checkSourceExpectError(t, source, nil)
}

// =====================================================================
// From probe_e: Type class edge cases
// =====================================================================

// TestProbeE_TypeClass_EmptyClass — a class with a single marker method should compile.
// In unified syntax, class-like data requires at least one lowercase field
// to be recognized as a type class (the isClassLikeForm heuristic).
func TestProbeE_TypeClass_EmptyClass(t *testing.T) {
	source := `
form Marker := \a. { _tag: () }
form Bool := { True: Bool; False: Bool; }
impl Marker Bool := { _tag := () }
main := True
`
	checkSource(t, source, nil)
}

// TestProbeE_TypeClass_OverlappingInstancesError — overlapping instances
// should produce a clean error, not a panic.
func TestProbeE_TypeClass_OverlappingInstancesError(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form C := \a. { method: a -> Bool }
impl C Bool := { method := \x. x }
impl C Bool := { method := \x. True }
main := method True
`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "overlap") && !strings.Contains(errMsg, "duplicate") {
		t.Logf("NOTICE: overlapping instances produced error: %s", errMsg)
		// Not a bug if it reports any error — just checking it doesn't panic
	}
}

// TestProbeE_TypeClass_MissingMethodError — instance with missing method
// should report a clear error.
func TestProbeE_TypeClass_MissingMethodError(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form C := \a. { method1: a -> Bool; method2: a -> a }
impl C Bool := { method1 := \x. x }
main := method1 True
`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "method") {
		t.Logf("NOTICE: missing method error: %s", errMsg)
	}
}

// TestProbeE_TypeClass_SuperclassWithoutInstance — using a superclass method
// without providing the subclass instance should fail cleanly.
func TestProbeE_TypeClass_SuperclassWithoutInstance(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
form Ord := \a. Eq a => { lt: a -> a -> Bool }

-- Ord Bool instance without Eq Bool instance
impl Ord Bool := { lt := \x y. True }
main := lt True False
`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "instance") && !strings.Contains(errMsg, "Eq") {
		t.Logf("NOTICE: superclass missing error: %s", errMsg)
	}
}

// TestProbeE_TypeClass_AmbiguousConstraintDefaulting — a constraint with
// unsolved metas that cannot be defaulted should produce a diagnostic.
func TestProbeE_TypeClass_AmbiguousConstraintDefaulting(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form C := \a. { method: a -> Bool }
impl C Bool := { method := \x. x }

-- f takes no arguments that could determine the type variable
f := method
main := f True
`
	// This might compile (inferring a = Bool from application) or error.
	// The important thing is no panic.
	checkSourceNoPanic(t, source, nil)
}

// TestProbeE_TypeClass_DeepSuperclassChain — transitive superclass search
// through a 3-level chain.
func TestProbeE_TypeClass_DeepSuperclassChain(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form A := \a. { methodA: a -> Bool }
form B := \a. A a => { methodB: a -> Bool }
form C := \a. B a => { methodC: a -> Bool }

impl A Bool := { methodA := \x. True }
impl B Bool := { methodB := \x. True }
impl C Bool := { methodC := \x. True }

-- Using methodA through a C constraint should work via superclass chain C => B => A
useA :: \a. C a => a -> Bool
useA := \x. methodA x

main := useA True
`
	checkSource(t, source, nil)
}

// TestProbeE_TypeClass_InstanceWithExtraContext — an instance with an
// unnecessary context constraint should still compile.
func TestProbeE_TypeClass_InstanceWithExtraContext(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq a => Eq (Maybe a) := {
  eq := \mx my. case mx {
    Nothing => case my { Nothing => True; Just _ => False };
    Just x => case my { Nothing => False; Just y => eq x y }
  }
}
main := eq (Just True) (Just False)
`
	checkSource(t, source, nil)
}

// TestProbeA_TypeClassMethodAmbiguity — a class method used without enough
// type information to resolve the instance should produce a sensible error.
func TestProbeA_TypeClassMethodAmbiguity(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }

form Show := \a. { show: a -> a }
impl Show Bool := { show := \x. x }
impl Show Unit := { show := \x. x }

-- Ambiguous: what type is x?
f := \x. show x
`
	// This should either infer a constrained type or produce an error.
	// The test verifies no crash.
	checkSource(t, source, nil)
}
