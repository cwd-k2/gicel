package eval

import "github.com/cwd-k2/gicel/internal/lang/ir"

// BuiltinGlobals constructs the base globals map with pure, bind, force,
// and optional fix/rec closures. pure and bind are first-class functions
// here; the checker also optimizes fully-applied pure/bind to direct
// Core.Pure/Core.Bind nodes for capability environment threading.
//
// Builtin closures use global variable references (Index = -1) because
// they are registered in the globals map, not the locals stack.
func BuiltinGlobals(enableFix, enableRec bool) map[string]Value {
	globals := make(map[string]Value, 8)

	// pure: a -> a (identity in CBV)
	// The param "_v" is a local variable at index 0 in the closure body.
	globals["pure"] = &Closure{
		Locals: nil, Param: "_v",
		Body: &ir.Var{Name: "_v", Index: 0},
	}

	// bind: m -> (a -> m) -> m (apply continuation)
	// _comp is the outer param, captured in the inner closure.
	// In the inner closure body: _f at index 0 (param), _comp at index 1 (captured).
	bindBody := &ir.Lam{
		Param:     "_f",
		FVIndices: []int{0}, // capture _comp from outer closure env
		Body:      &ir.App{Fun: &ir.Var{Name: "_f", Index: 0}, Arg: &ir.Var{Name: "_comp", Index: 1}},
	}
	globals["bind"] = &Closure{
		Locals: nil, Param: "_comp",
		Body: bindBody,
	}

	// force: Thunk -> Computation
	globals["force"] = &Closure{
		Locals: nil, Param: "_thk",
		Body: &ir.Force{Expr: &ir.Var{Name: "_thk", Index: 0}},
	}

	// Gated built-ins: rec and fix.
	if enableFix {
		globals["fix"] = &Closure{
			Locals: nil, Param: "_f",
			Body: fixBody(),
		}
	}
	if enableRec {
		globals["rec"] = &Closure{
			Locals: nil, Param: "_f",
			Body: recBody(),
		}
	}

	return globals
}

// FixBody returns the Fix IR node for the fix builtin.
func FixBody() *ir.Fix { return fixBody() }

// RecBody returns the Force(Fix(Thunk)) IR node for the rec builtin.
func RecBody() ir.Core { return recBody() }

// fixBody returns a Fix node for the fix builtin closure.
// fix f = the fixpoint of f, i.e. x where x = \arg. (f x) arg.
//
// Layout inside the Fix Lam body:
//
//	_arg at index 0 (Lam param)
//	_x at index 1 (Fix self-reference)
//	_f at index 2 (captured from enclosing scope)
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

// recBody returns a Force(Fix(Thunk)) node for the rec builtin closure.
// rec f = force (fix _thk (thunk (f _thk)))
//
// The Fix creates a self-referential ThunkVal. Passing the ThunkVal
// to f as-is (without forcing) avoids eager infinite recursion in CBV.
// When the ThunkVal appears in a Bind chain (as self-reference in the
// user's do-block), ForceEffectful auto-forces it, re-entering the
// computation with the current capability environment.
//
// Layout inside the Fix Thunk comp:
//
//	_thk at index 0 (Fix self-reference)
//	_f   at index 1 (captured from enclosing scope)
func recBody() ir.Core {
	return &ir.Force{
		Expr: &ir.Fix{
			Name: "_thk",
			Body: &ir.Thunk{
				FV:        []string{"_f"},
				FVIndices: []int{0}, // capture _f from Fix's enclosing scope
				Comp: &ir.App{
					Fun: &ir.Var{Name: "_f", Index: 1},
					Arg: &ir.Var{Name: "_thk", Index: 0},
				},
			},
		},
	}
}
