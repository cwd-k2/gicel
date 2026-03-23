package family

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// TypeFamilyInfo holds the elaborated information for a type family.
type TypeFamilyInfo struct {
	Name       string
	Params     []TFParam
	ResultKind types.Kind
	ResultName string  // non-empty if injective
	Deps       []TFDep // injectivity deps (elaborated)
	Equations  []TFEquation
	IsAssoc    bool   // true if declared as associated type in a class
	ClassName  string // non-empty if IsAssoc
}

// TFParam is a type family parameter with its name and kind.
type TFParam struct {
	Name string
	Kind types.Kind
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
