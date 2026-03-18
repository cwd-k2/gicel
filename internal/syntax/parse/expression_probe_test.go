//go:build probe

// Expression probe tests: lambda, record, list, tuple, case, block, row, qualified.
package parse

import (
	"fmt"
	"testing"

	. "github.com/cwd-k2/gicel/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage
)

// ===== From probe_b =====

// TestProbeB_LambdaChain checks chained lambda expressions.
func TestProbeB_LambdaChain(t *testing.T) {
	prog := parseMustSucceed(t, `main := \x. \y. x`)
	d := prog.Decls[0].(*DeclValueDef)
	lam1, ok := d.Expr.(*ExprLam)
	if !ok {
		t.Fatalf("expected ExprLam, got %T", d.Expr)
	}
	if len(lam1.Params) != 1 {
		t.Errorf("expected 1 param, got %d", len(lam1.Params))
	}
	lam2, ok := lam1.Body.(*ExprLam)
	if !ok {
		t.Fatalf("expected inner ExprLam, got %T", lam1.Body)
	}
	if len(lam2.Params) != 1 {
		t.Errorf("expected 1 param for inner lambda, got %d", len(lam2.Params))
	}
}

// TestProbeB_LambdaMultipleParams checks lambda with multiple params.
func TestProbeB_LambdaMultipleParams(t *testing.T) {
	prog := parseMustSucceed(t, `main := \x y z. x`)
	d := prog.Decls[0].(*DeclValueDef)
	lam, ok := d.Expr.(*ExprLam)
	if !ok {
		t.Fatalf("expected ExprLam, got %T", d.Expr)
	}
	if len(lam.Params) != 3 {
		t.Errorf("expected 3 params, got %d", len(lam.Params))
	}
}

// TestProbeB_RecordUpdateChain checks chained record updates.
func TestProbeB_RecordUpdateChain(t *testing.T) {
	prog := parseMustSucceed(t, `main := { { r | x: 1 } | y: 2 }`)
	d := prog.Decls[0].(*DeclValueDef)
	outerUpd, ok := d.Expr.(*ExprRecordUpdate)
	if !ok {
		t.Fatalf("expected ExprRecordUpdate for outer, got %T", d.Expr)
	}
	_, ok = outerUpd.Record.(*ExprRecordUpdate)
	if !ok {
		t.Fatalf("expected ExprRecordUpdate for inner, got %T", outerUpd.Record)
	}
}

// TestProbeB_RecordLiteral checks record literal.
func TestProbeB_RecordLiteral(t *testing.T) {
	prog := parseMustSucceed(t, `main := { x: 1, y: 2 }`)
	d := prog.Decls[0].(*DeclValueDef)
	r, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord, got %T", d.Expr)
	}
	if len(r.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(r.Fields))
	}
}

// TestProbeB_UnitExpr checks () as unit expression.
func TestProbeB_UnitExpr(t *testing.T) {
	prog := parseMustSucceed(t, "main := ()")
	d := prog.Decls[0].(*DeclValueDef)
	r, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for (), got %T", d.Expr)
	}
	if len(r.Fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(r.Fields))
	}
}

// TestProbeB_TupleExpr checks (1, 2, 3) as tuple expression.
func TestProbeB_TupleExpr(t *testing.T) {
	prog := parseMustSucceed(t, "main := (1, 2, 3)")
	d := prog.Decls[0].(*DeclValueDef)
	r, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for tuple, got %T", d.Expr)
	}
	if len(r.Fields) != 3 {
		t.Errorf("expected 3 fields, got %d", len(r.Fields))
	}
	for i, f := range r.Fields {
		expected := fmt.Sprintf("_%d", i+1)
		if f.Label != expected {
			t.Errorf("field %d: expected label %q, got %q", i, expected, f.Label)
		}
	}
}

// TestProbeB_EmptyCaseAlts checks case with empty alternatives.
func TestProbeB_EmptyCaseAlts(t *testing.T) {
	prog := parseMustSucceed(t, "main := case x {}")
	d := prog.Decls[0].(*DeclValueDef)
	c, ok := d.Expr.(*ExprCase)
	if !ok {
		t.Fatalf("expected ExprCase, got %T", d.Expr)
	}
	if len(c.Alts) != 0 {
		t.Errorf("expected 0 alts, got %d", len(c.Alts))
	}
}

// TestProbeB_ListLiteral checks [1, 2, 3].
func TestProbeB_ListLiteral(t *testing.T) {
	prog := parseMustSucceed(t, "main := [1, 2, 3]")
	d := prog.Decls[0].(*DeclValueDef)
	lst, ok := d.Expr.(*ExprList)
	if !ok {
		t.Fatalf("expected ExprList, got %T", d.Expr)
	}
	if len(lst.Elems) != 3 {
		t.Errorf("expected 3 elements, got %d", len(lst.Elems))
	}
}

