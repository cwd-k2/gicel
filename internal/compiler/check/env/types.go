package env

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// AliasInfo holds the definition of a type alias: parameter names, their kinds, and the body.
type AliasInfo struct {
	Params     []string
	ParamKinds []types.Kind
	Body       types.Type
}

// ClassInfo holds the elaborated information for a type class.
type ClassInfo struct {
	Name         string
	TyParams     []string
	TyParamKinds []types.Kind
	KindParams   []string     // implicit kind variables (e.g., "k" in f: k -> Type)
	Supers       []SuperInfo  // superclass constraints
	Methods      []MethodInfo // method signatures
	DictName     string       // e.g. "Eq$Dict" — used as both type and constructor name
	AssocTypes   []string     // associated type family names
}

// SuperInfo describes a superclass constraint.
type SuperInfo struct {
	ClassName string
	Args      []types.Type
}

// MethodInfo describes a class method.
type MethodInfo struct {
	Name string
	Type types.Type // the method type (with the class type params free)
}

// InstanceInfo holds the elaborated information for a type class instance.
type InstanceInfo struct {
	ClassName    string
	TypeArgs     []types.Type     // concrete type arguments
	Context      []ConstraintInfo // instance context constraints
	DictBindName string           // e.g. "Eq$Bool" or "Eq$(Maybe 'a)"
	UserName     string           // user-visible name from `impl name ::` ("" for anonymous)
	Module       string           // source module that defined this instance
	Private      bool             // true for impl _name (solver-invisible outside defining module)
	S            span.Span
}

// ConstraintInfo represents a constraint in instance context.
type ConstraintInfo struct {
	ClassName string
	Args      []types.Type
}
