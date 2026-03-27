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
	givenEqs       []*CtEq               // given equalities (skolem ~ concrete)
	metaIndex      map[int][]Ct          // metaID → constraints mentioning it
	ctMetas        map[Ct][]int          // constraint → meta IDs it's indexed under (reverse of metaIndex)
	resolutionKeys map[string]string     // canonical key → placeholder (for cache lookup)
	scopeDepth     int                   // current scope depth
	ctScope        map[Ct]int            // constraint → scope depth at insertion
	scopeCts       map[int][]Ct          // scope depth → constraints at that scope
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

// InsertEq adds a type equality constraint to the inert set.
func (is *InertSet) InsertEq(ct *CtEq, blockingOn []int) {
	is.indexMetas(ct, blockingOn)
	is.tagScope(ct)
}

// InsertGiven adds a given equality to the inert set for scope management.
// Given equalities are recorded separately from wanted CtEqs and are
// cleared on scope exit via LeaveScope.
func (is *InertSet) InsertGiven(ct *CtEq) {
	is.givenEqs = append(is.givenEqs, ct)
	is.tagScope(ct)
}

// KickOutMentioningSkolem removes and returns all CtFunEq and CtEq
// constraints whose type arguments mention the given skolem ID.
// Called when a given equality sk ~ T is installed, so constraints
// blocked on the skolem can be re-processed with the new information.
func (is *InertSet) KickOutMentioningSkolem(skolemID int) []Ct {
	var kicked []Ct

	// Check stuck type family equations.
	for name, eqs := range is.funEqs {
		var remaining []*CtFunEq
		for _, eq := range eqs {
			if typesMentionSkolem(eq.Args, skolemID) {
				kicked = append(kicked, eq)
				// Also remove from meta index.
				is.removeFromMetaIndex(eq)
				delete(is.ctScope, eq)
			} else {
				remaining = append(remaining, eq)
			}
		}
		if len(remaining) != len(eqs) {
			is.funEqs[name] = remaining
		}
	}

	// Check stuck wanted equalities in the meta index.
	// CtEq constraints are only indexed via metaIndex, not a dedicated map.
	// A CtEq blocked on multiple metas appears in multiple buckets;
	// deduplicate to avoid processing the same constraint twice.
	seenEqs := make(map[*CtEq]bool)
	for id, cts := range is.metaIndex {
		var remaining []Ct
		for _, ct := range cts {
			if eq, ok := ct.(*CtEq); ok {
				if typeMentionsSkolem(eq.Lhs, skolemID) || typeMentionsSkolem(eq.Rhs, skolemID) {
					if !seenEqs[eq] {
						seenEqs[eq] = true
						kicked = append(kicked, eq)
					}
					delete(is.ctScope, eq)
					continue
				}
			}
			remaining = append(remaining, ct)
		}
		if len(remaining) != len(cts) {
			is.metaIndex[id] = remaining
		}
	}

	return kicked
}

// removeFromMetaIndex removes a constraint from its meta index entries.
// Uses the reverse map (ctMetas) to visit only the relevant buckets.
func (is *InertSet) removeFromMetaIndex(ct Ct) {
	metaIDs := is.ctMetas[ct]
	for _, id := range metaIDs {
		cts := is.metaIndex[id]
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
	delete(is.ctMetas, ct)
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
		// Clean secondary maps.
		switch c := ct.(type) {
		case *CtClass:
			is.removeClass(c)
		case *CtFunEq:
			is.removeFunEq(c)
		case *CtEq:
			_ = c
		}
		// Remove from other meta index buckets (via reverse map)
		// to prevent duplicate re-processing.
		is.removeFromOtherMetaBuckets(ct, metaID)
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

// tagScope records the current scope depth for a constraint
// and appends to the per-scope constraint list.
func (is *InertSet) tagScope(ct Ct) {
	if is.ctScope == nil {
		is.ctScope = make(map[Ct]int)
	}
	is.ctScope[ct] = is.scopeDepth
	if is.scopeCts == nil {
		is.scopeCts = make(map[int][]Ct)
	}
	is.scopeCts[is.scopeDepth] = append(is.scopeCts[is.scopeDepth], ct)
}

// clearScope removes all constraints with scope depth >= d.
// Uses the per-scope constraint list (scopeCts) to visit only relevant constraints.
func (is *InertSet) clearScope(d int) {
	// Collect constraints at scope depths >= d.
	var toRemove []Ct
	for depth := d; ; depth++ {
		cts := is.scopeCts[depth]
		if len(cts) == 0 && depth > is.scopeDepth {
			break
		}
		toRemove = append(toRemove, cts...)
		delete(is.scopeCts, depth)
	}
	if len(toRemove) == 0 {
		return
	}
	// Build set for given-eq filtering.
	removedSet := make(map[Ct]bool, len(toRemove))
	for _, ct := range toRemove {
		removedSet[ct] = true
		switch c := ct.(type) {
		case *CtClass:
			is.removeClass(c)
		case *CtFunEq:
			is.removeFunEq(c)
		case *CtEq:
			_ = c // CtEq is only in the meta index
		}
		is.removeFromMetaIndex(ct)
		delete(is.ctScope, ct)
	}
	// Clear given equalities at this scope.
	if len(is.givenEqs) > 0 {
		var remaining []*CtEq
		for _, g := range is.givenEqs {
			if !removedSet[g] {
				remaining = append(remaining, g)
			}
		}
		is.givenEqs = remaining
	}
	// Clear resolution keys (conservative: clear all when any constraint is removed).
	clear(is.resolutionKeys)
}

// indexMetas adds ct to the meta index for each given meta ID
// and records the reverse mapping in ctMetas.
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
	if is.ctMetas == nil {
		is.ctMetas = make(map[Ct][]int)
	}
	is.ctMetas[ct] = metaIDs
}

// removeFromOtherMetaBuckets removes a constraint from all meta index buckets
// except the one identified by skipID (which has already been deleted).
func (is *InertSet) removeFromOtherMetaBuckets(ct Ct, skipID int) {
	metaIDs := is.ctMetas[ct]
	for _, id := range metaIDs {
		if id == skipID {
			continue
		}
		cts := is.metaIndex[id]
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
	delete(is.ctMetas, ct)
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
