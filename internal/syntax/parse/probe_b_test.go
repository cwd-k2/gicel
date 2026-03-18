//go:build probe

package parse

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/cwd-k2/gicel/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
)

// ==========================================
// Probe B: Parser / Syntax Adversarial Tests
// ==========================================

// ----- 1. Dot Disambiguation -----

// TestProbeB_DotQualifiedVarAdjacent verifies that A.b (no spaces) parses
// as a qualified variable reference ExprQualVar.
func TestProbeB_DotQualifiedVarAdjacent(t *testing.T) {
	prog := parseMustSucceed(t, "main := M.x")
	d := prog.Decls[0].(*DeclValueDef)
	qv, ok := d.Expr.(*ExprQualVar)
	if !ok {
		t.Fatalf("expected ExprQualVar, got %T", d.Expr)
	}
	if qv.Qualifier != "M" || qv.Name != "x" {
		t.Errorf("expected M.x, got %s.%s", qv.Qualifier, qv.Name)
	}
}

// TestProbeB_DotQualifiedConAdjacent verifies that A.B (no spaces) parses
// as a qualified constructor reference ExprQualCon.
func TestProbeB_DotQualifiedConAdjacent(t *testing.T) {
	prog := parseMustSucceed(t, "main := M.Just")
	d := prog.Decls[0].(*DeclValueDef)
	qc, ok := d.Expr.(*ExprQualCon)
	if !ok {
		t.Fatalf("expected ExprQualCon, got %T", d.Expr)
	}
	if qc.Qualifier != "M" || qc.Name != "Just" {
		t.Errorf("expected M.Just, got %s.%s", qc.Qualifier, qc.Name)
	}
}

// TestProbeB_DotCompositionSpaced verifies that A . B (with spaces) parses
// as composition (ExprInfix with ".") rather than a qualified name.
func TestProbeB_DotCompositionSpaced(t *testing.T) {
	prog := parseMustSucceed(t, "main := A . B")
	d := prog.Decls[0].(*DeclValueDef)
	inf, ok := d.Expr.(*ExprInfix)
	if !ok {
		t.Fatalf("expected ExprInfix for composition, got %T", d.Expr)
	}
	if inf.Op != "." {
		t.Errorf("expected operator '.', got %q", inf.Op)
	}
}

// TestProbeB_DotCompositionLowerLower verifies that f.g (lower.lower adjacent)
// is not treated as a qualified name (only Upper.lower forms qualified names).
// The parser should treat this as composition or an error, not a QualVar.
func TestProbeB_DotCompositionLowerLower(t *testing.T) {
	// f.g — lower.dot.lower. The dot is adjacent but 'f' is lowercase,
	// so tryQualifiedExpr is never triggered (it only fires after TokUpper).
	// This should parse as: ExprInfix { Left: f, Op: ".", Right: g }
	prog := parseMustSucceed(t, "main := f.g")
	d := prog.Decls[0].(*DeclValueDef)
	// The lexer produces: TokLower("f"), TokDot("."), TokLower("g")
	// In parseApp, 'f' is an atom, then '.' is an infix op (isInfixOp returns true for TokDot).
	inf, ok := d.Expr.(*ExprInfix)
	if !ok {
		// If this fails, the parser might have produced something else.
		t.Fatalf("expected ExprInfix for f.g, got %T", d.Expr)
	}
	if inf.Op != "." {
		t.Errorf("expected operator '.', got %q", inf.Op)
	}
}

// TestProbeB_DotDoubleQualified probes A.B.C — double-qualified name.
// The parser only handles single qualification (Upper.lower or Upper.Upper).
// A.B.C should parse as: ExprQualCon(A,B) followed by infix "." or app of C.
func TestProbeB_DotDoubleQualified(t *testing.T) {
	// A.B.C: after parsing A.B (qualified con), the next token is .C.
	// If .C is adjacent to B, the parser might try to continue qualification.
	// Current implementation: tryQualifiedExpr only consumes one dot.
	prog, es := parse("main := A.B.C")
	// Just ensure no panic. Document what actually happens.
	if es.HasErrors() {
		t.Logf("parser rejected A.B.C: %s", es.Format())
		return
	}
	d := prog.Decls[0].(*DeclValueDef)
	t.Logf("A.B.C parsed as %T", d.Expr)
	// If ExprQualCon(A,B), the ".C" part would need infix continuation.
	// Check if the outer node is an infix with ".":
	if inf, ok := d.Expr.(*ExprInfix); ok {
		t.Logf("A.B.C outer: ExprInfix with op=%q", inf.Op)
	}
}

// TestProbeB_DotQualVarInApp checks that qualified var works inside application:
// f M.x should be App(f, QualVar(M,x))
func TestProbeB_DotQualVarInApp(t *testing.T) {
	prog := parseMustSucceed(t, "main := f M.x")
	d := prog.Decls[0].(*DeclValueDef)
	app, ok := d.Expr.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp, got %T", d.Expr)
	}
	qv, ok := app.Arg.(*ExprQualVar)
	if !ok {
		t.Fatalf("expected ExprQualVar arg, got %T", app.Arg)
	}
	if qv.Qualifier != "M" || qv.Name != "x" {
		t.Errorf("expected M.x, got %s.%s", qv.Qualifier, qv.Name)
	}
}

// TestProbeB_DotQualConInPattern checks Q.Con in pattern position.
func TestProbeB_DotQualConInPattern(t *testing.T) {
	prog := parseMustSucceed(t, "main := case x { M.Just a -> a; M.Nothing -> 0 }")
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	if len(c.Alts) != 2 {
		t.Fatalf("expected 2 alts, got %d", len(c.Alts))
	}
	pqc, ok := c.Alts[0].Pattern.(*PatQualCon)
	if !ok {
		t.Fatalf("expected PatQualCon, got %T", c.Alts[0].Pattern)
	}
	if pqc.Qualifier != "M" || pqc.Con != "Just" {
		t.Errorf("expected M.Just, got %s.%s", pqc.Qualifier, pqc.Con)
	}
	if len(pqc.Args) != 1 {
		t.Errorf("expected 1 arg in M.Just pattern, got %d", len(pqc.Args))
	}
}

