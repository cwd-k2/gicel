//go:build probe

// Bidir higher-rank probe tests — polymorphic function arguments, rank-2+ type checking.
// Does NOT cover: bidir_inference_test.go, bidir_typerecord_test.go, bidir_case_probe_test.go.
package check

import (
	"testing"
)

// =============================================================================
// Higher-rank type probe tests — polymorphic function arguments, rank-2+
// type checking, wrong-rank errors, returning polymorphic functions,
// and deep quantifier nesting.
// =============================================================================

// =====================================================================
// From probe_a: Higher-rank types
// =====================================================================

// TestProbeA_HigherRank_PolyFnAsArg — pass a polymorphic function as argument
// to a function with a higher-rank type.
func TestProbeA_HigherRank_PolyFnAsArg(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

id :: \ a. a -> a
id := \x. x

apply :: (\ a. a -> a) -> Bool
apply := \f. f True

main := apply id
`
	checkSource(t, source, nil)
}

// TestProbeA_HigherRank_PolyFnUsedAtTwoTypes — the polymorphic argument
// is applied at two different types inside the body.
func TestProbeA_HigherRank_PolyFnUsedAtTwoTypes(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

applyBoth :: (\ a. a -> a) -> (Bool, Maybe Bool)
applyBoth := \f. (f True, f (Just False))

main := applyBoth (\x. x)
`
	checkSource(t, source, nil)
}

// TestProbeA_HigherRank_WrongRankError — passing a monomorphic function where
// a higher-rank type is expected should fail.
func TestProbeA_HigherRank_WrongRankError(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

apply :: (\ a. a -> a) -> Bool
apply := \f. f True

-- notId is Bool -> Bool, not \ a. a -> a.
notId :: Bool -> Bool
notId := \x. True

main := apply notId
`
	checkSourceExpectError(t, source, nil)
}

// TestProbeA_HigherRank_ReturnPolyFn — a function that returns a
// polymorphic function.
// BUG: high — When a function is annotated to return a higher-rank type
// (e.g., `\ a. a -> a`), and the result is parenthesized and applied
// `(mkId True) False`, the checker emits "expected function type, got
// \a. a -> a" (E0204). The issue is that the inferred result type from
// the application `mkId True` is `\ a. a -> a` (a forall), but
// `matchArrow` does not instantiate the forall before trying to decompose
// it into an arrow type. In check mode, the subsumption path through
// `checkApp` -> `matchArrow` should instantiate the returned forall to
// produce a monomorphic arrow, but `matchArrow` sees the forall and tries
// to unify it with `?m1 -> ?m2`, which fails.
func TestProbeA_HigherRank_ReturnPolyFn(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

mkId :: Bool -> (\ a. a -> a)
mkId := \b. \x. x

-- Workaround: bind the result to a name with annotation.
applied :: \ a. a -> a
applied := mkId True

main := applied False
`
	checkSource(t, source, nil)
}

// TestProbeA_HigherRank_ReturnPolyFnDirect — direct application of a
// higher-rank-returning function. matchArrow now instantiates foralls
// before arrow decomposition, so this succeeds.
func TestProbeA_HigherRank_ReturnPolyFnDirect(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

mkId :: Bool -> (\ a. a -> a)
mkId := \b. \x. x

main := (mkId True) False
`
	checkSource(t, source, nil)
}

// TestProbeA_HigherRank_FourLevels — four nested quantifier levels with
// subsumption. Exercises deep skolem/meta interplay.
func TestProbeA_HigherRank_FourLevels(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

id :: \ a. a -> a
id := \x. x

applyId :: (\ a. a -> a) -> Bool
applyId := \f. f True

applyApplyId :: ((\ a. a -> a) -> Bool) -> Bool
applyApplyId := \g. g id

applyApplyApplyId :: (((\ a. a -> a) -> Bool) -> Bool) -> Bool
applyApplyApplyId := \h. h applyId

main := applyApplyApplyId applyApplyId
`
	checkSource(t, source, nil)
}

// =====================================================================
// Quick Look impredicativity — multi-arg constructor with polytype.
// Uses self-contained form definitions (no Prelude dependency).
// =====================================================================

// TestProbeA_QuickLook_PairLambdaPoly — Mk (\x.x) True :: Pair (\a. a -> a) Bool
func TestProbeA_QuickLook_PairLambdaPoly(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Pair := \a b. { Mk: a -> b -> Pair a b; }
main :: Pair (\a. a -> a) Bool
main := Mk (\x. x) True
`
	checkSource(t, source, nil)
}

// TestProbeA_QuickLook_ListLambdaPoly — MkCons (\x.x) MkNil :: MyList (\a. a -> a)
func TestProbeA_QuickLook_ListLambdaPoly(t *testing.T) {
	source := `
form MyList := \a. { MkCons: a -> MyList a -> MyList a; MkNil: MyList a; }
id :: \a. a -> a
id := \x. x
main :: MyList (\a. a -> a)
main := MkCons (\x. x) MkNil
`
	checkSource(t, source, nil)
}

// TestProbeA_QuickLook_ListIdPoly — MkCons id MkNil :: MyList (\a. a -> a)
func TestProbeA_QuickLook_ListIdPoly(t *testing.T) {
	source := `
form MyList := \a. { MkCons: a -> MyList a -> MyList a; MkNil: MyList a; }
id :: \a. a -> a
id := \x. x
main :: MyList (\a. a -> a)
main := MkCons id MkNil
`
	checkSource(t, source, nil)
}

// TestProbeA_QuickLook_NestedConsPoly — nested cons with polytype.
func TestProbeA_QuickLook_NestedConsPoly(t *testing.T) {
	source := `
form MyList := \a. { MkCons: a -> MyList a -> MyList a; MkNil: MyList a; }
main :: MyList (\a. a -> a)
main := MkCons (\x. x) (MkCons (\y. y) MkNil)
`
	checkSource(t, source, nil)
}
