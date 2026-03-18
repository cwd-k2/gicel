//go:build probe

// Do-notation probe tests: do-blocks, bind, pure, nesting, edge cases.
package parse

import (
	"testing"

	. "github.com/cwd-k2/gicel/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage
)

// ===== From probe_b =====

// TestProbeB_EmptyDoBlock checks empty do block: do {}.
func TestProbeB_EmptyDoBlock(t *testing.T) {
	prog := parseMustSucceed(t, "main := do {}")
	d := prog.Decls[0].(*DeclValueDef)
	doExpr, ok := d.Expr.(*ExprDo)
	if !ok {
		t.Fatalf("expected ExprDo, got %T", d.Expr)
	}
	if len(doExpr.Stmts) != 0 {
		t.Errorf("expected 0 stmts, got %d", len(doExpr.Stmts))
	}
}

// TestProbeB_DoBlockWithPureBind checks do block with pure and bind.
func TestProbeB_DoBlockWithPureBind(t *testing.T) {
	src := "main := do { x <- pure 1; y <- pure 2; pure x }"
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 3 {
		t.Errorf("expected 3 stmts, got %d", len(doExpr.Stmts))
	}
}

// TestProbeB_DoBlockWildcardBind checks do block with wildcard bind.
func TestProbeB_DoBlockWildcardBind(t *testing.T) {
	src := "main := do { _ <- pure 1; pure 2 }"
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 2 {
		t.Errorf("expected 2 stmts, got %d", len(doExpr.Stmts))
	}
}

// TestProbeB_SemicolonInBraces checks semicolons as separators inside braces.
func TestProbeB_SemicolonInBraces(t *testing.T) {
	src := "main := do { x <- a; y <- b; pure x }"
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 3 {
		t.Errorf("expected 3 stmts, got %d", len(doExpr.Stmts))
	}
}

// ===== From probe_d =====

// TestProbeD_NewlineInDoBlock verifies newlines as statement separators in do-blocks.
// BUG: medium — newlines inside do-blocks are not treated as semicolons.
func TestProbeD_NewlineInDoBlock(t *testing.T) {
	source := "main := do {\n  x <- pure 1\n  y <- pure 2\n  pure x\n}"
	prog, es := parse(source)
	if es.HasErrors() {
		// BUG: medium — newline-separated do-block statements produce errors
		t.Logf("BUG: medium — do-block newline separation produces errors: %s", es.Format())
	}
	if prog != nil && len(prog.Decls) > 0 {
		d := prog.Decls[0].(*DeclValueDef)
		doExpr, ok := d.Expr.(*ExprDo)
		if ok {
			t.Logf("do-block parsed %d statements", len(doExpr.Stmts))
		}
	}
}

// TestProbeD_EmptyDoBlock verifies `do {}` does not crash.
func TestProbeD_EmptyDoBlock(t *testing.T) {
	prog, es := parse("main := do {}")
	if es.HasErrors() {
		t.Logf("empty do block: %s", es.Format())
	}
	if prog != nil && len(prog.Decls) > 0 {
		if vd, ok := prog.Decls[0].(*DeclValueDef); ok {
			if doExpr, ok := vd.Expr.(*ExprDo); ok {
				if len(doExpr.Stmts) != 0 {
					t.Errorf("expected 0 statements in empty do, got %d", len(doExpr.Stmts))
				}
			}
		}
	}
}

// ===== From probe_e =====

// TestProbeE_DoBlockEmpty verifies `do {}` — empty do block.
func TestProbeE_DoBlockEmpty(t *testing.T) {
	source := `main := do {}`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr, ok := d.Expr.(*ExprDo)
	if !ok {
		t.Fatalf("expected ExprDo, got %T", d.Expr)
	}
	if len(doExpr.Stmts) != 0 {
		t.Errorf("expected 0 stmts, got %d", len(doExpr.Stmts))
	}
}

// TestProbeE_DoBlockOnlyBind verifies do block with only bind statements.
func TestProbeE_DoBlockOnlyBind(t *testing.T) {
	source := `main := do { x <- pure 1; y <- pure 2; pure x }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr, ok := d.Expr.(*ExprDo)
	if !ok {
		t.Fatalf("expected ExprDo, got %T", d.Expr)
	}
	if len(doExpr.Stmts) != 3 {
		t.Errorf("expected 3 stmts, got %d", len(doExpr.Stmts))
	}
}

// TestProbeE_DoBlockOnlyPureBind verifies do block with pure bind.
func TestProbeE_DoBlockOnlyPureBind(t *testing.T) {
	source := `main := do { x <- pure 42; pure x }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 2 {
		t.Errorf("expected 2 stmts, got %d", len(doExpr.Stmts))
	}
}

// TestProbeE_DoBlockOnlyExpr verifies do block with only expression statement.
func TestProbeE_DoBlockOnlyExpr(t *testing.T) {
	source := `main := do { pure 1 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr, ok := d.Expr.(*ExprDo)
	if !ok {
		t.Fatalf("expected ExprDo, got %T", d.Expr)
	}
	if len(doExpr.Stmts) != 1 {
		t.Errorf("expected 1 stmt, got %d", len(doExpr.Stmts))
	}
}

// TestProbeE_DoBlockWildcardBind verifies do block with wildcard bind.
func TestProbeE_DoBlockWildcardBind(t *testing.T) {
	source := `main := do { _ <- pure 1; pure 2 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 2 {
		t.Errorf("expected 2 stmts, got %d", len(doExpr.Stmts))
	}
}

// TestProbeE_DoBlockNestedDo verifies nested do blocks.
func TestProbeE_DoBlockNestedDo(t *testing.T) {
	source := `main := do { x <- do { pure 1 }; pure x }`
	parseMustSucceed(t, source)
}

// TestProbeE_DoBlockSemicolonSeparated verifies semicolons separate do stmts.
func TestProbeE_DoBlockSemicolonSeparated(t *testing.T) {
	source := `main := do { x <- pure 1; y <- pure 2; pure x }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 3 {
		t.Errorf("expected 3 stmts, got %d", len(doExpr.Stmts))
	}
}

// TestProbeE_DoBlockMissingBrace verifies do block without closing brace.
func TestProbeE_DoBlockMissingBrace(t *testing.T) {
	_, es := parse(`main := do { x <- pure 1`)
	if !es.HasErrors() {
		t.Error("expected error for missing closing brace in do")
	}
}

// TestProbeE_DoInsideCaseInsideLambda verifies complex nesting.
func TestProbeE_DoInsideCaseInsideLambda(t *testing.T) {
	source := `main := \x. case x {
  True -> do {
    y <- pure 1;
    pure y
  };
  False -> pure 0
}`
	parseMustSucceed(t, source)
}

// TestProbeE_DoBlockStall verifies that garbage in do block
// doesn't cause infinite loop.
func TestProbeE_DoBlockStall(t *testing.T) {
	source := `main := do { + + + }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for garbage in do block")
	}
}
