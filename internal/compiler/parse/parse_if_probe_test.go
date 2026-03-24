//go:build probe

// If-then-else probe tests — parser desugaring.
// Does NOT cover: type checking, evaluation.
package parse

import (
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive // dot import for tightly-coupled subpackage
)

// TestProbeF_IfBasic verifies `if True then 1 else 2` desugars to
// ExprCase with True/False alternatives.
func TestProbeF_IfBasic(t *testing.T) {
	prog := parseMustSucceed(t, `main := if True then 1 else 2`)
	d := prog.Decls[0].(*DeclValueDef)
	c, ok := d.Expr.(*ExprCase)
	if !ok {
		t.Fatalf("expected ExprCase, got %T", d.Expr)
	}
	if len(c.Alts) != 2 {
		t.Fatalf("expected 2 alts, got %d", len(c.Alts))
	}
	// Scrutinee is the constructor True.
	scrut, ok := c.Scrutinee.(*ExprCon)
	if !ok {
		t.Fatalf("expected ExprCon as scrutinee, got %T", c.Scrutinee)
	}
	if scrut.Name != "True" {
		t.Errorf("expected scrutinee True, got %q", scrut.Name)
	}
	// First alt: True => 1
	alt0 := c.Alts[0]
	pat0, ok := alt0.Pattern.(*PatCon)
	if !ok {
		t.Fatalf("expected PatCon for alt 0, got %T", alt0.Pattern)
	}
	if pat0.Con != "True" {
		t.Errorf("expected alt 0 pattern True, got %q", pat0.Con)
	}
	body0, ok := alt0.Body.(*ExprIntLit)
	if !ok {
		t.Fatalf("expected ExprIntLit for alt 0 body, got %T", alt0.Body)
	}
	if body0.Value != "1" {
		t.Errorf("expected alt 0 body 1, got %q", body0.Value)
	}
	// Second alt: False => 2
	alt1 := c.Alts[1]
	pat1, ok := alt1.Pattern.(*PatCon)
	if !ok {
		t.Fatalf("expected PatCon for alt 1, got %T", alt1.Pattern)
	}
	if pat1.Con != "False" {
		t.Errorf("expected alt 1 pattern False, got %q", pat1.Con)
	}
	body1, ok := alt1.Body.(*ExprIntLit)
	if !ok {
		t.Fatalf("expected ExprIntLit for alt 1 body, got %T", alt1.Body)
	}
	if body1.Value != "2" {
		t.Errorf("expected alt 1 body 2, got %q", body1.Value)
	}
}

// TestProbeF_IfVariable verifies `if x then a else b` with a variable condition.
func TestProbeF_IfVariable(t *testing.T) {
	prog := parseMustSucceed(t, `main := if x then a else b`)
	d := prog.Decls[0].(*DeclValueDef)
	c, ok := d.Expr.(*ExprCase)
	if !ok {
		t.Fatalf("expected ExprCase, got %T", d.Expr)
	}
	scrut, ok := c.Scrutinee.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar as scrutinee, got %T", c.Scrutinee)
	}
	if scrut.Name != "x" {
		t.Errorf("expected scrutinee x, got %q", scrut.Name)
	}
	if len(c.Alts) != 2 {
		t.Fatalf("expected 2 alts, got %d", len(c.Alts))
	}
	// True branch body is variable a.
	bodyA, ok := c.Alts[0].Body.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar for True branch, got %T", c.Alts[0].Body)
	}
	if bodyA.Name != "a" {
		t.Errorf("expected True branch body a, got %q", bodyA.Name)
	}
	// False branch body is variable b.
	bodyB, ok := c.Alts[1].Body.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar for False branch, got %T", c.Alts[1].Body)
	}
	if bodyB.Name != "b" {
		t.Errorf("expected False branch body b, got %q", bodyB.Name)
	}
}

// TestProbeF_IfNested verifies `if x then (if y then 1 else 2) else 3`.
func TestProbeF_IfNested(t *testing.T) {
	prog := parseMustSucceed(t, `main := if x then (if y then 1 else 2) else 3`)
	d := prog.Decls[0].(*DeclValueDef)
	outer, ok := d.Expr.(*ExprCase)
	if !ok {
		t.Fatalf("expected outer ExprCase, got %T", d.Expr)
	}
	if len(outer.Alts) != 2 {
		t.Fatalf("expected 2 outer alts, got %d", len(outer.Alts))
	}
	// True branch: parenthesized inner if-then-else.
	paren, ok := outer.Alts[0].Body.(*ExprParen)
	if !ok {
		t.Fatalf("expected ExprParen for True branch, got %T", outer.Alts[0].Body)
	}
	inner, ok := paren.Inner.(*ExprCase)
	if !ok {
		t.Fatalf("expected inner ExprCase inside parens, got %T", paren.Inner)
	}
	if len(inner.Alts) != 2 {
		t.Errorf("expected 2 inner alts, got %d", len(inner.Alts))
	}
	// False branch: literal 3.
	body3, ok := outer.Alts[1].Body.(*ExprIntLit)
	if !ok {
		t.Fatalf("expected ExprIntLit for False branch, got %T", outer.Alts[1].Body)
	}
	if body3.Value != "3" {
		t.Errorf("expected False branch body 3, got %q", body3.Value)
	}
}

