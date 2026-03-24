//go:build probe

// Case/if-then-else probe tests — do-block branch post-state unification.
// Does NOT cover: exhaustiveness (bidir_case.go), divergent post-states (elaborate_do_divergent_post_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// intStringConfig returns a CheckConfig with Int and String as ground types.
func intStringConfig() *CheckConfig {
	return &CheckConfig{
		RegisteredTypes: map[string]types.Kind{
			"Int":    types.KType{},
			"String": types.KType{},
		},
	}
}

// =============================================================================
// Core scenario: if-then-else with effectful do-blocks inside annotated lambda.
//
// Reported: post-state unification failure when checkCaseAlts creates fresh
// post-state metas for each branch and the do-block's internal bind threading
// produces compound types that don't properly unify with the case handler's
// fresh post-state.
//
// Regression guard: these tests verify the interaction between case branch
// post-state freshening and do-block checked-mode elaboration.
// =============================================================================

// TestProbe_CaseDoPostState_AnnotatedLambda — the primary reproduction case.
// Lambda with explicit Computation annotation, if-then-else with do-blocks
// in both branches performing get/put operations.
func TestProbe_CaseDoPostState_AnnotatedLambda(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
get :: Computation { state: Int } { state: Int } Int
get := assumption
put :: Int -> Computation { state: Int } { state: Int } ()
put := assumption
test :: Bool -> Computation { state: Int } { state: Int } String
test := \flag. if flag
  then do { x <- get; put x; pure "a" }
  else do { put 2; pure "b" }
`
	checkSource(t, source, intStringConfig())
}

// TestProbe_CaseDoPostState_UnannotatedLambda — the counterpart that should
// also work (infer mode). Both branches produce the same Computation type.
func TestProbe_CaseDoPostState_UnannotatedLambda(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
get :: Computation { state: Int } { state: Int } Int
get := assumption
put :: Int -> Computation { state: Int } { state: Int } ()
put := assumption
test := \flag. if flag
  then do { x <- get; put x; pure "a" }
  else do { put 2; pure "b" }
`
	checkSource(t, source, intStringConfig())
}

// TestProbe_CaseDoPostState_InsideDo — if-then-else nested inside a do-block,
// which provides the Computation context from outside.
func TestProbe_CaseDoPostState_InsideDo(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
get :: Computation { state: Int } { state: Int } Int
get := assumption
put :: Int -> Computation { state: Int } { state: Int } ()
put := assumption
main :: Computation { state: Int } { state: Int } String
main := do {
  flag := True;
  if flag
    then do { x <- get; put x; pure "a" }
    else do { put 2; pure "b" }
}
`
	checkSource(t, source, intStringConfig())
}

// TestProbe_CaseDoPostState_ExplicitCase — same scenario using explicit case
// syntax instead of if-then-else sugar.
func TestProbe_CaseDoPostState_ExplicitCase(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
get :: Computation { state: Int } { state: Int } Int
get := assumption
put :: Int -> Computation { state: Int } { state: Int } ()
put := assumption
test :: Bool -> Computation { state: Int } { state: Int } String
test := \flag. case flag {
  True => do { x <- get; put x; pure "a" };
  False => do { put 2; pure "b" }
}
`
	checkSource(t, source, intStringConfig())
}

// TestProbe_CaseDoPostState_AsymmetricBindCount — branches with different
// numbers of bind operations. The post-state freshening must handle both
// branches independently.
func TestProbe_CaseDoPostState_AsymmetricBindCount(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
get :: Computation { state: Int } { state: Int } Int
get := assumption
put :: Int -> Computation { state: Int } { state: Int } ()
put := assumption
test :: Bool -> Computation { state: Int } { state: Int } Int
test := \flag. if flag
  then do { a <- get; _ <- get; put a; pure a }
  else do { put 42; get }
`
	checkSource(t, source, intStringConfig())
}

// TestProbe_CaseDoPostState_NestedIf — nested if-then-else where the outer
// if is inside a do-block and the inner if has do-blocks in branches.
func TestProbe_CaseDoPostState_NestedIf(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
get :: Computation { state: Int } { state: Int } Int
get := assumption
put :: Int -> Computation { state: Int } { state: Int } ()
put := assumption
test :: Bool -> Bool -> Computation { state: Int } { state: Int } Int
test := \a. \b. do {
  x <- get;
  if a
    then do {
      put x;
      if b
        then do { y <- get; pure y }
        else do { pure x }
    }
    else do { put x; get }
}
`
	checkSource(t, source, intStringConfig())
}