// TestProbeB_EmptyList checks [].
func TestProbeB_EmptyList(t *testing.T) {
	prog := parseMustSucceed(t, "main := []")
	d := prog.Decls[0].(*DeclValueDef)
	lst, ok := d.Expr.(*ExprList)
	if !ok {
		t.Fatalf("expected ExprList, got %T", d.Expr)
	}
	if len(lst.Elems) != 0 {
		t.Errorf("expected 0 elements, got %d", len(lst.Elems))
	}
}

// TestProbeB_RecordProjection checks r.#x projection.
func TestProbeB_RecordProjection(t *testing.T) {
	prog := parseMustSucceed(t, "main := r.#x")
	d := prog.Decls[0].(*DeclValueDef)
	proj, ok := d.Expr.(*ExprProject)
	if !ok {
		t.Fatalf("expected ExprProject, got %T", d.Expr)
	}
	if proj.Label != "x" {
		t.Errorf("expected label 'x', got %q", proj.Label)
	}
}

// TestProbeB_ChainedProjection checks r.#x.#y chained projections.
func TestProbeB_ChainedProjection(t *testing.T) {
	prog := parseMustSucceed(t, "main := r.#x.#y")
	d := prog.Decls[0].(*DeclValueDef)
	outer, ok := d.Expr.(*ExprProject)
	if !ok {
		t.Fatalf("expected ExprProject for outer, got %T", d.Expr)
	}
	if outer.Label != "y" {
		t.Errorf("expected outer label 'y', got %q", outer.Label)
	}
	inner, ok := outer.Record.(*ExprProject)
	if !ok {
		t.Fatalf("expected ExprProject for inner, got %T", outer.Record)
	}
	if inner.Label != "x" {
		t.Errorf("expected inner label 'x', got %q", inner.Label)
	}
}

// TestProbeB_BlockExpr checks block expression { x := 1; x }.
func TestProbeB_BlockExpr(t *testing.T) {
	prog := parseMustSucceed(t, "main := { x := 1; x }")
	d := prog.Decls[0].(*DeclValueDef)
	blk, ok := d.Expr.(*ExprBlock)
	if !ok {
		t.Fatalf("expected ExprBlock, got %T", d.Expr)
	}
	if len(blk.Binds) != 1 {
		t.Errorf("expected 1 bind, got %d", len(blk.Binds))
	}
}

// TestProbeB_EmptyRecord checks {} in expression position (empty record).
func TestProbeB_EmptyRecord(t *testing.T) {
	prog := parseMustSucceed(t, "main := {}")
	d := prog.Decls[0].(*DeclValueDef)
	r, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for {}, got %T", d.Expr)
	}
	if len(r.Fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(r.Fields))
	}
}

// TestProbeB_CaseWithSemicolonAlts checks case alternatives separated by semicolons.
func TestProbeB_CaseWithSemicolonAlts(t *testing.T) {
	src := `main := case x { True -> 1; False -> 0 }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	if len(c.Alts) != 2 {
		t.Errorf("expected 2 alts, got %d", len(c.Alts))
	}
}

// TestProbeB_MultipleCaseAltsNewline checks case alts separated by newlines inside braces.
// Per spec: "Inside braces (do, case, GADT bodies), semicolons are required."
// Newlines alone do NOT separate case alternatives. Semicolons are mandatory.
func TestProbeB_MultipleCaseAltsNewline(t *testing.T) {
	// This SHOULD fail because newlines alone don't separate alts inside braces.
	errMsg := parseMustFail(t, "main := case x {\n  True -> 1\n  False -> 0\n}")
	_ = errMsg
	// But with semicolons, it works:
	prog := parseMustSucceed(t, "main := case x {\n  True -> 1;\n  False -> 0\n}")
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	if len(c.Alts) != 2 {
		t.Errorf("expected 2 alts with semicolons, got %d", len(c.Alts))
	}
}

// ===== From probe_e =====

// TestProbeE_BlockExprSimple verifies block expression parsing.
func TestProbeE_BlockExprSimple(t *testing.T) {
	source := `main := { x := 1; x }`
	parseMustSucceed(t, source)
}

// TestProbeE_RecordLiteralSimple verifies record literal parsing.
func TestProbeE_RecordLiteralSimple(t *testing.T) {
	source := `main := { x: 1, y: 2 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	rec, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord, got %T", d.Expr)
	}
	if len(rec.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(rec.Fields))
	}
}

