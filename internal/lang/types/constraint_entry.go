// ConstraintEntry variants — structured representation of constraint row entries.
//
// The previous design used a single `ConstraintEntry` struct with 8 fields
// whose validity depended on implicit convention: ClassName/Args were unused
// when IsEquality was set, ConstraintVar was unused on plain class entries,
// and Quantified coexisted with ClassName/Args as a "cached head" duplication
// that no type rule enforced. The same anti-pattern the unifier outgrew in
// commit 7c2ddb2 (UnifyError → 10 variant types).
//
// This file replaces the struct with an interface + four concrete variants,
// matching the variant pattern already used by eval.Value, unify.UnifyError,
// and types.EvidenceEntries. Each variant carries only the fields that are
// valid for its case; field overloading is gone.
//
// Variants:
//
//	*ClassEntry           Eq a, Functor f a — simple class constraint
//	*EqualityEntry        a ~ Int, F xs ~ List Int — type-level equality
//	*VarEntry             c : Constraint — unresolved constraint-kinded variable
//	*QuantifiedConstraint forall a. Ctx => Head — universally quantified
//
// QuantifiedConstraint keeps its historical name for continuity with the
// resolver and the solve package, and now implements ConstraintEntry
// directly (no wrapper type). Its Head is typed as *ClassEntry to capture
// the invariant that every quantified constraint has a class-constraint
// head — enforced structurally, not by convention.
//
// The VarEntry → ClassEntry transition happens during zonking: when a
// constraint-kinded metavariable resolves to a concrete TyCon head, the
// zonker returns a *ClassEntry in place of the *VarEntry. No hybrid state
// — the entry is one variant or the other at every observable moment.

package types

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// ConstraintEntry is the sealed interface implemented by every kind of
// constraint row entry. Concrete implementations are the four variant
// types defined in this file.
type ConstraintEntry interface {
	constraintEntryNode()
	EntrySpan() span.Span
}

// ClassEntry is a simple type-class constraint: `Eq a`, `Functor f a`.
type ClassEntry struct {
	ClassName string
	Args      []Type
	S         span.Span
}

func (*ClassEntry) constraintEntryNode()   {}
func (e *ClassEntry) EntrySpan() span.Span { return e.S }

// EqualityEntry is a type-level equality constraint: `a ~ Int`,
// `F xs ~ List Int`. Surface syntax: `a ~ T => Body`.
type EqualityEntry struct {
	Lhs, Rhs Type
	S        span.Span
}

func (*EqualityEntry) constraintEntryNode()   {}
func (e *EqualityEntry) EntrySpan() span.Span { return e.S }

// VarEntry is an unresolved constraint-kinded variable: `c : Constraint`.
// During zonking, if Var resolves to a concrete application with a TyCon
// head, the zonker returns a *ClassEntry in place of the VarEntry — there
// is no "resolved VarEntry" state.
type VarEntry struct {
	Var Type
	S   span.Span
}

func (*VarEntry) constraintEntryNode()   {}
func (e *VarEntry) EntrySpan() span.Span { return e.S }

// QuantifiedConstraint is a universally quantified constraint:
//
//	forall vars. context => head
//
// Evidence for this constraint is a function from context dicts to a
// head dict. Head is typed as *ClassEntry because every quantified
// constraint has a class-constraint head by construction (resolver and
// class-header elaborator enforce this). Context entries are allowed to
// be any ConstraintEntry variant so equality premises remain expressible.
type QuantifiedConstraint struct {
	Vars    []ForallBinder
	Context []ConstraintEntry
	Head    *ClassEntry
	S       span.Span
}

func (*QuantifiedConstraint) constraintEntryNode()    {}
func (qc *QuantifiedConstraint) EntrySpan() span.Span { return qc.S }

// Compile-time interface conformance checks. If a variant accidentally
// drops a required method, this fails to compile rather than at runtime.
var (
	_ ConstraintEntry = (*ClassEntry)(nil)
	_ ConstraintEntry = (*EqualityEntry)(nil)
	_ ConstraintEntry = (*VarEntry)(nil)
	_ ConstraintEntry = (*QuantifiedConstraint)(nil)
)

// HeadClassName returns the class name of a class-headed constraint
// entry (ClassEntry or QuantifiedConstraint), or "" for other variants.
//
// Used by callers that want to index or match entries by class name
// without dispatching on variant. The type-switch is trivially inlined
// by the Go compiler.
func HeadClassName(e ConstraintEntry) string {
	switch e := e.(type) {
	case *ClassEntry:
		return e.ClassName
	case *QuantifiedConstraint:
		if e.Head != nil {
			return e.Head.ClassName
		}
	}
	return ""
}

// HeadClassArgs returns the class-head arguments of a class-headed
// constraint entry (ClassEntry or QuantifiedConstraint), or nil for
// other variants.
func HeadClassArgs(e ConstraintEntry) []Type {
	switch e := e.(type) {
	case *ClassEntry:
		return e.Args
	case *QuantifiedConstraint:
		if e.Head != nil {
			return e.Head.Args
		}
	}
	return nil
}