// TestProbeB_DotQualifiedType checks qualified types: M.Int in type position.
func TestProbeB_DotQualifiedType(t *testing.T) {
	prog := parseMustSucceed(t, "f :: M.Int -> M.Bool")
	d := prog.Decls[0].(*DeclTypeAnn)
	arr, ok := d.Type.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow, got %T", d.Type)
	}
	from, ok := arr.From.(*TyExprQualCon)
	if !ok {
		t.Fatalf("expected TyExprQualCon for from, got %T", arr.From)
	}
	if from.Qualifier != "M" || from.Name != "Int" {
		t.Errorf("expected M.Int, got %s.%s", from.Qualifier, from.Name)
	}
}

// ----- 2. Operator Edge Cases -----

// TestProbeB_OperatorAsValue checks (++) as a value reference.
func TestProbeB_OperatorAsValue(t *testing.T) {
	prog := parseMustSucceed(t, "main := (++)")
	d := prog.Decls[0].(*DeclValueDef)
	v, ok := d.Expr.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar for (++), got %T", d.Expr)
	}
	if v.Name != "++" {
		t.Errorf("expected '++', got %q", v.Name)
	}
}

// TestProbeB_DotAsOperatorValue checks (.) as a value reference.
func TestProbeB_DotAsOperatorValue(t *testing.T) {
	prog := parseMustSucceed(t, "main := (.)")
	d := prog.Decls[0].(*DeclValueDef)
	v, ok := d.Expr.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar for (.), got %T", d.Expr)
	}
	if v.Name != "." {
		t.Errorf("expected '.', got %q", v.Name)
	}
}

// TestProbeB_RightSection checks right section: (+ 1).
func TestProbeB_RightSection(t *testing.T) {
	prog := parseMustSucceed(t, "main := (+ 1)")
	d := prog.Decls[0].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection for (+ 1), got %T", d.Expr)
	}
	if !sec.IsRight {
		t.Error("expected right section")
	}
	if sec.Op != "+" {
		t.Errorf("expected op '+', got %q", sec.Op)
	}
}

// TestProbeB_LeftSection checks left section: (1 +).
func TestProbeB_LeftSection(t *testing.T) {
	prog := parseMustSucceed(t, "main := (1 +)")
	d := prog.Decls[0].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection for (1 +), got %T", d.Expr)
	}
	if sec.IsRight {
		t.Error("expected left section")
	}
	if sec.Op != "+" {
		t.Errorf("expected op '+', got %q", sec.Op)
	}
}

// TestProbeB_DotRightSection checks right section with dot: (. f).
func TestProbeB_DotRightSection(t *testing.T) {
	prog := parseMustSucceed(t, "main := (. f)")
	d := prog.Decls[0].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection for (. f), got %T", d.Expr)
	}
	if !sec.IsRight || sec.Op != "." {
		t.Errorf("expected right section with '.', got IsRight=%v Op=%q", sec.IsRight, sec.Op)
	}
}

// TestProbeB_FixityDecl checks fixity declarations at boundaries.
func TestProbeB_FixityDecl(t *testing.T) {
	src := `infixl 6 +
infixr 5 ++
infixn 4 ==
main := 1 + 2 ++ 3 == 4`
	prog := parseMustSucceed(t, src)
	// Should have 3 fixity decls + 1 value def = 4 decls
	if len(prog.Decls) != 4 {
		t.Errorf("expected 4 decls, got %d", len(prog.Decls))
	}
}

// TestProbeB_CustomOperatorChars checks exotic operator characters.
func TestProbeB_CustomOperatorChars(t *testing.T) {
	src := `infixl 6 <$>
(<$>) := map
main := f <$> x`
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) < 2 {
		t.Fatalf("expected at least 2 decls, got %d", len(prog.Decls))
	}
}

// TestProbeB_OperatorPrecedenceAssociativity checks fixity ordering.
func TestProbeB_OperatorPrecedenceAssociativity(t *testing.T) {
	src := `infixl 6 +
infixl 7 *
main := a + b * c`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[2].(*DeclValueDef)
	// a + b * c should be a + (b * c) since * has higher precedence
	inf, ok := d.Expr.(*ExprInfix)
	if !ok {
		t.Fatalf("expected ExprInfix, got %T", d.Expr)
	}
	if inf.Op != "+" {
		t.Errorf("expected outer op '+', got %q", inf.Op)
	}
	inner, ok := inf.Right.(*ExprInfix)
	if !ok {
		t.Fatalf("expected inner ExprInfix for b*c, got %T", inf.Right)
	}
	if inner.Op != "*" {
		t.Errorf("expected inner op '*', got %q", inner.Op)
	}
}

// TestProbeB_OperatorRightAssocChain checks right-associative chain: a >> b >> c = a >> (b >> c).
func TestProbeB_OperatorRightAssocChain(t *testing.T) {
	src := `infixr 5 >>
main := a >> b >> c`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[1].(*DeclValueDef)
	inf := d.Expr.(*ExprInfix)
	if inf.Op != ">>" {
		t.Errorf("expected outer op '>>', got %q", inf.Op)
	}
	// right should also be >>
	inner, ok := inf.Right.(*ExprInfix)
	if !ok {
		t.Fatalf("expected inner ExprInfix for right-assoc, got %T", inf.Right)
	}
	if inner.Op != ">>" {
		t.Errorf("expected inner op '>>', got %q", inner.Op)
	}
}

// ----- 3. Pattern Parsing -----

