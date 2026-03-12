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

func TestUnifyRowOpenOpen(t *testing.T) {
	u := NewUnifier()

	// r1 = { a: Int, b: Bool | ?1 }
	// r2 = { a: Int, c: Str  | ?2 }
	// After unification:
	//   shared: a (Int ~ Int ok)
	//   onlyLeft:  b: Bool  (in r1 not r2)
	//   onlyRight: c: Str   (in r2 not r1)
	//   ?1 = { c: Str | ?fresh }
	//   ?2 = { b: Bool | ?fresh }
	m1 := &types.TyMeta{ID: 100, Kind: types.KRow{}}
	m2 := &types.TyMeta{ID: 101, Kind: types.KRow{}}

	r1 := types.OpenRow([]types.RowField{
		{Label: "a", Type: types.Con("Int")},
		{Label: "b", Type: types.Con("Bool")},
	}, m1)

	r2 := types.OpenRow([]types.RowField{
		{Label: "a", Type: types.Con("Int")},
		{Label: "c", Type: types.Con("Str")},
	}, m2)

	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("open-open row unification should succeed: %v", err)
	}

	// ?1 should be solved to { c: Str | ?fresh }
	soln1 := u.Zonk(m1)
	row1, ok := soln1.(*types.TyRow)
	if !ok {
		t.Fatalf("?1 should be solved to a row, got %T: %s", soln1, types.Pretty(soln1))
	}
	if len(row1.Fields) != 1 || row1.Fields[0].Label != "c" {
		t.Errorf("?1 should have field 'c', got %s", types.Pretty(row1))
	}
	if !types.Equal(row1.Fields[0].Type, types.Con("Str")) {
		t.Errorf("?1.c should be Str, got %s", types.Pretty(row1.Fields[0].Type))
	}
	if row1.Tail == nil {
		t.Error("?1 should have an open tail (the fresh meta)")
	}

	// ?2 should be solved to { b: Bool | ?fresh }
	soln2 := u.Zonk(m2)
	row2, ok := soln2.(*types.TyRow)
	if !ok {
		t.Fatalf("?2 should be solved to a row, got %T: %s", soln2, types.Pretty(soln2))
	}
	if len(row2.Fields) != 1 || row2.Fields[0].Label != "b" {
		t.Errorf("?2 should have field 'b', got %s", types.Pretty(row2))
	}
	if !types.Equal(row2.Fields[0].Type, types.Con("Bool")) {
		t.Errorf("?2.b should be Bool, got %s", types.Pretty(row2.Fields[0].Type))
	}
	if row2.Tail == nil {
		t.Error("?2 should have an open tail (the fresh meta)")
	}

	// Both tails should be the same fresh metavariable.
	if row1.Tail != nil && row2.Tail != nil {
		tail1, ok1 := row1.Tail.(*types.TyMeta)
		tail2, ok2 := row2.Tail.(*types.TyMeta)
		if ok1 && ok2 {
			if tail1.ID != tail2.ID {
				t.Errorf("both row tails should share the same fresh meta, got ?%d and ?%d", tail1.ID, tail2.ID)
			}
		}
	}
}

func TestUnifyRowOpenOpenShared(t *testing.T) {
	// Open-Open where both rows have the same labels → tails unify to same fresh.
	u := NewUnifier()

	m1 := &types.TyMeta{ID: 200, Kind: types.KRow{}}
	m2 := &types.TyMeta{ID: 201, Kind: types.KRow{}}

	r1 := types.OpenRow([]types.RowField{
		{Label: "x", Type: types.Con("Int")},
	}, m1)

	r2 := types.OpenRow([]types.RowField{
		{Label: "x", Type: types.Con("Int")},
	}, m2)

	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("open-open row unification (same labels) should succeed: %v", err)
	}

	// Both tails should point to the same fresh meta (with no extra fields).
	soln1 := u.Zonk(m1)
	soln2 := u.Zonk(m2)

	// When both rows have identical fields, the solutions should both be a row
	// with no extra fields and a shared fresh tail.
	row1, ok1 := soln1.(*types.TyRow)
	row2, ok2 := soln2.(*types.TyRow)
	if ok1 && ok2 {
		if len(row1.Fields) != 0 {
			t.Errorf("?200 should have no extra fields, got %s", types.Pretty(row1))
		}
		if len(row2.Fields) != 0 {
			t.Errorf("?201 should have no extra fields, got %s", types.Pretty(row2))
		}
	}
}