// TestProbeF_IfAsAtom verifies `f (if x then 1 else 2)` — if-then-else
// in function argument position (parenthesized).
func TestProbeF_IfAsAtom(t *testing.T) {
	prog := parseMustSucceed(t, `main := f (if x then 1 else 2)`)
	d := prog.Decls[0].(*DeclValueDef)
	app, ok := d.Expr.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp, got %T", d.Expr)
	}
	fun, ok := app.Fun.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar for function, got %T", app.Fun)
	}
	if fun.Name != "f" {
		t.Errorf("expected function name f, got %q", fun.Name)
	}
	paren, ok := app.Arg.(*ExprParen)
	if !ok {
		t.Fatalf("expected ExprParen as argument, got %T", app.Arg)
	}
	inner, ok := paren.Inner.(*ExprCase)
	if !ok {
		t.Fatalf("expected ExprCase inside parens, got %T", paren.Inner)
	}
	if len(inner.Alts) != 2 {
		t.Errorf("expected 2 alts, got %d", len(inner.Alts))
	}
}

// TestProbeF_IfWithApplication verifies `if f x then g y else h z` —
// applications in all three positions (condition, then, else).
func TestProbeF_IfWithApplication(t *testing.T) {
	prog := parseMustSucceed(t, `main := if f x then g y else h z`)
	d := prog.Decls[0].(*DeclValueDef)
	c, ok := d.Expr.(*ExprCase)
	if !ok {
		t.Fatalf("expected ExprCase, got %T", d.Expr)
	}
	// Scrutinee: f x (application).
	scrutApp, ok := c.Scrutinee.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp as scrutinee, got %T", c.Scrutinee)
	}
	scrutFun, ok := scrutApp.Fun.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar for scrutinee function, got %T", scrutApp.Fun)
	}
	if scrutFun.Name != "f" {
		t.Errorf("expected scrutinee function f, got %q", scrutFun.Name)
	}
	// True branch: g y (application).
	thenApp, ok := c.Alts[0].Body.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp for True branch, got %T", c.Alts[0].Body)
	}
	thenFun, ok := thenApp.Fun.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar for True branch function, got %T", thenApp.Fun)
	}
	if thenFun.Name != "g" {
		t.Errorf("expected True branch function g, got %q", thenFun.Name)
	}
	// False branch: h z (application).
	elseApp, ok := c.Alts[1].Body.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp for False branch, got %T", c.Alts[1].Body)
	}
	elseFun, ok := elseApp.Fun.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar for False branch function, got %T", elseApp.Fun)
	}
	if elseFun.Name != "h" {
		t.Errorf("expected False branch function h, got %q", elseFun.Name)
	}
}

// TestProbeF_IfInDoBlock verifies if-then-else inside a do block:
// `do { x <- if b then pure 1 else pure 2; pure x }`.
func TestProbeF_IfInDoBlock(t *testing.T) {
	prog := parseMustSucceed(t, `main := do { x <- if b then pure 1 else pure 2; pure x }`)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr, ok := d.Expr.(*ExprDo)
	if !ok {
		t.Fatalf("expected ExprDo, got %T", d.Expr)
	}
	if len(doExpr.Stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(doExpr.Stmts))
	}
	// First statement: x <- if b then pure 1 else pure 2
	bind, ok := doExpr.Stmts[0].(*StmtBind)
	if !ok {
		t.Fatalf("expected StmtBind for first stmt, got %T", doExpr.Stmts[0])
	}
	ifCase, ok := bind.Comp.(*ExprCase)
	if !ok {
		t.Fatalf("expected ExprCase (desugared if) in bind, got %T", bind.Comp)
	}
	if len(ifCase.Alts) != 2 {
		t.Errorf("expected 2 alts in desugared if, got %d", len(ifCase.Alts))
	}
}

// TestProbeF_IfWithLambda verifies if-then-else with lambda bodies:
// `if b then \x. x else \x. x`.
func TestProbeF_IfWithLambda(t *testing.T) {
	prog := parseMustSucceed(t, `main := if b then \x. x else \x. x`)
	d := prog.Decls[0].(*DeclValueDef)
	c, ok := d.Expr.(*ExprCase)
	if !ok {
		t.Fatalf("expected ExprCase, got %T", d.Expr)
	}
	// True branch: \x. x
	thenLam, ok := c.Alts[0].Body.(*ExprLam)
	if !ok {
		t.Fatalf("expected ExprLam for True branch, got %T", c.Alts[0].Body)
	}
	if len(thenLam.Params) != 1 {
		t.Errorf("expected 1 param for True branch lambda, got %d", len(thenLam.Params))
	}
	// False branch: \x. x
	elseLam, ok := c.Alts[1].Body.(*ExprLam)
	if !ok {
		t.Fatalf("expected ExprLam for False branch, got %T", c.Alts[1].Body)
	}
	if len(elseLam.Params) != 1 {
		t.Errorf("expected 1 param for False branch lambda, got %d", len(elseLam.Params))
	}
}

// TestProbeF_IfKeywordsReserved verifies that `if`, `then`, `else` are reserved
// and cannot be used as variable names.
func TestProbeF_IfKeywordsReserved(t *testing.T) {
	// Each keyword used as a binding name should fail parsing.
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