// TestProbeE_EmptyRecord verifies `{}` is an empty record.
func TestProbeE_EmptyRecord(t *testing.T) {
	source := `main := {}`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	rec, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for {}, got %T", d.Expr)
	}
	if len(rec.Fields) != 0 {
		t.Errorf("expected 0 fields for {}, got %d", len(rec.Fields))
	}
}

// TestProbeE_RecordUpdateBasic verifies record update syntax.
func TestProbeE_RecordUpdateBasic(t *testing.T) {
	source := `main := { r | x: 1 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	_, ok := d.Expr.(*ExprRecordUpdate)
	if !ok {
		t.Fatalf("expected ExprRecordUpdate, got %T", d.Expr)
	}
}

// TestProbeE_TupleSingleton verifies (x) is just grouping, not a 1-tuple.
func TestProbeE_TupleSingleton(t *testing.T) {
	source := `main := (42)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	paren, ok := d.Expr.(*ExprParen)
	if !ok {
		t.Fatalf("expected ExprParen for (42), got %T", d.Expr)
	}
	_, ok = paren.Inner.(*ExprIntLit)
	if !ok {
		t.Fatalf("expected ExprIntLit inside parens, got %T", paren.Inner)
	}
}

// TestProbeE_TuplePair verifies (x, y) is a record with _1, _2.
func TestProbeE_TuplePair(t *testing.T) {
	source := `main := (1, 2)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	rec, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for (1, 2), got %T", d.Expr)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rec.Fields))
	}
	if rec.Fields[0].Label != "_1" || rec.Fields[1].Label != "_2" {
		t.Errorf("expected _1, _2; got %q, %q", rec.Fields[0].Label, rec.Fields[1].Label)
	}
}

// TestProbeE_TupleTrailingComma verifies (x, y,) behavior.
func TestProbeE_TupleTrailingComma(t *testing.T) {
	source := `main := (1, 2,)`
	_, es := parse(source)
	// Trailing comma: parseExpr will try to parse `)` as expr after the comma.
	// This should produce an error.
	if !es.HasErrors() {
		t.Error("expected error for trailing comma in tuple")
	}
}

// TestProbeE_ListEmpty verifies [].
func TestProbeE_ListEmpty(t *testing.T) {
	source := `main := []`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	lst, ok := d.Expr.(*ExprList)
	if !ok {
		t.Fatalf("expected ExprList, got %T", d.Expr)
	}
	if len(lst.Elems) != 0 {
		t.Errorf("expected 0 elements, got %d", len(lst.Elems))
	}
}

// TestProbeE_ListSingleton verifies [x].
func TestProbeE_ListSingleton(t *testing.T) {
	source := `main := [1]`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	lst := d.Expr.(*ExprList)
	if len(lst.Elems) != 1 {
		t.Errorf("expected 1 element, got %d", len(lst.Elems))
	}
}

// TestProbeE_ListMultiple verifies [a, b, c].
func TestProbeE_ListMultiple(t *testing.T) {
	source := `main := [1, 2, 3]`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	lst := d.Expr.(*ExprList)
	if len(lst.Elems) != 3 {
		t.Errorf("expected 3 elements, got %d", len(lst.Elems))
	}
}

// TestProbeE_ListTrailingComma verifies [a,] behavior.
func TestProbeE_ListTrailingComma(t *testing.T) {
	source := `main := [1,]`
	_, es := parse(source)
	// After the comma, parseExpr sees ']' which is not an atom start.
	// This should produce an error.
	if !es.HasErrors() {
		t.Error("expected error for trailing comma in list")
	}
}

// TestProbeE_LambdaNoParams verifies `\. expr` — lambda with no params.
func TestProbeE_LambdaNoParams(t *testing.T) {
	source := `main := \. 42`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	lam, ok := d.Expr.(*ExprLam)
	if !ok {
		t.Fatalf("expected ExprLam, got %T", d.Expr)
	}
	if len(lam.Params) != 0 {
		t.Errorf("expected 0 params, got %d", len(lam.Params))
	}
}

// TestProbeE_LambdaMissingBody verifies `\x.` with missing body errors.
func TestProbeE_LambdaMissingBody(t *testing.T) {
	_, es := parse(`main := \x.`)
	if !es.HasErrors() {
		t.Error("expected error for lambda with missing body")
	}
}

// TestProbeE_LambdaMissingDot verifies `\x x` with missing dot errors.
func TestProbeE_LambdaMissingDot(t *testing.T) {
	// \x x — the parser expects a dot after params. 'x' is a pattern atom,
	// so it consumes both x's as params, then expects dot, finds EOF.
	_, es := parse(`main := \x y`)
	if !es.HasErrors() {
		t.Error("expected error for lambda with missing dot")
	}
}

// TestProbeE_CaseEmptyAlts verifies `case x {}` — case with no alternatives.
func TestProbeE_CaseEmptyAlts(t *testing.T) {
	source := `main := case x {}`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	caseExpr, ok := d.Expr.(*ExprCase)
	if !ok {
		t.Fatalf("expected ExprCase, got %T", d.Expr)
	}
	if len(caseExpr.Alts) != 0 {
		t.Errorf("expected 0 alts, got %d", len(caseExpr.Alts))
	}
}

// TestProbeE_CaseMissingArrow verifies case with missing -> in alternative.
func TestProbeE_CaseMissingArrow(t *testing.T) {
	_, es := parse(`main := case x { y 1 }`)
	if !es.HasErrors() {
		t.Error("expected error for missing -> in case alt")
	}
}

// TestProbeE_CaseNoBraces verifies case without braces errors.
func TestProbeE_CaseNoBraces(t *testing.T) {
	_, es := parse(`main := case x y -> 1`)
	if !es.HasErrors() {
		t.Error("expected error for case without braces")
	}
}

// TestProbeE_RecordProjection verifies r.#field syntax.
func TestProbeE_RecordProjection(t *testing.T) {
	source := `main := r.#x`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	proj, ok := d.Expr.(*ExprProject)
	if !ok {
		t.Fatalf("expected ExprProject, got %T", d.Expr)
	}
	if proj.Label != "x" {
		t.Errorf("expected label 'x', got %q", proj.Label)
	}
}

