package syntax

import (
	"testing"

	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
)

func lex(input string) []Token {
	src := span.NewSource("test", input)
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	return tokens
}

func parse(input string) (*AstProgram, *errs.Errors) {
	src := span.NewSource("test", input)
	l := NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return nil, lexErrs
	}
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	prog := p.ParseProgram()
	return prog, es
}

// --- Lexer tests ---

func TestLexKeywords(t *testing.T) {
	tokens := lex("case of do data type forall infixl infixr infixn")
	expected := []TokenKind{TokCase, TokOf, TokDo, TokData, TokType, TokForall, TokInfixl, TokInfixr, TokInfixn, TokEOF}
	for i, want := range expected {
		if tokens[i].Kind != want {
			t.Errorf("token[%d]: got %v, want %v", i, tokens[i].Kind, want)
		}
	}
}

func TestLexPunctuation(t *testing.T) {
	tokens := lex("-> <- :: := : . \\ _ = | @ ( ) { } , ;")
	expected := []TokenKind{TokArrow, TokLArrow, TokColonColon, TokColonEq, TokColon, TokDot, TokBackslash, TokUnderscore, TokEq, TokPipe, TokAt, TokLParen, TokRParen, TokLBrace, TokRBrace, TokComma, TokSemicolon, TokEOF}
	for i, want := range expected {
		if i >= len(tokens) {
			t.Fatalf("not enough tokens, expected %d", len(expected))
		}
		if tokens[i].Kind != want {
			t.Errorf("token[%d]: got %v (%q), want %v", i, tokens[i].Kind, tokens[i].Text, want)
		}
	}
}

func TestLexIdentifiers(t *testing.T) {
	tokens := lex("foo Bar x' MyType")
	if tokens[0].Kind != TokLower || tokens[0].Text != "foo" {
		t.Error("expected lower 'foo'")
	}
	if tokens[1].Kind != TokUpper || tokens[1].Text != "Bar" {
		t.Error("expected upper 'Bar'")
	}
	if tokens[2].Kind != TokLower || tokens[2].Text != "x'" {
		t.Error("expected lower \"x'\"")
	}
	if tokens[3].Kind != TokUpper || tokens[3].Text != "MyType" {
		t.Error("expected upper 'MyType'")
	}
}

func TestLexOperators(t *testing.T) {
	tokens := lex("+ == >>= !!")
	for _, tok := range tokens[:4] {
		if tok.Kind != TokOp {
			t.Errorf("expected TokOp for %q, got %v", tok.Text, tok.Kind)
		}
	}
}

func TestLexLineComment(t *testing.T) {
	tokens := lex("x -- this is a comment\ny")
	if len(tokens) != 3 { // x, y, EOF
		t.Errorf("expected 3 tokens, got %d", len(tokens))
	}
}

func TestLexBlockComment(t *testing.T) {
	tokens := lex("x {- nested {- comment -} here -} y")
	if len(tokens) != 3 { // x, y, EOF
		t.Errorf("expected 3 tokens, got %d", len(tokens))
	}
}

