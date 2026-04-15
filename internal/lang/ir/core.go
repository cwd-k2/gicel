package ir

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// GenKind classifies the origin of compiler-generated IR nodes.
// Zero value (UserWritten) means user-authored. Non-zero values
// identify which compiler pass introduced the node.
type GenKind uint8

const (
	UserWritten    GenKind = 0 // user-authored (default zero value)
	GenDict        GenKind = 1 // dictionary parameter or binding (instance elaboration)
	GenAutoForce   GenKind = 2 // auto-force lazy field extraction (pattern desugar)
	GenSection     GenKind = 3 // operator section desugaring
	GenDictExtract GenKind = 4 // method selector / dict field extraction
	GenAutoBind    GenKind = 5 // CBPV implicit bind variable
)

// IsGenerated reports whether the node is compiler-generated.
func (g GenKind) IsGenerated() bool { return g != 0 }

// Core is a term in the core intermediate representation.
// 20 formers: Var, Lam, App, TyApp, TyLam, Con, Case, Fix, Pure, Bind, Thunk, Force, Merge, PrimOp, Lit, RecordLit, RecordProj, RecordUpdate, VariantLit, Error.
type Core interface {
	coreNode()
	Span() span.Span
}

// Var — variable reference.
// Module is non-empty for qualified imports (canonical module name, not alias).
// Key is the pre-computed environment lookup key (populated by AnnotateFreeVars).
// Index is the de Bruijn index (populated by AssignIndices); -1 = not yet assigned.
type Var struct {
	Name   string
	Module string // "" = local/open, "Std.Num" = qualified import origin
	Index  int    // de Bruijn index (-1 = unassigned)
	S      span.Span
}

// Lam — lambda abstraction.
//
// FV metadata (names and de Bruijn indices of captured variables) is stored
// in a separate *FVAnnotations side table, not on the node itself. This keeps
// Lam phase-invariant: its structural identity is independent of whether any
// free-variable analysis has been run.
type Lam struct {
	Param     string
	ParamType types.Type
	Body      Core
	Generated GenKind // non-zero when introduced by the compiler
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
	Kind    types.Type
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
	Comp      Core
	Var       string
	Body      Core
	Discard   bool    // true when the bound value is unused (wildcard bind)
	Generated GenKind // non-zero when Var is compiler-introduced
	S         span.Span
}

// Thunk — suspend computation (thunk c).
//
// FV metadata lives in the FVAnnotations side table; see Lam for rationale.
type Thunk struct {
	Comp Core
	S    span.Span
}

// Force — resume suspended computation (force e).
type Force struct {
	Expr Core
	S    span.Span
}

// Merge — parallel composition of two computations.
// Splits CapEnv by label sets, runs both computations, reunites CapEnvs.
// LeftLabels/RightLabels carry the final, refined capability label sets:
// the checker first records tentative labels at inferMerge time, then
// rewrites them after constraint resolution. The transient pre-state types
// needed to re-extract labels are kept in a checker-local side table
// (compiler/check/checker.go), not on the IR node, so the node has no
// hidden phase distinction — by the time the IR leaves the checker, the
// labels are final and the struct contains no transient state.
//
// FV metadata (separately for Left and Right) lives in the FVAnnotations
// side table; see Lam for rationale.
type Merge struct {
	Left        Core
	Right       Core
	LeftLabels  []string // capability labels for left computation
	RightLabels []string // capability labels for right computation
	S           span.Span
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
	Fields []Field
	S      span.Span
}

// Field is a label-value pair in a record literal or update.
type Field struct {
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
	Updates []Field
	S       span.Span
}

// VariantLit — variant construction (elaborated from inject @#tag expr).
type VariantLit struct {
	Tag   string
	Value Core
	S     span.Span
}

// Error — placeholder for erroneous expressions that failed type checking.
// Never reaches the evaluator; present only so downstream IR passes
// can propagate without crashing after errors have been reported.
type Error struct {
	S span.Span
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
func (*Merge) coreNode()        {}
func (*PrimOp) coreNode()       {}
func (*Lit) coreNode()          {}
func (*RecordLit) coreNode()    {}
func (*RecordProj) coreNode()   {}
func (*RecordUpdate) coreNode() {}
func (*VariantLit) coreNode()   {}
func (*Error) coreNode()        {}

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
func (c *Merge) Span() span.Span        { return c.S }
func (c *PrimOp) Span() span.Span       { return c.S }
func (c *Lit) Span() span.Span          { return c.S }
func (c *RecordLit) Span() span.Span    { return c.S }
func (c *RecordProj) Span() span.Span   { return c.S }
func (c *RecordUpdate) Span() span.Span { return c.S }
func (c *VariantLit) Span() span.Span   { return c.S }
func (c *Error) Span() span.Span        { return c.S }
