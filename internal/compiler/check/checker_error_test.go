// Checker error tests — unbound var/con, bad application/computation/thunk, skolem escape/rigid, duplicate labels.
// Does NOT cover: type family errors (type_family_test.go), instance errors (instance_test.go).

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func TestCheckUnboundVar(t *testing.T) {
	checkSourceExpectCode(t, "main := undefined_var", nil, diagnostic.ErrUnboundVar)
}

// --- Error code coverage tests ---

func TestErrorUnboundCon(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
main := case True { Foo=> True; _ => False }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrUnboundCon)
}

func TestErrorBadApplication(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
main := True True`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadApplication)
}

func TestErrorBadComputation(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
main := do { x <- True; pure x }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadComputation)
}

func TestErrorBadThunk(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
main := force True`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadThunk)
}

func TestErrorSpecialForm(t *testing.T) {
	// thunk and force remain special forms.
	source := `main := thunk`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrSpecialForm)
}

func TestErrorDuplicateLabel(t *testing.T) {
	// Trigger unify.UnifyDupLabel via the unifier's label context mechanism:
	// a row meta with label context {x} solved to a row containing x.
	u := unify.NewUnifier(&types.TypeOps{})
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfRows}
	// Register label context: the meta is the tail of a row with field "x".
	u.RegisterLabelContext(m.ID, map[string]struct{}{"x": {}})
	// Solve the meta to a row that also contains "x" → duplicate.
	row := types.ClosedRow(types.RowField{Label: "x", Type: types.MkCon("Int")})
	err := u.Unify(m, row)
	if err == nil {
		t.Fatal("expected duplicate label error, got nil")
	}
	ue, ok := err.(unify.UnifyError)
	if !ok {
		t.Fatalf("expected UnifyError, got %T: %v", err, err)
	}
	if ue.Kind() != unify.UnifyDupLabel {
		t.Errorf("expected unify.UnifyDupLabel, got %v: %s", ue.Kind(), ue.Error())
	}
}

func TestErrorDuplicateLabelEvidenceRow(t *testing.T) {
	// Same as TestErrorDuplicateLabel but for TyEvidenceRow (capability entries).
	u := unify.NewUnifier(&types.TypeOps{})
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfRows}
	u.RegisterLabelContext(m.ID, map[string]struct{}{"x": {}})
	evRow := types.ClosedRow(types.RowField{Label: "x", Type: types.MkCon("Int")})
	err := u.Unify(m, evRow)
	if err == nil {
		t.Fatal("expected duplicate label error for evidence row, got nil")
	}
	ue, ok := err.(unify.UnifyError)
	if !ok {
		t.Fatalf("expected UnifyError, got %T: %v", err, err)
	}
	if ue.Kind() != unify.UnifyDupLabel {
		t.Errorf("expected unify.UnifyDupLabel, got %v: %s", ue.Kind(), ue.Error())
	}
}

func TestErrorOccursCheck(t *testing.T) {
	source := `main := \x. x x`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrOccursCheck)
}

func TestErrorEmptyDo(t *testing.T) {
	source := `main := do {}`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrEmptyDo)
}

func TestErrorBadDoEnding(t *testing.T) {
	source := `main := do { x <- pure 1 }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadDoEnding)
}

func TestErrorBadClass(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
impl Phantom Bool := { foo := \x. x }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadClass)
}

func TestErrorMissingMethod(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := {}`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrMissingMethod)
}

func TestErrorSkolemEscape(t *testing.T) {
	// Existential type variable escapes via GADT pattern match:
	// MkExists packs an existential 'a'; extracting it leaks 'a' into the result.
	source := `form Exists := { MkExists: \ a. a -> Exists }
bad := \e. case e { MkExists x => x }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrSkolemEscape)
}

func TestErrorSkolemRigid(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
main :: \ a b. a -> b
main := \x. x`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrSkolemRigid)
}

// --- Regression tests (v0.16.3–v0.16.5) ---

func TestErrorListLiteralRequiresPrelude(t *testing.T) {
	errMsg := checkSourceExpectCode(t, "main := [1, 2, 3]", nil, diagnostic.ErrUnboundCon)
	if !strings.Contains(strings.ToLower(errMsg), "prelude") && !strings.Contains(strings.ToLower(errMsg), "list literal") {
		t.Errorf("expected error mentioning Prelude or list literal, got: %s", errMsg)
	}
}

func TestErrorFixRequiresRecursionHint(t *testing.T) {
	errMsg := checkSourceExpectCode(t, `main := fix (\self x. self x) 1`, nil, diagnostic.ErrUnboundVar)
	if !strings.Contains(strings.ToLower(errMsg), "recursion") {
		t.Errorf("expected error mentioning recursion, got: %s", errMsg)
	}
}

func TestErrorTupleExhaustivenessIncomplete(t *testing.T) {
	source := `form AB := { A: AB; B: AB; }
main := \x y. case (x, y) { (A, A) => 1; (B, B) => 2 }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
}

func TestErrorDuplicateValueBinding(t *testing.T) {
	source := `f := 1
f := 2
main := f`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrDuplicateDecl)
}
