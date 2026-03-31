package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// CtxEntry, CtxVar, CtxTyVar, CtxEvidence are defined in the env
// subpackage and re-exported here as type aliases for backward
// compatibility within the check package.
type CtxEntry = env.CtxEntry
type CtxVar = env.CtxVar
type CtxTyVar = env.CtxTyVar
type CtxEvidence = env.CtxEvidence

// Context is an ordered typing context (DK-style).
// Auxiliary indices enable O(1) class-name lookup for both CtxEvidence
// entries (evidence parameters) and CtxVar dict entries (instance dicts).
type Context struct {
	entries       []CtxEntry
	evidenceIndex map[string][]int // className → CtxEvidence positions
	dictVarIndex  map[string][]int // className → CtxVar dict positions
}

func NewContext() *Context {
	return &Context{
		evidenceIndex: make(map[string][]int),
		dictVarIndex:  make(map[string][]int),
	}
}

func (c *Context) Push(entry CtxEntry) {
	pos := len(c.entries)
	c.entries = append(c.entries, entry)
	if ev, ok := entry.(*CtxEvidence); ok && ev.ClassName != "" {
		c.evidenceIndex[ev.ClassName] = append(c.evidenceIndex[ev.ClassName], pos)
	}
	if v, ok := entry.(*CtxVar); ok && v.DictClassName != "" {
		c.dictVarIndex[v.DictClassName] = append(c.dictVarIndex[v.DictClassName], pos)
	}
}

func (c *Context) Pop() CtxEntry {
	if len(c.entries) == 0 {
		return nil
	}
	pos := len(c.entries) - 1
	e := c.entries[pos]
	c.entries = c.entries[:pos]
	if ev, ok := e.(*CtxEvidence); ok && ev.ClassName != "" {
		idxs := c.evidenceIndex[ev.ClassName]
		n := len(idxs)
		if n == 0 || idxs[n-1] != pos {
			panic("internal: evidenceIndex LIFO invariant violated on Pop")
		}
		c.evidenceIndex[ev.ClassName] = idxs[:n-1]
	}
	if v, ok := e.(*CtxVar); ok && v.DictClassName != "" {
		idxs := c.dictVarIndex[v.DictClassName]
		n := len(idxs)
		if n == 0 || idxs[n-1] != pos {
			panic("internal: dictVarIndex LIFO invariant violated on Pop")
		}
		c.dictVarIndex[v.DictClassName] = idxs[:n-1]
	}
	return e
}

func (c *Context) LookupVar(name string) (types.Type, bool) {
	for i := len(c.entries) - 1; i >= 0; i-- {
		if v, ok := c.entries[i].(*CtxVar); ok && v.Name == name {
			return v.Type, true
		}
	}
	return nil, false
}

// LookupVarFull returns the type and source module for a variable.
func (c *Context) LookupVarFull(name string) (types.Type, string, bool) {
	for i := len(c.entries) - 1; i >= 0; i-- {
		if v, ok := c.entries[i].(*CtxVar); ok && v.Name == name {
			return v.Type, v.Module, true
		}
	}
	return nil, "", false
}

func (c *Context) LookupTyVar(name string) (types.Type, bool) {
	for i := len(c.entries) - 1; i >= 0; i-- {
		if v, ok := c.entries[i].(*CtxTyVar); ok && v.Name == name {
			return v.Kind, true
		}
	}
	return nil, false
}

// Scan iterates entries from most recent to oldest, calling fn for each.
// Iteration stops when fn returns false.
func (c *Context) Scan(fn func(CtxEntry) bool) {
	for i := len(c.entries) - 1; i >= 0; i-- {
		if !fn(c.entries[i]) {
			return
		}
	}
}

// LookupEvidence returns all CtxEvidence entries for a given class name,
// ordered from most recent to oldest.
func (c *Context) LookupEvidence(className string) []*CtxEvidence {
	idxs := c.evidenceIndex[className]
	if len(idxs) == 0 {
		return nil
	}
	result := make([]*CtxEvidence, len(idxs))
	for i, pos := range idxs {
		// Reverse: index stores oldest-first, callers expect newest-first.
		result[len(idxs)-1-i] = c.entries[pos].(*CtxEvidence)
	}
	return result
}

// DictVarClasses returns all class names that have dict variables in the context.
func (c *Context) DictVarClasses() []string {
	names := make([]string, 0, len(c.dictVarIndex))
	for k, idxs := range c.dictVarIndex {
		if len(idxs) > 0 {
			names = append(names, k)
		}
	}
	return names
}

// LookupDictVar returns all non-invisible CtxVar dict entries for a given
// class name, ordered from most recent to oldest.
func (c *Context) LookupDictVar(className string) []*CtxVar {
	idxs := c.dictVarIndex[className]
	if len(idxs) == 0 {
		return nil
	}
	result := make([]*CtxVar, 0, len(idxs))
	for i := len(idxs) - 1; i >= 0; i-- {
		v := c.entries[idxs[i]].(*CtxVar)
		if !v.SolverInvisible {
			result = append(result, v)
		}
	}
	return result
}
