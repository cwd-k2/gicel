package check

import "github.com/cwd-k2/gicel/internal/lang/types"

// makeBuiltinTypes constructs built-in type signatures using the given TypeOps.
// When fully applied, pure and bind are optimized to Core.Pure / Core.Bind
// by the checker; as standalone values they resolve through builtinTypes.
// thunk and force are handled as special forms and elaborate to Core nodes.
func makeBuiltinTypes(ops *types.TypeOps) map[string]types.Type {
	return map[string]types.Type{
		// rec : \ (r: Row) a g. (Computation r r a @g -> Computation r r a @g) -> Computation r r a @g
		// Computation-level fixpoint; requires pre = post.
		"rec": ops.Forall("r", types.TypeOfRows,
			ops.Forall("a", types.TypeOfTypes,
				ops.Forall("g", types.TypeOfTypes,
					ops.Arrow(
						ops.Arrow(
							ops.Comp(ops.Var("r"), ops.Var("r"), ops.Var("a"), ops.Var("g")),
							ops.Comp(ops.Var("r"), ops.Var("r"), ops.Var("a"), ops.Var("g")),
						),
						ops.Comp(ops.Var("r"), ops.Var("r"), ops.Var("a"), ops.Var("g")),
					)))),

		// fix : \ a. (a -> a) -> a
		// Value-level fixpoint combinator.
		"fix": ops.Forall("a", types.TypeOfTypes,
			ops.Arrow(
				ops.Arrow(ops.Var("a"), ops.Var("a")),
				ops.Var("a"),
			)),

		// pure : \ a (r: Row) g. a -> Computation r r a @g
		"pure": ops.Forall("a", types.TypeOfTypes,
			ops.Forall("r", types.TypeOfRows,
				ops.Forall("g", types.TypeOfTypes,
					ops.Arrow(
						ops.Var("a"),
						ops.Comp(ops.Var("r"), ops.Var("r"), ops.Var("a"), ops.Var("g")),
					)))),

		// bind : \ a b g1 g2 g3 (r1: Row) (r2: Row) (r3: Row).
		//   Computation g1 r1 r2 a -> (a -> Computation g2 r2 r3 b) -> Computation g3 r1 r3 b
		// g3 is the composed grade — resolved by inferBind (composeGrades) or
		// unified with GradeCompose(e1,e2) when used as GIMonad gibind.
		"bind": ops.Forall("a", types.TypeOfTypes,
			ops.Forall("b", types.TypeOfTypes,
				ops.Forall("g1", types.TypeOfTypes,
					ops.Forall("g2", types.TypeOfTypes,
						ops.Forall("g3", types.TypeOfTypes,
							ops.Forall("r1", types.TypeOfRows,
								ops.Forall("r2", types.TypeOfRows,
									ops.Forall("r3", types.TypeOfRows,
										ops.Arrow(
											ops.Comp(ops.Var("r1"), ops.Var("r2"), ops.Var("a"), ops.Var("g1")),
											ops.Arrow(
												ops.Arrow(ops.Var("a"), ops.Comp(ops.Var("r2"), ops.Var("r3"), ops.Var("b"), ops.Var("g2"))),
												ops.Comp(ops.Var("r1"), ops.Var("r3"), ops.Var("b"), ops.Var("g3")),
											),
										))))))))),
	}
}

// gatedBuiltins are built-ins that require host opt-in.
var gatedBuiltins = map[string]bool{
	"rec": true,
	"fix": true,
}
