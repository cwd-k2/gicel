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
