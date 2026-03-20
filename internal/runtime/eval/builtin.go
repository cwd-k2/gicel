package eval

import "github.com/cwd-k2/gicel/internal/lang/ir"

// BuiltinEnv constructs the base environment with pure, bind, force,
// and optional fix/rec closures. pure and bind are first-class functions
// here; the checker also optimizes fully-applied pure/bind to direct
// Core.Pure/Core.Bind nodes for capability environment threading.
func BuiltinEnv(enableFix, enableRec bool) *Env {
	env := EmptyEnv()

	// pure: a -> a (identity in CBV)
	env = env.Extend("pure", &Closure{
		Env: EmptyEnv(), Param: "_v",
		Body: &ir.Var{Name: "_v"},
	})

	// bind: m -> (a -> m) -> m (apply continuation)
	bindBody := &ir.Lam{
		Param: "_f",
		Body:  &ir.App{Fun: &ir.Var{Name: "_f"}, Arg: &ir.Var{Name: "_comp"}},
	}
	env = env.Extend("bind", &Closure{
		Env: EmptyEnv(), Param: "_comp",
		Body: bindBody,
	})

	// force: Thunk -> Computation
	env = env.Extend("force", &Closure{
		Env: EmptyEnv(), Param: "_thk",
		Body: &ir.Force{Expr: &ir.Var{Name: "_thk"}},
	})

	// Gated built-ins: rec and fix (enabled via EnableRecursion).
	// Both share the same fixpoint body; two names exist for user ergonomics
	// (fix for explicit fixed-point, rec for recursive let sugar).
	if enableFix {
		env = env.Extend("fix", &Closure{
			Env: EmptyEnv(), Param: "_f",
			Body: fixBody(),
		})
	}
	if enableRec {
		env = env.Extend("rec", &Closure{
			Env: EmptyEnv(), Param: "_f",
			Body: fixBody(),
		})
	}

	return env
}

// fixBody returns a Fix node for the fix/rec builtin closures.
// fix f = the fixpoint of f, i.e. x where x = \arg. (f x) arg.
func fixBody() *ir.Fix {
	return &ir.Fix{
		Name: "_x",
		Body: &ir.Lam{Param: "_arg", Body: &ir.App{
			Fun: &ir.App{Fun: &ir.Var{Name: "_f"}, Arg: &ir.Var{Name: "_x"}},
			Arg: &ir.Var{Name: "_arg"},
		}},
	}
}
