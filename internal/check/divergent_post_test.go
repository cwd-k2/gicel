package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/types"
)

// ==========================================
// Phase 6: Divergent Post-States — TDD
// ==========================================

// --- Equal post-states (v0 baseline) ---

func TestDivergentPostEqualBranches(t *testing.T) {
	// Both branches consume the same caps → same post-state → OK.
	source := `
data Bool = True | False
data Unit = Unit
consumeAll :: Computation { a : Unit, b : Unit } {} Unit
consumeAll := assumption
f :: Bool -> Computation { a : Unit, b : Unit } {} Unit
f := \b -> case b {
  True -> consumeAll;
  False -> consumeAll
}
`
	checkSource(t, source, nil)
}

func TestDivergentPostSingleBranch(t *testing.T) {
	source := `
data Unit = MkUnit
consume :: Computation { a : Unit } {} Unit
consume := assumption
f :: Unit -> Computation { a : Unit } {} Unit
f := \u -> case u {
  MkUnit -> consume
}
`
	checkSource(t, source, nil)
}

// --- Divergent post-states: currently an error (v0) ---

func TestDivergentPostPartialOverlap(t *testing.T) {
	// Branch True:  post = { b : Unit }  (a consumed)
	// Branch False: post = { a : Unit, b : Unit }  (nothing consumed)
	// Intersection: post = { b : Unit }  (b is in both branches)
	source := `
data Bool = True | False
data Unit = Unit
consumeA :: Computation { a : Unit, b : Unit } { b : Unit } Unit
consumeA := assumption
noop :: Computation { a : Unit, b : Unit } { a : Unit, b : Unit } Unit
noop := assumption
f :: Bool -> Computation { a : Unit, b : Unit } { b : Unit } Unit
f := \b -> case b {
  True -> consumeA;
  False -> noop
}
`
	checkSource(t, source, nil)
}

// --- With multiplicity ---

func TestDivergentPostWithMultEqual(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Bool = True | False
data Unit = Unit
close :: Computation { h : Unit @Linear } {} Unit
close := assumption
f :: Bool -> Computation { h : Unit @Linear } {} Unit
f := \b -> case b {
  True -> close;
  False -> close
}
`
	checkSource(t, source, nil)
}

// --- LUB type family defined (infrastructure readiness) ---

func TestDivergentPostLUBDefined(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
type LUB (m1 : Mult) (m2 : Mult) :: Mult = {
  LUB Linear _ = Linear;
  LUB _ Linear = Linear;
  LUB Affine _ = Affine;
  LUB _ Affine = Affine;
  LUB Unrestricted Unrestricted = Unrestricted
}
`
	checkSource(t, source, nil)
}

// --- Non-computation case still works ---

func TestCasePureValueUnchanged(t *testing.T) {
	source := `
data Bool = True | False
data Unit = Unit
f :: Bool -> Unit
f := \b -> case b {
  True -> Unit;
  False -> Unit
}
`
	checkSource(t, source, nil)
}

// --- Divergent post with LUB join (target behavior) ---
// This test is for the FUTURE when lubPostStates is fully connected.
// For now, it documents the desired behavior.

func TestDivergentPostWithLUBJoin(t *testing.T) {
	// Branch True:  post = { b : Unit }  (a consumed)
	// Branch False: post = { a : Unit }  (b consumed)
	// Intersection join: post = {}  (no label is in ALL branches)
	source := `
data Bool = True | False
data Unit = Unit
consumeA :: Computation { a : Unit, b : Unit } { b : Unit } Unit
consumeA := assumption
consumeB :: Computation { a : Unit, b : Unit } { a : Unit } Unit
consumeB := assumption
f :: Bool -> Computation { a : Unit, b : Unit } {} Unit
f := \b -> case b {
  True -> consumeA;
  False -> consumeB
}
`
	checkSource(t, source, nil)
}

// --- Typehelpers with Mult ---

func TestTypehelpersWithMult(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{
			"Int":    types.KType{},
			"Linear": types.KType{},
		},
	}
	source := `
data Mult = Unrestricted | Affine | Linear
f :: { x : Int @Linear } -> Int
f := \r -> 0
`
	checkSource(t, source, config)
}
