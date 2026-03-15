package stdlib

// CoreSource contains Computation-essential definitions: IxMonad class,
// Computation instance, kind-lifting alias, effect alias, and the then combinator.
// Always loaded as the first section of the Prelude module.
var CoreSource = mustReadSource("core")

// PreludeSource is the default prelude: standard data types, type classes, and instances.
// Auto-loaded after CoreSource unless NoPrelude is set.
var PreludeSource = mustReadSource("prelude")
