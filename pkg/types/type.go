package types

import "github.com/cwd-k2/gomputation/internal/span"

// Type is the unified representation for value types, computation types, and row types.
type Type interface {
	typeNode()
	Span() span.Span
	Children() []Type
}

// TyVar is a type or row variable.
type TyVar struct {
	Name string
	S    span.Span
}

// TyCon is a named type constructor.
type TyCon struct {
	Name string
	S    span.Span
}

// TyApp is a general type application (F T).
type TyApp struct {
	Fun Type
	Arg Type
	S   span.Span
}

// TyArrow is a function type (A -> B).
type TyArrow struct {
	From Type
	To   Type
	S    span.Span
}

// TyForall is a universal quantification (forall a:K. T).
type TyForall struct {
	Var  string
	Kind Kind
	Body Type
	S    span.Span
}

// TyComp is a Computation pre post a type.
type TyComp struct {
	Pre    Type
	Post   Type
	Result Type
	S      span.Span
}

// TyThunk is a Thunk pre post a type.
type TyThunk struct {
	Pre    Type
	Post   Type
	Result Type
	S      span.Span
}

// TyRow is a row type { l1:T1, ..., ln:Tn | tail? }.
type TyRow struct {
	Fields []RowField
	Tail   Type // nil = closed row, TyVar or TyMeta = open row
	S      span.Span
}

// RowField is a single label:type pair in a row.
type RowField struct {
	Label string
	Type  Type
	S     span.Span
}

// TyMeta is a unification metavariable (created by the checker).
type TyMeta struct {
	ID   int
	Kind Kind
	S    span.Span
}

// TyError is a poison type for error recovery.
type TyError struct {
	S span.Span
}

// --- typeNode markers ---

func (*TyVar) typeNode()    {}
func (*TyCon) typeNode()    {}
func (*TyApp) typeNode()    {}
func (*TyArrow) typeNode()  {}
func (*TyForall) typeNode() {}
func (*TyComp) typeNode()   {}
func (*TyThunk) typeNode()  {}
func (*TyRow) typeNode()    {}
func (*TyMeta) typeNode()   {}
func (*TyError) typeNode()  {}

// --- Span accessors ---

func (t *TyVar) Span() span.Span    { return t.S }
func (t *TyCon) Span() span.Span    { return t.S }
func (t *TyApp) Span() span.Span    { return t.S }
func (t *TyArrow) Span() span.Span  { return t.S }
func (t *TyForall) Span() span.Span { return t.S }
func (t *TyComp) Span() span.Span   { return t.S }
func (t *TyThunk) Span() span.Span  { return t.S }
func (t *TyRow) Span() span.Span    { return t.S }
func (t *TyMeta) Span() span.Span   { return t.S }
func (t *TyError) Span() span.Span  { return t.S }

// --- Children ---

func (t *TyVar) Children() []Type    { return nil }
func (t *TyCon) Children() []Type    { return nil }
func (t *TyApp) Children() []Type    { return []Type{t.Fun, t.Arg} }
func (t *TyArrow) Children() []Type  { return []Type{t.From, t.To} }
func (t *TyForall) Children() []Type { return []Type{t.Body} }
func (t *TyComp) Children() []Type   { return []Type{t.Pre, t.Post, t.Result} }
func (t *TyThunk) Children() []Type  { return []Type{t.Pre, t.Post, t.Result} }
func (t *TyMeta) Children() []Type   { return nil }
func (t *TyError) Children() []Type  { return nil }

func (t *TyRow) Children() []Type {
	ch := make([]Type, 0, len(t.Fields)+1)
	for _, f := range t.Fields {
		ch = append(ch, f.Type)
	}
	if t.Tail != nil {
		ch = append(ch, t.Tail)
	}
	return ch
}
