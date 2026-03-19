package env

import (
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
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
	KindParams   []string      // implicit kind variables (e.g., "k" in f: k -> Type)
	Supers       []SuperInfo   // superclass constraints
	Methods      []MethodInfo  // method signatures
	DictName     string        // e.g. "Eq$Dict" — used as both type and constructor name
	AssocTypes   []string      // associated type family names
	FunDeps      []ClassFunDep // functional dependencies: | a =: b
}

// ClassFunDep is an elaborated functional dependency on a class.
// From params determine To params: | a =: b means knowing a determines b.
type ClassFunDep struct {
	From []int // indices into TyParams
	To   []int // indices into TyParams
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
	Methods      map[string]syntax.Expr
	DictBindName string // e.g. "Eq$Bool" or "Eq$(Maybe 'a)"
	Module       string // source module that defined this instance
	S            span.Span
}

// ConstraintInfo represents a constraint in instance context.
type ConstraintInfo struct {
	ClassName string
	Args      []types.Type
}
