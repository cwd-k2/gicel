//go:build legacy_syntax

package parse

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// parseMustSucceed is a helper that parses source and fails on any error.
func parseMustSucceed(t *testing.T, source string) *AstProgram {
	t.Helper()
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected parse error: %s", es.Format())
	}
	return prog
}

// parseMustFail is a helper that parses source and expects at least one error.
func parseMustFail(t *testing.T, source string) string {
	t.Helper()
	_, es := parse(source)
	if !es.HasErrors() {
		t.Fatal("expected parse error, got none")
	}
	return es.Format()
}

// ==========================================
// Pathological Parser Inputs
// ==========================================

// (a) Extremely long type family name (1000+ characters)
func TestPathologicalLongTypeFamilyName(t *testing.T) {
	name := strings.Repeat("A", 1024)
	source := fmt.Sprintf(`data Unit := Unit
type %s (a: Type) :: Type := {
  %s a =: Unit
}`, name, name)
	prog := parseMustSucceed(t, source)
	// Verify the long name round-trips.
	found := false
	for _, d := range prog.Decls {
		if tf, ok := d.(*DeclTypeFamily); ok && tf.Name == name {
			found = true
			if len(tf.Equations) != 1 {
				t.Errorf("expected 1 equation, got %d", len(tf.Equations))
			}
			if tf.Equations[0].Name != name {
				t.Errorf("equation name mismatch: got %q", tf.Equations[0].Name)
			}
		}
	}
	if !found {
		t.Error("type family with long name not found in AST")
	}
}

// (b) Type family equation with 100 pattern parameters
func TestPathological100PatternParams(t *testing.T) {
	var paramDecls strings.Builder
	var eqPatterns strings.Builder
	for i := 0; i < 100; i++ {
		if i > 0 {
			paramDecls.WriteString(" ")
			eqPatterns.WriteString(" ")
		}
		paramDecls.WriteString(fmt.Sprintf("(p%d: Type)", i))
		eqPatterns.WriteString(fmt.Sprintf("p%d", i))
	}
	source := fmt.Sprintf(`data Unit := Unit
type F %s :: Type := {
  F %s =: Unit
}`, paramDecls.String(), eqPatterns.String())
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if tf, ok := d.(*DeclTypeFamily); ok && tf.Name == "F" {
			if len(tf.Params) != 100 {
				t.Errorf("expected 100 params, got %d", len(tf.Params))
			}
			if len(tf.Equations) != 1 {
				t.Fatalf("expected 1 equation, got %d", len(tf.Equations))
			}
			if len(tf.Equations[0].Patterns) != 100 {
				t.Errorf("expected 100 equation patterns, got %d", len(tf.Equations[0].Patterns))
			}
		}
	}
}

// (c) Deeply nested type expression in type family RHS: Maybe (Maybe (Maybe ... Int ...)) 50 levels
func TestPathologicalDeeplyNestedRHS(t *testing.T) {
	// Build: Maybe (Maybe (Maybe ... (Maybe Int) ...))
	rhs := "Int"
	for i := 0; i < 50; i++ {
		rhs = fmt.Sprintf("(Maybe %s)", rhs)
	}
	source := fmt.Sprintf(`data Maybe a := Nothing | Just a
type F (a: Type) :: Type := {
  F a =: %s
}`, rhs)
	prog := parseMustSucceed(t, source)
	found := false
	for _, d := range prog.Decls {
		if tf, ok := d.(*DeclTypeFamily); ok && tf.Name == "F" {
			found = true
			if len(tf.Equations) != 1 {
				t.Errorf("expected 1 equation, got %d", len(tf.Equations))
			}
		}
	}
	if !found {
		t.Error("type family F not found in AST")
	}
}

// (d) Empty class body with fundeps
func TestPathologicalEmptyClassBodyWithFunDeps(t *testing.T) {
	source := `class C a b | a =: b {}`
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if cl, ok := d.(*DeclClass); ok && cl.Name == "C" {
			if len(cl.FunDeps) != 1 {
				t.Errorf("expected 1 fundep, got %d", len(cl.FunDeps))
			}
			if len(cl.Methods) != 0 {
				t.Errorf("expected 0 methods, got %d", len(cl.Methods))
			}
		}
	}
}

// (e) Type family equation name is a lowercase identifier (should error)
func TestPathologicalLowercaseEquationName(t *testing.T) {
	// The equation parser calls expectUpper(), so a lowercase name
	// will produce a parse error.
	source := `data Unit := Unit
type F (a: Type) :: Type := {
  f a =: Unit
}`
	errMsg := parseMustFail(t, source)
	if !strings.Contains(errMsg, "uppercase") {
		t.Errorf("expected 'uppercase' in error, got: %s", errMsg)
	}
}

// (f) Fundep with no "to" parameters: | a =:
// BUG FOUND: parseFunDepList does not validate that the "to" list is non-empty.
// A fundep `| a =: {}` silently produces FunDep{From: "a", To: nil},
// which is a fundep that determines nothing — semantically meaningless.
// This test documents the current (buggy) behavior.
func TestPathologicalFunDepNoToParams(t *testing.T) {
	source := `class C a b | a =: {
  m :: a -> b
}`
	// Current behavior: parses successfully with an empty "to" list.
	// This is a bug — the parser should reject empty "to" in fundeps.
	prog, es := parse(source)
	if es.HasErrors() {
		// If the parser is ever fixed to reject this, that's correct.
		t.Logf("parser correctly rejects empty fundep target: %s", es.Format())
		return
	}
	// Document the bug: the fundep has no "to" parameters.
	for _, d := range prog.Decls {
		if cl, ok := d.(*DeclClass); ok && cl.Name == "C" {
			if len(cl.FunDeps) != 1 {
				t.Fatalf("expected 1 fundep, got %d", len(cl.FunDeps))
			}
			if len(cl.FunDeps[0].To) != 0 {
				t.Fatalf("expected empty to-list, got %v", cl.FunDeps[0].To)
			}
			t.Log("BUG CONFIRMED: fundep with empty 'to' list accepted by parser")
		}
	}
}