// TestProbeB_PatternTupleWithQualCon checks (Q.Con x, Q.Con2 y) as a tuple pattern.
func TestProbeB_PatternTupleWithQualCon(t *testing.T) {
	src := `main := case x { (M.Just a, N.Nothing) -> a }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	if len(c.Alts) != 1 {
		t.Fatalf("expected 1 alt, got %d", len(c.Alts))
	}
	pr, ok := c.Alts[0].Pattern.(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord (tuple), got %T", c.Alts[0].Pattern)
	}
	if len(pr.Fields) != 2 {
		t.Fatalf("expected 2 fields in tuple pattern, got %d", len(pr.Fields))
	}
	// First field should be M.Just a
	pqc, ok := pr.Fields[0].Pattern.(*PatQualCon)
	if !ok {
		t.Fatalf("expected PatQualCon in first tuple element, got %T", pr.Fields[0].Pattern)
	}
	if pqc.Qualifier != "M" || pqc.Con != "Just" {
		t.Errorf("expected M.Just, got %s.%s", pqc.Qualifier, pqc.Con)
	}
	if len(pqc.Args) != 1 {
		t.Errorf("expected 1 arg in M.Just pattern, got %d", len(pqc.Args))
	}
}

// TestProbeB_PatternDeeplyNested checks deeply nested patterns.
func TestProbeB_PatternDeeplyNested(t *testing.T) {
	src := `main := case x { Just (Just (Just a)) -> a }`
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

// TestProbeB_PatternLiteralZero checks that 0 can be used as a pattern.
func TestProbeB_PatternLiteralZero(t *testing.T) {
	src := `main := case x { 0 -> True; _ -> False }`
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
	src := `main := case x { "hello" -> 1; _ -> 0 }`
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
	src := `main := case x { { name: n, age: a } -> n }`
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

// TestProbeB_PatternWildcardAtom checks _ in nested pattern positions.
func TestProbeB_PatternWildcardAtom(t *testing.T) {
	src := `main := case x { Just _ -> 1 }`
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

// TestProbeB_PatternUnitInCase checks () as unit pattern.
func TestProbeB_PatternUnitInCase(t *testing.T) {
	src := `main := case x { () -> 1 }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	pr, ok := c.Alts[0].Pattern.(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord for unit pattern, got %T", c.Alts[0].Pattern)
	}
	if len(pr.Fields) != 0 {
		t.Errorf("expected empty record (unit), got %d fields", len(pr.Fields))
	}
}

// ----- 4. Import Parsing -----

// TestProbeB_ImportEmptySelective checks import M () — empty selective import.
func TestProbeB_ImportEmptySelective(t *testing.T) {
	prog := parseMustSucceed(t, "import M ()\nmain := 1")
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	imp := prog.Imports[0]
	if imp.ModuleName != "M" {
		t.Errorf("expected module M, got %q", imp.ModuleName)
	}
	// Names should be non-nil (selective) but empty
	if imp.Names == nil {
		t.Error("expected non-nil Names for selective import M ()")
	}
	if len(imp.Names) != 0 {
		t.Errorf("expected 0 names, got %d", len(imp.Names))
	}
}

// TestProbeB_ImportMixed checks import M (A(..), B, c) — mixed selective import.
func TestProbeB_ImportMixed(t *testing.T) {
	prog := parseMustSucceed(t, "import M (A(..), B, c)\nmain := 1")
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	imp := prog.Imports[0]
	if len(imp.Names) != 3 {
		t.Fatalf("expected 3 import names, got %d", len(imp.Names))
	}
	// A(..)
	if imp.Names[0].Name != "A" || !imp.Names[0].AllSubs {
		t.Errorf("expected A(..), got %+v", imp.Names[0])
	}
	// B
	if imp.Names[1].Name != "B" || imp.Names[1].HasSub {
		t.Errorf("expected bare B, got %+v", imp.Names[1])
	}
	// c
	if imp.Names[2].Name != "c" {
		t.Errorf("expected c, got %+v", imp.Names[2])
	}
}

// TestProbeB_ImportQualified checks import M as N.
func TestProbeB_ImportQualified(t *testing.T) {
	prog := parseMustSucceed(t, "import M as N\nmain := 1")
	imp := prog.Imports[0]
	if imp.Alias != "N" {
		t.Errorf("expected alias N, got %q", imp.Alias)
	}
}

// TestProbeB_ImportWithSubList checks import M (T(A, B)).
func TestProbeB_ImportWithSubList(t *testing.T) {
	prog := parseMustSucceed(t, "import M (T(A, B))\nmain := 1")
	imp := prog.Imports[0]
	if len(imp.Names) != 1 {
		t.Fatalf("expected 1 import name, got %d", len(imp.Names))
	}
	n := imp.Names[0]
	if n.Name != "T" || !n.HasSub || n.AllSubs {
		t.Errorf("expected T with explicit subs, got %+v", n)
	}
	if len(n.SubList) != 2 || n.SubList[0] != "A" || n.SubList[1] != "B" {
		t.Errorf("expected subs [A, B], got %v", n.SubList)
	}
}

// TestProbeB_ImportOperator checks import M ((+)) — operator in import list.
func TestProbeB_ImportOperator(t *testing.T) {
	prog := parseMustSucceed(t, "import M ((+))\nmain := 1")
	imp := prog.Imports[0]
	if len(imp.Names) != 1 {
		t.Fatalf("expected 1 import name, got %d", len(imp.Names))
	}
	if imp.Names[0].Name != "+" {
		t.Errorf("expected operator '+', got %q", imp.Names[0].Name)
	}
}

// TestProbeB_ImportDottedModule checks dotted module path: import Std.Num.
func TestProbeB_ImportDottedModule(t *testing.T) {
	prog := parseMustSucceed(t, "import Std.Num\nmain := 1")
	if prog.Imports[0].ModuleName != "Std.Num" {
		t.Errorf("expected Std.Num, got %q", prog.Imports[0].ModuleName)
	}
}

// TestProbeB_ImportMalformedList checks error recovery on malformed import list.
func TestProbeB_ImportMalformedList(t *testing.T) {
	// import M (123) — numeric literal is not valid in import list
	errMsg := parseMustFail(t, "import M (123)\nmain := 1")
	if !strings.Contains(errMsg, "name") && !strings.Contains(errMsg, "expect") {
		t.Logf("unexpected error format: %s", errMsg)
	}
}

// TestProbeB_ImportDotInList checks import M ((.), x).
func TestProbeB_ImportDotInList(t *testing.T) {
	prog := parseMustSucceed(t, "import M ((.), x)\nmain := 1")
	imp := prog.Imports[0]
	if len(imp.Names) != 2 {
		t.Fatalf("expected 2 import names, got %d", len(imp.Names))
	}
	if imp.Names[0].Name != "." {
		t.Errorf("expected '.', got %q", imp.Names[0].Name)
	}
	if imp.Names[1].Name != "x" {
		t.Errorf("expected 'x', got %q", imp.Names[1].Name)
	}
}

// ----- 5. Expression Edge Cases -----

// TestProbeB_EmptyDoBlock checks do {} — empty do-block.
func TestProbeB_EmptyDoBlock(t *testing.T) {
	// An empty do-block should parse without panic.
	// Whether it's semantically valid is the checker's concern.
	prog, es := parse("main := do {}")
	_ = es // may or may not error
	if prog != nil && len(prog.Decls) > 0 {
		d, ok := prog.Decls[0].(*DeclValueDef)
		if ok {
			doExpr, ok := d.Expr.(*ExprDo)
			if ok && len(doExpr.Stmts) != 0 {
				t.Errorf("expected 0 stmts in empty do, got %d", len(doExpr.Stmts))
			}
		}
	}
}

