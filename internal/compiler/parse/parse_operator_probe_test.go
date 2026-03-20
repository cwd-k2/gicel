//go:build probe

// Operator probe tests: dot disambiguation, fixity, precedence, associativity, sections.
package parse

import (
	"strings"
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive // dot import for tightly-coupled subpackage
)

// ===== From probe_b =====

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
	prog, es := parse("main := A.B.C")
	if es.HasErrors() {
		t.Logf("double-qualified parsed with errors: %s", es.Format())
		return
	}
	d := prog.Decls[0].(*DeclValueDef)
	// The parser should produce ExprInfix { Left: QualCon(A,B), Op: ".", Right: Con(C) }
	// or some other valid decomposition.
	t.Logf("A.B.C parsed as %T", d.Expr)
}

// TestProbeB_DotQualVarInApp verifies that M.f x parses as App(QualVar(M,f), Var(x)).
func TestProbeB_DotQualVarInApp(t *testing.T) {
	prog := parseMustSucceed(t, "main := M.f x")
	d := prog.Decls[0].(*DeclValueDef)
	app, ok := d.Expr.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp, got %T", d.Expr)
	}
	qv, ok := app.Fun.(*ExprQualVar)
	if !ok {
		t.Fatalf("expected ExprQualVar as function, got %T", app.Fun)
	}
	if qv.Qualifier != "M" || qv.Name != "f" {
		t.Errorf("expected M.f, got %s.%s", qv.Qualifier, qv.Name)
	}
}

// TestProbeB_DotQualConInPattern verifies M.Just in pattern position.
func TestProbeB_DotQualConInPattern(t *testing.T) {
	prog := parseMustSucceed(t, "main := case x { M.Just y -> y }")
	d := prog.Decls[0].(*DeclValueDef)
	c := d.Expr.(*ExprCase)
	pqc, ok := c.Alts[0].Pattern.(*PatQualCon)
	if !ok {
		t.Fatalf("expected PatQualCon, got %T", c.Alts[0].Pattern)
	}
	if pqc.Qualifier != "M" || pqc.Con != "Just" {
		t.Errorf("expected M.Just, got %s.%s", pqc.Qualifier, pqc.Con)
	}
}

// TestProbeB_DotQualifiedType verifies M.T in type position.
func TestProbeB_DotQualifiedType(t *testing.T) {
	prog := parseMustSucceed(t, "f :: M.T -> Int")
	d := prog.Decls[0].(*DeclTypeAnn)
	arr, ok := d.Type.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow, got %T", d.Type)
	}
	qc, ok := arr.From.(*TyExprQualCon)
	if !ok {
		t.Fatalf("expected TyExprQualCon, got %T", arr.From)
	}
	if qc.Qualifier != "M" || qc.Name != "T" {
		t.Errorf("expected M.T, got %s.%s", qc.Qualifier, qc.Name)
	}
}

// TestProbeB_OperatorAsValue verifies (+) as operator value.
func TestProbeB_OperatorAsValue(t *testing.T) {
	prog := parseMustSucceed(t, "main := (+)")
	d := prog.Decls[0].(*DeclValueDef)
	v, ok := d.Expr.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar for (+), got %T", d.Expr)
	}
	if v.Name != "+" {
		t.Errorf("expected '+', got %q", v.Name)
	}
}

// TestProbeB_DotAsOperatorValue verifies (.) as operator value.
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

// TestProbeB_RightSection verifies (+ 1) as right section.
func TestProbeB_RightSection(t *testing.T) {
	prog := parseMustSucceed(t, "main := (+ 1)")
	d := prog.Decls[0].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection, got %T", d.Expr)
	}
	if !sec.IsRight {
		t.Error("expected right section, got left")
	}
	if sec.Op != "+" {
		t.Errorf("expected op '+', got %q", sec.Op)
	}
}

// TestProbeB_LeftSection verifies (1 +) as left section.
func TestProbeB_LeftSection(t *testing.T) {
	prog := parseMustSucceed(t, "main := (1 +)")
	d := prog.Decls[0].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection, got %T", d.Expr)
	}
	if sec.IsRight {
		t.Error("expected left section, got right")
	}
	if sec.Op != "+" {
		t.Errorf("expected op '+', got %q", sec.Op)
	}
}

