package parse

import (
	"testing"

	. "github.com/cwd-k2/gicel/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
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
	tokens := lex("case do data type forall infixl infixr infixn")
	expected := []TokenKind{TokCase, TokDo, TokData, TokType, TokForall, TokInfixl, TokInfixr, TokInfixn, TokEOF}
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

func TestLexBangHash(t *testing.T) {
	tokens := lex("r!#x")
	// r, !#, x, EOF
	if len(tokens) != 4 {
		t.Fatalf("expected 4 tokens, got %d", len(tokens))
	}
	if tokens[0].Kind != TokLower || tokens[0].Text != "r" {
		t.Errorf("expected 'r', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
	if tokens[1].Kind != TokBangHash || tokens[1].Text != "!#" {
		t.Errorf("expected TokBangHash '!#', got %v %q", tokens[1].Kind, tokens[1].Text)
	}
	if tokens[2].Kind != TokLower || tokens[2].Text != "x" {
		t.Errorf("expected 'x', got %v %q", tokens[2].Kind, tokens[2].Text)
	}
}

func TestLexBangAloneStillOp(t *testing.T) {
	tokens := lex("! x")
	if tokens[0].Kind != TokOp || tokens[0].Text != "!" {
		t.Errorf("expected TokOp '!', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

func TestLexDoubleBangStillOp(t *testing.T) {
	tokens := lex("!!")
	if tokens[0].Kind != TokOp || tokens[0].Text != "!!" {
		t.Errorf("expected TokOp '!!', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

func TestLexBangHashInLongerOp(t *testing.T) {
	// !#= should be a single TokOp (not TokBangHash + TokEq)
	tokens := lex("!#=")
	if tokens[0].Kind != TokOp || tokens[0].Text != "!#=" {
		t.Errorf("expected TokOp '!#=', got %v %q", tokens[0].Kind, tokens[0].Text)
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

func TestLexStringLiteral(t *testing.T) {
	tokens := lex(`"hello"`)
	if tokens[0].Kind != TokStrLit || tokens[0].Text != "hello" {
		t.Errorf("expected string 'hello', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

func TestLexStringEscape(t *testing.T) {
	tokens := lex(`"a\nb\\c"`)
	want := "a\nb\\c"
	if tokens[0].Kind != TokStrLit || tokens[0].Text != want {
		t.Errorf("expected string %q, got %v %q", want, tokens[0].Kind, tokens[0].Text)
	}
}

func TestLexRuneLiteral(t *testing.T) {
	tokens := lex(`'a'`)
	if tokens[0].Kind != TokRuneLit || tokens[0].Text != "a" {
		t.Errorf("expected rune 'a', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

func TestLexRuneEscape(t *testing.T) {
	tokens := lex(`'\n'`)
	if tokens[0].Kind != TokRuneLit || tokens[0].Text != "\n" {
		t.Errorf("expected rune '\\n', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

func TestLexIntLitInExpr(t *testing.T) {
	tokens := lex("f 42 x")
	if tokens[0].Kind != TokLower || tokens[0].Text != "f" {
		t.Errorf("expected lower 'f', got %v", tokens[0])
	}
	if tokens[1].Kind != TokIntLit || tokens[1].Text != "42" {
		t.Errorf("expected int 42, got %v %q", tokens[1].Kind, tokens[1].Text)
	}
	if tokens[2].Kind != TokLower || tokens[2].Text != "x" {
		t.Errorf("expected lower 'x', got %v", tokens[2])
	}
}

// --- Parser tests ---

func TestParseIntLiteral(t *testing.T) {
	prog, es := parse("main := 42")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclValueDef)
	lit, ok := d.Expr.(*ExprIntLit)
	if !ok {
		t.Fatalf("expected ExprIntLit, got %T", d.Expr)
	}
	if lit.Value != "42" {
		t.Errorf("expected 42, got %s", lit.Value)
	}
}

func TestParseStringLiteral(t *testing.T) {
	prog, es := parse(`main := "hello"`)
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclValueDef)
	lit, ok := d.Expr.(*ExprStrLit)
	if !ok {
		t.Fatalf("expected ExprStrLit, got %T", d.Expr)
	}
	if lit.Value != "hello" {
		t.Errorf("expected hello, got %s", lit.Value)
	}
}

func TestParseRuneLiteral(t *testing.T) {
	prog, es := parse("main := 'a'")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclValueDef)
	lit, ok := d.Expr.(*ExprRuneLit)
	if !ok {
		t.Fatalf("expected ExprRuneLit, got %T", d.Expr)
	}
	if lit.Value != 'a' {
		t.Errorf("expected 'a', got %c", lit.Value)
	}
}

func TestParseLiteralInExpr(t *testing.T) {
	prog, es := parse(`main := f 42 "hello"`)
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclValueDef)
	// f 42 "hello" → App(App(f, 42), "hello")
	app, ok := d.Expr.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp, got %T", d.Expr)
	}
	_, ok = app.Arg.(*ExprStrLit)
	if !ok {
		t.Fatalf("expected ExprStrLit as outer arg, got %T", app.Arg)
	}
	inner, ok := app.Fun.(*ExprApp)
	if !ok {
		t.Fatalf("expected inner ExprApp, got %T", app.Fun)
	}
	_, ok = inner.Arg.(*ExprIntLit)
	if !ok {
		t.Fatalf("expected ExprIntLit as inner arg, got %T", inner.Arg)
	}
}

func TestParseOperatorTypeAnn(t *testing.T) {
	prog, es := parse("(+) :: Int -> Int -> Int")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(prog.Decls))
	}
	d, ok := prog.Decls[0].(*DeclTypeAnn)
	if !ok {
		t.Fatalf("expected DeclTypeAnn, got %T", prog.Decls[0])
	}
	if d.Name != "+" {
		t.Errorf("expected name '+', got %s", d.Name)
	}
}

func TestParseOperatorValueDef(t *testing.T) {
	prog, es := parse("(+) := add")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(prog.Decls))
	}
	d, ok := prog.Decls[0].(*DeclValueDef)
	if !ok {
		t.Fatalf("expected DeclValueDef, got %T", prog.Decls[0])
	}
	if d.Name != "+" {
		t.Errorf("expected name '+', got %s", d.Name)
	}
}

func TestParseOperatorInModule(t *testing.T) {
	src := `data Int = MkInt
add :: Int -> Int -> Int
add := \x -> \y -> x
infixl 6 +
(+) :: Int -> Int -> Int
(+) := add`
	prog, es := parse(src)
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	// data + add :: + add := + infixl + (+) :: + (+) :=
	foundOpAnn := false
	foundOpDef := false
	for _, d := range prog.Decls {
		switch d := d.(type) {
		case *DeclTypeAnn:
			if d.Name == "+" {
				foundOpAnn = true
			}
		case *DeclValueDef:
			if d.Name == "+" {
				foundOpDef = true
			}
		}
	}
	if !foundOpAnn {
		t.Error("expected operator type annotation for +")
	}
	if !foundOpDef {
		t.Error("expected operator value definition for +")
	}
}

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
	prog, es := parse("main := case x { True -> a; False -> b }")
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
	prog, es := parse("instance Eq Bool { eq := \\x -> \\y -> True }")
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
	prog, es := parse("instance Eq a => Eq (Maybe a) { eq := \\x -> \\y -> True }")
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

// --- Import parser tests ---

func TestLexImportKeyword(t *testing.T) {
	tokens := lex("import MyModule")
	if tokens[0].Kind != TokImport || tokens[0].Text != "import" {
		t.Errorf("expected TokImport, got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

func TestParseImportDecl(t *testing.T) {
	prog, es := parse("import MyModule\nmain := True")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	if prog.Imports[0].ModuleName != "MyModule" {
		t.Errorf("expected MyModule, got %s", prog.Imports[0].ModuleName)
	}
}

func TestParseMultipleImports(t *testing.T) {
	prog, es := parse("import Foo\nimport Bar\nimain := True")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(prog.Imports))
	}
	if prog.Imports[0].ModuleName != "Foo" || prog.Imports[1].ModuleName != "Bar" {
		t.Errorf("expected Foo and Bar, got %s and %s",
			prog.Imports[0].ModuleName, prog.Imports[1].ModuleName)
	}
}

func TestParseImportBeforeDecl(t *testing.T) {
	prog, es := parse("import Lib\ndata Bool = True | False\nmain := True")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	if len(prog.Decls) != 2 { // data + value def
		t.Fatalf("expected 2 decls, got %d", len(prog.Decls))
	}
}

func TestAtDeclBoundaryImport(t *testing.T) {
	// import should be recognized at a declaration boundary.
	tokens := lex("import")
	if tokens[0].Kind != TokImport {
		t.Errorf("expected TokImport, got %v", tokens[0].Kind)
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

func TestSemicolonTopLevelSeparator(t *testing.T) {
	// Semicolons separate top-level declarations (same as newlines).
	prog, es := parse("f :: Int; f := 42; g := True")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Decls) != 3 {
		t.Fatalf("expected 3 decls, got %d", len(prog.Decls))
	}
}

func TestSemicolonBetweenImports(t *testing.T) {
	prog, es := parse("import Foo; import Bar\nf := 1")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Imports) != 2 {
		t.Errorf("expected 2 imports, got %d", len(prog.Imports))
	}
	if len(prog.Decls) != 1 {
		t.Errorf("expected 1 decl, got %d", len(prog.Decls))
	}
}

func TestSemicolonMixedWithNewlines(t *testing.T) {
	// Both separators used together.
	prog, es := parse("f := 1;\ng := 2\nh := 3")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Decls) != 3 {
		t.Fatalf("expected 3 decls, got %d", len(prog.Decls))
	}
}

func TestSemicolonTrailing(t *testing.T) {
	// Trailing semicolons are harmless.
	prog, es := parse("f := 1; g := 2;")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Decls) != 2 {
		t.Fatalf("expected 2 decls, got %d", len(prog.Decls))
	}
}

func TestSemicolonMultiple(t *testing.T) {
	// Multiple consecutive semicolons are harmless.
	prog, es := parse("f := 1;;; g := 2")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	if len(prog.Decls) != 2 {
		t.Fatalf("expected 2 decls, got %d", len(prog.Decls))
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

// --- HKT: Kind sort in forall binders ---

func TestParseKindSort(t *testing.T) {
	// forall (k : Kind). k -> Type
	prog, es := parse("f :: forall (k : Kind). k -> Type")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclTypeAnn)
	fa := d.Type.(*TyExprForall)
	if fa.Binders[0].Kind == nil {
		t.Fatal("expected kind annotation")
	}
	_, ok := fa.Binders[0].Kind.(*KindExprSort)
	if !ok {
		t.Errorf("expected KindExprSort, got %T", fa.Binders[0].Kind)
	}
}

func TestParseKindSortInArrow(t *testing.T) {
	// forall (k : Kind). forall (f : k -> Type). f a -> f a
	prog, es := parse("f :: forall (k : Kind). forall (f : k -> Type). Int")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclTypeAnn)
	fa1 := d.Type.(*TyExprForall)
	if _, ok := fa1.Binders[0].Kind.(*KindExprSort); !ok {
		t.Errorf("expected KindExprSort for k, got %T", fa1.Binders[0].Kind)
	}
	fa2, ok := fa1.Body.(*TyExprForall)
	if !ok {
		t.Fatal("expected nested TyExprForall")
	}
	arrow, ok := fa2.Binders[0].Kind.(*KindExprArrow)
	if !ok {
		t.Fatalf("expected KindExprArrow for f, got %T", fa2.Binders[0].Kind)
	}
	// Left of arrow should be KindExprName "k" (a kind variable reference)
	kn, ok := arrow.From.(*KindExprName)
	if !ok {
		t.Errorf("expected KindExprName for arrow.From, got %T", arrow.From)
	} else if kn.Name != "k" {
		t.Errorf("expected kind name 'k', got %q", kn.Name)
	}
}

// --- Records ---

func TestParseRecordLiteral(t *testing.T) {
	prog, es := parse("r := { x = 1, y = 2 }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	rec, ok := bind.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord, got %T", bind.Expr)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rec.Fields))
	}
	if rec.Fields[0].Label != "x" {
		t.Errorf("field[0]: expected 'x', got %q", rec.Fields[0].Label)
	}
	if rec.Fields[1].Label != "y" {
		t.Errorf("field[1]: expected 'y', got %q", rec.Fields[1].Label)
	}
}

func TestParseEmptyRecord(t *testing.T) {
	prog, es := parse("r := {}")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	rec, ok := bind.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord, got %T", bind.Expr)
	}
	if len(rec.Fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(rec.Fields))
	}
}

func TestParseRecordUpdate(t *testing.T) {
	prog, es := parse("r2 := { r | x = 1 }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	upd, ok := bind.Expr.(*ExprRecordUpdate)
	if !ok {
		t.Fatalf("expected ExprRecordUpdate, got %T", bind.Expr)
	}
	v, ok := upd.Record.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar as record base, got %T", upd.Record)
	}
	if v.Name != "r" {
		t.Errorf("expected record base 'r', got %q", v.Name)
	}
	if len(upd.Updates) != 1 || upd.Updates[0].Label != "x" {
		t.Errorf("expected 1 update with label 'x', got %v", upd.Updates)
	}
}

func TestParseRecordProjection(t *testing.T) {
	prog, es := parse("v := r!#x")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	proj, ok := bind.Expr.(*ExprProject)
	if !ok {
		t.Fatalf("expected ExprProject, got %T", bind.Expr)
	}
	if proj.Label != "x" {
		t.Errorf("expected label 'x', got %q", proj.Label)
	}
	v, ok := proj.Record.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar as projection base, got %T", proj.Record)
	}
	if v.Name != "r" {
		t.Errorf("expected base 'r', got %q", v.Name)
	}
}

func TestParseChainedProjection(t *testing.T) {
	prog, es := parse("v := r!#x!#y")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	proj, ok := bind.Expr.(*ExprProject)
	if !ok {
		t.Fatalf("expected ExprProject, got %T", bind.Expr)
	}
	if proj.Label != "y" {
		t.Errorf("expected outer label 'y', got %q", proj.Label)
	}
	inner, ok := proj.Record.(*ExprProject)
	if !ok {
		t.Fatalf("expected nested ExprProject, got %T", proj.Record)
	}
	if inner.Label != "x" {
		t.Errorf("expected inner label 'x', got %q", inner.Label)
	}
}

func TestParseRecordPattern(t *testing.T) {
	prog, es := parse("f := \\{ x = a, y = b } -> a")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	lam, ok := bind.Expr.(*ExprLam)
	if !ok {
		t.Fatalf("expected ExprLam, got %T", bind.Expr)
	}
	rpat, ok := lam.Params[0].(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord, got %T", lam.Params[0])
	}
	if len(rpat.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rpat.Fields))
	}
	if rpat.Fields[0].Label != "x" {
		t.Errorf("field[0]: expected 'x', got %q", rpat.Fields[0].Label)
	}
}

func TestParseProjectionPrecedence(t *testing.T) {
	// f r!#x should parse as App(f, Project(r, x)), not Project(App(f, r), x)
	prog, es := parse("v := f r!#x")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	app, ok := bind.Expr.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp, got %T", bind.Expr)
	}
	// f is the function
	fn, ok := app.Fun.(*ExprVar)
	if !ok || fn.Name != "f" {
		t.Errorf("expected function 'f', got %T", app.Fun)
	}
	// r!#x is the argument
	proj, ok := app.Arg.(*ExprProject)
	if !ok {
		t.Fatalf("expected ExprProject as argument, got %T", app.Arg)
	}
	if proj.Label != "x" {
		t.Errorf("expected label 'x', got %q", proj.Label)
	}
}

// --- Tuples + Unit ---

func TestParseTupleLiteral(t *testing.T) {
	prog, es := parse("t := (1, 2)")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	rec, ok := bind.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord (desugared tuple), got %T", bind.Expr)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rec.Fields))
	}
	if rec.Fields[0].Label != "_1" || rec.Fields[1].Label != "_2" {
		t.Errorf("expected _1, _2, got %q, %q", rec.Fields[0].Label, rec.Fields[1].Label)
	}
}