func TestUnifyRowOpenOpenDisjoint(t *testing.T) {
	// Open-Open where rows have entirely different labels.
	u := NewUnifier()

	m1 := &types.TyMeta{ID: 300, Kind: types.KRow{}}
	m2 := &types.TyMeta{ID: 301, Kind: types.KRow{}}

	r1 := types.OpenRow([]types.RowField{
		{Label: "a", Type: types.Con("Int")},
	}, m1)

	r2 := types.OpenRow([]types.RowField{
		{Label: "b", Type: types.Con("Bool")},
	}, m2)

	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("open-open row unification (disjoint labels) should succeed: %v", err)
	}

	// ?1 = { b: Bool | ?fresh }
	soln1 := u.Zonk(m1)
	row1, ok := soln1.(*types.TyRow)
	if !ok {
		t.Fatalf("?300 should be solved to a row, got %s", types.Pretty(soln1))
	}
	if len(row1.Fields) != 1 || row1.Fields[0].Label != "b" {
		t.Errorf("?300 should have field 'b', got %s", types.Pretty(row1))
	}

	// ?2 = { a: Int | ?fresh }
	soln2 := u.Zonk(m2)
	row2, ok := soln2.(*types.TyRow)
	if !ok {
		t.Fatalf("?301 should be solved to a row, got %s", types.Pretty(soln2))
	}
	if len(row2.Fields) != 1 || row2.Fields[0].Label != "a" {
		t.Errorf("?301 should have field 'a', got %s", types.Pretty(row2))
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

// --- Instance resolution tests ---

func TestResolveMissingInstanceError(t *testing.T) {
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
f :: forall a. Eq a => a -> a -> Bool
f := \x -> \y -> eq x y
main := f True False`
	// Should fail: no instance Eq Bool defined.
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "no instance") {
		t.Errorf("expected 'no instance' error, got: %s", errMsg)
	}
}

func TestResolveSimpleInstance(t *testing.T) {
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x -> \y -> True }
f :: forall a. Eq a => a -> a -> Bool
f := \x -> \y -> eq x y
main := f True False`
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

func TestResolveContextualInstance(t *testing.T) {
	source := `data Bool = True | False
data Maybe a = Just a | Nothing
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x -> \y -> True }
instance Eq a => Eq (Maybe a) { eq := \x -> \y -> True }
f :: forall a. Eq a => a -> a -> Bool
f := \x -> \y -> eq x y
main := f (Just True) (Just False)`
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

// --- Exhaustiveness tests ---

// --- Type class elaboration tests ---

func TestClassElaboratesDataDecl(t *testing.T) {
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }`
	prog := checkSource(t, source, nil)
	// Should have generated Eq$Dict data declaration.
	found := false
	for _, d := range prog.DataDecls {
		if d.Name == "Eq$Dict" {
			found = true
			if len(d.Cons) != 1 || d.Cons[0].Name != "Eq$Dict" {
				t.Errorf("expected single constructor Eq$Dict")
			}
			if len(d.TyParams) != 1 {
				t.Errorf("expected 1 type param, got %d", len(d.TyParams))
			}
		}
	}
	if !found {
		t.Error("expected Eq$Dict data declaration")
	}
}

func TestClassElaboratesSelectors(t *testing.T) {
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }`
	prog := checkSource(t, source, nil)
	// Should have generated eq binding (selector).
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "eq" {
			found = true
			// eq :: forall a. Eq$Dict a -> a -> a -> Bool
			// Elaborated as a Lam that pattern-matches the dict.
		}
	}
	if !found {
		t.Error("expected 'eq' selector binding")
	}
}

func TestClassMethodInScope(t *testing.T) {
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
f :: Eq a => a -> a -> Bool
f := \x y -> eq x y`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "f" {
			found = true
		}
	}
	if !found {
		t.Error("expected binding 'f'")
	}
}

func TestSuperclassDictField(t *testing.T) {
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }`
	prog := checkSource(t, source, nil)
	found := false
	for _, d := range prog.DataDecls {
		if d.Name == "Ord$Dict" {
			found = true
			// First field should be Eq$Dict a (superclass dict)
			if len(d.Cons) != 1 {
				t.Fatalf("expected 1 constructor")
			}
			con := d.Cons[0]
			if len(con.Fields) != 2 { // Eq$Dict a, then a -> a -> Bool
				t.Errorf("expected 2 fields (super dict + method), got %d", len(con.Fields))
			}
		}
	}
	if !found {
		t.Error("expected Ord$Dict data declaration")
	}
}

func TestInstanceElaboratesBinding(t *testing.T) {
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "Eq$Bool" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Eq$Bool' dictionary binding")
	}
}

func TestInstanceWithContextElaborates(t *testing.T) {
	// instance Eq a => Eq (Maybe a) → dictionary function
	source := `data Bool = True | False
data Maybe a = Just a | Nothing
class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq (Maybe a) { eq := \x y -> True }`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "Eq$Maybe" {
			found = true
			// Should be a lambda (dict function) since it has context.
			if _, ok := b.Expr.(*core.Lam); !ok {
				t.Errorf("expected Lam for contextual instance, got %T", b.Expr)
			}
		}
	}
	if !found {
		t.Error("expected 'Eq$Maybe' dictionary function binding")
	}
}

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