// (g) Associated type with same name as the class
func TestPathologicalAssocTypeSameNameAsClass(t *testing.T) {
	// Syntactically valid — the parser doesn't forbid same-name assoc types.
	// The checker might catch this, but the parser should not crash.
	source := `class MyClass a {
  type MyClass a :: Type
}`
	// This should parse successfully (no crash).
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if cl, ok := d.(*DeclClass); ok && cl.Name == "MyClass" {
			if len(cl.AssocTypes) != 1 {
				t.Errorf("expected 1 assoc type, got %d", len(cl.AssocTypes))
			}
			if cl.AssocTypes[0].Name != "MyClass" {
				t.Errorf("expected assoc type name MyClass, got %s", cl.AssocTypes[0].Name)
			}
		}
	}
}

// (h) Multiple pipe separators: class C a b | a =: b | b =: a {}
func TestPathologicalMultipleFunDeps(t *testing.T) {
	// The GICEL syntax uses comma-separated fundeps after a single pipe.
	// The second | would not be recognized as starting new fundeps.
	// Syntax: class C a b | a =: b, b =: a {}
	// Let's test the comma form.
	source := `class C a b | a =: b, b =: a {}`
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if cl, ok := d.(*DeclClass); ok && cl.Name == "C" {
			if len(cl.FunDeps) != 2 {
				t.Errorf("expected 2 fundeps, got %d", len(cl.FunDeps))
			}
		}
	}
}

// Test the actual double-pipe case: class C a b | a =: b | b =: a {}
// The second | should cause confusion in the parser.
func TestPathologicalDoublePipeFunDeps(t *testing.T) {
	source := `class C a b | a =: b | b =: a {}`
	// This might parse but misinterpret the second |,
	// or it might fail. Either way, it should not crash.
	_, es := parse(source)
	// Record the behavior: we mainly care that there is no panic.
	_ = es
}

// (i) Type family equation with no RHS (just patterns, missing =:)
func TestPathologicalEquationNoRHS(t *testing.T) {
	source := `data Unit := Unit
type F (a: Type) :: Type := {
  F Unit
}`
	errMsg := parseMustFail(t, source)
	if !strings.Contains(errMsg, "=:") && !strings.Contains(errMsg, "expect") {
		t.Errorf("expected error about missing '=:' or 'expect', got: %s", errMsg)
	}
}

// (j) Instance with both assoc type and assoc data of same name
// This is structurally odd but the parser processes them independently.
func TestPathologicalAssocTypeAndDataSameName(t *testing.T) {
	source := `data Unit := Unit
class C a {
  type Elem a :: Type;
  data Elem a :: Type
}
instance C Unit {
  type Elem Unit =: Unit;
  data Elem Unit =: ElemCon
}`
	// The parser should handle this without crashing.
	// Whether or not it's semantically valid is the checker's business.
	_, es := parse(source)
	// Just verify no panic. If there are parse errors that's acceptable.
	_ = es
}

// --- Additional parser pathological tests ---

// Test the parser safety harness: does maxRecurseDepth prevent stack overflow?
func TestPathologicalDeepParenNesting(t *testing.T) {
	// Build (((((...))))) with 300 levels of nesting.
	var b strings.Builder
	for i := 0; i < 300; i++ {
		b.WriteByte('(')
	}
	b.WriteString("x")
	for i := 0; i < 300; i++ {
		b.WriteByte(')')
	}
	source := fmt.Sprintf("main := %s", b.String())
	src := span.NewSource("test", source)
	l := NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), tokens, es)
	// This should not stack overflow — the parser has a recurseDepth guard.
	_ = p.ParseProgram()
	// We just care that there's no panic; errors are acceptable.
}

// Test the parser step limit: a token stream that causes lots of reprocessing.
func TestPathologicalManyDeclarations(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString(fmt.Sprintf("data T%d := C%d\n", i, i))
	}
	prog := parseMustSucceed(t, b.String())
	if len(prog.Decls) != 200 {
		t.Errorf("expected 200 decls, got %d", len(prog.Decls))
	}
}

// Equation with only uppercase name and no patterns and no =: (bare)
func TestPathologicalBareEquationName(t *testing.T) {
	source := `data Unit := Unit
type F :: Type := {
  F
}`
	// F alone with no =: should hit the expect(TokEqColon) error path.
	errMsg := parseMustFail(t, source)
	_ = errMsg
}

// TestParseBodyIterationLimit verifies that a very long instance body
// that would have previously caused an infinite loop (V6 pattern)
// terminates with an error rather than hanging.
func TestParseBodyIterationLimit(t *testing.T) {
	// Construct an instance with a very long body of malformed method definitions.
	// This exercises the parseBody iteration limit guard.
	var b strings.Builder
	b.WriteString("data Unit := Unit\n")
	b.WriteString("class C a { m :: a -> a }\n")
	b.WriteString("instance C Unit {\n")
	for i := 0; i < 5000; i++ {
		// Each line is a malformed method: lowercase name followed by garbage.
		// The parser will attempt to parse each, fail, and recover.
		b.WriteString(fmt.Sprintf("  x%d @@@ ~~~\n", i))
	}
	b.WriteString("}\n")

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = parse(b.String())
	}()
	select {
	case <-done:
		// Terminated — success regardless of parse errors.
	case <-time.After(10 * time.Second):
		t.Fatal("parseBody iteration limit did not prevent hang within 10s")
	}
}
