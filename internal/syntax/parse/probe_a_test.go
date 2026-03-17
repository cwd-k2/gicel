package parse

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/cwd-k2/gicel/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
)

// parseExprOnly parses a single expression from source text.
func parseExprOnly(input string) (Expr, *errs.Errors) {
	src := span.NewSource("test", input)
	l := NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return nil, lexErrs
	}
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	expr := p.ParseExpr()
	return expr, es
}

// parseTypeOnly parses a single type expression from source text.
func parseTypeOnly(input string) (TypeExpr, *errs.Errors) {
	src := span.NewSource("test", input)
	l := NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return nil, lexErrs
	}
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	ty := p.ParseType()
	return ty, es
}

// ====================================================
// Import Parsing Edge Cases
// ====================================================

func TestProbeA_ImportBasic(t *testing.T) {
	// Basic open import should work.
	prog, es := parse("import Foo\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	if prog.Imports[0].ModuleName != "Foo" {
		t.Errorf("expected module name Foo, got %s", prog.Imports[0].ModuleName)
	}
	if prog.Imports[0].Alias != "" {
		t.Errorf("expected no alias, got %q", prog.Imports[0].Alias)
	}
	if prog.Imports[0].Names != nil {
		t.Errorf("expected nil Names (open import), got %v", prog.Imports[0].Names)
	}
}

