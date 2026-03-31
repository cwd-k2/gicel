package check

import "github.com/cwd-k2/gicel/internal/lang/types"

// Built-in type signatures.
// When fully applied, pure and bind are optimized to Core.Pure / Core.Bind
// by the checker; as standalone values they resolve through builtinTypes.
// thunk and force are handled as special forms and elaborate to Core nodes.
var builtinTypes = map[string]types.Type{
	// rec : \ (r: Row) a g. (Computation r r a @g -> Computation r r a @g) -> Computation r r a @g
	// Computation-level fixpoint; requires pre = post.
	"rec": types.MkForall("r", types.TypeOfRows,
		types.MkForall("a", types.TypeOfTypes,
			types.MkForall("g", types.TypeOfTypes,
				types.MkArrow(
					types.MkArrow(
						types.MkCompGraded(types.Var("r"), types.Var("r"), types.Var("a"), types.Var("g")),
						types.MkCompGraded(types.Var("r"), types.Var("r"), types.Var("a"), types.Var("g")),
					),
					types.MkCompGraded(types.Var("r"), types.Var("r"), types.Var("a"), types.Var("g")),
				)))),

	// fix : \ a. (a -> a) -> a
	// Value-level fixpoint combinator.
	"fix": types.MkForall("a", types.TypeOfTypes,
		types.MkArrow(
			types.MkArrow(types.Var("a"), types.Var("a")),
			types.Var("a"),
		)),

	// pure : \ a (r: Row) g. a -> Computation r r a @g
	"pure": types.MkForall("a", types.TypeOfTypes,
		types.MkForall("r", types.TypeOfRows,
			types.MkForall("g", types.TypeOfTypes,
				types.MkArrow(
					types.Var("a"),
					types.MkCompGraded(types.Var("r"), types.Var("r"), types.Var("a"), types.Var("g")),
				)))),

	// bind : \ a b g1 g2 (r1: Row) (r2: Row) (r3: Row).
	//   Computation r1 r2 a @g1 -> (a -> Computation r2 r3 b @g2) -> Computation r1 r3 b @g2
	"bind": types.MkForall("a", types.TypeOfTypes,
		types.MkForall("b", types.TypeOfTypes,
			types.MkForall("g1", types.TypeOfTypes,
				types.MkForall("g2", types.TypeOfTypes,
					types.MkForall("r1", types.TypeOfRows,
						types.MkForall("r2", types.TypeOfRows,
							types.MkForall("r3", types.TypeOfRows,
								types.MkArrow(
									types.MkCompGraded(types.Var("r1"), types.Var("r2"), types.Var("a"), types.Var("g1")),
									types.MkArrow(
										types.MkArrow(types.Var("a"), types.MkCompGraded(types.Var("r2"), types.Var("r3"), types.Var("b"), types.Var("g2"))),
										types.MkCompGraded(types.Var("r1"), types.Var("r3"), types.Var("b"), types.Var("g2")),
									),
								)))))))),
}

// gatedBuiltins are built-ins that require host opt-in.
var gatedBuiltins = map[string]bool{
	"rec": true,
	"fix": true,
}
