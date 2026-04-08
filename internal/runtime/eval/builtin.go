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

// ForceBody returns the IR for the force builtin: \thk. force thk
func ForceBody() *ir.Lam {
	return &ir.Lam{Param: "_thk", Body: &ir.Force{Expr: &ir.Var{Name: "_thk"}}}
}

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
