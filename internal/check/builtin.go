package check

import "github.com/cwd-k2/gomputation/internal/types"

// Built-in type signatures.
// pure, bind, thunk, force are handled as special forms in the checker
// and elaborate directly to Core nodes (Pure, Bind, Thunk, Force).
var builtinTypes = map[string]types.Type{
	// rec : forall (r : Row) a. (Computation r r a -> Computation r r a) -> Computation r r a
	// Computation-level fixpoint; requires pre = post.
	"rec": types.MkForall("r", types.KRow{},
		types.MkForall("a", types.KType{},
			types.MkArrow(
				types.MkArrow(
					types.MkComp(types.Var("r"), types.Var("r"), types.Var("a")),
					types.MkComp(types.Var("r"), types.Var("r"), types.Var("a")),
				),
				types.MkComp(types.Var("r"), types.Var("r"), types.Var("a")),
			))),

	// fix : forall a. (a -> a) -> a
	// Value-level fixpoint combinator.
	"fix": types.MkForall("a", types.KType{},
		types.MkArrow(
			types.MkArrow(types.Var("a"), types.Var("a")),
			types.Var("a"),
		)),
}

// gatedBuiltins are built-ins that require host opt-in.
var gatedBuiltins = map[string]bool{
	"rec": true,
	"fix": true,
}