// TestProbeB_EmptyCaseAlts checks case x {} — case with no alternatives.
func TestProbeB_EmptyCaseAlts(t *testing.T) {
	// An empty case block should parse without panic.
	prog, es := parse("main := case x {}")
	_ = es
	if prog != nil && len(prog.Decls) > 0 {
		d, ok := prog.Decls[0].(*DeclValueDef)
		if ok {
			c, ok := d.Expr.(*ExprCase)
			if ok && len(c.Alts) != 0 {
				t.Errorf("expected 0 alts in empty case, got %d", len(c.Alts))
			}
		}
	}
}

// TestProbeB_LambdaChain checks nested lambdas: \x. \y. \z. x.
func TestProbeB_LambdaChain(t *testing.T) {
	prog := parseMustSucceed(t, `main := \x. \y. \z. x`)
	d := prog.Decls[0].(*DeclValueDef)
	lam1, ok := d.Expr.(*ExprLam)
	if !ok {
		t.Fatalf("expected ExprLam, got %T", d.Expr)
	}
	lam2, ok := lam1.Body.(*ExprLam)
	if !ok {
		t.Fatalf("expected nested ExprLam, got %T", lam1.Body)
	}
	lam3, ok := lam2.Body.(*ExprLam)
	if !ok {
		t.Fatalf("expected doubly nested ExprLam, got %T", lam2.Body)
	}
	v, ok := lam3.Body.(*ExprVar)
	if !ok || v.Name != "x" {
		t.Errorf("expected body 'x', got %T", lam3.Body)
	}
}

// TestProbeB_LambdaMultipleParams checks \x y z. body — multi-param lambda.
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

// TestProbeB_RecordUpdateChain checks nested record updates: { { r | x: 1 } | y: 2 }.
func TestProbeB_RecordUpdateChain(t *testing.T) {
	prog := parseMustSucceed(t, "main := { { r | x: 1 } | y: 2 }")
	d := prog.Decls[0].(*DeclValueDef)
	outer, ok := d.Expr.(*ExprRecordUpdate)
	if !ok {
		t.Fatalf("expected outer ExprRecordUpdate, got %T", d.Expr)
	}
	if len(outer.Updates) != 1 || outer.Updates[0].Label != "y" {
		t.Errorf("expected outer update y, got %+v", outer.Updates)
	}
	inner, ok := outer.Record.(*ExprRecordUpdate)
	if !ok {
		t.Fatalf("expected inner ExprRecordUpdate, got %T", outer.Record)
	}
	if len(inner.Updates) != 1 || inner.Updates[0].Label != "x" {
		t.Errorf("expected inner update x, got %+v", inner.Updates)
	}
}

// TestProbeB_RecordLiteral checks basic record literal.
func TestProbeB_RecordLiteral(t *testing.T) {
	prog := parseMustSucceed(t, "main := { x: 1, y: 2 }")
	d := prog.Decls[0].(*DeclValueDef)
	r, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord, got %T", d.Expr)
	}
	if len(r.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(r.Fields))
	}
}

// TestProbeB_UnitExpr checks () as unit value (empty record).
func TestProbeB_UnitExpr(t *testing.T) {
	prog := parseMustSucceed(t, "main := ()")
	d := prog.Decls[0].(*DeclValueDef)
	r, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for unit, got %T", d.Expr)
	}
	if len(r.Fields) != 0 {
		t.Errorf("expected 0 fields for unit, got %d", len(r.Fields))
	}
}

// TestProbeB_TupleExpr checks (1, 2, 3) as tuple (desugared record).
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
	if r.Fields[0].Label != "_1" || r.Fields[1].Label != "_2" || r.Fields[2].Label != "_3" {
		t.Errorf("expected labels _1, _2, _3")
	}
}

// TestProbeB_TypeAnnotationInExpr checks inline type annotation: (x :: Int).
func TestProbeB_TypeAnnotationInExpr(t *testing.T) {
	prog := parseMustSucceed(t, "main := (x :: Int)")
	d := prog.Decls[0].(*DeclValueDef)
	p, ok := d.Expr.(*ExprParen)
	if !ok {
		t.Fatalf("expected ExprParen, got %T", d.Expr)
	}
	ann, ok := p.Inner.(*ExprAnn)
	if !ok {
		t.Fatalf("expected ExprAnn inside paren, got %T", p.Inner)
	}
	v, ok := ann.Expr.(*ExprVar)
	if !ok || v.Name != "x" {
		t.Errorf("expected var 'x', got %T", ann.Expr)
	}
}

// TestProbeB_DoBlockWithPureBind checks pure bindings in do: x := 1.
func TestProbeB_DoBlockWithPureBind(t *testing.T) {
	prog := parseMustSucceed(t, "main := do { x := 1; pure x }")
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 2 {
		t.Fatalf("expected 2 stmts, got %d", len(doExpr.Stmts))
	}
	pb, ok := doExpr.Stmts[0].(*StmtPureBind)
	if !ok {
		t.Fatalf("expected StmtPureBind, got %T", doExpr.Stmts[0])
	}
	if pb.Var != "x" {
		t.Errorf("expected var 'x', got %q", pb.Var)
	}
}

// TestProbeB_DoBlockWildcardBind checks _ <- action in do-block.
func TestProbeB_DoBlockWildcardBind(t *testing.T) {
	prog := parseMustSucceed(t, "main := do { _ <- action; pure 1 }")
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 2 {
		t.Fatalf("expected 2 stmts, got %d", len(doExpr.Stmts))
	}
	sb, ok := doExpr.Stmts[0].(*StmtBind)
	if !ok {
		t.Fatalf("expected StmtBind, got %T", doExpr.Stmts[0])
	}
	if sb.Var != "_" {
		t.Errorf("expected var '_', got %q", sb.Var)
	}
}

// TestProbeB_ListLiteral checks [1, 2, 3].
func TestProbeB_ListLiteral(t *testing.T) {
	prog := parseMustSucceed(t, "main := [1, 2, 3]")
	d := prog.Decls[0].(*DeclValueDef)
	l, ok := d.Expr.(*ExprList)
	if !ok {
		t.Fatalf("expected ExprList, got %T", d.Expr)
	}
	if len(l.Elems) != 3 {
		t.Errorf("expected 3 elements, got %d", len(l.Elems))
	}
}