func TestParseUnitLiteral(t *testing.T) {
	prog, es := parse("u := ()")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	rec, ok := bind.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord (unit), got %T", bind.Expr)
	}
	if len(rec.Fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(rec.Fields))
	}
}

func TestParseGroupingNotTuple(t *testing.T) {
	prog, es := parse("g := (1)")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	_, ok := bind.Expr.(*ExprParen)
	if !ok {
		t.Fatalf("expected ExprParen (grouping), got %T", bind.Expr)
	}
}

func TestParseTuplePattern(t *testing.T) {
	prog, es := parse("f := \\(a, b) -> a")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	lam := bind.Expr.(*ExprLam)
	rpat, ok := lam.Params[0].(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord (desugared tuple pattern), got %T", lam.Params[0])
	}
	if len(rpat.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rpat.Fields))
	}
	if rpat.Fields[0].Label != "_1" || rpat.Fields[1].Label != "_2" {
		t.Errorf("expected _1, _2, got %q, %q", rpat.Fields[0].Label, rpat.Fields[1].Label)
	}
}

func TestParseUnitPattern(t *testing.T) {
	prog, es := parse("f := \\() -> 1")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	lam := bind.Expr.(*ExprLam)
	rpat, ok := lam.Params[0].(*PatRecord)
	if !ok {
		t.Fatalf("expected PatRecord (unit pattern), got %T", lam.Params[0])
	}
	if len(rpat.Fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(rpat.Fields))
	}
}

