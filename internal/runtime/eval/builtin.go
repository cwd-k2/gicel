package eval

import "github.com/cwd-k2/gicel/internal/lang/ir"

// BuiltinEnv constructs the base environment with pure, bind, force,
// and optional fix/rec closures. pure and bind are first-class functions
// here; the checker also optimizes fully-applied pure/bind to direct
// Core.Pure/Core.Bind nodes for capability environment threading.
//
// Builtin closures use global variable references (Index = -1) because
// they are registered in the globals map, not the locals stack.
func BuiltinEnv(enableFix, enableRec bool) *Env {
	env := EmptyEnv()

	// pure: a -> a (identity in CBV)
	// The param "_v" is a local variable at index 0 in the closure body.
	env.Extend("pure", &Closure{
		Env: EmptyEnv(), Param: "_v",
		Body: &ir.Var{Name: "_v", Index: 0},
	})

	// bind: m -> (a -> m) -> m (apply continuation)
	// _comp is the outer param, captured in the inner closure.
	// In the inner closure body: _f at index 0 (param), _comp at index 1 (captured).
	bindBody := &ir.Lam{
		Param:     "_f",
		FVIndices: []int{0}, // capture _comp from outer closure env
		Body:      &ir.App{Fun: &ir.Var{Name: "_f", Index: 0}, Arg: &ir.Var{Name: "_comp", Index: 1}},
	}
	env.Extend("bind", &Closure{
		Env: EmptyEnv(), Param: "_comp",
		Body: bindBody,
	})

	// force: Thunk -> Computation
	env.Extend("force", &Closure{
		Env: EmptyEnv(), Param: "_thk",
		Body: &ir.Force{Expr: &ir.Var{Name: "_thk", Index: 0}},
	})

	// Gated built-ins: rec and fix.
	if enableFix {
		env.Extend("fix", &Closure{
			Env: EmptyEnv(), Param: "_f",
			Body: fixBody(),
		})
	}
	if enableRec {
		env.Extend("rec", &Closure{
			Env: EmptyEnv(), Param: "_f",
			Body: fixBody(),
		})
	}

	return env
}

// fixBody returns a Fix node for the fix/rec builtin closures.
// fix f = the fixpoint of f, i.e. x where x = \arg. (f x) arg.
//
// After capture (see assignLam in index.go for Fix):
//   _f is global (Index = -1) — captured from the enclosing closure
//   Actually _f is the param of the outer fix closure, so it's local.
//
// Layout inside the Fix Lam body:
//   _arg at index 0 (Lam param)
//   _x at index 1 (Fix self-reference)
//   _f at index 2 (captured from enclosing scope)
func fixBody() *ir.Fix {
	return &ir.Fix{
		Name: "_x",
		Body: &ir.Lam{
			Param:     "_arg",
			FVIndices: []int{0}, // capture _f from Fix's enclosing scope
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