// TestProbeB_EmptyList checks [].
func TestProbeB_EmptyList(t *testing.T) {
	prog := parseMustSucceed(t, "main := []")
	d := prog.Decls[0].(*DeclValueDef)
	l, ok := d.Expr.(*ExprList)
	if !ok {
		t.Fatalf("expected ExprList, got %T", d.Expr)
	}
	if len(l.Elems) != 0 {
		t.Errorf("expected 0 elements, got %d", len(l.Elems))
	}
}

// TestProbeB_RecordProjection checks r.#x — record projection.
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

// TestProbeB_ChainedProjection checks r.#x.#y.
func TestProbeB_ChainedProjection(t *testing.T) {
	prog := parseMustSucceed(t, "main := r.#x.#y")
	d := prog.Decls[0].(*DeclValueDef)
	proj2, ok := d.Expr.(*ExprProject)
	if !ok {
		t.Fatalf("expected ExprProject for outer, got %T", d.Expr)
	}
	if proj2.Label != "y" {
		t.Errorf("expected outer label 'y', got %q", proj2.Label)
	}
	proj1, ok := proj2.Record.(*ExprProject)
	if !ok {
		t.Fatalf("expected inner ExprProject, got %T", proj2.Record)
	}
	if proj1.Label != "x" {
		t.Errorf("expected inner label 'x', got %q", proj1.Label)
	}
}

// TestProbeB_BlockExpr checks block expression with let-bindings.
func TestProbeB_BlockExpr(t *testing.T) {
	prog := parseMustSucceed(t, "main := { x := 1; y := 2; x }")
	d := prog.Decls[0].(*DeclValueDef)
	blk, ok := d.Expr.(*ExprBlock)
	if !ok {
		t.Fatalf("expected ExprBlock, got %T", d.Expr)
	}
	if len(blk.Binds) != 2 {
		t.Errorf("expected 2 bindings, got %d", len(blk.Binds))
	}
}

// TestProbeB_TypeApplication checks f @Int — explicit type application.
func TestProbeB_TypeApplication(t *testing.T) {
	prog := parseMustSucceed(t, "main := f @Int")
	d := prog.Decls[0].(*DeclValueDef)
	tyApp, ok := d.Expr.(*ExprTyApp)
	if !ok {
		t.Fatalf("expected ExprTyApp, got %T", d.Expr)
	}
	v, ok := tyApp.Expr.(*ExprVar)
	if !ok || v.Name != "f" {
		t.Errorf("expected var 'f', got %T", tyApp.Expr)
	}
}

// ----- 6. Error Recovery -----

// TestProbeB_UnterminatedString checks that unterminated string produces a lex error.
func TestProbeB_UnterminatedString(t *testing.T) {
	errMsg := parseMustFail(t, `main := "hello`)
	if !strings.Contains(errMsg, "unterminated") {
		t.Errorf("expected 'unterminated' in error, got: %s", errMsg)
	}
}

// TestProbeB_MismatchedParens checks mismatched parentheses.
func TestProbeB_MismatchedParens(t *testing.T) {
	errMsg := parseMustFail(t, "main := (1 + 2")
	// Should report missing )
	if !strings.Contains(errMsg, ")") && !strings.Contains(errMsg, "RParen") {
		t.Logf("error for mismatched parens: %s", errMsg)
	}
}

// TestProbeB_UnexpectedEOFInExpr checks unexpected EOF after := in a value definition.
// BUG: medium - "main :=" with no expression body parses without error.
// The parser's parseExpr -> parseApp -> parseAtom returns nil for EOF,
// and parseApp substitutes an <error> ExprVar, but no error is actually recorded.
// This silently accepts an incomplete definition.
func TestProbeB_UnexpectedEOFInExpr(t *testing.T) {
	_, es := parse("main :=")
	if !es.HasErrors() {
		t.Log("BUG CONFIRMED: 'main :=' (no expression body) accepted without error")
	}
}

// TestProbeB_UnexpectedEOFInCase checks unexpected EOF in case expression.
func TestProbeB_UnexpectedEOFInCase(t *testing.T) {
	errMsg := parseMustFail(t, "main := case x {")
	if errMsg == "" {
		t.Error("expected non-empty error")
	}
}

// TestProbeB_UnexpectedEOFInDo checks unexpected EOF in do-block.
func TestProbeB_UnexpectedEOFInDo(t *testing.T) {
	errMsg := parseMustFail(t, "main := do {")
	if errMsg == "" {
		t.Error("expected non-empty error")
	}
}

// TestProbeB_MissingArrowInCaseAlt checks missing -> in case alternative.
func TestProbeB_MissingArrowInCaseAlt(t *testing.T) {
	errMsg := parseMustFail(t, "main := case x { True 1 }")
	if errMsg == "" {
		t.Error("expected error for missing ->")
	}
}

// TestProbeB_ExtraCommaInTuple checks trailing comma in tuple.
func TestProbeB_ExtraCommaInTuple(t *testing.T) {
	// (1, 2,) — trailing comma. The parser will try to parse another expr after the comma.
	_, es := parse("main := (1, 2,)")
	// Should either parse or produce a meaningful error, but not panic.
	_ = es
}

// TestProbeB_MissingColonInRecord checks missing : in record literal.
// NOTE: { x 1 } is ambiguous. The parser's block disambiguation sees
// "lower" followed by neither ":" nor "|", so falls through to the block
// expression path. In block mode, "x" is parsed as an expression, then
// "1" is consumed as function application, then "}" closes the block.
// This is actually expected behavior (block expression), not a bug.
func TestProbeB_MissingColonInRecord(t *testing.T) {
	prog, es := parse("main := { x 1 }")
	if es.HasErrors() {
		t.Logf("parser rejected { x 1 }: %s", es.Format())
		return
	}
	d := prog.Decls[0].(*DeclValueDef)
	// It should parse as a block expression with body "x 1" (App(x, 1)).
	if blk, ok := d.Expr.(*ExprBlock); ok {
		t.Logf("parsed as block expression with %d binds", len(blk.Binds))
	} else {
		t.Logf("parsed { x 1 } as %T", d.Expr)
	}
}

