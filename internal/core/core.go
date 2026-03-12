package core

import (
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/types"
)

// Core is a term in the core intermediate representation.
// 14 formers: Var, Lam, App, TyApp, TyLam, Con, Case, LetRec, Pure, Bind, Thunk, Force, PrimOp, Lit.
type Core interface {
	coreNode()
	Span() span.Span
}

// Var — variable reference.
type Var struct {
	Name string
	S    span.Span
}

// Lam — lambda abstraction.
type Lam struct {
	Param     string
	ParamType types.Type
	Body      Core
	S         span.Span
}

// App — function application.
type App struct {
	Fun Core
	Arg Core
	S   span.Span
}

// TyApp — type application (e @T). Erased at runtime.
type TyApp struct {
	Expr  Core
	TyArg types.Type
	S     span.Span
}

// TyLam — type abstraction (elaboration of forall). Erased at runtime.
type TyLam struct {
	TyParam string
	Kind    types.Kind
	Body    Core
	S       span.Span
}

// Con — constructor application (C e1 ... en).
type Con struct {
	Name string
	Args []Core
	S    span.Span
}

// Case — case analysis.
type Case struct {
	Scrutinee Core
	Alts      []Alt
	S         span.Span
}

// LetRec — mutually recursive bindings.
type LetRec struct {
	Bindings []Binding
	Body     Core
	S        span.Span
}

// Pure — computation introduction (pure e).
type Pure struct {
	Expr Core
	S    span.Span
}

// Bind — computation sequencing (bind c (\x -> e)).
type Bind struct {
	Comp Core
	Var  string
	Body Core
	S    span.Span
}

// Thunk — suspend computation (thunk c).
type Thunk struct {
	Comp Core
	S    span.Span
}

// Force — resume suspended computation (force e).
type Force struct {
	Expr Core
	S    span.Span
}

// PrimOp — host-provided primitive operation.
// Effectful marks primitives whose return type is Computation (they access CapEnv).
// Effectful PrimOps are deferred at evaluation time and forced only in Bind or at top-level.
type PrimOp struct {
	Name      string
	Arity     int
	Effectful bool
	Args      []Core
	S         span.Span
}

// Lit — literal value (Int, String, Rune).
type Lit struct {
	Value any // int64, string, or rune
	S     span.Span
}

// --- coreNode markers ---
func (*Var) coreNode()    {}
func (*Lam) coreNode()    {}
func (*App) coreNode()    {}
func (*TyApp) coreNode()  {}
func (*TyLam) coreNode()  {}
func (*Con) coreNode()    {}
func (*Case) coreNode()   {}
func (*LetRec) coreNode() {}
func (*Pure) coreNode()   {}
func (*Bind) coreNode()   {}
func (*Thunk) coreNode()  {}
func (*Force) coreNode()  {}
func (*PrimOp) coreNode() {}
func (*Lit) coreNode()    {}

// --- Span accessors ---
func (c *Var) Span() span.Span    { return c.S }
func (c *Lam) Span() span.Span    { return c.S }
func (c *App) Span() span.Span    { return c.S }
func (c *TyApp) Span() span.Span  { return c.S }
func (c *TyLam) Span() span.Span  { return c.S }
func (c *Con) Span() span.Span    { return c.S }
func (c *Case) Span() span.Span   { return c.S }
func (c *LetRec) Span() span.Span { return c.S }
func (c *Pure) Span() span.Span   { return c.S }
func (c *Bind) Span() span.Span   { return c.S }
func (c *Thunk) Span() span.Span  { return c.S }
func (c *Force) Span() span.Span  { return c.S }
func (c *PrimOp) Span() span.Span { return c.S }
func (c *Lit) Span() span.Span    { return c.S }
