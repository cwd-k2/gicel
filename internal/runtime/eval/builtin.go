package eval

import "github.com/cwd-k2/gicel/internal/lang/ir"

// These builders emit phase-invariant Core IR: they contain no FV metadata.
// Callers must run ir.AnnotateFreeVars followed by ir.AssignIndices on the
// returned tree (or compose it into a larger tree that undergoes the same
// pipeline) before handing it to the bytecode compiler.

// PureBody returns the IR for the pure builtin: \v. v
func PureBody() *ir.Lam {
	return &ir.Lam{Param: "_v", Body: &ir.Var{Name: "_v"}}
}

// BindBody returns the IR for the bind builtin: \comp. \f. f comp
func BindBody() *ir.Lam {
	return &ir.Lam{
		Param: "_comp",
		Body: &ir.Lam{
			Param: "_f",
			Body:  &ir.App{Fun: &ir.Var{Name: "_f"}, Arg: &ir.Var{Name: "_comp"}},
		},
	}
}

// Note: force has no first-class Prelude value. In CBV it would be
// expressible as `\thk. force thk`, but the checker deliberately treats
// `force` as a pure syntactic form (symmetric with `thunk`, which in CBV
// cannot be a function at all). See bidir.go's dispatch on ExprVar and
// the CBPV auto-coercion path in subsCheck / doElaborator / decl.go —
// the applied form `force e` elaborates directly to ir.Force without
// going through a Prelude value, and all indirect uses (do bindings,
// case arms, entry points, function arguments) are handled by the
// type-directed coercion. A bare `force` reference therefore has no
// runtime representation and the checker raises a syntactic error.

// FixBody returns the Fix IR node for the fix builtin.
func FixBody() *ir.Fix { return fixBody() }

// RecBody returns the Force(Fix(Thunk)) IR node for the rec builtin.
func RecBody() ir.Core { return recBody() }

// fixBody: fix f = let x = \arg. (f x) arg in x
func fixBody() *ir.Fix {
	return &ir.Fix{
		Name: "_x",
		Body: &ir.Lam{
			Param: "_arg",
			Body: &ir.App{
				Fun: &ir.App{
					Fun: &ir.Var{Name: "_f"},
					Arg: &ir.Var{Name: "_x"},
				},
				Arg: &ir.Var{Name: "_arg"},
			},
		},
	}
}

// recBody: rec f = force (fix _thk (thunk (f _thk)))
func recBody() ir.Core {
	return &ir.Force{
		Expr: &ir.Fix{
			Name: "_thk",
			Body: &ir.Thunk{
				Comp: &ir.App{
					Fun: &ir.Var{Name: "_f"},
					Arg: &ir.Var{Name: "_thk"},
				},
			},
		},
	}
}
