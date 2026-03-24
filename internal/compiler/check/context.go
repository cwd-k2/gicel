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
type Context struct {
	entries []CtxEntry
}

func NewContext() *Context {
	return &Context{}
}

func (c *Context) Push(entry CtxEntry) {
	c.entries = append(c.entries, entry)
}

func (c *Context) Pop() CtxEntry {
	if len(c.entries) == 0 {
		return nil
	}
	e := c.entries[len(c.entries)-1]
	c.entries = c.entries[:len(c.entries)-1]
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

func (c *Context) LookupTyVar(name string) (types.Kind, bool) {
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

// LookupEvidence returns all CtxEvidence entries for a given class name.
func (c *Context) LookupEvidence(className string) []*CtxEvidence {
	var result []*CtxEvidence
	for i := len(c.entries) - 1; i >= 0; i-- {
		if e, ok := c.entries[i].(*CtxEvidence); ok && e.ClassName == className {
			result = append(result, e)
		}
	}
	return result
}
