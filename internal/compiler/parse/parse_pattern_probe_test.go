//go:build probe

// Pattern probe tests — pattern matching, nesting, qualified constructors, literals, records.
// Does NOT cover: parse_pattern_bind_probe_test.go, parse_expr_probe_test.go, parse_crash_probe_test.go.
package parse

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive // dot import for tightly-coupled subpackage
)

// ===== From probe_b =====

// TestProbeB_PatternTupleWithQualCon checks tuple pattern with qualified constructors.
func TestProbeB_PatternTupleWithQualCon(t *testing.T) {
	src := `main := case x { (M.Just a, M.Nothing) => a }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	// The tuple pattern should be a PatRecord with _1, _2 labels.
	pr, ok := c.Alts[0].Pattern.(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord for tuple pattern, got %T", c.Alts[0].Pattern)
	}
	if len(pr.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(pr.Fields))
	}
	// Check first field is PatQualCon M.Just with one arg.
	pqc1, ok := pr.Fields[0].Pattern.(*PatQualCon)
	if !ok {
		t.Fatalf("field[0]: expected PatQualCon, got %T", pr.Fields[0].Pattern)
	}
	if pqc1.Qualifier != "M" || pqc1.Con != "Just" {
		t.Errorf("field[0]: expected M.Just, got %s.%s", pqc1.Qualifier, pqc1.Con)
	}
	// Check second field is PatQualCon M.Nothing with zero args.
	pqc2, ok := pr.Fields[1].Pattern.(*PatQualCon)
	if !ok {
		t.Fatalf("field[1]: expected PatQualCon, got %T", pr.Fields[1].Pattern)
	}
	if pqc2.Qualifier != "M" || pqc2.Con != "Nothing" {
		t.Errorf("field[1]: expected M.Nothing, got %s.%s", pqc2.Qualifier, pqc2.Con)
	}
}

// TestProbeB_PatternDeeplyNested checks deeply nested patterns.
func TestProbeB_PatternDeeplyNested(t *testing.T) {
	src := `main := case x { Just (Just (Just a)) => a }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	if len(c.Alts) != 1 {
		t.Fatalf("expected 1 alt, got %d", len(c.Alts))
	}
	pc, ok := c.Alts[0].Pattern.(*PatCon)
	if !ok {
		t.Fatalf("expected PatCon, got %T", c.Alts[0].Pattern)
	}
	if pc.Con != "Just" || len(pc.Args) != 1 {
		t.Fatalf("expected Just with 1 arg")
	}
	// Inner: (Just (Just a))
	inner, ok := pc.Args[0].(*PatParen)
	if !ok {
		t.Fatalf("expected PatParen, got %T", pc.Args[0])
	}
	innerCon, ok := inner.Inner.(*PatCon)
	if !ok {
		t.Fatalf("expected inner PatCon, got %T", inner.Inner)
	}
	if innerCon.Con != "Just" || len(innerCon.Args) != 1 {
		t.Fatalf("expected inner Just with 1 arg")
	}
}

// TestProbeB_PatternLiteralZero checks pattern matching on literal 0.
func TestProbeB_PatternLiteralZero(t *testing.T) {
	src := `main := case x { 0 => 1; _ => 0 }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	if len(c.Alts) != 2 {
		t.Fatalf("expected 2 alts, got %d", len(c.Alts))
	}
	pl, ok := c.Alts[0].Pattern.(*PatLit)
	if !ok {
		t.Fatalf("expected PatLit for 0, got %T", c.Alts[0].Pattern)
	}
	if pl.Value != "0" || pl.Kind != LitInt {
		t.Errorf("expected int literal 0, got %q kind=%d", pl.Value, pl.Kind)
	}
}

// TestProbeB_PatternStringLit checks string literal in pattern.
func TestProbeB_PatternStringLit(t *testing.T) {
	src := `main := case x { "hello" => 1; _ => 0 }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	pl, ok := c.Alts[0].Pattern.(*PatLit)
	if !ok {
		t.Fatalf("expected PatLit for string, got %T", c.Alts[0].Pattern)
	}
	if pl.Kind != LitString {
		t.Errorf("expected string literal pattern, got kind=%d", pl.Kind)
	}
}