// TestProbeB_DoubleColonColonInDecl checks malformed declaration: f :: :: Int.
func TestProbeB_DoubleColonColonInDecl(t *testing.T) {
	_, es := parse("f :: :: Int")
	// Should produce error but not panic.
	if !es.HasErrors() {
		t.Log("parser accepted 'f :: :: Int' — check if AST is sensible")
	}
}

// ----- 7. Crash Resistance -----

// TestProbeB_DeepNestedParens200 checks 200 nested parens don't crash.
func TestProbeB_DeepNestedParens200(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 200; i++ {
		b.WriteByte('(')
	}
	b.WriteString("x")
	for i := 0; i < 200; i++ {
		b.WriteByte(')')
	}
	src := span.NewSource("test", b.String())
	l := NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex error:", lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	_ = p.ParseProgram()
	// No panic = success. Errors are acceptable.
}

// TestProbeB_LongIdentifier100 checks a 100-char identifier parses correctly.
func TestProbeB_LongIdentifier100(t *testing.T) {
	name := strings.Repeat("x", 100)
	src := fmt.Sprintf("%s := 42", name)
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(prog.Decls))
	}
	d := prog.Decls[0].(*DeclValueDef)
	if d.Name != name {
		t.Errorf("expected 100-char name, got len=%d", len(d.Name))
	}
}

// TestProbeB_SequentialSemicolons50 checks that 50 sequential semicolons don't crash.
func TestProbeB_SequentialSemicolons50(t *testing.T) {
	src := "main := 1" + strings.Repeat(";", 50) + "\nf := 2"
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) < 2 {
		t.Errorf("expected at least 2 decls, got %d", len(prog.Decls))
	}
}

// TestProbeB_OnlySemicolons checks a source with nothing but semicolons.
func TestProbeB_OnlySemicolons(t *testing.T) {
	_, es := parse(strings.Repeat(";", 50))
	// Should not panic. May or may not error.
	_ = es
}

// TestProbeB_EmptySource checks empty string input.
func TestProbeB_EmptySource(t *testing.T) {
	prog := parseMustSucceed(t, "")
	if len(prog.Decls) != 0 || len(prog.Imports) != 0 {
		t.Errorf("expected empty program")
	}
}

// TestProbeB_DeepNestedTypes checks deeply nested type: Maybe (Maybe (Maybe ... a ...)).
func TestProbeB_DeepNestedTypes(t *testing.T) {
	ty := "a"
	for i := 0; i < 100; i++ {
		ty = fmt.Sprintf("(Maybe %s)", ty)
	}
	src := fmt.Sprintf("f :: %s", ty)
	_, es := parse(src)
	// May hit recursion limit, but should not panic.
	_ = es
}

// TestProbeB_DeepNestedLambda checks deeply nested lambda: \x. \x. \x. ... x.
func TestProbeB_DeepNestedLambda(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 100; i++ {
		b.WriteString(`\x. `)
	}
	b.WriteString("x")
	_, es := parse(b.String())
	// May hit recursion limit, but should not panic.
	_ = es
}

// TestProbeB_DeepNestedCase checks deeply nested case expressions.
func TestProbeB_DeepNestedCase(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 50; i++ {
		b.WriteString("case x { True -> ")
	}
	b.WriteString("1")
	for i := 0; i < 50; i++ {
		b.WriteString(" }")
	}
	_, es := parse(b.String())
	// Should not panic.
	_ = es
}

// TestProbeB_ManyDeclarations checks 100 declarations.
func TestProbeB_ManyDeclarations(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString(fmt.Sprintf("x%d := %d\n", i, i))
	}
	prog := parseMustSucceed(t, b.String())
	if len(prog.Decls) != 100 {
		t.Errorf("expected 100 decls, got %d", len(prog.Decls))
	}
}

// TestProbeB_ManyImports checks many imports.
func TestProbeB_ManyImports(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString(fmt.Sprintf("import M%d\n", i))
	}
	b.WriteString("main := 1")
	prog := parseMustSucceed(t, b.String())
	if len(prog.Imports) != 50 {
		t.Errorf("expected 50 imports, got %d", len(prog.Imports))
	}
}

// ----- 8. Boundary and Corner Cases -----

// TestProbeB_NewlineAsSeparator checks that newline works as separator.
func TestProbeB_NewlineAsSeparator(t *testing.T) {
	src := "x := 1\ny := 2\nz := 3"
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) != 3 {
		t.Errorf("expected 3 decls, got %d", len(prog.Decls))
	}
}

// TestProbeB_SemicolonAsSeparator checks that semicolon works as separator.
func TestProbeB_SemicolonAsSeparator(t *testing.T) {
	src := "x := 1; y := 2; z := 3"
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) != 3 {
		t.Errorf("expected 3 decls, got %d", len(prog.Decls))
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

// TestProbeB_GADTDecl checks GADT-style data declaration.
func TestProbeB_GADTDecl(t *testing.T) {
	src := `data Expr a := {
  Lit :: Int -> Expr Int;
  Add :: Expr Int -> Expr Int -> Expr Int
}`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclData)
	if d.Name != "Expr" {
		t.Errorf("expected 'Expr', got %q", d.Name)
	}
	if len(d.GADTCons) != 2 {
		t.Errorf("expected 2 GADT cons, got %d", len(d.GADTCons))
	}
}

// TestProbeB_ForallType checks forall type: \a. a -> a.
func TestProbeB_ForallType(t *testing.T) {
	prog := parseMustSucceed(t, `id :: \a. a -> a`)
	d := prog.Decls[0].(*DeclTypeAnn)
	fa, ok := d.Type.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall, got %T", d.Type)
	}
	if len(fa.Binders) != 1 || fa.Binders[0].Name != "a" {
		t.Errorf("expected binder 'a'")
	}
}

// TestProbeB_ForallKindedBinder checks kinded forall: \(a: Type). a -> a.
func TestProbeB_ForallKindedBinder(t *testing.T) {
	prog := parseMustSucceed(t, `id :: \(a: Type). a -> a`)
	d := prog.Decls[0].(*DeclTypeAnn)
	fa, ok := d.Type.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall, got %T", d.Type)
	}
	if len(fa.Binders) != 1 || fa.Binders[0].Name != "a" {
		t.Errorf("expected binder 'a'")
	}
	if fa.Binders[0].Kind == nil {
		t.Error("expected kinded binder, got nil kind")
	}
}