func TestProbeA_ImportEmptySelectiveList(t *testing.T) {
	// Empty selective list: import M ()
	// The parser loop breaks on RParen immediately, yielding an empty Names slice.
	//
	// BUG: severity=low, description="import M () produces nil Names (not empty slice),
	// making it indistinguishable from open import 'import M'. parseImportList returns
	// nil when no names are parsed, but semantically 'import M ()' should mean 'import
	// nothing' while 'import M' means 'import everything'. The DeclImport.Names field
	// cannot distinguish these two cases."
	prog, es := parse("import M ()\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	imp := prog.Imports[0]
	if imp.Names == nil {
		t.Log("BUG CONFIRMED: import M () produces nil Names, indistinguishable from import M")
	}
	if len(imp.Names) != 0 {
		t.Errorf("expected 0 import names, got %d", len(imp.Names))
	}
}

func TestProbeA_ImportTrailingComma(t *testing.T) {
	// Trailing comma: import M (x,)
	// After parsing "x", the parser sees comma, advances, then the loop condition
	// checks peek() != TokRParen — which is false, so it breaks.
	// Result: trailing comma is silently accepted with only "x" in the list.
	//
	// BUG: severity=info, description="Trailing comma in import list silently accepted.
	// 'import M (x,)' parses identically to 'import M (x)'. This is arguably fine
	// (Go-style trailing comma tolerance) but is undocumented behavior. The loop in
	// parseImportList consumes the comma then breaks when it sees RParen."
	prog, es := parse("import M (x,)\nmain := 1")
	if !es.HasErrors() {
		t.Log("trailing comma in import list accepted without error (silent tolerance)")
		if len(prog.Imports) != 1 {
			t.Fatalf("expected 1 import, got %d", len(prog.Imports))
		}
		imp := prog.Imports[0]
		t.Logf("import names: %d", len(imp.Names))
		for i, n := range imp.Names {
			t.Logf("  name[%d]: %q HasSub=%v AllSubs=%v SubList=%v", i, n.Name, n.HasSub, n.AllSubs, n.SubList)
		}
	} else {
		errStr := es.Format()
		t.Logf("trailing comma causes error: %s", errStr)
	}
}

func TestProbeA_ImportOperator(t *testing.T) {
	// Operator import: import M ((+))
	prog, es := parse("import M ((+))\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	imp := prog.Imports[0]
	if len(imp.Names) != 1 {
		t.Fatalf("expected 1 import name, got %d", len(imp.Names))
	}
	if imp.Names[0].Name != "+" {
		t.Errorf("expected operator name '+', got %q", imp.Names[0].Name)
	}
}

func TestProbeA_ImportDotOperator(t *testing.T) {
	// Import the composition operator (.): import M ((.))
	prog, es := parse("import M ((.))\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	imp := prog.Imports[0]
	if len(imp.Names) != 1 {
		t.Fatalf("expected 1 import name, got %d", len(imp.Names))
	}
	if imp.Names[0].Name != "." {
		t.Errorf("expected operator name '.', got %q", imp.Names[0].Name)
	}
}

func TestProbeA_ImportDottedModuleWithAs(t *testing.T) {
	// Dotted module name with alias: import Std.Num as N
	prog, es := parse("import Std.Num as N\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	imp := prog.Imports[0]
	if imp.ModuleName != "Std.Num" {
		t.Errorf("expected module name 'Std.Num', got %q", imp.ModuleName)
	}
	if imp.Alias != "N" {
		t.Errorf("expected alias 'N', got %q", imp.Alias)
	}
}

func TestProbeA_ImportVeryLongModuleName(t *testing.T) {
	// Very deep dotted module name: import A.B.C.D.E
	prog, es := parse("import A.B.C.D.E\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	if prog.Imports[0].ModuleName != "A.B.C.D.E" {
		t.Errorf("expected module name 'A.B.C.D.E', got %q", prog.Imports[0].ModuleName)
	}
}

func TestProbeA_AsContextualKeyword(t *testing.T) {
	// "as" used as a variable name in a non-import context.
	// "as" is NOT a keyword — it should be lexed as TokLower.
	prog, es := parse("as := 42")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(prog.Decls))
	}
	vd, ok := prog.Decls[0].(*DeclValueDef)
	if !ok {
		t.Fatalf("expected DeclValueDef, got %T", prog.Decls[0])
	}
	if vd.Name != "as" {
		t.Errorf("expected name 'as', got %q", vd.Name)
	}
}

func TestProbeA_ImportMissingAlias(t *testing.T) {
	// import M as  (with missing alias — expects TokUpper after "as")
	_, es := parse("import M as\nmain := 1")
	if !es.HasErrors() {
		t.Fatal("expected error for missing alias after 'as', got none")
	}
	errStr := es.Format()
	if !strings.Contains(errStr, "uppercase") {
		t.Logf("error message: %s", errStr)
	}
}

func TestProbeA_ImportMissingCloseParen(t *testing.T) {
	// import M ( with missing close paren
	_, es := parse("import M (x\nmain := 1")
	if !es.HasErrors() {
		t.Fatal("expected error for missing close paren, got none")
	}
}

func TestProbeA_ImportSelectiveAndQualifiedRejected(t *testing.T) {
	// Combined selective + qualified: import M as N (x)
	// After parsing "import M as N", the parser returns. The "(x)" should be
	// left as an orphaned expression that causes a parse error or is treated
	// as a separate declaration.
	prog, es := parse("import M as N (x)")
	// The parser might or might not produce errors.
	// Key question: does it parse as expected?
	t.Logf("errors: %v", es.HasErrors())
	if es.HasErrors() {
		t.Logf("error: %s", es.Format())
	}
	t.Logf("imports: %d, decls: %d", len(prog.Imports), len(prog.Decls))
	if len(prog.Imports) > 0 {
		imp := prog.Imports[0]
		t.Logf("import: module=%q alias=%q names=%v", imp.ModuleName, imp.Alias, imp.Names)
	}
	// The key invariant: import M as N should not also have Names.
	if len(prog.Imports) == 1 && prog.Imports[0].Alias != "" && prog.Imports[0].Names != nil {
		t.Error("BUG: import has both Alias and Names set — should be mutually exclusive")
	}
}

func TestProbeA_ImportSelectiveThenQualified(t *testing.T) {
	// import M (x) as N — selective first, then "as"
	// After parsing "import M (x)", the parser returns. "as N" is orphaned.
	prog, es := parse("import M (x) as N")
	t.Logf("errors: %v", es.HasErrors())
	if es.HasErrors() {
		t.Logf("error: %s", es.Format())
	}
	if len(prog.Imports) > 0 {
		imp := prog.Imports[0]
		t.Logf("import: module=%q alias=%q names=%v", imp.ModuleName, imp.Alias, imp.Names)
		// "as" is a lowercase ident so it may be parsed as part of the next declaration.
	}
}

func TestProbeA_ImportWithoutModuleName(t *testing.T) {
	// import without module name: just "import"
	_, es := parse("import\nmain := 1")
	if !es.HasErrors() {
		t.Fatal("expected error for import without module name, got none")
	}
}

func TestProbeA_ImportMultipleInSequence(t *testing.T) {
	// Multiple imports parsed correctly in sequence.
	source := "import A\nimport B.C\nimport D as E\nimport F (x, Y(..))\nmain := 1"
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 4 {
		t.Fatalf("expected 4 imports, got %d", len(prog.Imports))
	}
	// Verify each import.
	if prog.Imports[0].ModuleName != "A" {
		t.Errorf("import[0] module: got %q, want A", prog.Imports[0].ModuleName)
	}
	if prog.Imports[1].ModuleName != "B.C" {
		t.Errorf("import[1] module: got %q, want B.C", prog.Imports[1].ModuleName)
	}
	if prog.Imports[2].ModuleName != "D" || prog.Imports[2].Alias != "E" {
		t.Errorf("import[2]: got module=%q alias=%q, want D as E", prog.Imports[2].ModuleName, prog.Imports[2].Alias)
	}
	if prog.Imports[3].ModuleName != "F" || len(prog.Imports[3].Names) != 2 {
		t.Errorf("import[3]: got module=%q names=%d, want F with 2 names", prog.Imports[3].ModuleName, len(prog.Imports[3].Names))
	}
}

func TestProbeA_ImportSelectiveWithSubList(t *testing.T) {
	// import M (Bool(..), Maybe(Just, Nothing), add)
	prog, es := parse("import M (Bool(..), Maybe(Just, Nothing), add)\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	names := prog.Imports[0].Names
	if len(names) != 3 {
		t.Fatalf("expected 3 import names, got %d", len(names))
	}
	// Bool(..)
	if names[0].Name != "Bool" || !names[0].HasSub || !names[0].AllSubs {
		t.Errorf("names[0]: got %+v, want Bool(..)", names[0])
	}
	// Maybe(Just, Nothing)
	if names[1].Name != "Maybe" || !names[1].HasSub || names[1].AllSubs {
		t.Errorf("names[1]: got %+v, want Maybe(Just, Nothing)", names[1])
	}
	if len(names[1].SubList) != 2 || names[1].SubList[0] != "Just" || names[1].SubList[1] != "Nothing" {
		t.Errorf("names[1].SubList: got %v, want [Just, Nothing]", names[1].SubList)
	}
	// add
	if names[2].Name != "add" || names[2].HasSub {
		t.Errorf("names[2]: got %+v, want plain add", names[2])
	}
}

// ====================================================
// Qualified Name Adjacency
// ====================================================

func TestProbeA_QualVarAdjacent(t *testing.T) {
	// N.x (adjacent) → ExprQualVar
	expr, es := parseExprOnly("N.x")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	qv, ok := expr.(*ExprQualVar)
	if !ok {
		t.Fatalf("expected ExprQualVar, got %T", expr)
	}
	if qv.Qualifier != "N" || qv.Name != "x" {
		t.Errorf("expected N.x, got %s.%s", qv.Qualifier, qv.Name)
	}
}

func TestProbeA_QualVarSpaced(t *testing.T) {
	// N . x (spaced) → should NOT be ExprQualVar.
	// Should be: ExprCon(N) . ExprVar(x) — i.e., infix composition.
	expr, es := parseExprOnly("N . x")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if _, ok := expr.(*ExprQualVar); ok {
		t.Error("BUG: spaced 'N . x' parsed as ExprQualVar instead of composition")
	}
	// Should be ExprInfix with op "."
	inf, ok := expr.(*ExprInfix)
	if !ok {
		t.Fatalf("expected ExprInfix, got %T", expr)
	}
	if inf.Op != "." {
		t.Errorf("expected op '.', got %q", inf.Op)
	}
}

func TestProbeA_QualConAdjacent(t *testing.T) {
	// N.X (adjacent, both upper) → ExprQualCon
	expr, es := parseExprOnly("N.X")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	qc, ok := expr.(*ExprQualCon)
	if !ok {
		t.Fatalf("expected ExprQualCon, got %T", expr)
	}
	if qc.Qualifier != "N" || qc.Name != "X" {
		t.Errorf("expected N.X, got %s.%s", qc.Qualifier, qc.Name)
	}
}

func TestProbeA_QualNameNotIdentAfterDot(t *testing.T) {
	// N.123 — adjacent but the token after dot is not an identifier.
	// Should not parse as qualified; should be ExprCon(N) followed by something.
	expr, es := parseExprOnly("N.123")
	if es.HasErrors() {
		// Some error from attempting to parse is acceptable.
		t.Logf("errors (may be expected): %s", es.Format())
	}
	if _, ok := expr.(*ExprQualVar); ok {
		t.Error("BUG: N.123 parsed as ExprQualVar")
	}
	if _, ok := expr.(*ExprQualCon); ok {
		t.Error("BUG: N.123 parsed as ExprQualCon")
	}
	t.Logf("N.123 parsed as %T", expr)
}

func TestProbeA_QualNameDotAtEndOfInput(t *testing.T) {
	// N. at end of input — should not crash.
	// The parser should handle this gracefully: either treat as ExprCon(N) + error,
	// or treat . as infix op with missing RHS.
	expr, es := parseExprOnly("N.")
	// We mainly care that there's no panic.
	if expr == nil {
		t.Fatal("parser returned nil expression for 'N.'")
	}
	t.Logf("N. parsed as %T, errors=%v", expr, es.HasErrors())
}

func TestProbeA_QualVarInFuncPosition(t *testing.T) {
	// N.add x → application of qualified var to x
	prog, es := parse("main := N.add x")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	d := prog.Decls[0].(*DeclValueDef)
	app, ok := d.Expr.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp, got %T", d.Expr)
	}
	qv, ok := app.Fun.(*ExprQualVar)
	if !ok {
		t.Fatalf("expected ExprQualVar in function position, got %T", app.Fun)
	}
	if qv.Qualifier != "N" || qv.Name != "add" {
		t.Errorf("expected N.add, got %s.%s", qv.Qualifier, qv.Name)
	}
}

func TestProbeA_QualConInArgPosition(t *testing.T) {
	// f N.Just → application of f to qualified constructor
	prog, es := parse("main := f N.Just")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	d := prog.Decls[0].(*DeclValueDef)
	app, ok := d.Expr.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp, got %T", d.Expr)
	}
	qc, ok := app.Arg.(*ExprQualCon)
	if !ok {
		t.Fatalf("expected ExprQualCon in arg position, got %T", app.Arg)
	}
	if qc.Qualifier != "N" || qc.Name != "Just" {
		t.Errorf("expected N.Just, got %s.%s", qc.Qualifier, qc.Name)
	}
}

func TestProbeA_QualVarWithRecordProjection(t *testing.T) {
	// N.x.#field — qualified var followed by record projection.
	// Should parse as ExprProject(ExprQualVar(N, x), "field").
	expr, es := parseExprOnly("N.x.#field")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	proj, ok := expr.(*ExprProject)
	if !ok {
		t.Fatalf("expected ExprProject, got %T", expr)
	}
	if proj.Label != "field" {
		t.Errorf("expected label 'field', got %q", proj.Label)
	}
	qv, ok := proj.Record.(*ExprQualVar)
	if !ok {
		t.Fatalf("expected ExprQualVar under projection, got %T", proj.Record)
	}
	if qv.Qualifier != "N" || qv.Name != "x" {
		t.Errorf("expected N.x, got %s.%s", qv.Qualifier, qv.Name)
	}
}

func TestProbeA_QualTypeInAnnotation(t *testing.T) {
	// Qualified type in annotation position: f :: N.Int -> N.Int
	ty, es := parseTypeOnly("N.Int -> N.Int")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	arr, ok := ty.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow, got %T", ty)
	}
	fromQC, ok := arr.From.(*TyExprQualCon)
	if !ok {
		t.Fatalf("expected TyExprQualCon for From, got %T", arr.From)
	}
	if fromQC.Qualifier != "N" || fromQC.Name != "Int" {
		t.Errorf("expected N.Int, got %s.%s", fromQC.Qualifier, fromQC.Name)
	}
	toQC, ok := arr.To.(*TyExprQualCon)
	if !ok {
		t.Fatalf("expected TyExprQualCon for To, got %T", arr.To)
	}
	if toQC.Qualifier != "N" || toQC.Name != "Int" {
		t.Errorf("expected N.Int, got %s.%s", toQC.Qualifier, toQC.Name)
	}
}