func TestLexIntLit(t *testing.T) {
	tokens := lex("42")
	if tokens[0].Kind != TokIntLit || tokens[0].Text != "42" {
		t.Errorf("expected int 42, got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

// --- Parser tests ---

func TestParseDataDecl(t *testing.T) {
	prog, es := parse("data Bool = True | False")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(prog.Decls))
	}
	d, ok := prog.Decls[0].(*DeclData)
	if !ok {
		t.Fatal("expected DeclData")
	}
	if d.Name != "Bool" || len(d.Cons) != 2 {
		t.Errorf("expected Bool with 2 cons, got %s with %d", d.Name, len(d.Cons))
	}
}

func TestParseTypeAlias(t *testing.T) {
	prog, es := parse("type Effect r a = Computation r r a")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d, ok := prog.Decls[0].(*DeclTypeAlias)
	if !ok {
		t.Fatal("expected DeclTypeAlias")
	}
	if d.Name != "Effect" || len(d.Params) != 2 {
		t.Errorf("expected Effect with 2 params")
	}
}

func TestParseValueDef(t *testing.T) {
	prog, es := parse("id := \\x -> x")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d, ok := prog.Decls[0].(*DeclValueDef)
	if !ok {
		t.Fatal("expected DeclValueDef")
	}
	if d.Name != "id" {
		t.Errorf("expected 'id'")
	}
	_, ok = d.Expr.(*ExprLam)
	if !ok {
		t.Error("expected lambda expression")
	}
}

func TestParseTypeAnnotation(t *testing.T) {
	prog, es := parse("id :: forall a. a -> a")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d, ok := prog.Decls[0].(*DeclTypeAnn)
	if !ok {
		t.Fatal("expected DeclTypeAnn")
	}
	if d.Name != "id" {
		t.Errorf("expected 'id'")
	}
}

func TestParseFunctionApplication(t *testing.T) {
	src := span.NewSource("test", "f x y")
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	expr := p.ParseExpr()
	// Should be App(App(f, x), y)
	app, ok := expr.(*ExprApp)
	if !ok {
		t.Fatal("expected ExprApp")
	}
	inner, ok := app.Fun.(*ExprApp)
	if !ok {
		t.Fatal("expected nested ExprApp")
	}
	if v, ok := inner.Fun.(*ExprVar); !ok || v.Name != "f" {
		t.Error("expected f")
	}
}

func TestParseCaseExpr(t *testing.T) {
	prog, es := parse("main := case x of { True -> a; False -> b }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclValueDef)
	c, ok := d.Expr.(*ExprCase)
	if !ok {
		t.Fatal("expected ExprCase")
	}
	if len(c.Alts) != 2 {
		t.Errorf("expected 2 alts, got %d", len(c.Alts))
	}
}

func TestParseDoBlock(t *testing.T) {
	prog, es := parse("main := do { x <- comp; pure x }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclValueDef)
	doExpr, ok := d.Expr.(*ExprDo)
	if !ok {
		t.Fatal("expected ExprDo")
	}
	if len(doExpr.Stmts) != 2 {
		t.Errorf("expected 2 stmts, got %d", len(doExpr.Stmts))
	}
	if _, ok := doExpr.Stmts[0].(*StmtBind); !ok {
		t.Error("expected StmtBind")
	}
}

func TestParseRowType(t *testing.T) {
	prog, es := parse("f :: { db : DB, log : Log | r } -> Int")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclTypeAnn)
	arrow, ok := d.Type.(*TyExprArrow)
	if !ok {
		t.Fatal("expected arrow type")
	}
	row, ok := arrow.From.(*TyExprRow)
	if !ok {
		t.Fatal("expected row type")
	}
	if len(row.Fields) != 2 {
		t.Errorf("expected 2 row fields, got %d", len(row.Fields))
	}
	if row.Tail == nil || row.Tail.Name != "r" {
		t.Error("expected open row with tail 'r'")
	}
}

func TestParseAssumption(t *testing.T) {
	prog, es := parse("dbOpen := assumption")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclValueDef)
	v, ok := d.Expr.(*ExprVar)
	if !ok || v.Name != "assumption" {
		t.Error("expected ExprVar 'assumption'")
	}
}

func TestParseFixity(t *testing.T) {
	prog, es := parse("infixl 6 +\nmain := a + b")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	var fixDecl *DeclFixity
	for _, d := range prog.Decls {
		if f, ok := d.(*DeclFixity); ok {
			fixDecl = f
		}
	}
	if fixDecl == nil || fixDecl.Op != "+" || fixDecl.Prec != 6 {
		t.Error("expected fixity decl for + with prec 6")
	}
}

func TestParseForallKindedBinder(t *testing.T) {
	prog, es := parse("f :: forall (r : Row). Computation r r a")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d, ok := prog.Decls[0].(*DeclTypeAnn)
	if !ok {
		t.Fatal("expected DeclTypeAnn")
	}
	fa, ok := d.Type.(*TyExprForall)
	if !ok {
		t.Fatal("expected TyExprForall")
	}
	if len(fa.Binders) != 1 {
		t.Fatalf("expected 1 binder, got %d", len(fa.Binders))
	}
	b := fa.Binders[0]
	if b.Name != "r" {
		t.Errorf("expected binder name 'r', got %q", b.Name)
	}
	if b.Kind == nil {
		t.Fatal("expected kind annotation on binder")
	}
	if _, ok := b.Kind.(*KindExprRow); !ok {
		t.Errorf("expected KindExprRow, got %T", b.Kind)
	}
}

// --- Type class lexer tests ---

func TestLexClassInstanceKeywords(t *testing.T) {
	tokens := lex("class instance")
	if tokens[0].Kind != TokClass || tokens[0].Text != "class" {
		t.Errorf("expected TokClass, got %v %q", tokens[0].Kind, tokens[0].Text)
	}
	if tokens[1].Kind != TokInstance || tokens[1].Text != "instance" {
		t.Errorf("expected TokInstance, got %v %q", tokens[1].Kind, tokens[1].Text)
	}
}

func TestLexConstraintArrow(t *testing.T) {
	tokens := lex("Eq a => a -> Bool")
	// Eq(Upper), a(Lower), =>(FatArrow), a(Lower), ->(Arrow), Bool(Upper), EOF
	expected := []TokenKind{TokUpper, TokLower, TokFatArrow, TokLower, TokArrow, TokUpper, TokEOF}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, want := range expected {
		if tokens[i].Kind != want {
			t.Errorf("token[%d]: got %v (%q), want %v", i, tokens[i].Kind, tokens[i].Text, want)
		}
	}
	if tokens[2].Text != "=>" {
		t.Errorf("expected text '=>', got %q", tokens[2].Text)
	}
}

// --- Type class parser tests ---

func TestParseConstraintType(t *testing.T) {
	// f :: Eq a => a -> Bool
	prog, es := parse("f :: Eq a => a -> Bool")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d, ok := prog.Decls[0].(*DeclTypeAnn)
	if !ok {
		t.Fatal("expected DeclTypeAnn")
	}
	qual, ok := d.Type.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual, got %T", d.Type)
	}
	// Constraint: Eq a (TyExprApp(TyExprCon("Eq"), TyExprVar("a")))
	app, ok := qual.Constraint.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp for constraint, got %T", qual.Constraint)
	}
	if con, ok := app.Fun.(*TyExprCon); !ok || con.Name != "Eq" {
		t.Error("expected Eq constraint")
	}
	// Body: a -> Bool
	_, ok = qual.Body.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow for body, got %T", qual.Body)
	}
}