func TestParseTupleType(t *testing.T) {
	prog, es := parse("f :: (Int, Bool) -> Int")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclTypeAnn)
	arrow, ok := d.Type.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow, got %T", d.Type)
	}
	app, ok := arrow.From.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp (Record applied to row), got %T", arrow.From)
	}
	con, ok := app.Fun.(*TyExprCon)
	if !ok || con.Name != "Record" {
		t.Fatalf("expected TyExprCon 'Record', got %T %v", app.Fun, app.Fun)
	}
	row, ok := app.Arg.(*TyExprRow)
	if !ok {
		t.Fatalf("expected TyExprRow, got %T", app.Arg)
	}
	if len(row.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(row.Fields))
	}
	if row.Fields[0].Label != "_1" || row.Fields[1].Label != "_2" {
		t.Errorf("expected _1, _2, got %q, %q", row.Fields[0].Label, row.Fields[1].Label)
	}
}

func TestParseUnitType(t *testing.T) {
	prog, es := parse("f :: () -> Int")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	d := prog.Decls[0].(*DeclTypeAnn)
	arrow := d.Type.(*TyExprArrow)
	app, ok := arrow.From.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp (Record {}), got %T", arrow.From)
	}
	con := app.Fun.(*TyExprCon)
	if con.Name != "Record" {
		t.Errorf("expected 'Record', got %q", con.Name)
	}
	row := app.Arg.(*TyExprRow)
	if len(row.Fields) != 0 {
		t.Errorf("expected 0 fields for unit type, got %d", len(row.Fields))
	}
}

