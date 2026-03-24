package solve

// InertSet holds constraints in canonical form: class constraints indexed
// by class name, and stuck type family equations indexed by family name.
// A secondary meta index enables O(1) kick-out when a metavariable is solved.
// A resolution cache maps canonical constraint keys to resolved Core expressions.
type InertSet struct {
	classMap       map[string][]*CtClass // className → class constraints
	funEqs         map[string][]*CtFunEq // familyName → stuck equations
	metaIndex      map[int][]Ct          // metaID → constraints mentioning it
	resolutionKeys map[string]string     // canonical key → placeholder (for cache lookup)
}

// NewInertSet returns an empty InertSet.
func NewInertSet() InertSet {
	return InertSet{}
}

// InsertClass adds a class constraint to the inert set.
func (is *InertSet) InsertClass(ct *CtClass, canonKey string) {
	if is.classMap == nil {
		is.classMap = make(map[string][]*CtClass)
	}
	is.classMap[ct.ClassName] = append(is.classMap[ct.ClassName], ct)
	is.indexMetas(ct, collectMetaIDs(ct.Args))
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

// Reset clears all constraints from the inert set.
func (is *InertSet) Reset() {
	clear(is.classMap)
	clear(is.funEqs)
	clear(is.metaIndex)
	clear(is.resolutionKeys)
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