// TestProbeB_QualifiedConstraint checks qualified type: Eq a => a -> a -> Bool.
func TestProbeB_QualifiedConstraint(t *testing.T) {
	prog := parseMustSucceed(t, "eq :: Eq a => a -> a -> Bool")
	d := prog.Decls[0].(*DeclTypeAnn)
	qual, ok := d.Type.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual, got %T", d.Type)
	}
	_ = qual
}

// TestProbeB_RowTypeWithTail checks { x: Int, y: Bool | r }.
func TestProbeB_RowTypeWithTail(t *testing.T) {
	prog := parseMustSucceed(t, "f :: { x: Int, y: Bool | r } -> Int")
	d := prog.Decls[0].(*DeclTypeAnn)
	arr, ok := d.Type.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow, got %T", d.Type)
	}
	row, ok := arr.From.(*TyExprRow)
	if !ok {
		t.Fatalf("expected TyExprRow, got %T", arr.From)
	}
	if len(row.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(row.Fields))
	}
	if row.Tail == nil {
		t.Error("expected row tail")
	}
}

// TestProbeB_EmptyRowType checks {}.
func TestProbeB_EmptyRowType(t *testing.T) {
	prog := parseMustSucceed(t, "f :: {} -> Int")
	d := prog.Decls[0].(*DeclTypeAnn)
	arr, ok := d.Type.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow, got %T", d.Type)
	}
	row, ok := arr.From.(*TyExprRow)
	if !ok {
		t.Fatalf("expected TyExprRow, got %T", arr.From)
	}
	if len(row.Fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(row.Fields))
	}
}

// TestProbeB_ClassWithSuperclass checks class with superclass: class Eq a => Ord a { ... }.
func TestProbeB_ClassWithSuperclass(t *testing.T) {
	src := `class Eq a => Ord a {
  compare :: a -> a -> Int
}`
	prog := parseMustSucceed(t, src)
	cl := prog.Decls[0].(*DeclClass)
	if cl.Name != "Ord" {
		t.Errorf("expected class 'Ord', got %q", cl.Name)
	}
	if len(cl.Supers) != 1 {
		t.Errorf("expected 1 superclass, got %d", len(cl.Supers))
	}
	if len(cl.Methods) != 1 {
		t.Errorf("expected 1 method, got %d", len(cl.Methods))
	}
}

// TestProbeB_InstanceWithContext checks instance with context.
func TestProbeB_InstanceWithContext(t *testing.T) {
	src := `data Maybe a := Nothing | Just a
class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq (Maybe a) {
  eq := \x y. True
}`
	prog := parseMustSucceed(t, src)
	// Find instance decl
	for _, d := range prog.Decls {
		if inst, ok := d.(*DeclInstance); ok {
			if inst.ClassName != "Eq" {
				t.Errorf("expected class 'Eq', got %q", inst.ClassName)
			}
			if len(inst.Context) != 1 {
				t.Errorf("expected 1 context constraint, got %d", len(inst.Context))
			}
			return
		}
	}
	t.Error("instance declaration not found")
}

// TestProbeB_DotQualConAtomInPattern checks qualified constructor as atom
// in pattern position (0-arg qualified con nested under another con).
func TestProbeB_DotQualConAtomInPattern(t *testing.T) {
	src := `main := case x { Pair M.Nothing M.Nothing -> 1 }`
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

// TestProbeB_FixityDotOperator checks infixr 9 . — fixity for dot.
func TestProbeB_FixityDotOperator(t *testing.T) {
	src := `infixr 9 .
main := f . g . h`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[1].(*DeclValueDef)
	// Right-assoc: f . (g . h)
	inf := d.Expr.(*ExprInfix)
	if inf.Op != "." {
		t.Errorf("expected outer op '.', got %q", inf.Op)
	}
	inner, ok := inf.Right.(*ExprInfix)
	if !ok {
		t.Fatalf("expected inner ExprInfix for right-assoc dot, got %T", inf.Right)
	}
	if inner.Op != "." {
		t.Errorf("expected inner op '.', got %q", inner.Op)
	}
}

// TestProbeB_MixedNewlineAndSemicolon checks mixed separators.
func TestProbeB_MixedNewlineAndSemicolon(t *testing.T) {
	src := "x := 1\ny := 2; z := 3\nw := 4"
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) != 4 {
		t.Errorf("expected 4 decls, got %d", len(prog.Decls))
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

// TestProbeB_NullByte checks that null byte in source doesn't cause panic.
func TestProbeB_NullByte(t *testing.T) {
	_, es := parse("main := \x00")
	// Should not panic; error is acceptable.
	_ = es
}

// TestProbeB_OnlyImportsNoDecls checks that only imports with no decls is valid.
func TestProbeB_OnlyImportsNoDecls(t *testing.T) {
	prog := parseMustSucceed(t, "import M")
	if len(prog.Imports) != 1 {
		t.Errorf("expected 1 import, got %d", len(prog.Imports))
	}
	if len(prog.Decls) != 0 {
		t.Errorf("expected 0 decls, got %d", len(prog.Decls))
	}
}

// TestProbeB_MultipleImportForms checks all three import forms together.
func TestProbeB_MultipleImportForms(t *testing.T) {
	src := `import A
import B (x, Y(..))
import C as D
main := 1`
	prog := parseMustSucceed(t, src)
	if len(prog.Imports) != 3 {
		t.Fatalf("expected 3 imports, got %d", len(prog.Imports))
	}
	// Open
	if prog.Imports[0].Names != nil || prog.Imports[0].Alias != "" {
		t.Errorf("import A should be open")
	}
	// Selective
	if len(prog.Imports[1].Names) != 2 {
		t.Errorf("import B should have 2 names")
	}
	// Qualified
	if prog.Imports[2].Alias != "D" {
		t.Errorf("import C should have alias D")
	}
}

// TestProbeB_TupleTypeDesugaring checks (Int, Bool) type desugars to Record.
func TestProbeB_TupleTypeDesugaring(t *testing.T) {
	prog := parseMustSucceed(t, "f :: (Int, Bool) -> Int")
	d := prog.Decls[0].(*DeclTypeAnn)
	arr := d.Type.(*TyExprArrow)
	app, ok := arr.From.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp for tuple type, got %T", arr.From)
	}
	con, ok := app.Fun.(*TyExprCon)
	if !ok || con.Name != "Record" {
		t.Fatalf("expected Record con, got %T %v", app.Fun, app.Fun)
	}
}

// TestProbeB_UnitTypeDesugaring checks () type desugars to Record {}.
func TestProbeB_UnitTypeDesugaring(t *testing.T) {
	prog := parseMustSucceed(t, "f :: () -> Int")
	d := prog.Decls[0].(*DeclTypeAnn)
	arr := d.Type.(*TyExprArrow)
	app, ok := arr.From.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp for unit type, got %T", arr.From)
	}
	con, ok := app.Fun.(*TyExprCon)
	if !ok || con.Name != "Record" {
		t.Fatalf("expected Record con, got %T", app.Fun)
	}
}