func TestParseBlockStillWorks(t *testing.T) {
	// Ensure { name := expr; body } still parses as a block
	prog, es := parse("r := { x := 1; x }")
	if es.HasErrors() {
		t.Fatal(es.Format())
	}
	bind := prog.Decls[0].(*DeclValueDef)
	_, ok := bind.Expr.(*ExprBlock)
	if !ok {
		t.Fatalf("expected ExprBlock, got %T", bind.Expr)
	}
}

// --- Parser stall guard tests ---
// These verify that the parser terminates on malformed input
// rather than entering an infinite loop.

func TestParseDoStallGuard(t *testing.T) {
	// "let" is not a keyword; "= 42" cannot be parsed as an expression.
	// The parser must not hang on unrecognizable tokens inside do blocks.
	_, es := parse("main := do { let x = 42; pure x }")
	if !es.HasErrors() {
		t.Fatal("expected parse errors for invalid do-block syntax")
	}
}

func TestParseDoStallGuard_EqToken(t *testing.T) {
	// Bare '=' inside a do block — parser must terminate.
	_, es := parse("main := do { = }")
	if !es.HasErrors() {
		t.Fatal("expected parse errors for bare '=' in do-block")
	}
}

func TestParseCaseStallGuard(t *testing.T) {
	// Unrecognizable token sequence in case alts.
	_, es := parse("main := case x { = }")
	if !es.HasErrors() {
		t.Fatal("expected parse errors for invalid case alt")
	}
}

