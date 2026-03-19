package core

import (
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// Core is a term in the core intermediate representation.
// 17 formers: Var, Lam, App, TyApp, TyLam, Con, Case, Fix, Pure, Bind, Thunk, Force, PrimOp, Lit, RecordLit, RecordProj, RecordUpdate.
type Core interface {
	coreNode()
	Span() span.Span
}

// Var — variable reference.
// Module is non-empty for qualified imports (canonical module name, not alias).
type Var struct {
	Name   string
	Module string // "" = local/open, "Std.Num" = qualified import origin
	S      span.Span
}

// Lam — lambda abstraction.
type Lam struct {
	Param     string
	ParamType types.Type
	Body      Core
	FV        []string // Free variables (populated by AnnotateFreeVars)
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

// TyLam — type abstraction (elaboration of \). Erased at runtime.
type TyLam struct {
	TyParam string
	Kind    types.Kind
	Body    Core
	S       span.Span
}

// Con — constructor application (C e1 ... en).
// Module is non-empty for qualified imports (canonical module name, not alias).
type Con struct {
	Name   string
	Module string // "" = local/open, "Std.Num" = qualified import origin
	Args   []Core
	S      span.Span
}

// Case — case analysis.
type Case struct {
	Scrutinee Core
	Alts      []Alt
	S         span.Span
}

// Fix — fixed-point combinator.
// Name is bound in Body. Evaluation peels type erasure (TyLam),
// expects a Lam at the core, and ties the knot so that Name
// refers to the resulting closure itself.
type Fix struct {
	Name string
	Body Core
	S    span.Span
}

// Pure — computation introduction (pure e).
type Pure struct {
	Expr Core
	S    span.Span
}

// Bind — computation sequencing (bind c (\x. e)).
type Bind struct {
	Comp Core
	Var  string
	Body Core
	S    span.Span
}

// Thunk — suspend computation (thunk c).
type Thunk struct {
	Comp Core
	FV   []string // Free variables (populated by AnnotateFreeVars)
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

// Lit — literal value (Int, Double, String, Rune).
type Lit struct {
	Value any // int64, float64, string, or rune
	S     span.Span
}

// RecordLit — record construction { l1: e1, ..., ln: en }.
type RecordLit struct {
	Fields []RecordField
	S      span.Span
}

// RecordField is a label-value pair in a record literal or update.
type RecordField struct {
	Label string
	Value Core
}

// RecordProj — field projection r.#l.
type RecordProj struct {
	Record Core
	Label  string
	S      span.Span
}

// RecordUpdate — record update { r | l1: e1, ..., ln: en }.
type RecordUpdate struct {
	Record  Core
	Updates []RecordField
	S       span.Span
}

// --- coreNode markers ---
func (*Var) coreNode()          {}
func (*Lam) coreNode()          {}
func (*App) coreNode()          {}
func (*TyApp) coreNode()        {}
func (*TyLam) coreNode()        {}
func (*Con) coreNode()          {}
func (*Case) coreNode()         {}
func (*Fix) coreNode()          {}
func (*Pure) coreNode()         {}
func (*Bind) coreNode()         {}
func (*Thunk) coreNode()        {}
func (*Force) coreNode()        {}
func (*PrimOp) coreNode()       {}
func (*Lit) coreNode()          {}
func (*RecordLit) coreNode()    {}
func (*RecordProj) coreNode()   {}
func (*RecordUpdate) coreNode() {}

// --- Span accessors ---
func (c *Var) Span() span.Span          { return c.S }
func (c *Lam) Span() span.Span          { return c.S }
func (c *App) Span() span.Span          { return c.S }
func (c *TyApp) Span() span.Span        { return c.S }
func (c *TyLam) Span() span.Span        { return c.S }
func (c *Con) Span() span.Span          { return c.S }
func (c *Case) Span() span.Span         { return c.S }
func (c *Fix) Span() span.Span          { return c.S }
func (c *Pure) Span() span.Span         { return c.S }
func (c *Bind) Span() span.Span         { return c.S }
func (c *Thunk) Span() span.Span        { return c.S }
func (c *Force) Span() span.Span        { return c.S }
func (c *PrimOp) Span() span.Span       { return c.S }
func (c *Lit) Span() span.Span          { return c.S }
func (c *RecordLit) Span() span.Span    { return c.S }
func (c *RecordProj) Span() span.Span   { return c.S }
func (c *RecordUpdate) Span() span.Span { return c.S }