// TestProbeE_RecordProjectionChain verifies r.#x.#y chaining.
func TestProbeE_RecordProjectionChain(t *testing.T) {
	source := `main := r.#x.#y`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	outer, ok := d.Expr.(*ExprProject)
	if !ok {
		t.Fatalf("expected outer ExprProject, got %T", d.Expr)
	}
	if outer.Label != "y" {
		t.Errorf("expected outer label 'y', got %q", outer.Label)
	}
	inner, ok := outer.Record.(*ExprProject)
	if !ok {
		t.Fatalf("expected inner ExprProject, got %T", outer.Record)
	}
	if inner.Label != "x" {
		t.Errorf("expected inner label 'x', got %q", inner.Label)
	}
}

// TestProbeE_QualifiedVarWithSpace verifies M . x (spaces) is composition, not qualified.
func TestProbeE_QualifiedVarWithSpace(t *testing.T) {
	source := `main := M . x`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	// Should be: ExprInfix { Left: ExprCon(M), Op: ".", Right: ExprVar(x) }
	inf, ok := d.Expr.(*ExprInfix)
	if !ok {
		t.Fatalf("expected ExprInfix for 'M . x', got %T", d.Expr)
	}
	if inf.Op != "." {
		t.Errorf("expected op '.', got %q", inf.Op)
	}
}

// TestProbeE_QualifiedConAdjacent verifies M.Just (no spaces) is qualified con.
func TestProbeE_QualifiedConAdjacent(t *testing.T) {
	source := `main := M.Just`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	qc, ok := d.Expr.(*ExprQualCon)
	if !ok {
		t.Fatalf("expected ExprQualCon, got %T", d.Expr)
	}
	if qc.Qualifier != "M" || qc.Name != "Just" {
		t.Errorf("expected M.Just, got %s.%s", qc.Qualifier, qc.Name)
	}
}

// TestProbeE_CaseScrutineeNoBrace verifies that `{` is NOT an atom start
// inside a case scrutinee (noBraceAtom flag).
func TestProbeE_CaseScrutineeNoBrace(t *testing.T) {
	// `case x { ... }` — the `{` should start the case body, not be parsed
	// as a record/block expression.
	source := `main := case x { True -> 1; False -> 0 }`
	parseMustSucceed(t, source)
}

// TestProbeE_CaseScrutineeComplexExpr verifies complex scrutinee parsing.
func TestProbeE_CaseScrutineeComplexExpr(t *testing.T) {
	source := `main := case f x y { True -> 1 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	caseExpr, ok := d.Expr.(*ExprCase)
	if !ok {
		t.Fatalf("expected ExprCase, got %T", d.Expr)
	}
	// The scrutinee should be App(App(f, x), y).
	app, ok := caseExpr.Scrutinee.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp as scrutinee, got %T", caseExpr.Scrutinee)
	}
	_ = app
}

// TestProbeE_CaseAltStall verifies that garbage in case alternatives
// doesn't cause infinite loop.
func TestProbeE_CaseAltStall(t *testing.T) {
	source := `main := case x { + + + }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for garbage in case alts")
	}
}