// --- Parser resource limit tests ---
// These verify that the parser terminates on pathological input
// that would otherwise cause stack overflow or memory exhaustion.

func TestParseDeepNestingTerminates(t *testing.T) {
	// Deep nested parentheses: (((((...))))) — must not stack overflow.
	// With recursion depth limit, this should report an error and terminate.
	const depth = 2000
	src := ""
	for range depth {
		src += "("
	}
	src += "x"
	for range depth {
		src += ")"
	}
	_, es := parse("main := " + src)
	if !es.HasErrors() {
		t.Fatal("expected depth limit error for deeply nested expression")
	}
}

func TestParseDeepNestedLambdas(t *testing.T) {
	// Nested lambdas: \x -> \x -> \x -> ... -> x
	const depth = 2000
	src := "main := "
	for range depth {
		src += "\\x -> "
	}
	src += "x"
	_, es := parse(src)
	if !es.HasErrors() {
		t.Fatal("expected depth limit error for deeply nested lambdas")
	}
}

func TestParseDeepNestedDo(t *testing.T) {
	// Nested do blocks: do { do { do { ... } } }
	const depth = 2000
	src := "main := "
	for range depth {
		src += "do { "
	}
	src += "pure x"
	for range depth {
		src += " }"
	}
	_, es := parse(src)
	if !es.HasErrors() {
		t.Fatal("expected depth limit error for deeply nested do blocks")
	}
}