func TestProbeA_QualConAsConstructorRegardlessOfKnowledge(t *testing.T) {
	// N.X where N is not a known qualifier but just a constructor — parser
	// shouldn't care (that's the checker's job), it should produce ExprQualCon.
	expr, es := parseExprOnly("UnknownModule.SomeCon")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	qc, ok := expr.(*ExprQualCon)
	if !ok {
		t.Fatalf("expected ExprQualCon, got %T", expr)
	}
	if qc.Qualifier != "UnknownModule" || qc.Name != "SomeCon" {
		t.Errorf("expected UnknownModule.SomeCon, got %s.%s", qc.Qualifier, qc.Name)
	}
}

func TestProbeA_QualTypeSpaced(t *testing.T) {
	// N . Int (spaced) in type position — should NOT be TyExprQualCon.
	// The type parser correctly checks adjacency. With spaces, N is parsed as TyExprCon
	// and . is not consumed (type parser doesn't recognize dot as a type operator).
	// The remaining ". Int" is left unconsumed (no error in isolation, but would fail in
	// a full program context).
	ty, es := parseTypeOnly("N . Int")
	t.Logf("N . Int parsed as %T, errors=%v", ty, es.HasErrors())
	if es.HasErrors() {
		t.Logf("errors: %s", es.Format())
	}
	if qc, ok := ty.(*TyExprQualCon); ok {
		t.Errorf("BUG: spaced 'N . Int' parsed as TyExprQualCon(%s.%s)", qc.Qualifier, qc.Name)
	}
}

