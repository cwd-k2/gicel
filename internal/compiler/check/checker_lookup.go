// Checker type-namespace lookups — alias and family resolution.
// Does NOT cover: variable/constructor lookup (bidir_lookup.go).
package check

// lookupAlias searches for a type alias by name: first in the Registry
// (populated during declaration phases), then in Scope's injected aliases
// (populated from qualified references).
func (ch *Checker) lookupAlias(name string) (*AliasInfo, bool) {
	if info, ok := ch.reg.LookupAlias(name); ok {
		return info, true
	}
	if ch.scope != nil && ch.scope.injectedAliases != nil {
		if info, ok := ch.scope.injectedAliases[name]; ok {
			return info, true
		}
	}
	return nil, false
}

// lookupFamily searches for a type family by name: first in the Registry,
// then in Scope's injected families.
func (ch *Checker) lookupFamily(name string) (*TypeFamilyInfo, bool) {
	if info, ok := ch.reg.LookupFamily(name); ok {
		return info, true
	}
	if ch.scope != nil && ch.scope.injectedFamilies != nil {
		if info, ok := ch.scope.injectedFamilies[name]; ok {
			return info, true
		}
	}
	return nil, false
}
