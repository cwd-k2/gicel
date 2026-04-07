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
	for _, a := range e.Args {
		if !fn(a) {
			return false
		}
	}
	if e.IsEquality {
		if !fn(e.EqLhs) || !fn(e.EqRhs) {
			return false
		}
	}
	if e.ConstraintVar != nil {
		if !fn(e.ConstraintVar) {
			return false
		}
	}
	if e.Quantified != nil {
		for _, c := range e.Quantified.Context {
			if !forEachConstraintEntryChild(c, fn) {
				return false
			}
		}
		if !forEachConstraintEntryChild(e.Quantified.Head, fn) {
			return false
		}
	}
	return true
}

func constraintEntryChildren(e ConstraintEntry) []Type {
	ch := make([]Type, 0, len(e.Args)+3) // +3 for potential EqLhs, EqRhs, ConstraintVar
	ch = append(ch, e.Args...)
	if e.IsEquality {
		ch = append(ch, e.EqLhs, e.EqRhs)
	}
	if e.ConstraintVar != nil {
		ch = append(ch, e.ConstraintVar)
	}
	if e.Quantified != nil {
		for _, c := range e.Quantified.Context {
			ch = append(ch, constraintEntryChildren(c)...)
		}
		ch = append(ch, constraintEntryChildren(e.Quantified.Head)...)
	}
	return ch
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
	changed := false

	var args []Type
	for j, a := range e.Args {
		fa := f(a)
		if args == nil && fa != a {
			args = make([]Type, len(e.Args))
			copy(args[:j], e.Args[:j])
			changed = true
		}
		if args != nil {
			args[j] = fa
		}
	}
	if args == nil {
		args = e.Args
	}

	newCV := e.ConstraintVar
	if e.ConstraintVar != nil {
		newCV = f(e.ConstraintVar)
		if newCV != e.ConstraintVar {
			changed = true
		}
	}

	newQ := e.Quantified
	if e.Quantified != nil {
		var qChanged bool
		newQ, qChanged = mapQuantifiedConstraintChanged(e.Quantified, f)
		if qChanged {
			changed = true
		}
	}

	if e.IsEquality {
		newLhs := f(e.EqLhs)
		newRhs := f(e.EqRhs)
		if newLhs != e.EqLhs || newRhs != e.EqRhs {
			changed = true
		}
		if !changed {
			return e, false
		}
		return ConstraintEntry{ClassName: e.ClassName, Args: args, ConstraintVar: newCV, Quantified: newQ, IsEquality: true, EqLhs: newLhs, EqRhs: newRhs, S: e.S}, true
	}

	if !changed {
		return e, false
	}
	return ConstraintEntry{ClassName: e.ClassName, Args: args, ConstraintVar: newCV, Quantified: newQ, S: e.S}, true
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
	head, headChanged := mapConstraintEntryChanged(qc.Head, f)
	if headChanged {
		changed = true
	}
	if !changed {
		return qc, false
	}
	return &QuantifiedConstraint{Vars: qc.Vars, Context: ctx, Head: head}, true
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
// original unchanged when no child type was modified. Handles quantified
// sub-structure and constraint variable decomposition.
func zonkConstraintEntry(e ConstraintEntry, zonk func(Type) Type) (ConstraintEntry, bool) {
	changed := false

	// Zonk Args with lazy alloc.
	var args []Type
	for j, a := range e.Args {
		za := zonk(a)
		if args == nil && za != a {
			args = make([]Type, len(e.Args))
			copy(args[:j], e.Args[:j])
			changed = true
		}
		if args != nil {
			args[j] = za
		}
	}
	if args == nil {
		args = e.Args
	}

	// Zonk ConstraintVar.
	newCV := e.ConstraintVar
	if e.ConstraintVar != nil {
		newCV = zonk(e.ConstraintVar)
		if newCV != e.ConstraintVar {
			changed = true
		}
	}

	// Zonk Quantified sub-structure.
	newQ := e.Quantified
	if e.Quantified != nil {
		var qChanged bool
		newQ, qChanged = zonkQuantifiedConstraint(e.Quantified, zonk)
		if qChanged {
			changed = true
		}
	}

	// Zonk equality constraint sides.
	var newLhs, newRhs Type
	if e.IsEquality {
		newLhs = zonk(e.EqLhs)
		newRhs = zonk(e.EqRhs)
		if newLhs != e.EqLhs || newRhs != e.EqRhs {
			changed = true
		}
	}

	if !changed {
		return e, false
	}

	result := ConstraintEntry{ClassName: e.ClassName, Args: args, IsEquality: e.IsEquality, S: e.S}
	if e.IsEquality {
		result.EqLhs = newLhs
		result.EqRhs = newRhs
	}
	if e.ConstraintVar != nil {
		result.ConstraintVar = newCV
		// If zonked ConstraintVar is now concrete, decompose into ClassName + Args.
		if result.ClassName == "" {
			head, tArgs := UnwindApp(newCV)
			if con, ok := head.(*TyCon); ok {
				result.ClassName = con.Name
				result.Args = tArgs
			}
		}
	}
	if e.Quantified != nil {
		result.Quantified = newQ
	}
	return result, true
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

	head, headChanged := zonkConstraintEntry(qc.Head, zonk)
	if headChanged {
		changed = true
	}

	if !changed {
		return qc, false
	}
	return &QuantifiedConstraint{Vars: qc.Vars, Context: ctx, Head: head}, true
}

// --- Constraint evidence row builders ---

// EmptyConstraintRow creates an empty closed constraint evidence row.
func EmptyConstraintRow() *TyEvidenceRow {
	return &TyEvidenceRow{Entries: &ConstraintEntries{}, Flags: FlagMetaFree}
}

// SingleConstraint creates a constraint evidence row with one entry.
func SingleConstraint(className string, args []Type) *TyEvidenceRow {
	entries := &ConstraintEntries{
		Entries: []ConstraintEntry{{ClassName: className, Args: args}},
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
	if e.IsEquality {
		return "~ " + TypeKey(e.EqLhs) + " " + TypeKey(e.EqRhs)
	}
	parts := []string{e.ClassName}
	for _, a := range e.Args {
		parts = append(parts, TypeKey(a))
	}
	if e.ConstraintVar != nil {
		parts = append(parts, "$"+TypeKey(e.ConstraintVar))
	}
	return strings.Join(parts, " ")
}