func TestParseForallConstraintType(t *testing.T) {
	prog, es := parse("f :: forall a. Eq a => a -> Bool")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclTypeAnn)
	fa, ok := d.Type.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall, got %T", d.Type)
	}
	qual, ok := fa.Body.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual inside forall, got %T", fa.Body)
	}
	_ = qual
}

func TestParseCurriedConstraints(t *testing.T) {
	prog, es := parse("f :: Eq a => Ord b => a -> b -> Bool")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclTypeAnn)
	// Eq a => (Ord b => a -> b -> Bool)
	q1, ok := d.Type.(*TyExprQual)
	if !ok {
		t.Fatalf("expected outer TyExprQual, got %T", d.Type)
	}
	q2, ok := q1.Body.(*TyExprQual)
	if !ok {
		t.Fatalf("expected inner TyExprQual, got %T", q1.Body)
	}
	_, ok = q2.Body.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow in innermost body, got %T", q2.Body)
	}
}

func TestParseClassDecl(t *testing.T) {
	prog, es := parse("class Eq a { eq :: a -> a -> Bool }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(prog.Decls))
	}
	cls, ok := prog.Decls[0].(*DeclClass)
	if !ok {
		t.Fatalf("expected DeclClass, got %T", prog.Decls[0])
	}
	if cls.Name != "Eq" {
		t.Errorf("expected class name Eq, got %s", cls.Name)
	}
	if len(cls.TyParams) != 1 || cls.TyParams[0].Name != "a" {
		t.Errorf("expected 1 type param 'a'")
	}
	if len(cls.Methods) != 1 || cls.Methods[0].Name != "eq" {
		t.Errorf("expected 1 method 'eq'")
	}
}

