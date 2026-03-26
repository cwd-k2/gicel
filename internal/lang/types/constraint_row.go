package types

import "strings"

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
	return strings.Join(parts, " ")
}
