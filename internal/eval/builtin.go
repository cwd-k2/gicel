package eval

import "github.com/cwd-k2/gicel/internal/core"

// BuiltinEnv constructs the base environment with pure, bind, force,
// and optional fix/rec closures. pure and bind are first-class functions
// here; the checker also optimizes fully-applied pure/bind to direct
// Core.Pure/Core.Bind nodes for capability environment threading.
func BuiltinEnv(enableFix, enableRec bool) *Env {
	env := EmptyEnv()

	// pure : a -> a (identity in CBV)
	env = env.Extend("pure", &Closure{
		Env: EmptyEnv(), Param: "_v",
		Body: &core.Var{Name: "_v"},
	})

	// bind : m -> (a -> m) -> m (apply continuation)
	bindBody := &core.Lam{
		Param: "_f",
		Body:  &core.App{Fun: &core.Var{Name: "_f"}, Arg: &core.Var{Name: "_comp"}},
	}
	env = env.Extend("bind", &Closure{
		Env: EmptyEnv(), Param: "_comp",
		Body: bindBody,
	})

	// force : Thunk -> Computation
	env = env.Extend("force", &Closure{
		Env: EmptyEnv(), Param: "_thk",
		Body: &core.Force{Expr: &core.Var{Name: "_thk"}},
	})

	// Gated built-ins: rec and fix (enabled via EnableRecursion).
	// Both share the same fixpoint body; two names exist for user ergonomics
	// (fix for explicit fixed-point, rec for recursive let sugar).
	if enableFix {
		env = env.Extend("fix", &Closure{
			Env: EmptyEnv(), Param: "_f",
			Body: fixpointBody(),
		})
	}
	if enableRec {
		env = env.Extend("rec", &Closure{
			Env: EmptyEnv(), Param: "_f",
			Body: fixpointBody(),
		})
	}

	return env
}

// fixpointBody returns the LetRec body shared by fix and rec.
// letrec x = \arg -> (f x) arg in x
func fixpointBody() *core.LetRec {
	return &core.LetRec{
		Bindings: []core.Binding{{
			Name: "_x",
			Expr: &core.Lam{Param: "_arg", Body: &core.App{
				Fun: &core.App{Fun: &core.Var{Name: "_f"}, Arg: &core.Var{Name: "_x"}},
				Arg: &core.Var{Name: "_arg"},
			}},
		}},
		Body: &core.Var{Name: "_x"},
	}
}