func TestParseGADTStallGuard(t *testing.T) {
	// Malformed GADT body with unrecognizable tokens — must not hang.
	_, es := parse("data Foo where { = }")
	if !es.HasErrors() {
		t.Fatal("expected parse errors for invalid GADT body")
	}
}

func TestParseClassStallGuard(t *testing.T) {
	// Malformed class body — must not hang.
	_, es := parse("class Foo a { = }")
	if !es.HasErrors() {
		t.Fatal("expected parse errors for invalid class body")
	}
}

func TestParseDeepNestedTypes(t *testing.T) {
	// Deeply nested type expressions via parentheses: ((((... Int ))))
	// parseType calls enterRecurse — should hit depth limit.
	const depth = 2000
	src := "f :: "
	for range depth {
		src += "("
	}
	src += "Int"
	for range depth {
		src += ")"
	}
	src += "\nf := 42\nmain := f"
	_, es := parse(src)
	if !es.HasErrors() {
		t.Fatal("expected depth limit error for deeply nested type expression")
	}
}

func TestParseDeepNestedPatterns(t *testing.T) {
	// Deeply nested patterns via constructor nesting:
	// case x { (((... y ...))) -> y }
	// parsePattern calls enterRecurse — should hit depth limit.
	const depth = 2000
	src := "main := case x { "
	for range depth {
		src += "("
	}
	src += "y"
	for range depth {
		src += ")"
	}
	src += " -> y }"
	_, es := parse(src)
	if !es.HasErrors() {
		t.Fatal("expected depth limit error for deeply nested patterns")
	}
}

