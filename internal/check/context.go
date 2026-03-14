package check

import "github.com/cwd-k2/gicel/internal/types"

// CtxEntry is an entry in the typing context.
type CtxEntry interface {
	ctxEntry()
}

type CtxVar struct {
	Name string
	Type types.Type
}

type CtxTyVar struct {
	Name string
	Kind types.Kind
}

// CtxEvidence records available type class evidence in the context.
type CtxEvidence struct {
	ClassName  string
	Args       []types.Type
	DictName   string     // context variable name for the dictionary
	DictType   types.Type // dictionary type
	Quantified *types.QuantifiedConstraint // non-nil for quantified constraints
}

func (*CtxVar) ctxEntry()      {}
func (*CtxTyVar) ctxEntry()    {}
func (*CtxEvidence) ctxEntry() {}

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
