package exhaust

import "github.com/cwd-k2/gicel/internal/types"

// DataTypeInfo carries constructor information for exhaustiveness checking.
type DataTypeInfo struct {
	Name         string
	Constructors []ConstructorInfo
}

// ConstructorInfo is a constructor's name, arity, and optional GADT return type.
type ConstructorInfo struct {
	Name       string
	Arity      int
	ReturnType types.Type // GADT: non-nil if constructor has refined return type
}