func TestParseStepLimitExceeded(t *testing.T) {
	// Test that the step counter is respected by creating a parser
	// with artificially low maxSteps.
	src := span.NewSource("test", "main := 1 + 2 + 3")
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	// Override maxSteps to something very small.
	p.maxSteps = 2 // will trigger almost immediately
	_ = p.ParseProgram()
	if !es.HasErrors() {
		t.Fatal("expected step limit error")
	}
}

func TestParseMaxStepsFloor(t *testing.T) {
	// Very short input: len(tokens)*4 would be < 100.
	// The floor of 100 ensures we don't reject valid short programs.
	src := span.NewSource("test", "main := 1")
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	// Verify the floor is applied: maxSteps should be >= 100.
	if p.maxSteps < 100 {
		t.Errorf("maxSteps should be at least 100 for short input, got %d", p.maxSteps)
	}
	_ = p.ParseProgram()
	if es.HasErrors() {
		t.Fatalf("valid short program should parse without errors: %s", es.Format())
	}
}

// --- Edge case / robustness tests ---

func lexWithErrors(input string) ([]Token, *errs.Errors) {
	src := span.NewSource("test", input)
	l := NewLexer(src)
	return l.Tokenize()
}

func TestLexUnterminatedBlockComment(t *testing.T) {
	_, es := lexWithErrors("{- hello")
	if !es.HasErrors() {
		t.Fatal("expected error for unterminated block comment")
	}
	if es.Errs[0].Code != errs.ErrUnterminatedLit {
		t.Errorf("expected ErrUnterminatedLit, got E%04d", es.Errs[0].Code)
	}
	// Span should point to the {- start, not EOF.
	if es.Errs[0].Span.Start != 0 {
		t.Errorf("expected span start at 0 (the '{-'), got %d", es.Errs[0].Span.Start)
	}
}

func TestLexUnterminatedBlockCommentNested(t *testing.T) {
	_, es := lexWithErrors("{- outer {- inner -}")
	if !es.HasErrors() {
		t.Fatal("expected error for unterminated nested block comment")
	}
	if es.Errs[0].Code != errs.ErrUnterminatedLit {
		t.Errorf("expected ErrUnterminatedLit, got E%04d", es.Errs[0].Code)
	}
}

func TestLexDeeplyNestedBlockComment(t *testing.T) {
	// Three levels of nesting, all properly closed.
	tokens := lex("x {- a {- b {- c -} b -} a -} y")
	if len(tokens) != 3 { // x, y, EOF
		t.Errorf("expected 3 tokens for triple-nested comment, got %d", len(tokens))
	}
}

func TestLexUnterminatedString(t *testing.T) {
	_, es := lexWithErrors(`"hello`)
	if !es.HasErrors() {
		t.Fatal("expected error for unterminated string")
	}
	if es.Errs[0].Code != errs.ErrUnterminatedLit {
		t.Errorf("expected ErrUnterminatedLit, got E%04d", es.Errs[0].Code)
	}
}

func TestLexUnterminatedStringNewline(t *testing.T) {
	_, es := lexWithErrors("\"hello\nworld\"")
	if !es.HasErrors() {
		t.Fatal("expected error for string terminated by newline")
	}
}

