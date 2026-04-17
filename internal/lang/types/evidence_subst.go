// EvidenceEntries substitution methods — single-variable and parallel forms.
// Co-locates variant-specific substitution logic with the variant types,
// completing the symmetry with MapChildren and ZonkEntries. Subst over a
// TyEvidenceRow dispatches to these methods rather than type-switching at
// the substDepth call site.
//
// Capability fibers additionally rewrite label-variable fields when the
// substituted variable is bound to a label literal. This handling is shared
// between SubstEntries (single-var) and SubstEntriesMany (parallel) so the
// two paths cannot drift on label semantics.

package types

// --- CapabilityEntries ---

// SubstEntries applies [varName := replacement] to a capability fiber.
// Field types and grades are substituted via substDepth; label-variable
// fields named varName are rewritten to the replacement's label name when
// replacement is a label literal (a TyCon at kind level).
func (c *CapabilityEntries) SubstEntries(ops *TypeOps, varName string, replacement Type, depth int) (EvidenceEntries, bool) {
	labelRepl, hasLabelRepl := capabilityLabelLiteral(replacement)
	var fields []RowField // nil until first change (lazy alloc)
	for i, f := range c.Fields {
		newT := substDepth(ops, f.Type, varName, replacement, depth)
		newLabel, newIsLabelVar, labelChanged := f.Label, f.IsLabelVar, false
		if hasLabelRepl && f.IsLabelVar && f.Label == varName {
			newLabel, newIsLabelVar, labelChanged = labelRepl, false, true
		}
		newGrades, gradesChanged := substRowGrades(ops, f.Grades, varName, replacement, depth)
		fieldChanged := newT != f.Type || labelChanged || gradesChanged
		if fieldChanged && fields == nil {
			fields = make([]RowField, len(c.Fields))
			copy(fields[:i], c.Fields[:i])
		}
		if fields != nil {
			fields[i] = RowField{
				Label:      newLabel,
				Type:       newT,
				Grades:     newGrades,
				IsLabelVar: newIsLabelVar,
				S:          f.S,
			}
		}
	}
	if fields == nil {
		return c, false
	}
	return &CapabilityEntries{Fields: fields}, true
}

// SubstEntriesMany applies a parallel substitution to a capability fiber.
func (c *CapabilityEntries) SubstEntriesMany(ops *TypeOps, subs map[string]Type, levelSubs map[string]LevelExpr, fvUnion *map[string]bool, depth int) (EvidenceEntries, bool) {
	var fields []RowField // nil until first change (lazy alloc)
	for i, f := range c.Fields {
		newT := substManyOpt(ops, f.Type, subs, levelSubs, fvUnion, depth)
		newLabel, newIsLabelVar, labelChanged := f.Label, f.IsLabelVar, false
		if f.IsLabelVar {
			if repl, ok := subs[f.Label]; ok {
				if labelRepl, hasLabelRepl := capabilityLabelLiteral(repl); hasLabelRepl {
					newLabel, newIsLabelVar, labelChanged = labelRepl, false, true
				}
			}
		}
		newGrades, gradesChanged := substManyRowGrades(ops, f.Grades, subs, levelSubs, fvUnion, depth)
		fieldChanged := newT != f.Type || labelChanged || gradesChanged
		if fieldChanged && fields == nil {
			fields = make([]RowField, len(c.Fields))
			copy(fields[:i], c.Fields[:i])
		}
		if fields != nil {
			fields[i] = RowField{
				Label:      newLabel,
				Type:       newT,
				Grades:     newGrades,
				IsLabelVar: newIsLabelVar,
				S:          f.S,
			}
		}
	}
	if fields == nil {
		return c, false
	}
	return &CapabilityEntries{Fields: fields}, true
}

// capabilityLabelLiteral returns the label name of a label-literal type and
// true when the type is a TyCon at kind level.
func capabilityLabelLiteral(t Type) (string, bool) {
	lc, ok := t.(*TyCon)
	if !ok || !IsKindLevel(lc.Level) {
		return "", false
	}
	return lc.Name, true
}

// substRowGrades applies single-var substDepth to a grade slice, lazy-allocating
// on first change.
func substRowGrades(ops *TypeOps, grades []Type, varName string, replacement Type, depth int) ([]Type, bool) {
	var out []Type // nil until first change
	for j, g := range grades {
		ng := substDepth(ops, g, varName, replacement, depth)
		if out == nil && ng != g {
			out = make([]Type, len(grades))
			copy(out[:j], grades[:j])
		}
		if out != nil {
			out[j] = ng
		}
	}
	if out == nil {
		return grades, false
	}
	return out, true
}

// substManyRowGrades applies parallel substManyOpt to a grade slice,
// lazy-allocating on first change.
func substManyRowGrades(ops *TypeOps, grades []Type, subs map[string]Type, levelSubs map[string]LevelExpr, fvUnion *map[string]bool, depth int) ([]Type, bool) {
	var out []Type // nil until first change
	for j, g := range grades {
		ng := substManyOpt(ops, g, subs, levelSubs, fvUnion, depth)
		if out == nil && ng != g {
			out = make([]Type, len(grades))
			copy(out[:j], grades[:j])
		}
		if out != nil {
			out[j] = ng
		}
	}
	if out == nil {
		return grades, false
	}
	return out, true
}

// --- ConstraintEntries ---

// SubstEntries applies [varName := replacement] to a constraint fiber.
func (c *ConstraintEntries) SubstEntries(ops *TypeOps, varName string, replacement Type, depth int) (EvidenceEntries, bool) {
	var entries []ConstraintEntry // nil until first change (lazy alloc)
	for i, e := range c.Entries {
		newE, entryChanged := substConstraintEntry(ops, e, varName, replacement, depth)
		if entryChanged && entries == nil {
			entries = make([]ConstraintEntry, len(c.Entries))
			copy(entries[:i], c.Entries[:i])
		}
		if entries != nil {
			entries[i] = newE
		}
	}
	if entries == nil {
		return c, false
	}
	return &ConstraintEntries{Entries: entries}, true
}

// SubstEntriesMany applies a parallel substitution to a constraint fiber.
func (c *ConstraintEntries) SubstEntriesMany(ops *TypeOps, subs map[string]Type, levelSubs map[string]LevelExpr, fvUnion *map[string]bool, depth int) (EvidenceEntries, bool) {
	var entries []ConstraintEntry // nil until first change (lazy alloc)
	for i, e := range c.Entries {
		newE, entryChanged := substManyConstraintEntry(ops, e, subs, levelSubs, fvUnion, depth)
		if entryChanged && entries == nil {
			entries = make([]ConstraintEntry, len(c.Entries))
			copy(entries[:i], c.Entries[:i])
		}
		if entries != nil {
			entries[i] = newE
		}
	}
	if entries == nil {
		return c, false
	}
	return &ConstraintEntries{Entries: entries}, true
}