// TestProbeB_DoubleLiteral checks 3.14 as double literal.
func TestProbeB_DoubleLiteral(t *testing.T) {
	prog := parseMustSucceed(t, "main := 3.14")
	d := prog.Decls[0].(*DeclValueDef)
	dl, ok := d.Expr.(*ExprDoubleLit)
	if !ok {
		t.Fatalf("expected ExprDoubleLit, got %T", d.Expr)
	}
	if dl.Value != "3.14" {
		t.Errorf("expected '3.14', got %q", dl.Value)
	}
}

// TestProbeB_ScientificNotation checks 1e10 as double literal.
func TestProbeB_ScientificNotation(t *testing.T) {
	prog := parseMustSucceed(t, "main := 1e10")
	d := prog.Decls[0].(*DeclValueDef)
	dl, ok := d.Expr.(*ExprDoubleLit)
	if !ok {
		t.Fatalf("expected ExprDoubleLit for 1e10, got %T", d.Expr)
	}
	if dl.Value != "1e10" {
		t.Errorf("expected '1e10', got %q", dl.Value)
	}
}

// TestProbeB_UnderscoreInNumber checks 100_000 as int literal.
func TestProbeB_UnderscoreInNumber(t *testing.T) {
	prog := parseMustSucceed(t, "main := 100_000")
	d := prog.Decls[0].(*DeclValueDef)
	il, ok := d.Expr.(*ExprIntLit)
	if !ok {
		t.Fatalf("expected ExprIntLit, got %T", d.Expr)
	}
	if il.Value != "100_000" {
		t.Errorf("expected '100_000', got %q", il.Value)
	}
}

// TestProbeB_StepLimitOnRepeatedTokens verifies the parser step limit
// prevents infinite loops on adversarial token streams.
func TestProbeB_StepLimitOnRepeatedTokens(t *testing.T) {
	// A long sequence of operators with no operands to parse — parser should hit step limit.
	var b strings.Builder
	b.WriteString("main := x")
	for i := 0; i < 500; i++ {
		b.WriteString(" + x")
	}
	_, es := parse(b.String())
	// Should succeed or produce step limit error, but not hang.
	_ = es
}

// TestProbeB_DotAfterQualConInExpr checks that Upper.Upper followed by space+lower
// does NOT extend the qualification to three parts.
func TestProbeB_DotAfterQualConInExpr(t *testing.T) {
	// "M.Just x" should be App(QualCon(M,Just), Var(x))
	prog := parseMustSucceed(t, "main := M.Just x")
	d := prog.Decls[0].(*DeclValueDef)
	app, ok := d.Expr.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp, got %T", d.Expr)
	}
	qc, ok := app.Fun.(*ExprQualCon)
	if !ok {
		t.Fatalf("expected ExprQualCon, got %T", app.Fun)
	}
	if qc.Qualifier != "M" || qc.Name != "Just" {
		t.Errorf("expected M.Just, got %s.%s", qc.Qualifier, qc.Name)
	}
	v, ok := app.Arg.(*ExprVar)
	if !ok || v.Name != "x" {
		t.Errorf("expected Var x, got %T", app.Arg)
	}
}

// TestProbeB_NestedBlockComment checks nested block comments: {- outer {- inner -} still comment -}.
func TestProbeB_NestedBlockComment(t *testing.T) {
	src := "main := {- outer {- inner -} still comment -} 42"
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	il, ok := d.Expr.(*ExprIntLit)
	if !ok {
		t.Fatalf("expected ExprIntLit, got %T", d.Expr)
	}
	if il.Value != "42" {
		t.Errorf("expected 42, got %q", il.Value)
	}
}

// TestProbeB_LineCommentAtEOF checks line comment at end of file with no trailing newline.
func TestProbeB_LineCommentAtEOF(t *testing.T) {
	src := "main := 42 -- comment at EOF"
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) != 1 {
		t.Errorf("expected 1 decl, got %d", len(prog.Decls))
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

// TestProbeB_ConstraintTupleDesugaring checks (Eq a, Ord a) => T desugars correctly.
func TestProbeB_ConstraintTupleDesugaring(t *testing.T) {
	src := "f :: (Eq a, Ord a) => a -> a"
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclTypeAnn)
	// Should desugar to Eq a => Ord a => a -> a
	qual1, ok := d.Type.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual for outer constraint, got %T", d.Type)
	}
	qual2, ok := qual1.Body.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual for inner constraint, got %T", qual1.Body)
	}
	_, ok = qual2.Body.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow as body, got %T", qual2.Body)
	}
}

// TestProbeB_InstanceTupleConstraint checks instance (Eq a, Show a) => MyClass a.
func TestProbeB_InstanceTupleConstraint(t *testing.T) {
	src := `class Eq a { eq :: a -> a }
class Show a { show :: a -> a }
class MyClass a { m :: a -> a }
instance (Eq a, Show a) => MyClass a {
  m := \x. x
}`
	prog := parseMustSucceed(t, src)
	for _, d := range prog.Decls {
		if inst, ok := d.(*DeclInstance); ok {
			if inst.ClassName != "MyClass" {
				continue
			}
			if len(inst.Context) != 2 {
				t.Errorf("expected 2 context constraints, got %d", len(inst.Context))
			}
			return
		}
	}
	t.Error("MyClass instance not found")
}

// TestProbeB_DataWithKindedParam checks data F (a: Type) := MkF a.
func TestProbeB_DataWithKindedParam(t *testing.T) {
	src := "data F (a: Type) := MkF a"
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclData)
	if len(d.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(d.Params))
	}
	if d.Params[0].Kind == nil {
		t.Error("expected kinded parameter, got nil kind")
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
