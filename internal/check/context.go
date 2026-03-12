package check

import "github.com/cwd-k2/gomputation/pkg/types"

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

type CtxMeta struct {
	ID   int
	Kind types.Kind
}

type CtxSolved struct {
	ID   int
	Kind types.Kind
	Soln types.Type
}

type CtxMarker struct {
	ID int
}

func (*CtxVar) ctxEntry()    {}
func (*CtxTyVar) ctxEntry()  {}
func (*CtxMeta) ctxEntry()   {}
func (*CtxSolved) ctxEntry() {}
func (*CtxMarker) ctxEntry() {}

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

// Apply walks the context and substitutes all solved metavariables in a type.
func (c *Context) Apply(t types.Type) types.Type {
	for _, e := range c.entries {
		if s, ok := e.(*CtxSolved); ok {
			t = types.Subst(t, metaName(s.ID), s.Soln)
		}
	}
	return t
}

func metaName(id int) string {
	return ""
}