func TestParseClassWithSuperclass(t *testing.T) {
	prog, es := parse("class Eq a => Ord a { compare :: a -> a -> Bool }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	cls := prog.Decls[0].(*DeclClass)
	if cls.Name != "Ord" {
		t.Errorf("expected Ord, got %s", cls.Name)
	}
	if len(cls.Supers) != 1 {
		t.Fatalf("expected 1 superclass, got %d", len(cls.Supers))
	}
}

func TestParseClassMultiParam(t *testing.T) {
	prog, es := parse("class Coercible a b { coerce :: a -> b }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	cls := prog.Decls[0].(*DeclClass)
	if cls.Name != "Coercible" {
		t.Errorf("expected Coercible, got %s", cls.Name)
	}
	if len(cls.TyParams) != 2 {
		t.Errorf("expected 2 type params, got %d", len(cls.TyParams))
	}
}

func TestParseInstanceDecl(t *testing.T) {
	prog, es := parse("instance Eq Bool { eq := \\x y -> True }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	inst, ok := prog.Decls[0].(*DeclInstance)
	if !ok {
		t.Fatalf("expected DeclInstance, got %T", prog.Decls[0])
	}
	if inst.ClassName != "Eq" {
		t.Errorf("expected class Eq, got %s", inst.ClassName)
	}
	if len(inst.TypeArgs) != 1 {
		t.Fatalf("expected 1 type arg, got %d", len(inst.TypeArgs))
	}
	if len(inst.Methods) != 1 || inst.Methods[0].Name != "eq" {
		t.Errorf("expected 1 method 'eq'")
	}
}

func TestParseInstanceWithContext(t *testing.T) {
	prog, es := parse("instance Eq a => Eq (Maybe a) { eq := \\x y -> True }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	inst := prog.Decls[0].(*DeclInstance)
	if inst.ClassName != "Eq" {
		t.Errorf("expected class Eq, got %s", inst.ClassName)
	}
	if len(inst.Context) != 1 {
		t.Fatalf("expected 1 context constraint, got %d", len(inst.Context))
	}
}

func TestParseKindConstraint(t *testing.T) {
	prog, es := parse("f :: forall (c : Constraint). c")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclTypeAnn)
	fa := d.Type.(*TyExprForall)
	if fa.Binders[0].Kind == nil {
		t.Fatal("expected kind annotation")
	}
	_, ok := fa.Binders[0].Kind.(*KindExprConstraint)
	if !ok {
		t.Errorf("expected KindExprConstraint, got %T", fa.Binders[0].Kind)
	}
}

func TestParseForallMixedBinders(t *testing.T) {
	// Mixed bare and kinded binders
	prog, es := parse("f :: forall a (r : Row) (k : Type -> Type). r")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d, ok := prog.Decls[0].(*DeclTypeAnn)
	if !ok {
		t.Fatal("expected DeclTypeAnn")
	}
	fa, ok := d.Type.(*TyExprForall)
	if !ok {
		t.Fatal("expected TyExprForall")
	}
	if len(fa.Binders) != 3 {
		t.Fatalf("expected 3 binders, got %d", len(fa.Binders))
	}
	// First binder: bare 'a'
	if fa.Binders[0].Name != "a" || fa.Binders[0].Kind != nil {
		t.Errorf("expected bare binder 'a', got name=%q kind=%v", fa.Binders[0].Name, fa.Binders[0].Kind)
	}
	// Second binder: (r : Row)
	if fa.Binders[1].Name != "r" {
		t.Errorf("expected binder name 'r', got %q", fa.Binders[1].Name)
	}
	if _, ok := fa.Binders[1].Kind.(*KindExprRow); !ok {
		t.Errorf("expected KindExprRow, got %T", fa.Binders[1].Kind)
	}
	// Third binder: (k : Type -> Type)
	if fa.Binders[2].Name != "k" {
		t.Errorf("expected binder name 'k', got %q", fa.Binders[2].Name)
	}
	arrow, ok := fa.Binders[2].Kind.(*KindExprArrow)
	if !ok {
		t.Fatalf("expected KindExprArrow, got %T", fa.Binders[2].Kind)
	}
	if _, ok := arrow.From.(*KindExprType); !ok {
		t.Errorf("expected KindExprType as arrow.From, got %T", arrow.From)
	}
	if _, ok := arrow.To.(*KindExprType); !ok {
		t.Errorf("expected KindExprType as arrow.To, got %T", arrow.To)
	}
}

// --- GADT parser tests ---

func TestParseGADTDecl(t *testing.T) {
	prog, es := parse("data Expr a = { IntLit :: Int -> Expr Int; BoolLit :: Bool -> Expr Bool }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d, ok := prog.Decls[0].(*DeclData)
	if !ok {
		t.Fatal("expected DeclData")
	}
	if d.Name != "Expr" {
		t.Errorf("expected Expr, got %s", d.Name)
	}
	if len(d.Params) != 1 || d.Params[0].Name != "a" {
		t.Errorf("expected 1 param 'a'")
	}
	if len(d.Cons) != 0 {
		t.Errorf("expected 0 ADT cons, got %d", len(d.Cons))
	}
	if len(d.GADTCons) != 2 {
		t.Fatalf("expected 2 GADT cons, got %d", len(d.GADTCons))
	}
	if d.GADTCons[0].Name != "IntLit" {
		t.Errorf("expected IntLit, got %s", d.GADTCons[0].Name)
	}
	if d.GADTCons[1].Name != "BoolLit" {
		t.Errorf("expected BoolLit, got %s", d.GADTCons[1].Name)
	}
}

func TestParseGADTMixedArity(t *testing.T) {
	prog, es := parse("data T a = { Nil :: T Unit; Cons :: a -> T a -> T a }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclData)
	if len(d.GADTCons) != 2 {
		t.Fatalf("expected 2 GADT cons, got %d", len(d.GADTCons))
	}
	// Nil :: T Unit  (no arrows)
	if _, ok := d.GADTCons[0].Type.(*TyExprArrow); ok {
		t.Error("Nil should not have arrow type")
	}
	// Cons :: a -> T a -> T a  (has arrows)
	if _, ok := d.GADTCons[1].Type.(*TyExprArrow); !ok {
		t.Error("Cons should have arrow type")
	}
}

func TestParseGADTvsADT(t *testing.T) {
	// ADT form
	adtProg, adtEs := parse("data Bool = True | False")
	if adtEs.HasErrors() {
		t.Fatal(adtEs.Format())
	}
	adtD := adtProg.Decls[0].(*DeclData)
	if len(adtD.Cons) != 2 {
		t.Errorf("ADT: expected 2 cons, got %d", len(adtD.Cons))
	}
	if len(adtD.GADTCons) != 0 {
		t.Errorf("ADT: expected 0 GADT cons, got %d", len(adtD.GADTCons))
	}

	// GADT form
	gadtProg, gadtEs := parse("data Expr a = { Lit :: Int -> Expr Int }")
	if gadtEs.HasErrors() {
		t.Fatal(gadtEs.Format())
	}
	gadtD := gadtProg.Decls[0].(*DeclData)
	if len(gadtD.Cons) != 0 {
		t.Errorf("GADT: expected 0 ADT cons, got %d", len(gadtD.Cons))
	}
	if len(gadtD.GADTCons) != 1 {
		t.Errorf("GADT: expected 1 GADT con, got %d", len(gadtD.GADTCons))
	}
}

func TestGADTConReturnType(t *testing.T) {
	prog, es := parse("data Expr a = { IntLit :: Int -> Expr Int; Add :: Expr Int -> Expr Int -> Expr Int; IsZero :: Expr Int -> Expr Bool }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclData)
	if len(d.GADTCons) != 3 {
		t.Fatalf("expected 3 GADT cons, got %d", len(d.GADTCons))
	}
	// Each constructor has a type that ends with Expr <something>
	names := []string{"IntLit", "Add", "IsZero"}
	for i, c := range d.GADTCons {
		if c.Name != names[i] {
			t.Errorf("con[%d]: expected %s, got %s", i, names[i], c.Name)
		}
		if c.Type == nil {
			t.Errorf("con[%d]: type is nil", i)
		}
	}
}
