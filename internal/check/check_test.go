package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/pkg/types"
)

func checkSource(t *testing.T, source string, config *CheckConfig) *core.Program {
	t.Helper()
	src := span.NewSource("test", source)
	l := syntax.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := syntax.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
	prog, checkErrs := Check(ast, src, config)
	if checkErrs.HasErrors() {
		t.Fatal("check errors:", checkErrs.Format())
	}
	return prog
}

func TestCheckDataDecl(t *testing.T) {
	prog := checkSource(t, "data Bool = True | False", nil)
	if len(prog.DataDecls) != 1 {
		t.Fatalf("expected 1 data decl, got %d", len(prog.DataDecls))
	}
	if prog.DataDecls[0].Name != "Bool" {
		t.Errorf("expected Bool, got %s", prog.DataDecls[0].Name)
	}
}

func TestCheckIdentity(t *testing.T) {
	source := `id := \x -> x`
	prog := checkSource(t, source, nil)
	if len(prog.Bindings) != 1 || prog.Bindings[0].Name != "id" {
		t.Fatal("expected binding 'id'")
	}
	_, ok := prog.Bindings[0].Expr.(*core.Lam)
	if !ok {
		t.Errorf("expected Lam, got %T", prog.Bindings[0].Expr)
	}
}

func TestCheckApplication(t *testing.T) {
	source := `data Bool = True | False
id := \x -> x
main := id True`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
}

func TestCheckAssumption(t *testing.T) {
	config := &CheckConfig{
		Assumptions: map[string]types.Type{
			"dbOpen": types.MkArrow(types.Con("Unit"), types.Con("Unit")),
		},
	}
	source := `data Unit = Unit
dbOpen := assumption`
	prog := checkSource(t, source, config)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "dbOpen" {
			if _, ok := b.Expr.(*core.PrimOp); ok {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected PrimOp for dbOpen")
	}
}

func TestCheckDoBlock(t *testing.T) {
	source := `data Unit = Unit
main := do { pure Unit }`
	prog := checkSource(t, source, nil)
	if len(prog.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(prog.Bindings))
	}
}

func TestCheckTypeAlias(t *testing.T) {
	// Test with inferred Computation type via pure.
	source := `data Unit = Unit
main := pure Unit`
	prog := checkSource(t, source, nil)
	if len(prog.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(prog.Bindings))
	}
}

func TestCheckHostBinding(t *testing.T) {
	config := &CheckConfig{
		Bindings:        map[string]types.Type{"x": types.Con("Int")},
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	source := `y := x`
	prog := checkSource(t, source, config)
	if len(prog.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(prog.Bindings))
	}
}

func TestCheckUnboundVar(t *testing.T) {
	src := span.NewSource("test", "main := undefined_var")
	l := syntax.NewLexer(src)
	tokens, _ := l.Tokenize()
	es := &errs.Errors{Source: src}
	p := syntax.NewParser(tokens, es)
	ast := p.ParseProgram()
	_, checkErrs := Check(ast, src, nil)
	if !checkErrs.HasErrors() {
		t.Error("expected type error for unbound variable")
	}
}

func TestUnifySimple(t *testing.T) {
	u := NewUnifier()
	if err := u.Unify(types.Con("Int"), types.Con("Int")); err != nil {
		t.Errorf("Int ~ Int should succeed: %v", err)
	}
	if err := u.Unify(types.Con("Int"), types.Con("Bool")); err == nil {
		t.Error("Int ~ Bool should fail")
	}
}

func TestUnifyMeta(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	if err := u.Unify(m, types.Con("Int")); err != nil {
		t.Errorf("?1 ~ Int should succeed: %v", err)
	}
	soln := u.Solve(1)
	if soln == nil {
		t.Fatal("?1 should be solved")
	}
	if con, ok := soln.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected Int, got %v", soln)
	}
}

func TestUnifyArrow(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	a := types.MkArrow(types.Con("Int"), m)
	b := types.MkArrow(types.Con("Int"), types.Con("Bool"))
	if err := u.Unify(a, b); err != nil {
		t.Errorf("should unify: %v", err)
	}
	if !types.Equal(u.Zonk(m), types.Con("Bool")) {
		t.Error("?1 should be Bool")
	}
}

func TestUnifyOccursCheck(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	if err := u.Unify(m, types.MkArrow(m, types.Con("Int"))); err == nil {
		t.Error("should fail: infinite type")
	}
}

func TestUnifyRow(t *testing.T) {
	u := NewUnifier()
	r1 := types.ClosedRow(types.RowField{Label: "a", Type: types.Con("Int")})
	r2 := types.ClosedRow(types.RowField{Label: "a", Type: types.Con("Int")})
	if err := u.Unify(r1, r2); err != nil {
		t.Errorf("identical rows should unify: %v", err)
	}
}

// checkSourceExpectError parses and type-checks source, expecting at least one error.
// Returns the formatted error string.
func checkSourceExpectError(t *testing.T, source string, config *CheckConfig) string {
	t.Helper()
	src := span.NewSource("test", source)
	l := syntax.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := syntax.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
	_, checkErrs := Check(ast, src, config)
	if !checkErrs.HasErrors() {
		t.Fatal("expected check errors, got none")
	}
	return checkErrs.Format()
}

func TestAliasCycleDirect(t *testing.T) {
	source := `type A = A`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "cyclic type alias") {
		t.Errorf("expected cyclic type alias error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "A -> A") {
		t.Errorf("expected cycle path A -> A, got: %s", errMsg)
	}
}

func TestAliasCycleMutual(t *testing.T) {
	source := `type A = B
type B = A`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "cyclic type alias") {
		t.Errorf("expected cyclic type alias error, got: %s", errMsg)
	}
}

func TestAliasNoCycle(t *testing.T) {
	// Eff references Computation, which is a built-in — not an alias.
	source := `type Eff r a = Computation r r a
data Unit = Unit
main := pure Unit`
	checkSource(t, source, nil)
}

// --- Exhaustiveness tests ---

func TestExhaustiveComplete(t *testing.T) {
	source := `data Bool = True | False
main := \b -> case b of { True -> True; False -> False }`
	checkSource(t, source, nil)
}

func TestExhaustiveIncomplete(t *testing.T) {
	source := `data Bool = True | False
main := \b -> case b of { True -> True }`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "non-exhaustive") {
		t.Errorf("expected non-exhaustive error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "False") {
		t.Errorf("expected missing constructor 'False' in error, got: %s", errMsg)
	}
}

func TestExhaustiveWildcard(t *testing.T) {
	source := `data Bool = True | False
main := \b -> case b of { _ -> True }`
	checkSource(t, source, nil)
}

func TestExhaustiveVarPattern(t *testing.T) {
	source := `data Bool = True | False
main := \b -> case b of { x -> x }`
	checkSource(t, source, nil)
}