// TestProbe_CaseDoPostState_PureOnlyBranch — one branch is a do-block with
// state operations, the other is just pure (no state ops). The post-state
// should still unify correctly via pure's r→r identity.
func TestProbe_CaseDoPostState_PureOnlyBranch(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
get :: Computation { state: Int } { state: Int } Int
get := assumption
put :: Int -> Computation { state: Int } { state: Int } ()
put := assumption
test :: Bool -> Computation { state: Int } { state: Int } String
test := \flag. if flag
  then do { x <- get; put x; pure "done" }
  else pure "skip"
`
	checkSource(t, source, intStringConfig())
}

// TestProbe_CaseDoPostState_MultipleEffects — annotation with multiple
// effect labels; only state is used in do-blocks.
func TestProbe_CaseDoPostState_MultipleEffects(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
get :: Computation { state: Int, log: Unit } { state: Int, log: Unit } Int
get := assumption
put :: Int -> Computation { state: Int, log: Unit } { state: Int, log: Unit } ()
put := assumption
test :: Bool -> Computation { state: Int, log: Unit } { state: Int, log: Unit } String
test := \flag. if flag
  then do { x <- get; put x; pure "a" }
  else do { put 2; pure "b" }
`
	checkSource(t, source, intStringConfig())
}

// TestProbe_CaseDoPostState_RowPolymorphicOps — get/put with open row tails,
// annotation with concrete row. Verifies row unification through case branches.
func TestProbe_CaseDoPostState_RowPolymorphicOps(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
get :: \(r: Row). Computation { state: Int | r } { state: Int | r } Int
get := assumption
put :: \(r: Row). Int -> Computation { state: Int | r } { state: Int | r } ()
put := assumption
test :: Bool -> Computation { state: Int } { state: Int } String
test := \flag. if flag
  then do { x <- get; put x; pure "a" }
  else do { put 2; pure "b" }
`
	checkSource(t, source, intStringConfig())
}

// TestProbe_CaseDoPostState_DifferentPrePost — annotation where pre and post
// states differ. Each branch must thread from pre to post correctly.
func TestProbe_CaseDoPostState_DifferentPrePost(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
consume :: Computation { handle: Unit, flag: Unit } { flag: Unit } Unit
consume := assumption
noop :: Computation { handle: Unit, flag: Unit } { handle: Unit, flag: Unit } Unit
noop := assumption
test :: Bool -> Computation { handle: Unit, flag: Unit } { flag: Unit } Unit
test := \b. if b
  then consume
  else do { noop; consume }
`
	checkSource(t, source, nil)
}

// TestProbe_CaseDoPostState_ThreeBranches — case with three branches (ADT),
// each with a do-block. Ensures lubPostStates works with > 2 branches.
func TestProbe_CaseDoPostState_ThreeBranches(t *testing.T) {
	source := `
form Color := { Red: Color; Green: Color; Blue: Color; }
get :: Computation { state: Int } { state: Int } Int
get := assumption
put :: Int -> Computation { state: Int } { state: Int } ()
put := assumption
test :: Color -> Computation { state: Int } { state: Int } String
test := \c. case c {
  Red   => do { put 1; pure "red" };
  Green => do { x <- get; put x; pure "green" };
  Blue  => do { _ <- get; put 0; pure "blue" }
}
`
	checkSource(t, source, intStringConfig())
}

// TestProbe_CaseDoPostState_ReturnFromGet — branches where the last operation
// is get (not pure), ensuring the post-state from get propagates correctly.
func TestProbe_CaseDoPostState_ReturnFromGet(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
get :: Computation { state: Int } { state: Int } Int
get := assumption
put :: Int -> Computation { state: Int } { state: Int } ()
put := assumption
test :: Bool -> Computation { state: Int } { state: Int } Int
test := \flag. if flag
  then do { put 1; get }
  else do { put 2; get }
`
	checkSource(t, source, intStringConfig())
}