// ====================================================
// Crash Resistance
// ====================================================

func TestProbeA_DeeplyNestedParensInImportList(t *testing.T) {
	// Deeply nested parens in import list.
	// The import list parser only has one level of parens for operators: ((op)).
	// Extra nesting like (((+))) should cause an error but not crash.
	var b strings.Builder
	b.WriteString("import M (")
	for i := 0; i < 10; i++ {
		b.WriteByte('(')
	}
	b.WriteByte('+')
	for i := 0; i < 10; i++ {
		b.WriteByte(')')
	}
	b.WriteString(")\nmain := 1")
	_, es := parse(b.String())
	// We just care about no panic.
	_ = es
}

func TestProbeA_ImportWithManyNames(t *testing.T) {
	// Import with many names (stress test).
	var b strings.Builder
	b.WriteString("import M (")
	for i := 0; i < 200; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("name%d", i))
	}
	b.WriteString(")\nmain := 1")
	prog, es := parse(b.String())
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	if len(prog.Imports[0].Names) != 200 {
		t.Errorf("expected 200 import names, got %d", len(prog.Imports[0].Names))
	}
}

func TestProbeA_ImportAfterDeclarations(t *testing.T) {
	// Import after declarations — imports are only parsed at the top.
	// An import in the wrong position should cause a parse error.
	source := "main := 1\nimport M"
	prog, es := parse(source)
	// The parser processes imports first, then declarations.
	// "import M" after a declaration would be seen in the decl loop,
	// where TokImport isn't handled → "expected declaration" error.
	if !es.HasErrors() {
		t.Log("import after declaration accepted — checking structure")
		t.Logf("imports: %d, decls: %d", len(prog.Imports), len(prog.Decls))
	} else {
		t.Logf("correctly rejected import after declaration: %s", es.Format())
	}
}