// TestProbeB_DotRightSection verifies (. f) as right section for dot.
func TestProbeB_DotRightSection(t *testing.T) {
	// (. f) — right section. Dot is an operator.
	prog := parseMustSucceed(t, "main := (. f)")
	d := prog.Decls[0].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection for (. f), got %T", d.Expr)
	}
	if sec.Op != "." {
		t.Errorf("expected op '.', got %q", sec.Op)
	}
}

// TestProbeB_FixityDecl checks infixl 6 + parses as fixity declaration.
func TestProbeB_FixityDecl(t *testing.T) {
	prog := parseMustSucceed(t, "infixl 6 +")
	d, ok := prog.Decls[0].(*DeclFixity)
	if !ok {
		t.Fatalf("expected DeclFixity, got %T", prog.Decls[0])
	}
	if d.Assoc != AssocLeft || d.Prec != 6 || d.Op != "+" {
		t.Errorf("expected infixl 6 +, got %v %d %s", d.Assoc, d.Prec, d.Op)
	}
}

// TestProbeB_CustomOperatorChars checks a custom operator with unusual characters.
func TestProbeB_CustomOperatorChars(t *testing.T) {
	prog := parseMustSucceed(t, "infixl 5 <*>")
	d := prog.Decls[0].(*DeclFixity)
	if d.Op != "<*>" {
		t.Errorf("expected '<*>', got %q", d.Op)
	}
}

// TestProbeB_OperatorPrecedenceAssociativity checks operator precedence parsing.
func TestProbeB_OperatorPrecedenceAssociativity(t *testing.T) {
	src := `infixl 6 +
infixl 7 *
(+) := \a b. a
(*) := \a b. a
main := 1 + 2 * 3`
	prog := parseMustSucceed(t, src)
	// Find the main declaration.
	for _, d := range prog.Decls {
		if vd, ok := d.(*DeclValueDef); ok && vd.Name == "main" {
			inf, ok := vd.Expr.(*ExprInfix)
			if !ok {
				t.Fatalf("expected ExprInfix, got %T", vd.Expr)
			}
			// + is prec 6, * is prec 7. So: 1 + (2 * 3).
			if inf.Op != "+" {
				t.Errorf("expected outer op '+', got %q", inf.Op)
			}
			inner, ok := inf.Right.(*ExprInfix)
			if !ok {
				t.Fatalf("expected inner ExprInfix, got %T", inf.Right)
			}
			if inner.Op != "*" {
				t.Errorf("expected inner op '*', got %q", inner.Op)
			}
		}
	}
}

