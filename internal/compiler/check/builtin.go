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
						types.MkCompGraded(types.MkVar("r"), types.MkVar("r"), types.MkVar("a"), types.MkVar("g")),
						types.MkCompGraded(types.MkVar("r"), types.MkVar("r"), types.MkVar("a"), types.MkVar("g")),
					),
					types.MkCompGraded(types.MkVar("r"), types.MkVar("r"), types.MkVar("a"), types.MkVar("g")),
				)))),

	// fix : \ a. (a -> a) -> a
	// Value-level fixpoint combinator.
	"fix": types.MkForall("a", types.TypeOfTypes,
		types.MkArrow(
			types.MkArrow(types.MkVar("a"), types.MkVar("a")),
			types.MkVar("a"),
		)),

	// pure : \ a (r: Row) g. a -> Computation r r a @g
	"pure": types.MkForall("a", types.TypeOfTypes,
		types.MkForall("r", types.TypeOfRows,
			types.MkForall("g", types.TypeOfTypes,
				types.MkArrow(
					types.MkVar("a"),
					types.MkCompGraded(types.MkVar("r"), types.MkVar("r"), types.MkVar("a"), types.MkVar("g")),
				)))),

	// bind : \ a b g1 g2 g3 (r1: Row) (r2: Row) (r3: Row).
	//   Computation g1 r1 r2 a -> (a -> Computation g2 r2 r3 b) -> Computation g3 r1 r3 b
	// g3 is the composed grade — resolved by inferBind (composeGrades) or
	// unified with GradeCompose(e1,e2) when used as GIMonad gibind.
	"bind": types.MkForall("a", types.TypeOfTypes,
		types.MkForall("b", types.TypeOfTypes,
			types.MkForall("g1", types.TypeOfTypes,
				types.MkForall("g2", types.TypeOfTypes,
					types.MkForall("g3", types.TypeOfTypes,
						types.MkForall("r1", types.TypeOfRows,
							types.MkForall("r2", types.TypeOfRows,
								types.MkForall("r3", types.TypeOfRows,
									types.MkArrow(
										types.MkCompGraded(types.MkVar("r1"), types.MkVar("r2"), types.MkVar("a"), types.MkVar("g1")),
										types.MkArrow(
											types.MkArrow(types.MkVar("a"), types.MkCompGraded(types.MkVar("r2"), types.MkVar("r3"), types.MkVar("b"), types.MkVar("g2"))),
											types.MkCompGraded(types.MkVar("r1"), types.MkVar("r3"), types.MkVar("b"), types.MkVar("g3")),
										),
									))))))))),
}

// gatedBuiltins are built-ins that require host opt-in.
var gatedBuiltins = map[string]bool{
	"rec": true,
	"fix": true,
}
