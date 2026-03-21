package env

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/exhaust"
	"github.com/cwd-k2/gicel/internal/compiler/check/family"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// ModuleExports carries the type-level information exported by a compiled module.
type ModuleExports struct {
	Types              map[string]types.Kind              // registered type constructors
	ConTypes           map[string]types.Type              // constructor → full type
	ConstructorInfo    map[string]*exhaust.DataTypeInfo   // constructor → data type info
	ConstructorsByType map[string][]string                // type name → constructor names (precomputed index)
	Aliases            map[string]*AliasInfo              // type aliases
	Classes            map[string]*ClassInfo              // class declarations
	Instances          []*InstanceInfo                    // instance declarations
	Values             map[string]types.Type              // top-level value types
	PromotedKinds      map[string]types.Kind              // DataKinds promotions
	PromotedCons       map[string]types.Kind              // promoted constructors
	TypeFamilies       map[string]*family.TypeFamilyInfo  // type family declarations
	OwnedTypeNames     map[string]bool                    // data type names defined by this module
	OwnedNames         map[string]bool                    // type names + constructor names defined by this module
}