// ====================================================
// Round-trip: AST Field Population
// ====================================================

func TestProbeA_ImportFieldsOpenImport(t *testing.T) {
	prog, es := parse("import Foo\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	imp := prog.Imports[0]
	if imp.ModuleName != "Foo" {
		t.Errorf("ModuleName: got %q, want Foo", imp.ModuleName)
	}
	if imp.Alias != "" {
		t.Errorf("Alias: got %q, want empty", imp.Alias)
	}
	if imp.Names != nil {
		t.Errorf("Names: got %v, want nil", imp.Names)
	}
	if imp.S.Start == imp.S.End {
		t.Error("Span is zero-width — should cover the import")
	}
}

func TestProbeA_ImportFieldsQualifiedImport(t *testing.T) {
	prog, es := parse("import Foo as F\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	imp := prog.Imports[0]
	if imp.ModuleName != "Foo" {
		t.Errorf("ModuleName: got %q, want Foo", imp.ModuleName)
	}
	if imp.Alias != "F" {
		t.Errorf("Alias: got %q, want F", imp.Alias)
	}
	if imp.Names != nil {
		t.Errorf("Names: got %v, want nil", imp.Names)
	}
}

func TestProbeA_ImportFieldsSelectiveImport(t *testing.T) {
	prog, es := parse("import Foo (bar, Baz(..), Qux(A, B))\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	imp := prog.Imports[0]
	if imp.ModuleName != "Foo" {
		t.Errorf("ModuleName: got %q, want Foo", imp.ModuleName)
	}
	if imp.Alias != "" {
		t.Errorf("Alias: got %q, want empty", imp.Alias)
	}
	if len(imp.Names) != 3 {
		t.Fatalf("Names: got %d entries, want 3", len(imp.Names))
	}
	// bar
	n0 := imp.Names[0]
	if n0.Name != "bar" || n0.HasSub || n0.AllSubs || n0.SubList != nil {
		t.Errorf("Names[0]: got %+v, want plain 'bar'", n0)
	}
	// Baz(..)
	n1 := imp.Names[1]
	if n1.Name != "Baz" || !n1.HasSub || !n1.AllSubs {
		t.Errorf("Names[1]: got %+v, want Baz(..)", n1)
	}
	// Qux(A, B)
	n2 := imp.Names[2]
	if n2.Name != "Qux" || !n2.HasSub || n2.AllSubs {
		t.Errorf("Names[2]: got %+v, want Qux(A, B)", n2)
	}
	if len(n2.SubList) != 2 || n2.SubList[0] != "A" || n2.SubList[1] != "B" {
		t.Errorf("Names[2].SubList: got %v, want [A, B]", n2.SubList)
	}
}

func TestProbeA_ImportSpanCoversFullDecl(t *testing.T) {
	// The import span should cover from 'import' to end of the import clause.
	source := "import Foo as Bar"
	prog, es := parse(source + "\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	imp := prog.Imports[0]
	if int(imp.S.Start) != 0 {
		t.Errorf("Span.Start: got %d, want 0", imp.S.Start)
	}
	if int(imp.S.End) < len("import Foo as Bar")-1 {
		t.Errorf("Span.End: got %d, want >= %d", imp.S.End, len("import Foo as Bar")-1)
	}
}

// ====================================================
// Adjacency function correctness
// ====================================================

func TestProbeA_TokensAdjacentCheck(t *testing.T) {
	// Verify that the lexer produces correct span offsets for adjacency.
	tokens := lex("N.x")
	// Should be: TokUpper("N"), TokDot("."), TokLower("x"), TokEOF
	if len(tokens) < 3 {
		t.Fatalf("expected at least 3 tokens, got %d", len(tokens))
	}
	if tokens[0].Kind != TokUpper || tokens[0].Text != "N" {
		t.Errorf("token[0]: got %v %q, want TokUpper 'N'", tokens[0].Kind, tokens[0].Text)
	}
	if tokens[1].Kind != TokDot {
		t.Errorf("token[1]: got %v, want TokDot", tokens[1].Kind)
	}
	if tokens[2].Kind != TokLower || tokens[2].Text != "x" {
		t.Errorf("token[2]: got %v %q, want TokLower 'x'", tokens[2].Kind, tokens[2].Text)
	}
	// Check adjacency: N(0..1) .(1..2) x(2..3)
	if !tokensAdjacent(tokens[0], tokens[1]) {
		t.Error("N and . should be adjacent")
	}
	if !tokensAdjacent(tokens[1], tokens[2]) {
		t.Error(". and x should be adjacent")
	}
}

func TestProbeA_TokensNotAdjacentCheck(t *testing.T) {
	// Verify that spaced tokens are NOT adjacent.
	tokens := lex("N . x")
	if len(tokens) < 3 {
		t.Fatalf("expected at least 3 tokens, got %d", len(tokens))
	}
	if tokensAdjacent(tokens[0], tokens[1]) {
		t.Error("N and . should NOT be adjacent with space between them")
	}
	if tokensAdjacent(tokens[1], tokens[2]) {
		t.Error(". and x should NOT be adjacent with space between them")
	}
}

// ====================================================
// Edge case: dotted module name in import vs qualified name
// ====================================================

func TestProbeA_ImportDottedModuleNotConfusedWithQualified(t *testing.T) {
	// In import context, dots between uppercase names form dotted module names.
	// import A.B means module "A.B", not qualified name A.B.
	prog, es := parse("import A.B\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if prog.Imports[0].ModuleName != "A.B" {
		t.Errorf("expected module name 'A.B', got %q", prog.Imports[0].ModuleName)
	}
}

// ====================================================
// Import dotted module: dots must be adjacent (lexer behavior)
// ====================================================

func TestProbeA_ImportDottedModuleWithSpaces(t *testing.T) {
	// "import A . B" — are dots between module parts required to be adjacent?
	// The parser uses p.peek().Kind == TokDot to chain module parts.
	// It does NOT check adjacency — so "import A . B" still parses as "A.B".
	//
	// BUG: severity=low, description="Import dotted module names do not require adjacency.
	// 'import A . B' parses identically to 'import A.B', unlike expression context where
	// adjacency matters (N.x vs N . x). The expression parser uses tokensAdjacent() to
	// disambiguate qualified names from composition, but parseImportDecl just checks for
	// TokDot without adjacency. This creates an inconsistency: in expressions, spacing
	// matters for '.', but in imports it does not."
	prog, es := parse("import A . B\nmain := 1")
	if es.HasErrors() {
		t.Logf("spaced import dots cause error: %s", es.Format())
	} else {
		imp := prog.Imports[0]
		t.Logf("BUG CONFIRMED: import A . B parsed as module=%q (no adjacency check)", imp.ModuleName)
	}
}

// ====================================================
// Import: Type with empty sub-list: import M (T())
// ====================================================

func TestProbeA_ImportTypeWithEmptySubList(t *testing.T) {
	// import M (T()) — T with explicitly empty sub-list.
	// parseImportName: after seeing TokLParen, checks for TokDot (for ..) or
	// TokRParen. Since TokRParen immediately, HasSub=true, AllSubs=false, SubList=nil.
	prog, es := parse("import M (T())\nmain := 1")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	names := prog.Imports[0].Names
	if len(names) != 1 {
		t.Fatalf("expected 1 name, got %d", len(names))
	}
	if !names[0].HasSub {
		t.Error("expected HasSub=true for T()")
	}
	if names[0].AllSubs {
		t.Error("expected AllSubs=false for T()")
	}
	if len(names[0].SubList) != 0 {
		t.Errorf("expected empty SubList for T(), got %v", names[0].SubList)
	}
}

// ====================================================
// Qualified name: only single-level qualified names supported
// ====================================================

func TestProbeA_QualifiedNameMultiLevel(t *testing.T) {
	// A.B.c — tryQualifiedExpr only handles one level.
	// After consuming A, it sees ".B" adjacent, produces ExprQualCon(A, B).
	// Then parseAtom returns and the .c part becomes a separate operation.
	expr, es := parseExprOnly("A.B.c")
	if es.HasErrors() {
		t.Logf("errors: %s", es.Format())
	}
	t.Logf("A.B.c parsed as %T", expr)
	// The result should be either:
	// - ExprQualCon(A, B) followed by infix . c (if .c isn't adjacent)
	// - Or some chained form
	// Key: it should not crash.
}

// ====================================================
// Pattern: qualified names in pattern position
// ====================================================

func TestProbeA_QualifiedNameInPattern(t *testing.T) {
	// case x { N.Just y -> y; _ -> 0 }
	// Pattern parsing does NOT support qualified constructors.
	// parseConPattern calls expectUpper() then collects args.
	// So "N.Just" in pattern would be parsed as PatCon("N") then ".Just" is orphaned.
	//
	// BUG: severity=medium, description="Qualified constructors not supported in pattern
	// position. 'case x { N.Just y -> ... }' fails with confusing errors because the
	// pattern parser (parseConPattern/parsePatternAtom) has no equivalent of tryQualifiedExpr.
	// The dot is consumed as part of subsequent tokens, leading to cascading errors about
	// expected '->'. If qualified imports are supported, qualified patterns should be too."
	prog, es := parse(`main := case x { N y -> y; _ -> 0 }`)
	if es.HasErrors() {
		t.Fatalf("unexpected error for basic pattern: %s", es.Format())
	}
	_ = prog

	// Now try with qualified constructor in pattern.
	_, es2 := parse(`main := case x { N.Just y -> y; _ -> 0 }`)
	if es2.HasErrors() {
		t.Logf("BUG CONFIRMED: qualified constructor in pattern causes cascading errors: %s", es2.Format())
	} else {
		t.Log("qualified constructor in pattern parsed without error — checking structure")
	}
}

// ====================================================
// forall keyword: "as" in import is contextual, verify no clash
// ====================================================

func TestProbeA_AsInImportVsAsInExpr(t *testing.T) {
	// "as" is not a keyword, so it can be used freely outside import context.
	source := `import M as N
as := 42
main := as`
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	if prog.Imports[0].Alias != "N" {
		t.Errorf("import alias: got %q, want N", prog.Imports[0].Alias)
	}
	// Find the value def for "as"
	foundAs := false
	for _, d := range prog.Decls {
		if vd, ok := d.(*DeclValueDef); ok && vd.Name == "as" {
			foundAs = true
		}
	}
	if !foundAs {
		t.Error("expected to find 'as := 42' declaration")
	}
}

// ====================================================
// Lexer: dot adjacency edge cases
// ====================================================

func TestProbeA_DotAdjacentToNumber(t *testing.T) {
	// "N.123" — how does the lexer tokenize this?
	// N → TokUpper, . → TokDot, 123 → TokIntLit
	tokens := lex("N.123")
	if len(tokens) < 3 {
		t.Fatalf("expected at least 3 tokens, got %d", len(tokens))
	}
	t.Logf("N.123 tokens: %v(%q) %v(%q) %v(%q)",
		tokens[0].Kind, tokens[0].Text,
		tokens[1].Kind, tokens[1].Text,
		tokens[2].Kind, tokens[2].Text)
	// Verify adjacency
	if !tokensAdjacent(tokens[0], tokens[1]) {
		t.Error("N and . should be adjacent in 'N.123'")
	}
	if !tokensAdjacent(tokens[1], tokens[2]) {
		t.Error(". and 123 should be adjacent in 'N.123'")
	}
}

func TestProbeA_DotImmediatelyBeforeEOF(t *testing.T) {
	// "N." — dot at end of input.
	tokens := lex("N.")
	if len(tokens) < 2 {
		t.Fatalf("expected at least 2 tokens, got %d", len(tokens))
	}
	t.Logf("N. tokens: %v(%q) %v(%q)",
		tokens[0].Kind, tokens[0].Text,
		tokens[1].Kind, tokens[1].Text)
	if tokens[0].Kind != TokUpper {
		t.Errorf("token[0]: expected TokUpper, got %v", tokens[0].Kind)
	}
	if tokens[1].Kind != TokDot {
		t.Errorf("token[1]: expected TokDot, got %v", tokens[1].Kind)
	}
}

// ====================================================
// Qualified type: only Upper.Upper, not Upper.lower
// ====================================================

func TestProbeA_QualTypeLowerAfterDot(t *testing.T) {
	// N.int in type position — the type parser only promotes Upper.Upper to TyExprQualCon.
	// N.int (lower after dot) would not match the qualified type check.
	ty, es := parseTypeOnly("N.int")
	// This should NOT be TyExprQualCon.
	t.Logf("N.int parsed as %T, errors=%v", ty, es.HasErrors())
	if es.HasErrors() {
		t.Logf("errors: %s", es.Format())
	}
	if _, ok := ty.(*TyExprQualCon); ok {
		t.Error("BUG: N.int (lowercase) parsed as TyExprQualCon")
	}
}

// ====================================================
// Stress: very long dotted module name
// ====================================================

func TestProbeA_ImportVeryDeeplyDottedModule(t *testing.T) {
	// import A.B.C.D.E.F.G.H.I.J (10 segments)
	var parts []string
	for c := 'A'; c <= 'J'; c++ {
		parts = append(parts, string(c))
	}
	moduleName := strings.Join(parts, ".")
	source := fmt.Sprintf("import %s\nmain := 1", moduleName)
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if prog.Imports[0].ModuleName != moduleName {
		t.Errorf("expected module %q, got %q", moduleName, prog.Imports[0].ModuleName)
	}
}
