// Do-monadic elaboration tests — IxMonad-only do dispatch, Monad pure rewriting.
// Does NOT cover: threading (elaborate_do_threading_test.go), multiplicity (elaborate_do_mult_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Finding 1: IxMonad-only types must work with do notation ---

func TestDoIxMonadOnlyType(t *testing.T) {
	// A type with IxMonad but no Monad instance should support do notation.
	// Before fix: ErrNoInstance because Monad was required.
	source := `
data Unit := { Unit: Unit; }
data IxMonad := \(m: Row -> Row -> Type -> Type). {
  ixpure: \a (r: Row). a -> m r r a;
  ixbind: \a b (r1: Row) (r2: Row) (r3: Row). m r1 r2 a -> (a -> m r2 r3 b) -> m r1 r3 b
}
data MyIx := \(pre: Row) (post: Row) a. { MkMyIx: a -> MyIx pre post a; }
_myIxPure :: \a (r: Row). a -> MyIx r r a
_myIxPure := assumption
_myIxBind :: \a b (r1: Row) (r2: Row) (r3: Row). MyIx r1 r2 a -> (a -> MyIx r2 r3 b) -> MyIx r1 r3 b
_myIxBind := assumption
impl IxMonad MyIx := {
  ixpure := _myIxPure;
  ixbind := _myIxBind
}
main :: MyIx {} {} Unit
main := do { ixpure Unit }
`
	checkSource(t, source, nil)
}

func TestDoIxMonadOnlyMultiStmt(t *testing.T) {
	// Multi-statement do block with IxMonad-only type.
	source := `
data Unit := { Unit: Unit; }
data IxMonad := \(m: Row -> Row -> Type -> Type). {
  ixpure: \a (r: Row). a -> m r r a;
  ixbind: \a b (r1: Row) (r2: Row) (r3: Row). m r1 r2 a -> (a -> m r2 r3 b) -> m r1 r3 b
}
data MyIx := \(pre: Row) (post: Row) a. { MkMyIx: a -> MyIx pre post a; }
_myIxPure :: \a (r: Row). a -> MyIx r r a
_myIxPure := assumption
_myIxBind :: \a b (r1: Row) (r2: Row) (r3: Row). MyIx r1 r2 a -> (a -> MyIx r2 r3 b) -> MyIx r1 r3 b
_myIxBind := assumption
impl IxMonad MyIx := {
  ixpure := _myIxPure;
  ixbind := _myIxBind
}
step :: MyIx {} {} Unit
step := assumption
main :: MyIx {} {} Unit
main := do { step; ixpure Unit }
`
	checkSource(t, source, nil)
}

// --- Finding 2: Monad do rewriting must cover mid-statement pure ---

func TestDoMonadMidStatementPureRewrite(t *testing.T) {
	// do { pure 42; pure 0 } :: Maybe Int
	// Before fix: mid-statement `pure 42` was not rewritten to `mpure 42`,
	// causing a type mismatch (Computation vs Maybe).
	source := `
data Maybe := \a. { Just: a -> Maybe a; Nothing: Maybe a; }
data Monad := \(m: Type -> Type). {
  mpure: \a. a -> m a;
  mbind: \a b. m a -> (a -> m b) -> m b
}
impl Monad Maybe := {
  mpure := Just;
  mbind := \ma f. case ma { Nothing => Nothing; Just a => f a }
}
main :: Maybe Int
main := do { pure 42; pure 0 }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

func TestDoMonadBindPureRewrite(t *testing.T) {
	// do { x <- pure 42; mpure x } :: Maybe Int
	// Before fix: `pure 42` in StmtBind was not rewritten to `mpure 42`.
	source := `
data Maybe := \a. { Just: a -> Maybe a; Nothing: Maybe a; }
data Monad := \(m: Type -> Type). {
  mpure: \a. a -> m a;
  mbind: \a b. m a -> (a -> m b) -> m b
}
impl Monad Maybe := {
  mpure := Just;
  mbind := \ma f. case ma { Nothing => Nothing; Just a => f a }
}
main :: Maybe Int
main := do { x <- pure 42; mpure x }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}
