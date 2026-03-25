package env

import "github.com/cwd-k2/gicel/internal/lang/types"

// ModuleOwnership carries the ownership signals for a compiled module:
// which type and value names were defined by this module (not inherited).
// Used by import resolution for ambiguity detection and selective imports.
type ModuleOwnership struct {
	OwnedTypeNames     map[string]bool     // data type names defined by this module
	OwnedNames         map[string]bool     // data type names + constructor names defined by this module
	ConstructorsByType map[string][]string // type name → constructor names (precomputed index)
}

// ModuleExports carries the type-level information exported by a compiled module.
// Semantic exports (Types through TypeFamilies) describe what the module provides.
// Embedded ModuleOwnership tracks which names this module defines vs. inherits.
type ModuleExports struct {
	Types           map[string]types.Kind      // registered type constructors
	ConTypes        map[string]types.Type      // constructor → full type
	ConstructorInfo map[string]*DataTypeInfo   // constructor → data type info
	Aliases         map[string]*AliasInfo      // type aliases
	Classes         map[string]*ClassInfo      // class declarations
	Instances       []*InstanceInfo            // instance declarations
	Values          map[string]types.Type      // top-level value types
	PromotedKinds   map[string]types.Kind      // DataKinds promotions
	PromotedCons    map[string]types.Kind      // promoted constructors
	TypeFamilies    map[string]*TypeFamilyInfo // type family declarations
	ModuleOwnership                            // ownership signals
}
