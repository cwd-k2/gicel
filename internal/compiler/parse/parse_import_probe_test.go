//go:build probe

// Import probe tests — open, selective, qualified, edge cases.
// Does NOT cover: parse_decl_probe_test.go, parse_expr_probe_test.go.
package parse

import (
	"fmt"
	"strings"
	"testing"
)

// ===== From probe_b =====

// TestProbeB_ImportEmptySelective checks import M () — empty selective import.
func TestProbeB_ImportEmptySelective(t *testing.T) {
	prog := parseMustSucceed(t, "import M ()\nmain := 1")
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	imp := prog.Imports[0]
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

// ===== From probe_d =====

// TestProbeD_ImportNoModuleName verifies `import` with nothing after it.
func TestProbeD_ImportNoModuleName(t *testing.T) {
	_, es := parse("import")
	if !es.HasErrors() {
		t.Error("expected error for `import` with no module name")
	}
}

// TestProbeD_ImportLowercaseName verifies `import foo` is rejected (expects Upper).
func TestProbeD_ImportLowercaseName(t *testing.T) {
	_, es := parse("import foo")
	if !es.HasErrors() {
		t.Error("expected error for `import foo` (lowercase module name)")
	}
}

// TestProbeD_ImportDottedNameEOF verifies `import M.` (dot at boundary).
func TestProbeD_ImportDottedNameEOF(t *testing.T) {
	_, es := parse("import M.")
	if !es.HasErrors() {
		t.Error("expected error for `import M.` (no name after dot)")
	}
}

// TestProbeD_ImportAsNoAlias verifies `import M as` with no alias.
func TestProbeD_ImportAsNoAlias(t *testing.T) {
	_, es := parse("import M as")
	if !es.HasErrors() {
		t.Error("expected error for `import M as` (no alias)")
	}
}

// TestProbeD_ImportAsLowercase verifies `import M as n` (lowercase alias).
func TestProbeD_ImportAsLowercase(t *testing.T) {
	_, es := parse("import M as n")
	// `as` is matched by TokLower + text=="as", then expectUpper() for alias.
	// `n` is lowercase, so expectUpper should fail.
	if !es.HasErrors() {
		t.Error("expected error for lowercase alias in qualified import")
	}
}

// TestProbeD_ImportEmptyParens verifies `import M ()` — selective import with no names.
func TestProbeD_ImportEmptyParens(t *testing.T) {
	prog, es := parse("import M ()")
	if es.HasErrors() {
		t.Logf("import M (): %s", es.Format())
		return
	}
	// Parser should produce a DeclImport with empty Names slice (non-nil).
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	imp := prog.Imports[0]
	if imp.Names == nil {
		t.Fatal("import M () produced nil Names, indistinguishable from open import")
	}
	if len(imp.Names) != 0 {
		t.Errorf("expected 0 names in import M (), got %d", len(imp.Names))
	}
}

// TestProbeD_ImportDottedMultiple verifies `import A.B.C` — deep dotted module name.
func TestProbeD_ImportDottedMultiple(t *testing.T) {
	prog, es := parse("import A.B.C")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	if prog.Imports[0].ModuleName != "A.B.C" {
		t.Errorf("expected module name A.B.C, got %q", prog.Imports[0].ModuleName)
	}
}

// TestProbeD_ImportSelectiveOperator verifies `import M ((+))`.
func TestProbeD_ImportSelectiveOperator(t *testing.T) {
	prog, es := parse("import M ((+))")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	names := prog.Imports[0].Names
	if len(names) != 1 || names[0].Name != "+" {
		t.Errorf("expected import of operator +, got %v", names)
	}
}

// TestProbeD_ImportSelectiveDot verifies `import M ((.))` — importing the dot operator.
func TestProbeD_ImportSelectiveDot(t *testing.T) {
	prog, es := parse("import M ((.))") // (.) in import list
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	names := prog.Imports[0].Names
	if len(names) != 1 || names[0].Name != "." {
		t.Errorf("expected import of operator '.', got %v", names)
	}
}

// ===== From probe_e =====

// TestProbeE_ImportBasic verifies basic open import.
func TestProbeE_ImportBasic(t *testing.T) {
	source := `import Prelude
main := 1`
	prog := parseMustSucceed(t, source)
	if len(prog.Imports) != 1 || prog.Imports[0].ModuleName != "Prelude" {
		t.Error("expected import Prelude")
	}
}

// TestProbeE_ImportQualified verifies qualified import.
func TestProbeE_ImportQualified(t *testing.T) {
	source := `import Prelude as P`
	prog := parseMustSucceed(t, source)
	if prog.Imports[0].Alias != "P" {
		t.Errorf("expected alias P, got %q", prog.Imports[0].Alias)
	}
}

// TestProbeE_ImportSelective verifies selective import.
func TestProbeE_ImportSelective(t *testing.T) {
	prog := parseMustSucceed(t, "import Prelude (map, Bool(..))\nmain := 1")
	if len(prog.Imports[0].Names) != 2 {
		t.Fatalf("expected 2 import names, got %d", len(prog.Imports[0].Names))
	}
	if prog.Imports[0].Names[0].Name != "map" {
		t.Errorf("expected 'map', got %s", prog.Imports[0].Names[0].Name)
	}
	if !prog.Imports[0].Names[1].AllSubs {
		t.Error("expected Bool(..) to have AllSubs=true")
	}
}

// TestProbeE_ImportSelectiveEmpty verifies import M () — empty selective list.
func TestProbeE_ImportSelectiveEmpty(t *testing.T) {
	prog := parseMustSucceed(t, "import M ()\nmain := 1")
	if prog.Imports[0].Names == nil {
		t.Error("expected non-nil Names for import M ()")
	}
	if len(prog.Imports[0].Names) != 0 {
		t.Errorf("expected 0 import names, got %d", len(prog.Imports[0].Names))
	}
}

// TestProbeE_ImportOperator verifies operator import.
func TestProbeE_ImportOperator(t *testing.T) {
	prog := parseMustSucceed(t, `import M ((+))`)
	names := prog.Imports[0].Names
	if len(names) != 1 || names[0].Name != "+" {
		t.Errorf("expected +, got %v", names)
	}
}

// TestProbeE_ImportDottedModule verifies dotted module import.
func TestProbeE_ImportDottedModule(t *testing.T) {
	prog := parseMustSucceed(t, "import A.B.C")
	if prog.Imports[0].ModuleName != "A.B.C" {
		t.Errorf("expected A.B.C, got %q", prog.Imports[0].ModuleName)
	}
}

// TestProbeE_ImportMalformedModuleName verifies `import 123`.
func TestProbeE_ImportMalformedModuleName(t *testing.T) {
	_, es := parse("import 123")
	if !es.HasErrors() {
		t.Error("expected error for malformed module name")
	}
}

// TestProbeE_ImportNoModuleName verifies `import` alone.
func TestProbeE_ImportNoModuleName(t *testing.T) {
	_, es := parse("import")
	if !es.HasErrors() {
		t.Error("expected error for import with no module name")
	}
}

// TestProbeE_ImportDuplicates verifies duplicate imports are accepted by the parser.
func TestProbeE_ImportDuplicates(t *testing.T) {
	source := `import A
import A`
	prog := parseMustSucceed(t, source)
	if len(prog.Imports) != 2 {
		t.Errorf("expected 2 imports, got %d", len(prog.Imports))
	}
}

// TestProbeE_ImportSelectiveSubList verifies import M (T(A, B)).
func TestProbeE_ImportSelectiveSubList(t *testing.T) {
	prog := parseMustSucceed(t, "import M (Maybe(Nothing, Just))\nmain := 1")
	names := prog.Imports[0].Names
	if len(names) != 1 {
		t.Fatalf("expected 1 import name, got %d", len(names))
	}
	if !names[0].HasSub {
		t.Error("expected HasSub=true")
	}
	if names[0].AllSubs {
		t.Error("expected AllSubs=false for explicit sub-list")
	}
	if len(names[0].SubList) != 2 {
		t.Errorf("expected 2 subs, got %d", len(names[0].SubList))
	}
}

// TestProbeE_ImportAfterDecl verifies that imports after declarations are NOT parsed as imports.
// The parser collects imports first, then declarations.
func TestProbeE_ImportAfterDecl(t *testing.T) {
	source := `main := 1
import Prelude`
	prog, es := parse(source)
	// The second import should be parsed as a declaration, which will fail.
	// Imports are only collected before the first non-import declaration.
	if len(prog.Imports) != 0 {
		t.Errorf("expected 0 imports (import after decl), got %d", len(prog.Imports))
	}
	if !es.HasErrors() {
		t.Error("expected error for import after declaration")
	}
}

// TestProbeE_ImportDotOperator verifies import M ((.)) works.
func TestProbeE_ImportDotOperator(t *testing.T) {
	prog := parseMustSucceed(t, "import M ((.));\nmain := 1")
	if len(prog.Imports[0].Names) != 1 {
		t.Fatalf("expected 1 import name, got %d", len(prog.Imports[0].Names))
	}
	if prog.Imports[0].Names[0].Name != "." {
		t.Errorf("expected '.', got %q", prog.Imports[0].Names[0].Name)
	}
}