func TestLexEmptyString(t *testing.T) {
	tokens := lex(`""`)
	if tokens[0].Kind != TokStrLit || tokens[0].Text != "" {
		t.Errorf("expected empty string literal, got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

func TestLexStringUnicode(t *testing.T) {
	tokens := lex(`"αβγ日本語"`)
	if tokens[0].Kind != TokStrLit || tokens[0].Text != "αβγ日本語" {
		t.Errorf("expected unicode string preserved, got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

func TestLexStringAllEscapes(t *testing.T) {
	tokens := lex(`"\n\t\r\\\"\'\0"`)
	if tokens[0].Kind != TokStrLit {
		t.Fatalf("expected string literal, got %v", tokens[0].Kind)
	}
	expected := "\n\t\r\\\"'\x00"
	if tokens[0].Text != expected {
		t.Errorf("expected all escapes decoded, got %q (want %q)", tokens[0].Text, expected)
	}
}

func TestLexBadEscape(t *testing.T) {
	_, es := lexWithErrors(`"\z"`)
	if !es.HasErrors() {
		t.Fatal("expected error for unknown escape sequence")
	}
	if es.Errs[0].Code != errs.ErrBadEscape {
		t.Errorf("expected ErrBadEscape, got E%04d", es.Errs[0].Code)
	}
}

func TestLexEmpty(t *testing.T) {
	tokens, es := lexWithErrors("")
	if es.HasErrors() {
		t.Fatalf("empty input should not produce errors: %s", es.Format())
	}
	if len(tokens) != 1 || tokens[0].Kind != TokEOF {
		t.Errorf("expected [EOF] for empty input, got %d tokens", len(tokens))
	}
}

func TestLexWhitespaceOnly(t *testing.T) {
	tokens, es := lexWithErrors("   \t\n  ")
	if es.HasErrors() {
		t.Fatalf("whitespace-only input should not produce errors: %s", es.Format())
	}
	if len(tokens) != 1 || tokens[0].Kind != TokEOF {
		t.Errorf("expected [EOF] for whitespace-only input, got %d tokens", len(tokens))
	}
}

func TestLexCommentOnly(t *testing.T) {
	tokens, es := lexWithErrors("-- just a comment")
	if es.HasErrors() {
		t.Fatalf("comment-only input should not produce errors: %s", es.Format())
	}
	if len(tokens) != 1 || tokens[0].Kind != TokEOF {
		t.Errorf("expected [EOF] for comment-only input, got %d tokens", len(tokens))
	}
}

func TestLexBlockCommentOnly(t *testing.T) {
	tokens, es := lexWithErrors("{- block comment -}")
	if es.HasErrors() {
		t.Fatalf("block-comment-only input should not produce errors: %s", es.Format())
	}
	if len(tokens) != 1 || tokens[0].Kind != TokEOF {
		t.Errorf("expected [EOF] for block-comment-only input, got %d tokens", len(tokens))
	}
}

func TestLexUnicodeIdentRejected(t *testing.T) {
	_, es := lexWithErrors("α := 42")
	if !es.HasErrors() {
		t.Fatal("expected error for unicode identifier start")
	}
	if es.Errs[0].Code != errs.ErrUnexpectedChar {
		t.Errorf("expected ErrUnexpectedChar, got E%04d", es.Errs[0].Code)
	}
}

func TestParseMultipleErrors(t *testing.T) {
	// Two distinct syntax errors in one program — both should be reported.
	_, es := parse("f := )\ng := )")
	if !es.HasErrors() {
		t.Fatal("expected parse errors")
	}
	if len(es.Errs) < 2 {
		t.Errorf("expected at least 2 errors, got %d", len(es.Errs))
	}
}

func TestParseEmptyInput(t *testing.T) {
	prog, es := parse("")
	if es.HasErrors() {
		t.Fatalf("empty input should parse without errors: %s", es.Format())
	}
	if len(prog.Decls) != 0 || len(prog.Imports) != 0 {
		t.Errorf("expected empty program, got %d decls, %d imports", len(prog.Decls), len(prog.Imports))
	}
}

func TestParseStepsResetBetweenPasses(t *testing.T) {
	// After collectFixity (first pass), steps are reset to 0.
	// Without the reset, collectFixity's steps would carry over,
	// potentially exceeding maxSteps during the second pass.
	src := span.NewSource("test", "infixl 6 +\nmain := 1 + 2")
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	// Set maxSteps to just enough for one pass but not two.
	// collectFixity scans all tokens (~advance per token).
	// If steps aren't reset, the second pass would exceed the limit.
	p.maxSteps = len(tokens) + 5
	_ = p.ParseProgram()
	if es.HasErrors() {
		t.Fatalf("valid program should parse when steps are properly reset: %s", es.Format())
	}
}
