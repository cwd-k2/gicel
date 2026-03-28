package eval

import "github.com/cwd-k2/gicel/internal/lang/ir"

// PureBody returns the IR for the pure builtin: \v. v
func PureBody() *ir.Lam {
	return &ir.Lam{Param: "_v", Body: &ir.Var{Name: "_v", Index: 0}}
}

// BindBody returns the IR for the bind builtin: \comp. \f. f comp
func BindBody() *ir.Lam {
	return &ir.Lam{
		Param: "_comp",
		Body: &ir.Lam{
			Param:     "_f",
			FV:        []string{"_comp"},
			FVIndices: []int{0},
			Body:      &ir.App{Fun: &ir.Var{Name: "_f", Index: 0}, Arg: &ir.Var{Name: "_comp", Index: 1}},
		},
	}
}

// ForceBody returns the IR for the force builtin: \thk. force thk
func ForceBody() *ir.Lam {
	return &ir.Lam{Param: "_thk", Body: &ir.Force{Expr: &ir.Var{Name: "_thk", Index: 0}}}
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
			Param:     "_arg",
			FV:        []string{"_f"},
			FVIndices: []int{0},
			Body: &ir.App{
				Fun: &ir.App{
					Fun: &ir.Var{Name: "_f", Index: 2},
					Arg: &ir.Var{Name: "_x", Index: 1},
				},
				Arg: &ir.Var{Name: "_arg", Index: 0},
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
				FV:        []string{"_f"},
				FVIndices: []int{0},
				Comp: &ir.App{
					Fun: &ir.Var{Name: "_f", Index: 1},
					Arg: &ir.Var{Name: "_thk", Index: 0},
				},
			},
		},
	}
}
