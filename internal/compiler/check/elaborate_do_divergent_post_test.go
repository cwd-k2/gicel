package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// ==========================================
// Phase 6: Divergent Post-States — TDD
// ==========================================

// --- Equal post-states (v0 baseline) ---

func TestDivergentPostEqualBranches(t *testing.T) {
	// Both branches consume the same caps → same post-state → OK.
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
consumeAll :: Computation { a: Unit, b: Unit } {} Unit
consumeAll := assumption
f :: Bool -> Computation { a: Unit, b: Unit } {} Unit
f := \b. case b {
  True => consumeAll;
  False => consumeAll
}
`
	checkSource(t, source, nil)
}

func TestDivergentPostSingleBranch(t *testing.T) {
	source := `
form Unit := { MkUnit: Unit; }
consume :: Computation { a: Unit } {} Unit
consume := assumption
f :: Unit -> Computation { a: Unit } {} Unit
f := \u. case u {
  MkUnit => consume
}
`
	checkSource(t, source, nil)
}

// --- Divergent post-states: currently an error (v0) ---

func TestDivergentPostPartialOverlap(t *testing.T) {
	// Branch True:  post = { b: Unit }  (a consumed)
	// Branch False: post = { a: Unit, b: Unit }  (nothing consumed)
	// Intersection: post = { b: Unit }  (b is in both branches)
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
consumeA :: Computation { a: Unit, b: Unit } { b: Unit } Unit
consumeA := assumption
noop :: Computation { a: Unit, b: Unit } { a: Unit, b: Unit } Unit
noop := assumption
f :: Bool -> Computation { a: Unit, b: Unit } { b: Unit } Unit
f := \b. case b {
  True => consumeA;
  False => noop
}
`
	checkSource(t, source, nil)
}

// --- With multiplicity ---

func TestDivergentPostWithMultEqual(t *testing.T) {
	source := `
form Mult := { Unrestricted: (); Affine: (); Linear: (); }
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
close :: Computation { h: Unit @Linear } {} Unit
close := assumption
f :: Bool -> Computation { h: Unit @Linear } {} Unit
f := \b. case b {
  True => close;
  False => close
}
`
	checkSource(t, source, nil)
}

// --- LUB type family defined (infrastructure readiness) ---

func TestDivergentPostLUBDefined(t *testing.T) {
	source := `
form Mult := { Unrestricted: (); Affine: (); Linear: (); }
type LUB :: Mult := \(m1: Mult) (m2: Mult). case (m1, m2) {
  (Linear, _) => Linear;
  (_, Linear) => Linear;
  (Affine, _) => Affine;
  (_, Affine) => Affine;
  (Unrestricted, Unrestricted) => Unrestricted
}
`
	checkSource(t, source, nil)
}

// --- Non-computation case still works ---

func TestCasePureValueUnchanged(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
f :: Bool -> Unit
f := \b. case b {
  True => Unit;
  False => Unit
}
`
	checkSource(t, source, nil)
}

// --- Divergent post with LUB join ---

func TestDivergentPostWithLUBJoin(t *testing.T) {
	// Branch True:  post = { b: Unit }  (a consumed)
	// Branch False: post = { a: Unit }  (b consumed)
	// Intersection join: post = {}  (no label is in ALL branches)
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
consumeA :: Computation { a: Unit, b: Unit } { b: Unit } Unit
consumeA := assumption
consumeB :: Computation { a: Unit, b: Unit } { a: Unit } Unit
consumeB := assumption
f :: Bool -> Computation { a: Unit, b: Unit } {} Unit
f := \b. case b {
  True => consumeA;
  False => consumeB
}
`
	checkSource(t, source, nil)
}

// --- Branch join with heterogeneous multiplicities via LUB ---

