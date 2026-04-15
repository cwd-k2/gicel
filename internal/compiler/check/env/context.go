package env

import "github.com/cwd-k2/gicel/internal/lang/types"

// CtxEntry is an entry in the typing context.
type CtxEntry interface {
	CtxEntry()
}

// CtxVar holds a variable binding in the context.
type CtxVar struct {
	Name            string
	Type            types.Type
	Module          string // source module ("" = local/builtin, "Prelude" = from module)
	IsSolverInvisible bool // true: not used by instance resolution (private instance user names)
	DictClassName   string // non-empty for class dictionary variables (enables indexed lookup)
}

// CtxTyVar holds a type variable binding in the context.
type CtxTyVar struct {
	Name string
	Kind types.Type
}

// CtxEvidence records available type class evidence in the context.
type CtxEvidence struct {
	ClassName  string
	Args       []types.Type
	DictName   string                      // context variable name for the dictionary
	DictType   types.Type                  // dictionary type
	Quantified *types.QuantifiedConstraint // non-nil for quantified constraints
}

func (*CtxVar) CtxEntry()      {}
func (*CtxTyVar) CtxEntry()    {}
func (*CtxEvidence) CtxEntry() {}

// HasDictClass reports whether this variable is a class dictionary variable (indexed lookup).
func (v *CtxVar) HasDictClass() bool { return v.DictClassName != "" }

// IsClassEvidence reports whether this evidence entry has an associated class.
func (e *CtxEvidence) IsClassEvidence() bool { return e.ClassName != "" }