// TestProbeB_OperatorRightAssocChain checks right-associative operator chaining.
func TestProbeB_OperatorRightAssocChain(t *testing.T) {
	src := `infixr 5 ++
(++) := \a b. a
main := 1 ++ 2 ++ 3`
	prog := parseMustSucceed(t, src)
	for _, d := range prog.Decls {
		if vd, ok := d.(*DeclValueDef); ok && vd.Name == "main" {
			inf, ok := vd.Expr.(*ExprInfix)
			if !ok {
				t.Fatalf("expected ExprInfix, got %T", vd.Expr)
			}
			// Right-assoc: 1 ++ (2 ++ 3)
			if inf.Op != "++" {
				t.Errorf("expected outer op '++', got %q", inf.Op)
			}
			inner, ok := inf.Right.(*ExprInfix)
			if !ok {
				t.Fatalf("expected right child to be ExprInfix for right-assoc, got %T", inf.Right)
			}
			if inner.Op != "++" {
				t.Errorf("expected inner op '++', got %q", inner.Op)
			}
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

// ===== From probe_e =====

// TestProbeE_InfixNonAssocSameOpChain verifies that `a == b == c` with infixn
// either errors or silently left-groups.
func TestProbeE_InfixNonAssocSameOpChain(t *testing.T) {
	source := `infixn 4 ==
(==) := \a b. a
main := 1 == 2 == 3`
	_, es := parse(source)
	// Non-associative: chaining should ideally error.
	_ = es
}

// TestProbeE_InfixNonAssocDiffOpChain verifies that `a == b /= c` with two
// different infixn operators at same precedence.
func TestProbeE_InfixNonAssocDiffOpChain(t *testing.T) {
	source := `infixn 4 ==
infixn 4 /=
(==) := \a b. a
(/=) := \a b. a
main := 1 == 2 /= 3`
	_, es := parse(source)
	// Document behavior: should ideally error but may silently left-group.
	_ = es
}

// TestProbeE_InfixNonAssocThenHigherPrec verifies `a == b + c` where +
// has higher precedence than ==.
func TestProbeE_InfixNonAssocThenHigherPrec(t *testing.T) {
	source := `infixn 4 ==
infixl 6 +
(==) := \a b. a
(+) := \a b. a
main := 1 == 2 + 3`
	parseMustSucceed(t, source)
}

// TestProbeE_InfixNonAssocResetByLeftAssoc verifies `a + b == c + d` grouping.
func TestProbeE_InfixNonAssocResetByLeftAssoc(t *testing.T) {
	source := `infixn 4 ==
infixl 6 +
(==) := \a b. a
(+) := \a b. a
main := 1 + 2 == 3 + 4`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[len(prog.Decls)-1].(*DeclValueDef)
	inf, ok := d.Expr.(*ExprInfix)
	if !ok {
		t.Fatalf("expected ExprInfix, got %T", d.Expr)
	}
	if inf.Op != "==" {
		t.Errorf("expected outer op '==', got %q", inf.Op)
	}
}

// TestProbeE_InfixNonAssocWithRightAssocBetween verifies non-assoc with right-assoc.
func TestProbeE_InfixNonAssocWithRightAssocBetween(t *testing.T) {
	source := `infixn 4 ==
infixr 5 ++
(==) := \a b. a
(++) := \a b. a
main := 1 == 2 ++ 3`
	parseMustSucceed(t, source)
}

// TestProbeE_InfixNonAssocDiffPrec verifies different precedences mix.
func TestProbeE_InfixNonAssocDiffPrec(t *testing.T) {
	source := `infixn 4 ==
infixn 5 /=
(==) := \a b. a
(/=) := \a b. a
main := 1 == 2 /= 3`
	parseMustSucceed(t, source)
}

// TestProbeE_InfixMixedAssocSamePrec verifies mixed associativity at same precedence.
func TestProbeE_InfixMixedAssocSamePrec(t *testing.T) {
	source := `infixl 6 +
infixr 6 ++
(+) := \a b. a
(++) := \a b. a
main := 1 + 2 ++ 3`
	// Mixed associativity at same precedence: should error or pick one.
	_, es := parse(source)
	// Document behavior.
	if es.HasErrors() {
		t.Logf("mixed assoc same prec error (expected): %s", es.Format())
	} else {
		t.Log("mixed assoc same prec accepted (no error)")
	}
}

// TestProbeE_LeftSectionInParen verifies (expr op) as left section.
func TestProbeE_LeftSectionInParen(t *testing.T) {
	source := `infixl 6 +
(+) := \x y. x
main := (1 +)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[len(prog.Decls)-1].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection, got %T", d.Expr)
	}
	if sec.IsRight {
		t.Error("expected left section")
	}
	if sec.Op != "+" {
		t.Errorf("expected op '+', got %q", sec.Op)
	}
}

// TestProbeE_RightSectionInParen verifies (op expr) as right section.
func TestProbeE_RightSectionInParen(t *testing.T) {
	source := `infixl 6 +
(+) := \x y. x
main := (+ 1)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[len(prog.Decls)-1].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection, got %T", d.Expr)
	}
	if !sec.IsRight {
		t.Error("expected right section")
	}
}

// TestProbeE_SectionWithDot verifies (. f) and (f .) section detection.
func TestProbeE_SectionWithDot(t *testing.T) {
	source := `main := (. f)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection, got %T", d.Expr)
	}
	if sec.Op != "." {
		t.Errorf("expected op '.', got %q", sec.Op)
	}
}

// TestProbeE_OperatorAsValueInParen verifies (+) produces ExprVar.
func TestProbeE_OperatorAsValueInParen(t *testing.T) {
	source := `infixl 6 +
(+) := \x y. x
main := (+)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[len(prog.Decls)-1].(*DeclValueDef)
	// (+) should be ExprVar with name "+".
	v, ok := d.Expr.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar for (+), got %T", d.Expr)
	}
	if v.Name != "+" {
		t.Errorf("expected '+', got %q", v.Name)
	}
}

// TestProbeE_NestedSections verifies nested section expressions.
func TestProbeE_NestedSections(t *testing.T) {
	// `f (+ 1)` — apply f to right section (+ 1).
	source := `infixl 6 +
(+) := \x y. x
main := f (+ 1)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[len(prog.Decls)-1].(*DeclValueDef)
	app, ok := d.Expr.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp, got %T", d.Expr)
	}
	v, ok := app.Fun.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar as function, got %T", app.Fun)
	}
	if v.Name != "f" {
		t.Errorf("expected 'f', got %q", v.Name)
	}
	sec, ok := app.Arg.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection as arg, got %T", app.Arg)
	}
	if !sec.IsRight {
		t.Error("expected right section")
	}
}

// TestProbeE_EmptyParens verifies () is unit (empty record).
func TestProbeE_EmptyParens(t *testing.T) {
	source := `main := ()`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	rec, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for (), got %T", d.Expr)
	}
	if len(rec.Fields) != 0 {
		t.Errorf("expected 0 fields for (), got %d", len(rec.Fields))
	}
}

// TestProbeE_StepLimitLongInfixChain verifies the parser handles extremely long
// infix chains within the step limit.
func TestProbeE_StepLimitLongInfixChain(t *testing.T) {
	var b strings.Builder
	b.WriteString("infixl 6 +\n(+) := \\x y. x\nmain := 1")
	for i := 0; i < 500; i++ {
		b.WriteString(" + 1")
	}
	_, es := parse(b.String())
	// May hit step limit — that's acceptable.
	_ = es
}

// TestProbeE_StepLimitLongApplicationChain verifies long application chains.
func TestProbeE_StepLimitLongApplicationChain(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := f")
	for i := 0; i < 2000; i++ {
		b.WriteString(" x")
	}
	_, es := parse(b.String())
	_ = es
}

// TestProbeE_FixityCollectionPrePass verifies that fixity declarations anywhere in the
// source are collected in the pre-pass before expression parsing.
func TestProbeE_FixityCollectionPrePass(t *testing.T) {
	source := `(+) := \x y. x
main := 1 + 2
infixl 6 +`
	prog := parseMustSucceed(t, source)
	// The fixity declaration comes AFTER the usage, but the pre-pass should
	// have collected it. Verify by checking the parse tree.
	d := prog.Decls[len(prog.Decls)-2].(*DeclValueDef)
	inf, ok := d.Expr.(*ExprInfix)
	if !ok {
		t.Fatalf("expected ExprInfix, got %T", d.Expr)
	}
	if inf.Op != "+" {
		t.Errorf("expected op '+', got %q", inf.Op)
	}
}

// TestProbeE_FixityOutOfRange verifies fixity with precedence > 9 or < 0.
func TestProbeE_FixityOutOfRange(t *testing.T) {
	// The parser just reads an int — it doesn't validate range.
	source := `infixl 100 ++
(++) := \x y. x
main := 1 ++ 2`
	prog, es := parse(source)
	if es.HasErrors() {
		t.Logf("errors (might be expected): %s", es.Format())
	}
	// Verify the fixity was stored even with out-of-range precedence.
	if prog != nil {
		found := false
		for _, d := range prog.Decls {
			if fix, ok := d.(*DeclFixity); ok && fix.Op == "++" {
				found = true
				if fix.Prec != 100 {
					t.Errorf("expected prec 100, got %d", fix.Prec)
				}
			}
		}
		if !found {
			t.Error("fixity decl for ++ not found")
		}
	}
}

// TestProbeE_SectionBacktrackOnComplexExpr verifies section detection backtracking
// when the expression after the operator is complex and doesn't end with ).
// The parser's right-section detection tries (op expr), and backtracks if no ).
func TestProbeE_SectionBacktrackOnComplexExpr(t *testing.T) {
	// (+ 1 2) — this could be ambiguous: is it a right section (+ (1 2))
	// or something else? The parser tries right section: op=+, expr=1 2.
	// After parsing "1 2" as App(1, 2), it checks for ). If found, it's a section.
	source := `infixl 6 +
(+) := \x y. x
main := (+ 1 2)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[len(prog.Decls)-1].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection, got %T", d.Expr)
	}
	if !sec.IsRight {
		t.Error("expected right section")
	}
	// The arg should be App(1, 2).
	_, ok = sec.Arg.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp as section arg, got %T", sec.Arg)
	}
}

// TestProbeE_FixityInsideClassBody verifies fixity declared inside a class body...
// Actually, fixity declarations only live at the top level. But what if someone
// writes one inside a class body?
func TestProbeE_FixityInsideClassBody(t *testing.T) {
	source := `class C a {
  infixl 6 +
}`
	_, es := parse(source)
	// The class body parser expects lower (method name), type, or data.
	// infixl is a keyword, so it should error.
	if !es.HasErrors() {
		t.Error("expected error for fixity inside class body")
	}
}
