// Constraint fiber — ConstraintEntries implementation, entry-level traversals,
// constraint evidence row builders, and ConstraintKey serialization.
// Does NOT cover: CapabilityEntries fiber (evidence.go),
//                 row-level builders/accessors (evidence.go),
//                 constraint row operations (evidence_row.go),
//                 subst traversal (subst.go),
//                 free variable traversal (free.go).

package types

import (
	"strings"
)

// ConstraintEntries holds type class constraint entries.
type ConstraintEntries struct {
	Entries []ConstraintEntry
}

func (*ConstraintEntries) evidenceEntries() {}

func (c *ConstraintEntries) EntryCount() int { return len(c.Entries) }

func (c *ConstraintEntries) AllChildren() []Type {
	var ch []Type
	for _, e := range c.Entries {
		ch = append(ch, constraintEntryChildren(e)...)
	}
	return ch
}

func (c *ConstraintEntries) ForEachChild(fn func(Type) bool) {
	for _, e := range c.Entries {
		if !forEachConstraintEntryChild(e, fn) {
			return
		}
	}
}

// forEachConstraintEntryChild calls fn for each child type in a constraint entry.
// Returns false if fn returned false (early exit).
func forEachConstraintEntryChild(e ConstraintEntry, fn func(Type) bool) bool {
	switch e := e.(type) {
	case *ClassEntry:
		for _, a := range e.Args {
			if !fn(a) {
				return false
			}
		}
		return true
	case *EqualityEntry:
		if !fn(e.Lhs) || !fn(e.Rhs) {
			return false
		}
		return true
	case *VarEntry:
		return fn(e.Var)
	case *QuantifiedConstraint:
		for _, c := range e.Context {
			if !forEachConstraintEntryChild(c, fn) {
				return false
			}
		}
		if e.Head != nil {
			for _, a := range e.Head.Args {
				if !fn(a) {
					return false
				}
			}
		}
		return true
	}
	return true
}

func constraintEntryChildren(e ConstraintEntry) []Type {
	switch e := e.(type) {
	case *ClassEntry:
		if len(e.Args) == 0 {
			return nil
		}
		ch := make([]Type, len(e.Args))
		copy(ch, e.Args)
		return ch
	case *EqualityEntry:
		return []Type{e.Lhs, e.Rhs}
	case *VarEntry:
		return []Type{e.Var}
	case *QuantifiedConstraint:
		var ch []Type
		for _, c := range e.Context {
			ch = append(ch, constraintEntryChildren(c)...)
		}
		if e.Head != nil {
			ch = append(ch, e.Head.Args...)
		}
		return ch
	}
	return nil
}

func (c *ConstraintEntries) MapChildren(f func(Type) Type) (EvidenceEntries, bool) {
	var entries []ConstraintEntry // nil until first change
	for i, e := range c.Entries {
		me, entryChanged := mapConstraintEntryChanged(e, f)
		if entries == nil && entryChanged {
			entries = make([]ConstraintEntry, len(c.Entries))
			copy(entries[:i], c.Entries[:i])
		}
		if entries != nil {
			entries[i] = me
		}
	}
	if entries == nil {
		return c, false
	}
	return &ConstraintEntries{Entries: entries}, true
}

// mapConstraintEntryChanged applies f to all type children in a ConstraintEntry,
// returning the original unchanged when no child was modified.
func mapConstraintEntryChanged(e ConstraintEntry, f func(Type) Type) (ConstraintEntry, bool) {
	switch e := e.(type) {
	case *ClassEntry:
		args, changed := mapTypeSlice(e.Args, f)
		if !changed {
			return e, false
		}
		return &ClassEntry{ClassName: e.ClassName, Args: args, S: e.S}, true
	case *EqualityEntry:
		newLhs := f(e.Lhs)
		newRhs := f(e.Rhs)
		if newLhs == e.Lhs && newRhs == e.Rhs {
			return e, false
		}
		return &EqualityEntry{Lhs: newLhs, Rhs: newRhs, S: e.S}, true
	case *VarEntry:
		newVar := f(e.Var)
		if newVar == e.Var {
			return e, false
		}
		return &VarEntry{Var: newVar, S: e.S}, true
	case *QuantifiedConstraint:
		newQC, changed := mapQuantifiedConstraintChanged(e, f)
		if !changed {
			return e, false
		}
		return newQC, true
	}
	return e, false
}

// mapTypeSlice applies f to every element of ts, returning the original slice
// unchanged (and false) when no element was modified.
func mapTypeSlice(ts []Type, f func(Type) Type) ([]Type, bool) {
	var out []Type // nil until first change
	for j, t := range ts {
		ft := f(t)
		if out == nil && ft != t {
			out = make([]Type, len(ts))
			copy(out[:j], ts[:j])
		}
		if out != nil {
			out[j] = ft
		}
	}
	if out == nil {
		return ts, false
	}
	return out, true
}

// mapQuantifiedConstraintChanged applies f to all type children inside a
// QuantifiedConstraint, returning the original unchanged when no child was modified.
func mapQuantifiedConstraintChanged(qc *QuantifiedConstraint, f func(Type) Type) (*QuantifiedConstraint, bool) {
	changed := false
	var ctx []ConstraintEntry
	for i, ce := range qc.Context {
		mc, cChanged := mapConstraintEntryChanged(ce, f)
		if ctx == nil && cChanged {
			ctx = make([]ConstraintEntry, len(qc.Context))
			copy(ctx[:i], qc.Context[:i])
			changed = true
		}
		if ctx != nil {
			ctx[i] = mc
		}
	}
	if ctx == nil {
		ctx = qc.Context
	}
	head := qc.Head
	if head != nil {
		newArgs, headChanged := mapTypeSlice(head.Args, f)
		if headChanged {
			head = &ClassEntry{ClassName: head.ClassName, Args: newArgs, S: head.S}
			changed = true
		}
	}
	if !changed {
		return qc, false
	}
	return &QuantifiedConstraint{Vars: qc.Vars, Context: ctx, Head: head, S: qc.S}, true
}

