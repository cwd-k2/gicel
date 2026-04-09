// UnifyError variants — structured representation of unification failures.
//
// The previous design used a single `*UnifyError` struct with 12 fields,
// where field validity depended on the `Kind` enum. Multiple Kind values
// shared overlapping fields (e.g. `Name` meant skolem name OR class name
// depending on Kind; `Label` meant grade label OR a level-mismatch string),
// which produced an implicit "tagged union via field convention" — a shape
// the type system could not enforce.
//
// This file replaces that struct with an interface + 10 concrete variants,
// matching the variant pattern already used by `eval.Value` (HostVal,
// ConVal, PAPVal, PrimVal, VMClosure, VMThunkVal). Each variant carries
// only the fields that are valid for its case; field overloading is gone.
//
// The `Kind() UnifyErrorKind` method preserves the existing diagnostic
// classification path (`bidir.go:unifyErrorCode`) — callers that switch on
// the kind keep working unchanged. The `Error() string` method on each
// variant produces the same human-readable text the legacy switch produced,
// so embedder error messages are unchanged.

package unify

import (
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// UnifyError is the interface implemented by every unification failure.
// Concrete implementations are the variant types defined in this file.
type UnifyError interface {
	error
	Kind() UnifyErrorKind
}

// MismatchError is a general type mismatch where both sides are known.
// This is the case `bidir.go:addUnifyError` treats specially: when both
// types are known the diagnostic context already conveys the expectation,
// so the unifier message is suppressed to avoid duplication.
type MismatchError struct {
	A, B types.Type
}

func (e *MismatchError) Kind() UnifyErrorKind { return UnifyMismatch }
func (e *MismatchError) Error() string {
	return "type mismatch: " + types.Pretty(e.A) + " vs " + types.Pretty(e.B)
}

// GradeMismatchError reports a grade count mismatch for a specific row label.
// Used when two row fields share a label but disagree on the number of grade
// annotations attached to the field type.
type GradeMismatchError struct {
	Label  string
	CountA int
	CountB int
}

func (e *GradeMismatchError) Kind() UnifyErrorKind { return UnifyMismatch }
func (e *GradeMismatchError) Error() string {
	return "grade count mismatch for label " + strconv.Quote(e.Label) +
		": " + strconv.Itoa(e.CountA) + " vs " + strconv.Itoa(e.CountB)
}

// LevelMismatchError reports a universe level mismatch. The two LevelExpr
// values are pre-formatted at construction site so the variant carries only
// the rendered strings — `level_unify.go` is the sole producer.
//
// This variant fixes the historical `Label` field overloading: the legacy
// struct stored "<lhs> vs <rhs>" in `Label`, conflating it with row labels.
type LevelMismatchError struct {
	A, B string
}

func (e *LevelMismatchError) Kind() UnifyErrorKind { return UnifyMismatch }
func (e *LevelMismatchError) Error() string {
	return "level mismatch: " + e.A + " vs " + e.B
}

// MessageError carries a free-form message for unification failures that
// don't fit a structured shape (cross-fiber row mismatch, normalize errors
// surfaced from `types.NormalizeRow`, unknown evidence fiber).
type MessageError struct {
	Message string
}

func (e *MessageError) Kind() UnifyErrorKind { return UnifyMismatch }
func (e *MessageError) Error() string        { return e.Message }

// OccursError reports an occurs check failure (infinite type or infinite
// level). `Type` is non-nil for term-level occurs checks; nil for level
// occurs checks (where only the meta ID is meaningful).
type OccursError struct {
	MetaID  int
	Type    types.Type // nil for level occurs check
	IsLevel bool       // discriminates "infinite type" vs "infinite level"
}

func (e *OccursError) Kind() UnifyErrorKind { return UnifyOccursCheck }
func (e *OccursError) Error() string {
	if e.IsLevel {
		return "infinite level: level variable occurs in itself"
	}
	return "infinite type: inferred type occurs in " + types.Pretty(e.Type)
}

// DupLabelError reports a duplicate label in a row.
type DupLabelError struct {
	Label string
}

func (e *DupLabelError) Kind() UnifyErrorKind { return UnifyDupLabel }
func (e *DupLabelError) Error() string {
	return "duplicate label " + strconv.Quote(e.Label) + " in row"
}

// ClassArgCountError reports that two constraint entries for the same class
// disagree on argument count.
//
// This variant fixes one half of the historical `Name` field overloading:
// the legacy struct stored the class name in `Name`, conflating it with
// the skolem name used by SkolemRigidError.
type ClassArgCountError struct {
	ClassName string
	CountA    int
	CountB    int
}

func (e *ClassArgCountError) Kind() UnifyErrorKind { return UnifyRowMismatch }
func (e *ClassArgCountError) Error() string {
	return "constraint arg count mismatch: " + e.ClassName +
		" has " + strconv.Itoa(e.CountA) + " args vs " + strconv.Itoa(e.CountB)
}

// RowMismatchError reports a row structure mismatch — extra entries on one
// or both sides that cannot be reconciled (closed row, missing tail).
// `Labels` carries the names of unmatched fields when known.
type RowMismatchError struct {
	CountA int
	CountB int
	Labels []string
}

func (e *RowMismatchError) Kind() UnifyErrorKind { return UnifyRowMismatch }
func (e *RowMismatchError) Error() string {
	if e.CountA > 0 && e.CountB > 0 {
		return "row mismatch: extra entries (left=" + strconv.Itoa(e.CountA) +
			", right=" + strconv.Itoa(e.CountB) + ")"
	}
	detail := strconv.Itoa(e.CountA + e.CountB)
	if len(e.Labels) > 0 {
		detail = strings.Join(e.Labels, ", ")
	}
	return "record has unmatched field(s): " + detail
}

// SkolemRigidError reports a unification attempt against a rigid (skolem)
// type variable. `Other` is the non-skolem side; `SkolemOnLeft` indicates
// which side of the original Unify call the skolem appeared on, so the
// error message can mirror caller intuition.
//
// This variant fixes the other half of the historical `Name` field
// overloading: the legacy struct stored the skolem name in `Name`,
// conflating it with the class name used by ClassArgCountError.
type SkolemRigidError struct {
	SkolemName   string
	SkolemID     int
	Other        types.Type
	SkolemOnLeft bool
}

func (e *SkolemRigidError) Kind() UnifyErrorKind { return UnifySkolemRigid }
func (e *SkolemRigidError) Error() string {
	skolemStr := "#" + e.SkolemName
	// Disambiguate when the other side is also a skolem with the same name
	// (e.g. nested forall scopes binding the same variable name).
	if other, ok := e.Other.(*types.TySkolem); ok && other.Name == e.SkolemName {
		skolemStr += "_" + strconv.Itoa(e.SkolemID)
		otherStr := "#" + other.Name + "_" + strconv.Itoa(other.ID)
		if e.SkolemOnLeft {
			return "cannot unify rigid type variable " + skolemStr + " with " + otherStr
		}
		return "cannot unify " + otherStr + " with rigid type variable " + skolemStr
	}
	if e.SkolemOnLeft {
		return "cannot unify rigid type variable " + skolemStr + " with " + types.Pretty(e.Other)
	}
	return "cannot unify " + types.Pretty(e.Other) + " with rigid type variable " + skolemStr
}

// UntouchableMetaError reports an attempt to solve a metavariable from a
// solver level deeper than where the meta was created. Skolemization rules
// require this to fail (DK §6: implication scoping).
type UntouchableMetaError struct {
	MetaID int
	Level  int // meta's creation level
	SLevel int // current solver level
}

func (e *UntouchableMetaError) Kind() UnifyErrorKind { return UnifyUntouchable }
func (e *UntouchableMetaError) Error() string {
	return "untouchable meta ?" + strconv.Itoa(e.MetaID) +
		" (level " + strconv.Itoa(e.Level) + ") at solver level " + strconv.Itoa(e.SLevel)
}

// Compile-time interface conformance checks. If a variant accidentally
// drops a required method, this fails to compile rather than at runtime.
var (
	_ UnifyError = (*MismatchError)(nil)
	_ UnifyError = (*GradeMismatchError)(nil)
	_ UnifyError = (*LevelMismatchError)(nil)
	_ UnifyError = (*MessageError)(nil)
	_ UnifyError = (*OccursError)(nil)
	_ UnifyError = (*DupLabelError)(nil)
	_ UnifyError = (*ClassArgCountError)(nil)
	_ UnifyError = (*RowMismatchError)(nil)
	_ UnifyError = (*SkolemRigidError)(nil)
	_ UnifyError = (*UntouchableMetaError)(nil)
)
