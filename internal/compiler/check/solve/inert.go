package solve

// InertSet holds constraints in canonical form: class constraints indexed
// by class name, and stuck type family equations indexed by family name.
// A secondary meta index enables O(1) kick-out when a metavariable is solved.
// A resolution cache maps canonical constraint keys to resolved Core expressions.
//
// Scope depth tracks constraint ownership for scope-aware reset:
// constraints inserted at depth d are cleared when Reset is called at depth d,
// but constraints from outer scopes (depth < d) are preserved.
type InertSet struct {
	classMap       map[string][]*CtClass // className → class constraints
	funEqs         map[string][]*CtFunEq // familyName → stuck equations
	metaIndex      map[int][]Ct          // metaID → constraints mentioning it
	resolutionKeys map[string]string     // canonical key → placeholder (for cache lookup)
	scopeDepth     int                   // current scope depth
	ctScope        map[Ct]int            // constraint → scope depth at insertion
}

// NewInertSet returns an empty InertSet.
func NewInertSet() InertSet {
	return InertSet{}
}

// EnterScope increments the scope depth for constraint ownership tracking.
func (is *InertSet) EnterScope() { is.scopeDepth++ }

// LeaveScope removes all constraints at or above the current scope depth,
// then decrements the depth.
func (is *InertSet) LeaveScope() {
	is.clearScope(is.scopeDepth)
	is.scopeDepth--
}

// ScopeDepth returns the current scope depth.
func (is *InertSet) ScopeDepth() int { return is.scopeDepth }

// InsertClass adds a class constraint to the inert set.
func (is *InertSet) InsertClass(ct *CtClass, canonKey string) {
	if is.classMap == nil {
		is.classMap = make(map[string][]*CtClass)
	}
	is.classMap[ct.ClassName] = append(is.classMap[ct.ClassName], ct)
	is.indexMetas(ct, collectMetaIDs(ct.Args))
	is.tagScope(ct)
	if canonKey != "" {
		if is.resolutionKeys == nil {
			is.resolutionKeys = make(map[string]string)
		}
		is.resolutionKeys[canonKey] = ct.Placeholder
	}
}

// LookupResolution returns the placeholder of a previously resolved constraint
// with the same canonical key, or "" if not found.
func (is *InertSet) LookupResolution(canonKey string) string {
	return is.resolutionKeys[canonKey]
}

// InsertFunEq adds a type family equation to the inert set.
func (is *InertSet) InsertFunEq(ct *CtFunEq) {
	if is.funEqs == nil {
		is.funEqs = make(map[string][]*CtFunEq)
	}
	is.funEqs[ct.FamilyName] = append(is.funEqs[ct.FamilyName], ct)
	is.indexMetas(ct, ct.BlockingOn)
	is.tagScope(ct)
}

// LookupClass returns all inert class constraints for the given class name.
func (is *InertSet) LookupClass(className string) []*CtClass {
	return is.classMap[className]
}

// LookupFunEq returns all inert family equations for the given family name.
func (is *InertSet) LookupFunEq(familyName string) []*CtFunEq {
	return is.funEqs[familyName]
}

// KickOut removes and returns all constraints that mention the given meta ID.
// Kicked-out constraints should be pushed to the front of the worklist.
func (is *InertSet) KickOut(metaID int) []Ct {
	cts := is.metaIndex[metaID]
	delete(is.metaIndex, metaID)
	for _, ct := range cts {
		switch c := ct.(type) {
		case *CtClass:
			is.removeClass(c)
		case *CtFunEq:
			is.removeFunEq(c)
		}
	}
	return cts
}

// CollectClassResiduals returns all remaining class constraints.
func (is *InertSet) CollectClassResiduals() []*CtClass {
	var result []*CtClass
	for _, cts := range is.classMap {
		result = append(result, cts...)
	}
	return result
}

// Reset clears constraints at the current scope depth from the inert set.
// Constraints from outer scopes (depth < current) are preserved.
func (is *InertSet) Reset() {
	is.clearScope(is.scopeDepth)
}

// ResetAll clears all constraints from the inert set regardless of scope.
func (is *InertSet) ResetAll() {
	clear(is.classMap)
	clear(is.funEqs)
	clear(is.metaIndex)
	clear(is.resolutionKeys)
	clear(is.ctScope)
}

// tagScope records the current scope depth for a constraint.
func (is *InertSet) tagScope(ct Ct) {
	if is.ctScope == nil {
		is.ctScope = make(map[Ct]int)
	}
	is.ctScope[ct] = is.scopeDepth
}

// clearScope removes all constraints with scope depth >= d.
func (is *InertSet) clearScope(d int) {
	// Collect constraints to remove.
	var toRemove []Ct
	for ct, scope := range is.ctScope {
		if scope >= d {
			toRemove = append(toRemove, ct)
		}
	}
	for _, ct := range toRemove {
		switch c := ct.(type) {
		case *CtClass:
			is.removeClass(c)
		case *CtFunEq:
			is.removeFunEq(c)
		}
		// Remove from meta index.
		for id, cts := range is.metaIndex {
			for i, indexed := range cts {
				if indexed == ct {
					last := len(cts) - 1
					cts[i] = cts[last]
					cts[last] = nil
					is.metaIndex[id] = cts[:last]
					break
				}
			}
		}
		delete(is.ctScope, ct)
	}
	// Clear resolution keys at this scope (conservative: clear all at scope >= d).
	// Resolution keys don't track scope, so we clear all of them when any scope is cleared.
	// This is safe: the resolution cache is an optimization, not a correctness concern.
	if len(toRemove) > 0 {
		clear(is.resolutionKeys)
	}
}

// indexMetas adds ct to the meta index for each given meta ID.
func (is *InertSet) indexMetas(ct Ct, metaIDs []int) {
	if len(metaIDs) == 0 {
		return
	}
	if is.metaIndex == nil {
		is.metaIndex = make(map[int][]Ct)
	}
	for _, id := range metaIDs {
		is.metaIndex[id] = append(is.metaIndex[id], ct)
	}
}

// removeClass removes a specific CtClass from the classMap using swap-remove.
// Order within a bucket is not significant (membership-only invariant).
func (is *InertSet) removeClass(ct *CtClass) {
	cts := is.classMap[ct.ClassName]
	for i, c := range cts {
		if c == ct {
			last := len(cts) - 1
			cts[i] = cts[last]
			cts[last] = nil
			is.classMap[ct.ClassName] = cts[:last]
			return
		}
	}
}

// removeFunEq removes a specific CtFunEq from the funEqs map using swap-remove.
func (is *InertSet) removeFunEq(ct *CtFunEq) {
	cts := is.funEqs[ct.FamilyName]
	for i, c := range cts {
		if c == ct {
			last := len(cts) - 1
			cts[i] = cts[last]
			cts[last] = nil
			is.funEqs[ct.FamilyName] = cts[:last]
			return
		}
	}
}
