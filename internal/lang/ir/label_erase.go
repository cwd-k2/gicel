package ir

import "github.com/cwd-k2/gicel/internal/lang/types"

// EraseLabelArgs converts type applications of label-kinded arguments
// into term-level string literal applications. This is the label erasure
// step for Named Capabilities: a TyApp whose TyArg is a TyCon at kind
// level (Label literal) is replaced by App{Fun: expr, Arg: Lit{name}}.
//
// Must run before optimization and free-variable annotation, since the
// transformation changes the Core IR structure (TyApp → App + Lit).
func EraseLabelArgs(c Core) Core {
	return Transform(c, eraseLabelArg)
}

// EraseLabelArgsProgram applies label erasure to all bindings.
func EraseLabelArgsProgram(prog *Program) {
	for i, b := range prog.Bindings {
		prog.Bindings[i].Expr = EraseLabelArgs(b.Expr)
	}
}

func eraseLabelArg(c Core) Core {
	ta, ok := c.(*TyApp)
	if !ok {
		return c
	}
	con, ok := ta.TyArg.(*types.TyCon)
	if !ok || !types.IsKindLevel(con.Level) {
		return c
	}
	// Only erase label literals (structurally marked with IsLabel).
	// Promoted data constructors, grade constants, and built-in kind
	// constants are NOT label literals and must not be erased.
	if !con.IsLabel {
		return c
	}
	// Erase: TyApp{Expr, TyCon{name, L1}} → App{Expr, Lit{name}}
	return &App{
		Fun: ta.Expr,
		Arg: &Lit{Value: con.Name},
		S:   ta.S,
	}
}