// TestProbeB_PatternRecordInCase checks record pattern matching.
func TestProbeB_PatternRecordInCase(t *testing.T) {
	src := `main := case x { { name: n, age: a } => n }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	pr, ok := c.Alts[0].Pattern.(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord, got %T", c.Alts[0].Pattern)
	}
	if len(pr.Fields) != 2 {
		t.Errorf("expected 2 record fields, got %d", len(pr.Fields))
	}
}

// TestProbeB_PatternWildcardAtom checks wildcard _ as pattern atom under constructor.
func TestProbeB_PatternWildcardAtom(t *testing.T) {
	src := `main := case x { Just _ => 1 }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	pc, ok := c.Alts[0].Pattern.(*PatCon)
	if !ok {
		t.Fatalf("expected PatCon, got %T", c.Alts[0].Pattern)
	}
	if len(pc.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(pc.Args))
	}
	_, ok = pc.Args[0].(*PatWild)
	if !ok {
		t.Fatalf("expected PatWild, got %T", pc.Args[0])
	}
}

// TestProbeB_PatternUnitInCase checks () pattern in case.
func TestProbeB_PatternUnitInCase(t *testing.T) {
	src := `main := case x { () => 1 }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	// () should be PatRecord with 0 fields.
	pr, ok := c.Alts[0].Pattern.(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord for (), got %T", c.Alts[0].Pattern)
	}
	if len(pr.Fields) != 0 {
		t.Errorf("expected 0 fields for unit pattern, got %d", len(pr.Fields))
	}
}

// TestProbeB_DotQualConAtomInPattern checks qualified constructor as atom
// in pattern position (0-arg qualified con nested under another con).
func TestProbeB_DotQualConAtomInPattern(t *testing.T) {
	src := `main := case x { Pair M.Nothing M.Nothing => 1 }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	pc, ok := c.Alts[0].Pattern.(*PatCon)
	if !ok {
		t.Fatalf("expected PatCon, got %T", c.Alts[0].Pattern)
	}
	if pc.Con != "Pair" {
		t.Errorf("expected Pair, got %q", pc.Con)
	}
	if len(pc.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(pc.Args))
	}
	for i, arg := range pc.Args {
		pqc, ok := arg.(*PatQualCon)
		if !ok {
			t.Errorf("arg[%d]: expected PatQualCon, got %T", i, arg)
			continue
		}
		if pqc.Qualifier != "M" || pqc.Con != "Nothing" {
			t.Errorf("arg[%d]: expected M.Nothing, got %s.%s", i, pqc.Qualifier, pqc.Con)
		}
	}
}

// ===== From probe_e =====

// TestProbeE_DeeplyNestedPattern verifies deeply nested constructor patterns.
func TestProbeE_DeeplyNestedPattern(t *testing.T) {
	// Build: Just (Just (Just ... x ...))
	depth := 10
	var b strings.Builder
	for i := 0; i < depth; i++ {
		b.WriteString("Just (")
	}
	b.WriteString("x")
	for i := 0; i < depth; i++ {
		b.WriteString(")")
	}
	source := fmt.Sprintf("main := case x { %s => x }", b.String())
	parseMustSucceed(t, source)
}

// TestProbeE_WildcardAsConstructorArg verifies `Just _` pattern.
func TestProbeE_WildcardAsConstructorArg(t *testing.T) {
	parseMustSucceed(t, `main := case x { Just _ => 1 }`)
}

// TestProbeE_NestedTuplePattern verifies `(x, (y, z))` pattern.
func TestProbeE_NestedTuplePattern(t *testing.T) {
	parseMustSucceed(t, `main := case x { (a, (b, c)) => a }`)
}

// TestProbeE_PatternLiterals verifies literal patterns.
func TestProbeE_PatternLiterals(t *testing.T) {
	parseMustSucceed(t, `main := case x { 0 => 1; "hello" => 2; _ => 3 }`)
}

// TestProbeE_RecordPattern verifies record pattern matching.
func TestProbeE_RecordPattern(t *testing.T) {
	parseMustSucceed(t, `main := case x { { a: 1, b: y } => y }`)
}

// TestProbeE_EmptyRecordPattern verifies {} pattern.
func TestProbeE_EmptyRecordPattern(t *testing.T) {
	parseMustSucceed(t, `main := case x { {} => 1 }`)
}

// TestProbeE_PatternParenUnit verifies () in pattern position.
func TestProbeE_PatternParenUnit(t *testing.T) {
	parseMustSucceed(t, `main := case x { () => 1 }`)
}
