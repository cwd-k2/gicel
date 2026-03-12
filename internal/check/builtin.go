package check

import "github.com/cwd-k2/gomputation/pkg/types"

// Built-in type signatures.
var builtinTypes = map[string]types.Type{
	// pure : forall a r. a -> Computation r r a
	"pure": types.MkForall("a", types.KType{},
		types.MkForall("r", types.KRow{},
			types.MkArrow(
				types.Var("a"),
				types.MkComp(types.Var("r"), types.Var("r"), types.Var("a")),
			))),

	// bind : forall a b r1 r2 r3. Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b
	"bind": types.MkForall("a", types.KType{},
		types.MkForall("b", types.KType{},
			types.MkForall("r1", types.KRow{},
				types.MkForall("r2", types.KRow{},
					types.MkForall("r3", types.KRow{},
						types.MkArrow(
							types.MkComp(types.Var("r1"), types.Var("r2"), types.Var("a")),
							types.MkArrow(
								types.MkArrow(types.Var("a"),
									types.MkComp(types.Var("r2"), types.Var("r3"), types.Var("b"))),
								types.MkComp(types.Var("r1"), types.Var("r3"), types.Var("b")),
							))))))),

	// thunk : forall a r1 r2. Computation r1 r2 a -> Thunk r1 r2 a
	"thunk": types.MkForall("a", types.KType{},
		types.MkForall("r1", types.KRow{},
			types.MkForall("r2", types.KRow{},
				types.MkArrow(
					types.MkComp(types.Var("r1"), types.Var("r2"), types.Var("a")),
					types.MkThunk(types.Var("r1"), types.Var("r2"), types.Var("a")),
				)))),

	// force : forall a r1 r2. Thunk r1 r2 a -> Computation r1 r2 a
	"force": types.MkForall("a", types.KType{},
		types.MkForall("r1", types.KRow{},
			types.MkForall("r2", types.KRow{},
				types.MkArrow(
					types.MkThunk(types.Var("r1"), types.Var("r2"), types.Var("a")),
					types.MkComp(types.Var("r1"), types.Var("r2"), types.Var("a")),
				)))),
}

// gatedBuiltins are built-ins that require host opt-in.
var gatedBuiltins = map[string]bool{
	"rec": true,
	"fix": true,
}