func TestDivergentPostLUBHeterogeneousMult(t *testing.T) {
	// Branch True:  post = { h: Unit @Linear }
	// Branch False: post = { h: Unit @Affine }
	// MultJoin(Linear, Affine) = Linear → joined post has h @Linear.
	source := `
form Mult := { Unrestricted: (); Affine: (); Linear: (); }
type MultJoin :: Mult := \(m1: Mult) (m2: Mult). case (m1, m2) {
  (Linear, _) => Linear;
  (_, Linear) => Linear;
  (Affine, _) => Affine;
  (_, Affine) => Affine;
  (Unrestricted, Unrestricted) => Unrestricted
}
type MultCompose :: Mult := MultJoin
type MultDrop :: Mult := Unrestricted
form GradeAlgebra := \(g: Kind). {
  type GradeJoin :: g -> g -> g;
  type GradeCompose :: g -> g -> g;
  type GradeDrop :: g;
  type GradeUnit :: g
}
impl GradeAlgebra Mult := {
  type GradeJoin := MultJoin;
  type GradeCompose := MultCompose;
  type GradeDrop := Unrestricted;
  type GradeUnit := Linear
}
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
makeLinear :: Computation { h: Unit @Affine } { h: Unit @Linear } Unit
makeLinear := assumption
noop :: Computation { h: Unit @Affine } { h: Unit @Affine } Unit
noop := assumption
f :: Bool -> Computation { h: Unit @Affine } { h: Unit @Linear } Unit
f := \b. case b {
  True => makeLinear;
  False => noop
}
`
	checkSource(t, source, nil)
}

func TestDivergentPostLUBSymmetric(t *testing.T) {
	// MultJoin(Affine, Linear) = Linear (symmetric with above).
	source := `
form Mult := { Unrestricted: (); Affine: (); Linear: (); }
type MultJoin :: Mult := \(m1: Mult) (m2: Mult). case (m1, m2) {
  (Linear, _) => Linear;
  (_, Linear) => Linear;
  (Affine, _) => Affine;
  (_, Affine) => Affine;
  (Unrestricted, Unrestricted) => Unrestricted
}
type MultCompose :: Mult := MultJoin
form GradeAlgebra := \(g: Kind). {
  type GradeJoin :: g -> g -> g;
  type GradeCompose :: g -> g -> g;
  type GradeDrop :: g;
  type GradeUnit :: g
}
impl GradeAlgebra Mult := {
  type GradeJoin := MultJoin;
  type GradeCompose := MultCompose;
  type GradeDrop := Unrestricted;
  type GradeUnit := Linear
}
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
makeAffine :: Computation { h: Unit @Unrestricted } { h: Unit @Affine } Unit
makeAffine := assumption
makeLinear :: Computation { h: Unit @Unrestricted } { h: Unit @Linear } Unit
makeLinear := assumption
f :: Bool -> Computation { h: Unit @Unrestricted } { h: Unit @Linear } Unit
f := \b. case b {
  True => makeAffine;
  False => makeLinear
}
`
	checkSource(t, source, nil)
}

func TestDivergentPostOneSidedMult(t *testing.T) {
	// One branch has @Linear, other has no annotation.
	// Result takes the annotation (more restrictive).
	source := `
form Mult := { Unrestricted: (); Affine: (); Linear: (); }
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
makeLinear :: Computation { h: Unit } { h: Unit @Linear } Unit
makeLinear := assumption
noop :: Computation { h: Unit } { h: Unit } Unit
noop := assumption
f :: Bool -> Computation { h: Unit } { h: Unit @Linear } Unit
f := \b. case b {
  True => makeLinear;
  False => noop
}
`
	checkSource(t, source, nil)
}

// --- Typehelpers with Mult ---

func TestTypehelpersWithMult(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{
			"Int":    types.TypeOfTypes,
			"Linear": types.TypeOfTypes,
		},
	}
	source := `
form Mult := { Unrestricted: (); Affine: (); Linear: (); }
f :: { x: Int @Linear } -> Int
f := \r. 0
`
	checkSource(t, source, config)
}
