// Do-monadic elaboration tests — IxMonad/GIMonad do dispatch, Monad pure rewriting.
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
form Unit := { Unit: Unit; }
form IxMonad := \(m: Row -> Row -> Type -> Type). {
  ixpure: \a (r: Row). a -> m r r a;
  ixbind: \a b (r1: Row) (r2: Row) (r3: Row). m r1 r2 a -> (a -> m r2 r3 b) -> m r1 r3 b
}
form MyIx := \(pre: Row) (post: Row) a. { MkMyIx: a -> MyIx pre post a; }
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
form Unit := { Unit: Unit; }
form IxMonad := \(m: Row -> Row -> Type -> Type). {
  ixpure: \a (r: Row). a -> m r r a;
  ixbind: \a b (r1: Row) (r2: Row) (r3: Row). m r1 r2 a -> (a -> m r2 r3 b) -> m r1 r3 b
}
form MyIx := \(pre: Row) (post: Row) a. { MkMyIx: a -> MyIx pre post a; }
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
form Maybe := \a. { Just: a -> Maybe a; Nothing: Maybe a; }
form Monad := \(m: Type -> Type). {
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
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

func TestDoMonadBindPureRewrite(t *testing.T) {
	// do { x <- pure 42; mpure x } :: Maybe Int
	// Before fix: `pure 42` in StmtBind was not rewritten to `mpure 42`.
	source := `
form Maybe := \a. { Just: a -> Maybe a; Nothing: Maybe a; }
form Monad := \(m: Type -> Type). {
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
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

// --- GIMonad: graded indexed monad do dispatch ---

// giMonadPreamble defines a minimal GIMonad setup for type-checking tests.
// Uses a simplified GIMonad without GradeAlgebra super constraint to isolate
// do-block dispatch testing from associated type family reduction.
const giMonadPreamble = `
form Unit := { Unit: Unit; }

form MyGrade := { Low: MyGrade; High: MyGrade; }

form GIMonad := \(g: Kind) (m: g -> Row -> Row -> Type -> Type). {
  gipure: \a (r: Row). a -> m Low r r a;
  gibind: \a b (e1: g) (e2: g) (r1: Row) (r2: Row) (r3: Row).
              m e1 r1 r2 a -> (a -> m e2 r2 r3 b) -> m e1 r1 r3 b
}

form MyGM := \(e: MyGrade) (pre: Row) (post: Row) a.
  { MkMyGM: a -> MyGM e pre post a; }

_gmPure :: \a (r: Row). a -> MyGM Low r r a
_gmPure := assumption
_gmBind :: \a b (e1: MyGrade) (e2: MyGrade) (r1: Row) (r2: Row) (r3: Row).
           MyGM e1 r1 r2 a -> (a -> MyGM e2 r2 r3 b) -> MyGM e1 r1 r3 b
_gmBind := assumption

impl GIMonad MyGrade MyGM := {
  gipure := _gmPure;
  gibind := _gmBind
}
`

func TestDoGIMonadPure(t *testing.T) {
	source := giMonadPreamble + `
main :: MyGM Low {} {} Unit
main := do { pure Unit }
`
	checkSource(t, source, nil)
}

func TestDoGIMonadMultiStmt(t *testing.T) {
	source := giMonadPreamble + `
step :: MyGM Low {} {} Unit
step := assumption
main :: MyGM Low {} {} Unit
main := do { step; pure Unit }
`
	checkSource(t, source, nil)
}

func TestDoGIMonadBind(t *testing.T) {
	source := giMonadPreamble + `
step :: MyGM Low {} {} Unit
step := assumption
main :: MyGM Low {} {} Unit
main := do { x <- step; pure x }
`
	checkSource(t, source, nil)
}

// --- GIMonad with GradeAlgebra superclass (real definition) ---

// realGIMonadPreamble uses the full GIMonad definition with GradeAlgebra
// superclass constraint and associated type families. This tests that
// saturateAssocFamilies correctly reduces GradeDrop/GradeCompose in
// instance method types.
const realGIMonadPreamble = `
form Unit := { Unit: Unit; }

form MyGrade := { Low: MyGrade; High: MyGrade; }

-- Trivial grade families for testing. The specific reduction behavior
-- does not matter for instance body checking — what matters is that
-- GradeDrop/GradeCompose resolve from the GradeAlgebra instance.
type MyGradeJoin :: MyGrade -> MyGrade -> MyGrade :=
  \(a: MyGrade) (b: MyGrade). a
type MyGradeCompose :: MyGrade -> MyGrade -> MyGrade :=
  \(a: MyGrade) (b: MyGrade). a

form GradeAlgebra := \(g: Kind). {
  type GradeJoin :: g -> g -> g;
  type GradeCompose :: g -> g -> g;
  type GradeDrop :: g
}

impl GradeAlgebra MyGrade := {
  type GradeJoin := MyGradeJoin;
  type GradeCompose := MyGradeCompose;
  type GradeDrop := Low
}

form GIMonad := \(g: Kind) (m: g -> Row -> Row -> Type -> Type). GradeAlgebra g => {
  gipure: \a (r: Row). a -> m GradeDrop r r a;
  gibind: \a b (e1: g) (e2: g) (r1: Row) (r2: Row) (r3: Row).
              m e1 r1 r2 a -> (a -> m e2 r2 r3 b) -> m (GradeCompose e1 e2) r1 r3 b
}

form MyGM := \(e: MyGrade) (pre: Row) (post: Row) a.
  { MkMyGM: a -> MyGM e pre post a; }

_gmPure :: \a (r: Row). a -> MyGM Low r r a
_gmPure := assumption
-- MyGradeCompose e1 e2 reduces to e1 (trivial family), so gibind's
-- result grade becomes e1 after type family reduction.
_gmBind :: \a b (e1: MyGrade) (e2: MyGrade) (r1: Row) (r2: Row) (r3: Row).
           MyGM e1 r1 r2 a -> (a -> MyGM e2 r2 r3 b) -> MyGM e1 r1 r3 b
_gmBind := assumption

impl GIMonad MyGrade MyGM := {
  gipure := _gmPure;
  gibind := _gmBind
}
`

func TestDoRealGIMonadPure(t *testing.T) {
	source := realGIMonadPreamble + `
main :: MyGM Low {} {} Unit
main := do { pure Unit }
`
	checkSource(t, source, nil)
}

func TestDoRealGIMonadBind(t *testing.T) {
	source := realGIMonadPreamble + `
step :: MyGM Low {} {} Unit
step := assumption
main :: MyGM Low {} {} Unit
main := do { x <- step; pure x }
`
	checkSource(t, source, nil)
}
