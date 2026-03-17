package check

import "github.com/cwd-k2/gicel/internal/types"

// Built-in type signatures.
// When fully applied, pure and bind are optimized to Core.Pure / Core.Bind
// by the checker; as standalone values they resolve through builtinTypes.
// thunk and force are handled as special forms and elaborate to Core nodes.
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

	// pure : forall a (r : Row). a -> Computation r r a
	"pure": types.MkForall("a", types.KType{},
		types.MkForall("r", types.KRow{},
			types.MkArrow(
				types.Var("a"),
				types.MkComp(types.Var("r"), types.Var("r"), types.Var("a")),
			))),

	// bind : forall a b (r1 : Row) (r2 : Row) (r3 : Row).
	//   Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b
	"bind": types.MkForall("a", types.KType{},
		types.MkForall("b", types.KType{},
			types.MkForall("r1", types.KRow{},
				types.MkForall("r2", types.KRow{},
					types.MkForall("r3", types.KRow{},
						types.MkArrow(
							types.MkComp(types.Var("r1"), types.Var("r2"), types.Var("a")),
							types.MkArrow(
								types.MkArrow(types.Var("a"), types.MkComp(types.Var("r2"), types.Var("r3"), types.Var("b"))),
								types.MkComp(types.Var("r1"), types.Var("r3"), types.Var("b")),
							),
						)))))),
}

// gatedBuiltins are built-ins that require host opt-in.
var gatedBuiltins = map[string]bool{
	"rec": true,
	"fix": true,
}