func (c *ConstraintEntries) FiberKind() Type { return TypeOfConstraints }

func (c *ConstraintEntries) Empty() EvidenceEntries { return &ConstraintEntries{} }

func (c *ConstraintEntries) ZonkEntries(zonk func(Type) Type) (EvidenceEntries, bool) {
	var ces []ConstraintEntry // nil until first change detected
	for i, e := range c.Entries {
		ze, entryChanged := zonkConstraintEntry(e, zonk)
		if ces == nil && entryChanged {
			ces = make([]ConstraintEntry, len(c.Entries))
			copy(ces[:i], c.Entries[:i]) // backfill unchanged prefix
		}
		if ces != nil {
			ces[i] = ze
		}
	}
	if ces == nil {
		return c, false
	}
	return &ConstraintEntries{Entries: ces}, true
}

// zonkConstraintEntry zonks a single constraint entry, returning the
// original unchanged when no child type was modified. A VarEntry whose
// variable resolves to a concrete class application is transitioned to
// a ClassEntry — this is the *only* point at which a variant changes
// identity during zonking, and it replaces the legacy "resolved CV
// with ClassName set" hybrid state with an explicit variant transition.
func zonkConstraintEntry(e ConstraintEntry, zonk func(Type) Type) (ConstraintEntry, bool) {
	switch e := e.(type) {
	case *ClassEntry:
		args, changed := mapTypeSlice(e.Args, zonk)
		if !changed {
			return e, false
		}
		return &ClassEntry{ClassName: e.ClassName, Args: args, S: e.S}, true
	case *EqualityEntry:
		newLhs := zonk(e.Lhs)
		newRhs := zonk(e.Rhs)
		if newLhs == e.Lhs && newRhs == e.Rhs {
			return e, false
		}
		return &EqualityEntry{Lhs: newLhs, Rhs: newRhs, S: e.S}, true
	case *VarEntry:
		newVar := zonk(e.Var)
		// Variant transition: if the zonked variable has a concrete
		// TyCon head, decompose into a ClassEntry.
		if className, args, ok := DecomposeConstraintType(newVar); ok {
			return &ClassEntry{ClassName: className, Args: args, S: e.S}, true
		}
		if newVar == e.Var {
			return e, false
		}
		return &VarEntry{Var: newVar, S: e.S}, true
	case *QuantifiedConstraint:
		newQC, changed := zonkQuantifiedConstraint(e, zonk)
		if !changed {
			return e, false
		}
		return newQC, true
	}
	return e, false
}

// zonkQuantifiedConstraint zonks a QuantifiedConstraint, returning the
// original unchanged when no child was modified.
func zonkQuantifiedConstraint(qc *QuantifiedConstraint, zonk func(Type) Type) (*QuantifiedConstraint, bool) {
	changed := false
	var ctx []ConstraintEntry
	for i, c := range qc.Context {
		zc, cChanged := zonkConstraintEntry(c, zonk)
		if ctx == nil && cChanged {
			ctx = make([]ConstraintEntry, len(qc.Context))
			copy(ctx[:i], qc.Context[:i])
			changed = true
		}
		if ctx != nil {
			ctx[i] = zc
		}
	}
	if ctx == nil {
		ctx = qc.Context
	}

	head := qc.Head
	if head != nil {
		newArgs, headChanged := mapTypeSlice(head.Args, zonk)
		if headChanged {
			head = &ClassEntry{ClassName: head.ClassName, Args: newArgs, S: head.S}
			changed = true
		}
	}

	if !changed {
		return qc, false
	}
	return &QuantifiedConstraint{Vars: qc.Vars, Context: ctx, Head: head, S: qc.S}, true
}

// --- Constraint evidence row builders ---

// EmptyConstraintRow creates an empty closed constraint evidence row.
func EmptyConstraintRow() *TyEvidenceRow {
	return &TyEvidenceRow{Entries: &ConstraintEntries{}, Flags: FlagMetaFree}
}

// SingleConstraint creates a constraint evidence row with one entry.
func SingleConstraint(className string, args []Type) *TyEvidenceRow {
	entries := &ConstraintEntries{
		Entries: []ConstraintEntry{&ClassEntry{ClassName: className, Args: args}},
	}
	return &TyEvidenceRow{
		Entries: entries,
		Flags:   EvidenceRowFlags(entries, nil),
	}
}

// ConstraintKey returns a canonical string for a constraint entry.
// Used for sorting/display, not for matching (matching uses className + type unification).
// Delegates to TypeKey for injective encoding.
func ConstraintKey(e ConstraintEntry) string {
	switch e := e.(type) {
	case *ClassEntry:
		parts := make([]string, 0, 1+len(e.Args))
		parts = append(parts, e.ClassName)
		for _, a := range e.Args {
			parts = append(parts, TypeKey(a))
		}
		return strings.Join(parts, " ")
	case *EqualityEntry:
		return "~ " + TypeKey(e.Lhs) + " " + TypeKey(e.Rhs)
	case *VarEntry:
		return "$" + TypeKey(e.Var)
	case *QuantifiedConstraint:
		if e.Head == nil {
			return "forall"
		}
		parts := make([]string, 0, 2+len(e.Head.Args))
		parts = append(parts, "forall", e.Head.ClassName)
		for _, a := range e.Head.Args {
			parts = append(parts, TypeKey(a))
		}
		return strings.Join(parts, " ")
	}
	return ""
}
