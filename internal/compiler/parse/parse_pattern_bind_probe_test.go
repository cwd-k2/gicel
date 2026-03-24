//go:build probe

// Pattern binding probe tests — do-block and block parsing.
// Does NOT cover: type checking, elaboration, evaluation.
package parse

import (
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

// ===== Do-block pattern binds =====

// TestProbeF_DoPatternBind verifies tuple pattern in monadic bind:
// do { (a, b) <- pure (1, 2); pure a }
func TestProbeF_DoPatternBind(t *testing.T) {
	src := `main := do { (a, b) <- pure (1, 2); pure a }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr, ok := d.Expr.(*ExprDo)
	if !ok {
		t.Fatalf("expected ExprDo, got %T", d.Expr)
	}
	if len(doExpr.Stmts) != 2 {
		t.Fatalf("expected 2 stmts, got %d", len(doExpr.Stmts))
	}
	bind, ok := doExpr.Stmts[0].(*StmtBind)
	if !ok {
		t.Fatalf("expected StmtBind, got %T", doExpr.Stmts[0])
	}
	rec, ok := bind.Pat.(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord for tuple pattern, got %T", bind.Pat)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("expected 2 fields in tuple pattern, got %d", len(rec.Fields))
	}
	if rec.Fields[0].Label != "_1" || rec.Fields[1].Label != "_2" {
		t.Errorf("expected labels _1, _2; got %q, %q", rec.Fields[0].Label, rec.Fields[1].Label)
	}
}

// TestProbeF_DoPurePatternBind verifies tuple pattern in pure bind:
// do { (x, y) := (3, 4); pure (x + y) }
func TestProbeF_DoPurePatternBind(t *testing.T) {
	src := `main := do { (x, y) := (3, 4); pure x }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr, ok := d.Expr.(*ExprDo)
	if !ok {
		t.Fatalf("expected ExprDo, got %T", d.Expr)
	}
	if len(doExpr.Stmts) != 2 {
		t.Fatalf("expected 2 stmts, got %d", len(doExpr.Stmts))
	}
	pureBind, ok := doExpr.Stmts[0].(*StmtPureBind)
	if !ok {
		t.Fatalf("expected StmtPureBind, got %T", doExpr.Stmts[0])
	}
	rec, ok := pureBind.Pat.(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord for tuple pattern, got %T", pureBind.Pat)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("expected 2 fields in tuple pattern, got %d", len(rec.Fields))
	}
	if rec.Fields[0].Label != "_1" || rec.Fields[1].Label != "_2" {
		t.Errorf("expected labels _1, _2; got %q, %q", rec.Fields[0].Label, rec.Fields[1].Label)
	}
}

// TestProbeF_DoWildcardPattern verifies wildcard in tuple pattern bind:
// do { (_, b) <- pure (1, 2); pure b }
func TestProbeF_DoWildcardPattern(t *testing.T) {
	src := `main := do { (_, b) <- pure (1, 2); pure b }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 2 {
		t.Fatalf("expected 2 stmts, got %d", len(doExpr.Stmts))
	}
	bind := doExpr.Stmts[0].(*StmtBind)
	rec, ok := bind.Pat.(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord for tuple pattern, got %T", bind.Pat)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rec.Fields))
	}
	if _, ok := rec.Fields[0].Pattern.(*PatWild); !ok {
		t.Errorf("expected PatWild for first field, got %T", rec.Fields[0].Pattern)
	}
	if v, ok := rec.Fields[1].Pattern.(*PatVar); !ok || v.Name != "b" {
		t.Errorf("expected PatVar 'b' for second field, got %T", rec.Fields[1].Pattern)
	}
}

// TestProbeF_DoNestedPattern verifies nested tuple pattern bind:
// do { ((a, b), c) <- pure x; pure a }
func TestProbeF_DoNestedPattern(t *testing.T) {
	src := `main := do { ((a, b), c) <- pure x; pure a }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 2 {
		t.Fatalf("expected 2 stmts, got %d", len(doExpr.Stmts))
	}
	bind := doExpr.Stmts[0].(*StmtBind)
	outer, ok := bind.Pat.(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord for outer tuple, got %T", bind.Pat)
	}
	if len(outer.Fields) != 2 {
		t.Fatalf("expected 2 fields in outer tuple, got %d", len(outer.Fields))
	}
	inner, ok := outer.Fields[0].Pattern.(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord for nested tuple, got %T", outer.Fields[0].Pattern)
	}
	if len(inner.Fields) != 2 {
		t.Fatalf("expected 2 fields in inner tuple, got %d", len(inner.Fields))
	}
	if v, ok := outer.Fields[1].Pattern.(*PatVar); !ok || v.Name != "c" {
		t.Errorf("expected PatVar 'c' for outer second field, got %T", outer.Fields[1].Pattern)
	}
}

// TestProbeF_DoRefutablePatternError verifies refutable pattern in monadic bind emits error:
// do { (Just x) <- pure Nothing; pure x }
func TestProbeF_DoRefutablePatternError(t *testing.T) {
	src := `main := do { (Just x) <- pure Nothing; pure x }`
	_, es := parse(src)
	if !es.HasErrors() {
		t.Fatal("expected error for refutable pattern in do bind")
	}
	found := false
	for _, e := range es.Errs {
		if e.Code == diagnostic.ErrInvalidPattern {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ErrInvalidPattern, got: %s", es.Format())
	}
}

// ===== Block expression pattern binds =====

// TestProbeF_BlockPatternBind verifies tuple pattern in block binding:
// { (a, b) := (1, 2); a + b }
func TestProbeF_BlockPatternBind(t *testing.T) {
	src := `main := { (a, b) := (1, 2); a }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	blk, ok := d.Expr.(*ExprBlock)
	if !ok {
		t.Fatalf("expected ExprBlock, got %T", d.Expr)
	}
	if len(blk.Binds) != 1 {
		t.Fatalf("expected 1 bind, got %d", len(blk.Binds))
	}
	rec, ok := blk.Binds[0].Pat.(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord for tuple pattern, got %T", blk.Binds[0].Pat)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rec.Fields))
	}
	if rec.Fields[0].Label != "_1" || rec.Fields[1].Label != "_2" {
		t.Errorf("expected labels _1, _2; got %q, %q", rec.Fields[0].Label, rec.Fields[1].Label)
	}
}

// TestProbeF_BlockMultiPatternBind verifies multiple pattern bindings in block:
// { (a, b) := e1; (c, d) := e2; a + c }
func TestProbeF_BlockMultiPatternBind(t *testing.T) {
	src := `main := { (a, b) := e1; (c, d) := e2; a }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	blk, ok := d.Expr.(*ExprBlock)
	if !ok {
		t.Fatalf("expected ExprBlock, got %T", d.Expr)
	}
	if len(blk.Binds) != 2 {
		t.Fatalf("expected 2 binds, got %d", len(blk.Binds))
	}
	for i, bind := range blk.Binds {
		rec, ok := bind.Pat.(*PatRecord)
		if !ok {
			t.Fatalf("bind %d: expected PatRecord, got %T", i, bind.Pat)
		}
		if len(rec.Fields) != 2 {
			t.Errorf("bind %d: expected 2 fields, got %d", i, len(rec.Fields))
		}
	}
}

// TestProbeF_BlockMixedBinds verifies mixed simple and pattern binds in block:
// { x := 1; (a, b) := (2, 3); x + a }
func TestProbeF_BlockMixedBinds(t *testing.T) {
	src := `main := { x := 1; (a, b) := (2, 3); x }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	blk, ok := d.Expr.(*ExprBlock)
	if !ok {
		t.Fatalf("expected ExprBlock, got %T", d.Expr)
	}
	if len(blk.Binds) != 2 {
		t.Fatalf("expected 2 binds, got %d", len(blk.Binds))
	}
	// First bind: simple variable
	if v, ok := blk.Binds[0].Pat.(*PatVar); !ok || v.Name != "x" {
		t.Errorf("bind 0: expected PatVar 'x', got %T", blk.Binds[0].Pat)
	}
	// Second bind: tuple pattern
	if _, ok := blk.Binds[1].Pat.(*PatRecord); !ok {
		t.Errorf("bind 1: expected PatRecord, got %T", blk.Binds[1].Pat)
	}
}

// TestProbeF_BlockPatternBacktrack verifies that `(a + b)` starting with `(`
// is not mistaken for a pattern bind but backtracks to a parenthesized expression.
func TestProbeF_BlockPatternBacktrack(t *testing.T) {
	src := `main := { (a) }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	// The block body should be a parenthesized expression, not a pattern bind.
	blk, ok := d.Expr.(*ExprBlock)
	if !ok {
		t.Fatalf("expected ExprBlock, got %T", d.Expr)
	}
	if len(blk.Binds) != 0 {
		t.Errorf("expected 0 binds (backtrack to expr), got %d", len(blk.Binds))
	}
	if blk.Body == nil {
		t.Fatal("expected block body expression, got nil")
	}
}

// ===== Speculation edge cases =====

// TestProbeF_DoPatternVsExpr verifies that `(1 + 2)` in do-block is parsed
// as an expression statement, not as a pattern bind.
func TestProbeF_DoPatternVsExpr(t *testing.T) {
	src := `main := do { (1); pure () }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 2 {
		t.Fatalf("expected 2 stmts, got %d", len(doExpr.Stmts))
	}
	// First statement should be an expression statement, not a bind.
	if _, ok := doExpr.Stmts[0].(*StmtExpr); !ok {
		t.Errorf("expected StmtExpr for parenthesized expression, got %T", doExpr.Stmts[0])
	}
}
