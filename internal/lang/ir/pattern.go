package ir

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Alt is a case alternative: pattern -> body.
type Alt struct {
	Pattern   Pattern
	Body      Core
	Generated bool // true when the entire alt is compiler-generated (dict extraction, method selector)
	S         span.Span
}

// Pattern in Core IR.
type Pattern interface {
	patternNode()
	Span() span.Span
	Bindings() []string
}

// PVar — variable pattern (binds a value).
type PVar struct {
	Name string
	S    span.Span
}

// PWild — wildcard pattern.
type PWild struct {
	S span.Span
}

// PCon — constructor pattern (C p1 ... pn).
// Module is non-empty for qualified patterns (Q.Con).
type PCon struct {
	Con    string
	Module string
	Args   []Pattern
	S      span.Span
}

// PRecord — record pattern { l1: p1, ..., ln: pn }.
type PRecord struct {
	Fields []PRecordField
	S      span.Span
}

// PRecordField is a label-pattern pair in a record pattern.
type PRecordField struct {
	Label   string
	Pattern Pattern
}

// PLit — literal pattern (matches a literal value by equality).
type PLit struct {
	Value interface{} // int64, float64, string, or rune
	S     span.Span
}

func (*PVar) patternNode()    {}
func (*PWild) patternNode()   {}
func (*PCon) patternNode()    {}
func (*PRecord) patternNode() {}
func (*PLit) patternNode()    {}

func (p *PVar) Span() span.Span    { return p.S }
func (p *PWild) Span() span.Span   { return p.S }
func (p *PCon) Span() span.Span    { return p.S }
func (p *PRecord) Span() span.Span { return p.S }
func (p *PLit) Span() span.Span    { return p.S }

func (p *PVar) Bindings() []string  { return []string{p.Name} }
func (p *PWild) Bindings() []string { return nil }
func (p *PLit) Bindings() []string  { return nil }
func (p *PCon) Bindings() []string {
	var bs []string
	for _, arg := range p.Args {
		bs = append(bs, arg.Bindings()...)
	}
	return bs
}
func (p *PRecord) Bindings() []string {
	var bs []string
	for _, f := range p.Fields {
		bs = append(bs, f.Pattern.Bindings()...)
	}
	return bs
}

// Binding is a named definition in LetRec or top-level.
type Binding struct {
	Name string
	Type types.Type
	Expr Core
	S    span.Span
}

// Program is a complete Core program (top-level bindings).
type Program struct {
	DataDecls []DataDecl
	Bindings  []Binding
}

// DataDecl is a data type declaration in Core.
type DataDecl struct {
	Name     string
	TyParams []TyParam
	Cons     []ConDecl
	S        span.Span
}

// TyParam is a type parameter with its kind.
type TyParam struct {
	Name string
	Kind types.Kind
}

// ConDecl is a single constructor declaration.
type ConDecl struct {
	Name       string
	Fields     []types.Type
	ReturnType types.Type // GADT: refined return type (nil for ADT)
	S          span.Span
}
