//go:build probe

// If-then-else probe tests — parser produces ExprIf (surface node).
// Desugaring to ExprCase happens in the desugar pass, not the parser.
// Does NOT cover: type checking, evaluation, desugaring.
package parse

import (
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive // dot import for tightly-coupled subpackage
)

// TestProbeF_IfBasic verifies `if True then 1 else 2` produces ExprIf.
func TestProbeF_IfBasic(t *testing.T) {
	prog := parseMustSucceed(t, `main := if True then 1 else 2`)
	d := prog.Decls[0].(*DeclValueDef)
	ifE, ok := d.Expr.(*ExprIf)
	if !ok {
		t.Fatalf("expected ExprIf, got %T", d.Expr)
	}
	if _, ok := ifE.Cond.(*ExprCon); !ok {
		t.Fatalf("expected ExprCon as condition, got %T", ifE.Cond)
	}
	if lit, ok := ifE.Then.(*ExprIntLit); !ok || lit.Value != "1" {
		t.Errorf("expected Then=1, got %T", ifE.Then)
	}
	if lit, ok := ifE.Else.(*ExprIntLit); !ok || lit.Value != "2" {
		t.Errorf("expected Else=2, got %T", ifE.Else)
	}
}

// TestProbeF_IfVariable verifies `if x then a else b` with variable condition.
func TestProbeF_IfVariable(t *testing.T) {
	prog := parseMustSucceed(t, `main := if x then a else b`)
	d := prog.Decls[0].(*DeclValueDef)
	ifE, ok := d.Expr.(*ExprIf)
	if !ok {
		t.Fatalf("expected ExprIf, got %T", d.Expr)
	}
	if v, ok := ifE.Cond.(*ExprVar); !ok || v.Name != "x" {
		t.Errorf("expected Cond=x, got %T", ifE.Cond)
	}
	if v, ok := ifE.Then.(*ExprVar); !ok || v.Name != "a" {
		t.Errorf("expected Then=a, got %T", ifE.Then)
	}
	if v, ok := ifE.Else.(*ExprVar); !ok || v.Name != "b" {
		t.Errorf("expected Else=b, got %T", ifE.Else)
	}
}

// TestProbeF_IfNested verifies `if x then (if y then 1 else 2) else 3`.
func TestProbeF_IfNested(t *testing.T) {
	prog := parseMustSucceed(t, `main := if x then (if y then 1 else 2) else 3`)
	d := prog.Decls[0].(*DeclValueDef)
	outer, ok := d.Expr.(*ExprIf)
	if !ok {
		t.Fatalf("expected outer ExprIf, got %T", d.Expr)
	}
	paren, ok := outer.Then.(*ExprParen)
	if !ok {
		t.Fatalf("expected ExprParen for Then branch, got %T", outer.Then)
	}
	if _, ok := paren.Inner.(*ExprIf); !ok {
		t.Fatalf("expected inner ExprIf inside parens, got %T", paren.Inner)
	}
	if lit, ok := outer.Else.(*ExprIntLit); !ok || lit.Value != "3" {
		t.Errorf("expected Else=3, got %T", outer.Else)
	}
}

// TestProbeF_IfAsAtom verifies `f (if x then 1 else 2)`.
func TestProbeF_IfAsAtom(t *testing.T) {
	prog := parseMustSucceed(t, `main := f (if x then 1 else 2)`)
	d := prog.Decls[0].(*DeclValueDef)
	app, ok := d.Expr.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp, got %T", d.Expr)
	}
	paren, ok := app.Arg.(*ExprParen)
	if !ok {
		t.Fatalf("expected ExprParen as argument, got %T", app.Arg)
	}
	if _, ok := paren.Inner.(*ExprIf); !ok {
		t.Fatalf("expected ExprIf inside parens, got %T", paren.Inner)
	}
}

// TestProbeF_IfWithApplication verifies `if f x then g y else h z`.
func TestProbeF_IfWithApplication(t *testing.T) {
	prog := parseMustSucceed(t, `main := if f x then g y else h z`)
	d := prog.Decls[0].(*DeclValueDef)
	ifE, ok := d.Expr.(*ExprIf)
	if !ok {
		t.Fatalf("expected ExprIf, got %T", d.Expr)
	}
	if _, ok := ifE.Cond.(*ExprApp); !ok {
		t.Fatalf("expected ExprApp as Cond, got %T", ifE.Cond)
	}
	if _, ok := ifE.Then.(*ExprApp); !ok {
		t.Fatalf("expected ExprApp as Then, got %T", ifE.Then)
	}
	if _, ok := ifE.Else.(*ExprApp); !ok {
		t.Fatalf("expected ExprApp as Else, got %T", ifE.Else)
	}
}

// TestProbeF_IfInDoBlock verifies if-then-else inside a do block.
func TestProbeF_IfInDoBlock(t *testing.T) {
	prog := parseMustSucceed(t, `main := do { x <- if b then pure 1 else pure 2; pure x }`)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr, ok := d.Expr.(*ExprDo)
	if !ok {
		t.Fatalf("expected ExprDo, got %T", d.Expr)
	}
	bind, ok := doExpr.Stmts[0].(*StmtBind)
	if !ok {
		t.Fatalf("expected StmtBind, got %T", doExpr.Stmts[0])
	}
	if _, ok := bind.Comp.(*ExprIf); !ok {
		t.Fatalf("expected ExprIf in bind, got %T", bind.Comp)
	}
}

// TestProbeF_IfWithLambda verifies if-then-else with lambda bodies.
func TestProbeF_IfWithLambda(t *testing.T) {
	prog := parseMustSucceed(t, `main := if b then \x. x else \x. x`)
	d := prog.Decls[0].(*DeclValueDef)
	ifE, ok := d.Expr.(*ExprIf)
	if !ok {
		t.Fatalf("expected ExprIf, got %T", d.Expr)
	}
	if lam, ok := ifE.Then.(*ExprLam); !ok || len(lam.Params) != 1 {
		t.Errorf("expected 1-param lambda for Then, got %T", ifE.Then)
	}
	if lam, ok := ifE.Else.(*ExprLam); !ok || len(lam.Params) != 1 {
		t.Errorf("expected 1-param lambda for Else, got %T", ifE.Else)
	}
}

// TestProbeF_IfKeywordsReserved verifies keywords cannot be variable names.
func TestProbeF_IfKeywordsReserved(t *testing.T) {
	for _, kw := range []string{"if", "then", "else"} {
		t.Run(kw, func(t *testing.T) {
			parseMustFail(t, kw+" := 1")
		})
	}
}

// TestProbeF_IfMissingElse verifies `if True then 1` produces a parse error.
func TestProbeF_IfMissingElse(t *testing.T) {
	parseMustFail(t, `main := if True then 1`)
}

// TestProbeF_IfMissingThen verifies `if True 1 else 2` produces a parse error.
func TestProbeF_IfMissingThen(t *testing.T) {
	parseMustFail(t, `main := if True 1 else 2`)
}
