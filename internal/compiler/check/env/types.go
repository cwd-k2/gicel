package env

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// CoreModuleName is the canonical name of the auto-imported Core module.
const CoreModuleName = "Core"

// DictName returns the dictionary type constructor name for a class.
// The suffix is sourced from ir.DictSuffix — the single canonical
// definition of the naming convention for type class elaboration.
func DictName(className string) string { return className + ir.DictSuffix }

// AliasInfo holds the definition of a type alias: parameter names, their kinds, and the body.
type AliasInfo struct {
	Params     []string
	ParamKinds []types.Type
	Body       types.Type
}

// ClassInfo holds the elaborated information for a type class.
type ClassInfo struct {
	Name         string
	TyParams     []string
	TyParamKinds []types.Type
	KindParams   []string        // implicit kind variables (e.g., "k" in f: k -> Type)
	Supers       []SuperInfo     // superclass constraints
	Methods      []MethodInfo    // method signatures
	DictName     string          // e.g. "Eq$Dict" — used as both type and constructor name
	AssocTypes   []string        // associated type family names
	SuperClosure map[string]bool // transitive superclass names (computed after validation)
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
	IsPrivate    bool             // true for impl _name (solver-invisible outside defining module)
	FreeVars     []FreeVarInfo    // cached free type variables with kinds (computed once at registration)
	S            span.Span
}

// IsNamed reports whether this instance has a user-visible name (impl name :: ...).
func (i *InstanceInfo) IsNamed() bool { return i.UserName != "" }

// FreeVarInfo pairs a free variable name with its inferred kind.
type FreeVarInfo struct {
	Name string
	Kind types.Type
}

// ConstraintInfo represents a constraint in instance context.
type ConstraintInfo struct {
	ClassName string
	Args      []types.Type
}

// --- Exhaustiveness checking types ---

// DataTypeInfo carries constructor information for exhaustiveness checking.
type DataTypeInfo struct {
	Name         string
	IsLazy       bool // lazy co-data: auto-thunk at construction, auto-force at pattern match
	Constructors []ConstructorInfo
}

// ConstructorInfo is a constructor's name, arity, and optional GADT return type.
type ConstructorInfo struct {
	Name       string
	Arity      int
	ReturnType types.Type // GADT: non-nil if constructor has refined return type
}

// IsGADT reports whether this constructor has a refined return type (GADT constructor).
func (c *ConstructorInfo) IsGADT() bool { return c.ReturnType != nil }

// --- Type family types ---

// TypeFamilyInfo holds the elaborated information for a type family.
type TypeFamilyInfo struct {
	Name       string
	Params     []TFParam
	ResultKind types.Type
	ResultName string  // non-empty if injective
	Deps       []TFDep // injectivity deps (elaborated)
	Equations  []TFEquation
	IsAssoc    bool   // true if declared as associated type in a class
	ClassName  string // non-empty if IsAssoc
}

// IsInjective reports whether this type family is declared as injective.
func (f *TypeFamilyInfo) IsInjective() bool { return f.ResultName != "" }

// IsAssocFor reports whether this type family is an associated type of the given class.
func (f *TypeFamilyInfo) IsAssocFor(className string) bool {
	return f.IsAssoc && f.ClassName == className
}

// TFParam is a type family parameter with its name and kind.
type TFParam struct {
	Name string
	Kind types.Type
}

// TFDep is an elaborated functional dependency.
type TFDep struct {
	From string   // result variable name
	To   []string // determined parameter names
}

// TFEquation is an elaborated type family equation with resolved types.
type TFEquation struct {
	Patterns []types.Type // LHS patterns (resolved)
	RHS      types.Type   // RHS body (resolved)
	S        span.Span
}

// Clone returns a deep copy of the TypeFamilyInfo, isolating the Equations
// slice so that subsequent compilations cannot mutate shared module metadata.
func (f *TypeFamilyInfo) Clone() *TypeFamilyInfo {
	cp := *f
	cp.Params = append([]TFParam(nil), f.Params...)
	cp.Deps = append([]TFDep(nil), f.Deps...)
	cp.Equations = append([]TFEquation(nil), f.Equations...)
	return &cp
}

// MatchResult classifies the outcome of type-level pattern matching.
type MatchResult int

const (
	MatchSuccess       MatchResult = iota // patterns matched, substitution available
	MatchFail                             // patterns definitely do not match
	MatchIndeterminate                    // cannot decide (unsolved metavariable vs concrete pattern)
)
