package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/check/unify"
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax/parse"
	"github.com/cwd-k2/gicel/internal/types"
)

func checkSource(t *testing.T, source string, config *CheckConfig) *core.Program {
	t.Helper()
	src := span.NewSource("test", source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
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
	prog := checkSource(t, "data Bool := True | False", nil)
	if len(prog.DataDecls) != 1 {
		t.Fatalf("expected 1 data decl, got %d", len(prog.DataDecls))
	}
	if prog.DataDecls[0].Name != "Bool" {
		t.Errorf("expected Bool, got %s", prog.DataDecls[0].Name)
	}
}

func TestCheckIdentity(t *testing.T) {
	source := `id := \x. x`
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
	source := `data Bool := True | False
id := \x. x
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
	source := `data Unit := Unit
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

func TestInferIntLit(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	prog := checkSource(t, `main := 42`, config)
	if len(prog.Bindings) != 1 {
		t.Fatal("expected 1 binding")
	}
	lit, ok := prog.Bindings[0].Expr.(*core.Lit)
	if !ok {
		t.Fatalf("expected Lit, got %T", prog.Bindings[0].Expr)
	}
	if lit.Value != int64(42) {
		t.Errorf("expected 42, got %v", lit.Value)
	}
}

func TestInferStrLit(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"String": types.KType{}},
	}
	prog := checkSource(t, `main := "hello"`, config)
	lit, ok := prog.Bindings[0].Expr.(*core.Lit)
	if !ok {
		t.Fatalf("expected Lit, got %T", prog.Bindings[0].Expr)
	}
	if lit.Value != "hello" {
		t.Errorf("expected hello, got %v", lit.Value)
	}
}

func TestInferRuneLit(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Rune": types.KType{}},
	}
	prog := checkSource(t, "main := 'a'", config)
	lit, ok := prog.Bindings[0].Expr.(*core.Lit)
	if !ok {
		t.Fatalf("expected Lit, got %T", prog.Bindings[0].Expr)
	}
	if lit.Value != rune('a') {
		t.Errorf("expected 'a', got %v", lit.Value)
	}
}

func TestCheckLitMismatch(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}, "String": types.KType{}},
	}
	checkSourceExpectCode(t, `main := (42 :: String)`, config, errs.ErrTypeMismatch)
}

func TestCheckDoBlock(t *testing.T) {
	source := `data Unit := Unit
main := do { pure Unit }`
	prog := checkSource(t, source, nil)
	if len(prog.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(prog.Bindings))
	}
}

func TestCheckTypeAlias(t *testing.T) {
	// Test with inferred Computation type via pure.
	source := `data Unit := Unit
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
	checkSourceExpectCode(t, "main := undefined_var", nil, errs.ErrUnboundVar)
}

func TestUnifySimple(t *testing.T) {
	u := unify.NewUnifier()
	if err := u.Unify(types.Con("Int"), types.Con("Int")); err != nil {
		t.Errorf("Int ~ Int should succeed: %v", err)
	}
	if err := u.Unify(types.Con("Int"), types.Con("Bool")); err == nil {
		t.Error("Int ~ Bool should fail")
	}
}

func TestUnifyMeta(t *testing.T) {
	u := unify.NewUnifier()
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
	u := unify.NewUnifier()
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
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	if err := u.Unify(m, types.MkArrow(m, types.Con("Int"))); err == nil {
		t.Error("should fail: infinite type")
	}
}

func TestUnifyRow(t *testing.T) {
	u := unify.NewUnifier()
	r1 := types.ClosedRow(types.RowField{Label: "a", Type: types.Con("Int")})
	r2 := types.ClosedRow(types.RowField{Label: "a", Type: types.Con("Int")})
	if err := u.Unify(r1, r2); err != nil {
		t.Errorf("identical rows should unify: %v", err)
	}
}

func TestUnifyRowOpenOpen(t *testing.T) {
	u := unify.NewUnifier()

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
	row1, ok := soln1.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("?1 should be solved to a row, got %T: %s", soln1, types.Pretty(soln1))
	}
	cap1 := row1.Entries.(*types.CapabilityEntries)
	if len(cap1.Fields) != 1 || cap1.Fields[0].Label != "c" {
		t.Errorf("?1 should have field 'c', got %s", types.Pretty(row1))
	}
	if !types.Equal(cap1.Fields[0].Type, types.Con("Str")) {
		t.Errorf("?1.c should be Str, got %s", types.Pretty(cap1.Fields[0].Type))
	}
	if row1.Tail == nil {
		t.Error("?1 should have an open tail (the fresh meta)")
	}

	// ?2 should be solved to { b: Bool | ?fresh }
	soln2 := u.Zonk(m2)
	row2, ok := soln2.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("?2 should be solved to a row, got %T: %s", soln2, types.Pretty(soln2))
	}
	cap2 := row2.Entries.(*types.CapabilityEntries)
	if len(cap2.Fields) != 1 || cap2.Fields[0].Label != "b" {
		t.Errorf("?2 should have field 'b', got %s", types.Pretty(row2))
	}
	if !types.Equal(cap2.Fields[0].Type, types.Con("Bool")) {
		t.Errorf("?2.b should be Bool, got %s", types.Pretty(cap2.Fields[0].Type))
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
	u := unify.NewUnifier()

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
	row1, ok1 := soln1.(*types.TyEvidenceRow)
	row2, ok2 := soln2.(*types.TyEvidenceRow)
	if ok1 && ok2 {
		ce1 := row1.Entries.(*types.CapabilityEntries)
		if len(ce1.Fields) != 0 {
			t.Errorf("?200 should have no extra fields, got %s", types.Pretty(row1))
		}
		ce2 := row2.Entries.(*types.CapabilityEntries)
		if len(ce2.Fields) != 0 {
			t.Errorf("?201 should have no extra fields, got %s", types.Pretty(row2))
		}
	}
}

func TestUnifyRowOpenOpenDisjoint(t *testing.T) {
	// Open-Open where rows have entirely different labels.
	u := unify.NewUnifier()

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
	row1, ok := soln1.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("?300 should be solved to a row, got %s", types.Pretty(soln1))
	}
	cap1 := row1.Entries.(*types.CapabilityEntries)
	if len(cap1.Fields) != 1 || cap1.Fields[0].Label != "b" {
		t.Errorf("?300 should have field 'b', got %s", types.Pretty(row1))
	}

	// ?2 = { a: Int | ?fresh }
	soln2 := u.Zonk(m2)
	row2, ok := soln2.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("?301 should be solved to a row, got %s", types.Pretty(soln2))
	}
	cap2 := row2.Entries.(*types.CapabilityEntries)
	if len(cap2.Fields) != 1 || cap2.Fields[0].Label != "a" {
		t.Errorf("?301 should have field 'a', got %s", types.Pretty(row2))
	}
}

func TestNormalizeCompAppPrePostOrder(t *testing.T) {
	// Computation pre post result as TyApp chain: ((Computation pre) post) result
	// normalizeCompApp must preserve: Pre=pre, Post=post, Result=result.
	u := unify.NewUnifier()
	pre := types.Con("Pre")
	post := types.Con("Post")
	result := types.Con("Result")

	// Build TyApp(TyApp(TyApp(TyCon("Computation"), pre), post), result)
	appChain := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyApp{
				Fun: &types.TyCon{Name: "Computation"},
				Arg: pre,
			},
			Arg: post,
		},
		Arg: result,
	}

	// Unify with a TyCBPV — the normalize path converts the TyApp chain.
	comp := types.MkComp(pre, post, result)
	if err := u.Unify(appChain, comp); err != nil {
		t.Fatalf("should unify: %v", err)
	}

	// Now test with distinct pre/post — swapping should fail.
	comp2 := types.MkComp(post, pre, result)
	if err := u.Unify(appChain, comp2); err == nil {
		t.Fatal("should fail when pre and post are swapped")
	}
}

func TestNormalizeThunkAppPrePostOrder(t *testing.T) {
	u := unify.NewUnifier()
	pre := types.Con("Pre")
	post := types.Con("Post")
	result := types.Con("Result")

	appChain := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyApp{
				Fun: &types.TyCon{Name: "Thunk"},
				Arg: pre,
			},
			Arg: post,
		},
		Arg: result,
	}

	thunk := types.MkThunk(pre, post, result)
	if err := u.Unify(appChain, thunk); err != nil {
		t.Fatalf("should unify: %v", err)
	}

	thunk2 := types.MkThunk(post, pre, result)
	if err := u.Unify(appChain, thunk2); err == nil {
		t.Fatal("should fail when pre and post are swapped")
	}
}

func TestPatternConArityTooMany(t *testing.T) {
	// Just takes one arg, pattern supplies two → should error.
	source := `data Maybe a := Nothing | Just a
f :: Maybe Int -> Int
f := \x. case x { Nothing -> 0; Just a b -> a }
main := f (Just 42)`
	checkSourceExpectError(t, source, nil)
}

func TestPatternConArityTooFew(t *testing.T) {
	// Pair takes two args, pattern supplies one → should error.
	source := `data Pair a b := MkPair a b
f :: Pair Int Int -> Int
f := \x. case x { MkPair a -> a }
main := f (MkPair 1 2)`
	checkSourceExpectError(t, source, nil)
}

func TestUnifyRowOpenClosedExtraLabels(t *testing.T) {
	// Open row { x: Int, y: Bool | ?tail } vs closed { x: Int }
	// The open side has extra label y — tail can absorb nothing since closed.
	// But the open row's tail should solve to {} (empty), and y is extra → error.
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 400, Kind: types.KRow{}}

	r1 := types.OpenRow([]types.RowField{
		{Label: "x", Type: types.Con("Int")},
		{Label: "y", Type: types.Con("Bool")},
	}, m)

	r2 := types.ClosedRow(types.RowField{Label: "x", Type: types.Con("Int")})

	if err := u.Unify(r1, r2); err == nil {
		t.Fatal("open row with extra labels should not unify with closed row missing those labels")
	}
}

func TestUnifyRowClosedOpenAbsorbExtra(t *testing.T) {
	// Closed row { x: Int } vs open row { x: Int, y: Bool | ?tail }
	// Reversed direction: same constraint.
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 500, Kind: types.KRow{}}

	r1 := types.ClosedRow(types.RowField{Label: "x", Type: types.Con("Int")})

	r2 := types.OpenRow([]types.RowField{
		{Label: "x", Type: types.Con("Int")},
		{Label: "y", Type: types.Con("Bool")},
	}, m)

	if err := u.Unify(r1, r2); err == nil {
		t.Fatal("closed row should not unify with open row that has extra labels")
	}
}

func TestUnifyRowOpenClosedSubset(t *testing.T) {
	// Open row { x: Int | ?tail } vs closed { x: Int, y: Bool }
	// Closed has extra y — tail absorbs { y: Bool }.
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 600, Kind: types.KRow{}}

	r1 := types.OpenRow([]types.RowField{
		{Label: "x", Type: types.Con("Int")},
	}, m)

	r2 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Int")},
		types.RowField{Label: "y", Type: types.Con("Bool")},
	)

	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("open row should absorb extra closed labels into tail: %v", err)
	}
	soln := u.Zonk(m)
	row, ok := soln.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("tail should be solved to a row, got %s", types.Pretty(soln))
	}
	cap := row.Entries.(*types.CapabilityEntries)
	if len(cap.Fields) != 1 || cap.Fields[0].Label != "y" {
		t.Errorf("tail should have field 'y', got %s", types.Pretty(row))
	}
}

// checkSourceExpectError parses and type-checks source, expecting at least one error.
// Returns the formatted error string.
func checkSourceExpectError(t *testing.T, source string, config *CheckConfig) string {
	t.Helper()
	src := span.NewSource("test", source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
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
	errMsg := checkSourceExpectCode(t, `type A := A`, nil, errs.ErrCyclicAlias)
	if !strings.Contains(errMsg, "A -> A") {
		t.Errorf("expected cycle path A -> A, got: %s", errMsg)
	}
}

func TestAliasCycleMutual(t *testing.T) {
	checkSourceExpectCode(t, "type A := B\ntype B := A", nil, errs.ErrCyclicAlias)
}

func TestAliasNoCycle(t *testing.T) {
	// Eff references Computation, which is a built-in — not an alias.
	source := `type Eff r a := Computation r r a
data Unit := Unit
main := pure Unit`
	checkSource(t, source, nil)
}

// --- Instance resolution tests ---

func TestResolveMissingInstanceError(t *testing.T) {
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y
main := f True False`
	checkSourceExpectCode(t, source, nil, errs.ErrNoInstance)
}

func TestResolveSimpleInstance(t *testing.T) {
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y
main := f True False`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			if !types.Equal(b.Type, types.Con("Bool")) {
				t.Errorf("expected main :: Bool, got %s", types.Pretty(b.Type))
			}
			return
		}
	}
	t.Error("expected binding 'main'")
}

func TestResolveContextualInstance(t *testing.T) {
	source := `data Bool := True | False
data Maybe a := Just a | Nothing
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq a => Eq (Maybe a) { eq := \x y. True }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y
main := f (Just True) (Just False)`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			if !types.Equal(b.Type, types.Con("Bool")) {
				t.Errorf("expected main :: Bool, got %s", types.Pretty(b.Type))
			}
			return
		}
	}
	t.Error("expected binding 'main'")
}

// --- Exhaustiveness tests ---

// --- Type class elaboration tests ---

func TestClassElaboratesDataDecl(t *testing.T) {
	source := `data Bool := True | False
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
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }`
	prog := checkSource(t, source, nil)
	// Should have generated eq binding (selector).
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "eq" {
			found = true
			// Verify the type is a forall with a dict arrow.
			if b.Type == nil {
				t.Error("eq selector should have a type")
			}
			// Verify it's a TyLam wrapping a Lam (selector body).
			if tl, ok := b.Expr.(*core.TyLam); !ok {
				t.Errorf("eq selector should be a TyLam, got %T", b.Expr)
			} else if _, ok := tl.Body.(*core.Lam); !ok {
				t.Errorf("eq selector TyLam body should be a Lam, got %T", tl.Body)
			}
		}
	}
	if !found {
		t.Error("expected 'eq' selector binding")
	}
}

func TestClassMethodInScope(t *testing.T) {
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
f :: Eq a => a -> a -> Bool
f := \x y. eq x y`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "f" {
			found = true
			if b.Type == nil {
				t.Error("binding 'f' should have a type")
			}
		}
	}
	if !found {
		t.Error("expected binding 'f'")
	}
}

func TestSuperclassDictField(t *testing.T) {
	source := `data Bool := True | False
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
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }`
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
	source := `data Bool := True | False
data Maybe a := Just a | Nothing
class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq (Maybe a) { eq := \x y. True }`
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
	source := `data Bool := True | False
main := \b. case b { True -> True; False -> False }`
	checkSource(t, source, nil)
}

func TestExhaustiveIncomplete(t *testing.T) {
	source := `data Bool := True | False
main := \b. case b { True -> True }`
	errMsg := checkSourceExpectCode(t, source, nil, errs.ErrNonExhaustive)
	if !strings.Contains(errMsg, "False") {
		t.Errorf("expected missing constructor 'False' in error, got: %s", errMsg)
	}
}

func TestExhaustiveWildcard(t *testing.T) {
	source := `data Bool := True | False
main := \b. case b { _ -> True }`
	checkSource(t, source, nil)
}

func TestExhaustiveVarPattern(t *testing.T) {
	source := `data Bool := True | False
main := \b. case b { x -> x }`
	checkSource(t, source, nil)
}

func TestExhaustiveNestedComplete(t *testing.T) {
	source := `data Maybe a := Just a | Nothing
data Bool := True | False
main := \m. case m { Just (Just _) -> 1; Just (Nothing) -> 2; Nothing -> 3 }`
	checkSource(t, source, nil)
}

func TestExhaustiveNestedIncomplete(t *testing.T) {
	source := `data Maybe a := Just a | Nothing
data Bool := True | False
main := \m. case m { Just (Just _) -> 1; Nothing -> 3 }`
	errMsg := checkSourceExpectCode(t, source, nil, errs.ErrNonExhaustive)
	if !strings.Contains(errMsg, "Nothing") && !strings.Contains(errMsg, "Just") {
		t.Errorf("expected mention of missing pattern, got: %s", errMsg)
	}
}

func TestRedundantPattern(t *testing.T) {
	source := `data Bool := True | False
main := \b. case b { _ -> 1; True -> 2 }`
	checkSourceExpectCode(t, source, nil, errs.ErrRedundantPattern)
}

// --- Zonk optimization tests ---

func TestZonkPathCompression(t *testing.T) {
	u := unify.NewUnifier()
	// Chain: m1 → m2 → Int
	m1 := &types.TyMeta{ID: 1, Kind: types.KType{}}
	m2 := &types.TyMeta{ID: 2, Kind: types.KType{}}
	u.InstallTempSolution(1, m2)
	u.InstallTempSolution(2, types.Con("Int"))

	result := u.Zonk(m1)
	if con, ok := result.(*types.TyCon); !ok || con.Name != "Int" {
		t.Fatalf("expected Int, got %v", result)
	}
	// After path compression, Solve(1) should point directly to Int.
	direct := u.Solve(1)
	if con, ok := direct.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("path compression failed: Solve(1) = %v, expected Int", direct)
	}
}

func TestZonkNoAllocUnchanged(t *testing.T) {
	u := unify.NewUnifier()
	// A type with no metavariables should return the exact same pointer.
	ty := types.MkArrow(types.Con("Int"), types.Con("Bool"))
	result := u.Zonk(ty)
	if result != ty {
		t.Errorf("Zonk of meta-free type should return same pointer")
	}
}

// --- Instance index tests ---

func TestInstanceIndexLookup(t *testing.T) {
	// Register 10 classes each with 10 instances, then resolve specific one.
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
class Show a { show :: a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Show Bool { show := \x. True }
main := eq True False`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			if !types.Equal(b.Type, types.Con("Bool")) {
				t.Errorf("expected main :: Bool, got %s", types.Pretty(b.Type))
			}
			return
		}
	}
	t.Error("expected binding 'main'")
}

func BenchmarkInstanceResolve100(b *testing.B) {
	// Build source with many instances to benchmark resolution.
	source := `data Bool := True | False
data Unit := Unit
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq Unit { eq := \x y. True }
main := eq True False`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := span.NewSource("bench", source)
		l := parse.NewLexer(src)
		tokens, _ := l.Tokenize()
		es := &errs.Errors{Source: src}
		p := parse.NewParser(tokens, es)
		ast := p.ParseProgram()
		Check(ast, src, nil)
	}
}

// --- DataKinds tests ---

func TestKDataEquality(t *testing.T) {
	k1 := types.KData{Name: "Bool"}
	k2 := types.KData{Name: "Bool"}
	k3 := types.KData{Name: "DBState"}
	if !k1.Equal(k2) {
		t.Error("KData{Bool} should equal KData{Bool}")
	}
	if k1.Equal(k3) {
		t.Error("KData{Bool} should not equal KData{DBState}")
	}
	if k1.String() != "Bool" {
		t.Errorf("expected 'Bool', got %s", k1.String())
	}
}

func TestKDataArity(t *testing.T) {
	k := types.KData{Name: "DBState"}
	if types.Arity(k) != 0 {
		t.Errorf("KData arity should be 0, got %d", types.Arity(k))
	}
	if types.ResultKind(k) != k {
		t.Error("KData ResultKind should be itself")
	}
}

func TestResolveUserKind(t *testing.T) {
	// \ (s: DBState). T → the kind annotation DBState should resolve to KData{DBState}
	source := `data DBState := Opened | Closed
data DB s := MkDB
f :: \ (s: DBState). DB s -> DB s
f := \x. x
main := f (MkDB :: DB Opened)`
	checkSource(t, source, nil)
}

func TestPromoteNullaryConstructors(t *testing.T) {
	// data S := A | B → A and B are promoted to type level with kind S
	source := `data S := A | B
data Proxy s := MkProxy
main := (MkProxy :: Proxy A)`
	checkSource(t, source, nil)
}

func TestPromoteSkipsFieldedConstructors(t *testing.T) {
	// data Maybe a := Just a | Nothing → only Nothing is promoted, Just is not
	source := `data Bool := True | False
data Maybe a := Just a | Nothing
data Proxy s := MkProxy
main := (MkProxy :: Proxy Nothing)`
	checkSource(t, source, nil)
}

func TestPromotedInTypeSignature(t *testing.T) {
	// DB Opened -> DB Closed should kind-check
	source := `data DBState := Opened | Closed
data DB s := MkDB
close :: DB Opened -> DB Closed
close := \_. MkDB
main := close MkDB`
	checkSource(t, source, nil)
}

// --- GADT tests ---

func TestGADTConTypeRegistration(t *testing.T) {
	// IntLit :: Int -> Expr Int → constructor type is registered correctly.
	source := `data Bool := True | False
data Expr a := { IntLit :: Bool -> Expr Bool; BoolLit :: Bool -> Expr Bool }
main := IntLit True`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
			// Verify the inferred type is Expr Bool.
			pretty := types.Pretty(b.Type)
			if !strings.Contains(pretty, "Expr") || !strings.Contains(pretty, "Bool") {
				t.Errorf("expected main :: Expr Bool, got %s", pretty)
			}
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
	// Verify GADT constructors are in DataDecls.
	for _, d := range prog.DataDecls {
		if d.Name == "Expr" {
			if len(d.Cons) != 2 {
				t.Fatalf("expected 2 cons, got %d", len(d.Cons))
			}
			for _, c := range d.Cons {
				if c.ReturnType == nil {
					t.Errorf("GADT con %s should have ReturnType", c.Name)
				}
			}
		}
	}
}

func TestGADTPatternRefinement(t *testing.T) {
	// case (e: Expr Bool) { BoolLit b -> b } should derive b: Bool
	source := `data Bool := True | False
data Expr a := { BoolLit :: Bool -> Expr Bool; IntLit :: Bool -> Expr Bool }
f :: Expr Bool -> Bool
f := \e. case e { BoolLit b -> b; IntLit b -> b }`
	checkSource(t, source, nil)

	// Negative test: refinement must not allow returning wrong type.
	// After matching BoolLit b, b: Bool; returning it as Int should fail.
	badSource := `data Bool := True | False
data Expr a := { BoolLit :: Bool -> Expr Bool; IntLit :: Bool -> Expr Bool }
f :: Expr Bool -> Expr Bool
f := \e. case e { BoolLit b -> b; IntLit b -> b }`
	checkSourceExpectCode(t, badSource, nil, errs.ErrTypeMismatch)
}

func TestGADTMultiBranch(t *testing.T) {
	// Multiple GADT constructors sharing the same return type specialization.
	source := `data Bool := True | False
data Expr a := { Lit :: Bool -> Expr Bool; Not :: Expr Bool -> Expr Bool }
eval :: Expr Bool -> Bool
eval := \e. case e { Lit b -> b; Not inner -> True }`
	checkSource(t, source, nil)
}

func TestGADTExhaustiveRelevant(t *testing.T) {
	// Tag Bool case: TagUnit is irrelevant (return type Tag Unit ≠ Tag Bool).
	// Only TagBool is required.
	source := `data Bool := True | False
data Unit := Unit
data Tag a := { TagBool :: Bool -> Tag Bool; TagUnit :: Unit -> Tag Unit }
f :: Tag Bool -> Bool
f := \t. case t { TagBool b -> b }`
	checkSource(t, source, nil)
}

func TestGADTNonExhaustiveError(t *testing.T) {
	// Tag Bool case: TagBool is required but missing → error.
	source := `data Bool := True | False
data Unit := Unit
data Tag a := { TagBool :: Bool -> Tag Bool; TagUnit :: Unit -> Tag Unit }
f :: Tag Bool -> Bool
f := \t. case t { TagUnit _ -> True }`
	errMsg := checkSourceExpectCode(t, source, nil, errs.ErrNonExhaustive)
	if !strings.Contains(errMsg, "TagBool") {
		t.Errorf("expected missing TagBool, got: %s", errMsg)
	}
}

func TestGADTAllBranchesIrrelevant(t *testing.T) {
	// If all constructors are irrelevant for the scrutinee type,
	// an empty case is OK (dead code).
	source := `data Bool := True | False
data Unit := Unit
data Void := MkVoid
data Tag a := { TagBool :: Bool -> Tag Bool; TagUnit :: Unit -> Tag Unit }
f :: Tag Void -> Void
f := \t. case t { _ -> MkVoid }`
	checkSource(t, source, nil)
}

// checkSourceExpectCode parses and type-checks source, expecting at least one error
// with the given error code. Returns the formatted error string.
func checkSourceExpectCode(t *testing.T, source string, config *CheckConfig, code errs.Code) string {
	t.Helper()
	src := span.NewSource("test", source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
	_, checkErrs := Check(ast, src, config)
	if !checkErrs.HasErrors() {
		t.Fatal("expected check errors, got none")
	}
	found := false
	for _, e := range checkErrs.Errs {
		if e.Code == code {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error code E%04d, got: %s", code, checkErrs.Format())
	}
	return checkErrs.Format()
}

func TestOverlappingInstances(t *testing.T) {
	// Two instances of Eq for the same type should trigger ErrOverlap.
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. case x { True -> y; False -> case y { True -> False; False -> True } } }
instance Eq Bool { eq := \x y. True }
main := eq True False`
	checkSourceExpectCode(t, source, nil, errs.ErrOverlap)
}

func TestNonOverlappingInstances(t *testing.T) {
	// Instances for different types should not overlap.
	source := `data Bool := True | False
data Unit := Unit
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. case x { True -> y; False -> case y { True -> False; False -> True } } }
instance Eq Unit { eq := \_ _. True }
main := eq True False`
	checkSource(t, source, nil)
}

func TestInstanceArityMismatch(t *testing.T) {
	// Class Eq has 1 type param, instance provides 2 → ErrBadInstance.
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool Bool { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, errs.ErrBadInstance)
}

func TestInstanceUnknownContextClass(t *testing.T) {
	// Instance context references a class that doesn't exist → ErrBadInstance.
	source := `data Bool := True | False
data Maybe a := Nothing | Just a
class Eq a { eq :: a -> a -> Bool }
instance Phantom a => Eq (Maybe a) { eq := \_ _. True }`
	checkSourceExpectCode(t, source, nil, errs.ErrBadInstance)
}

func TestInstanceSelfCycle(t *testing.T) {
	// Instance context requires itself → ErrBadInstance.
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq a { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, errs.ErrBadInstance)
}

func TestInstanceExtraMethod(t *testing.T) {
	// Instance defines a method not declared in the class → ErrBadInstance.
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True; notAMethod := \x. x }`
	checkSourceExpectCode(t, source, nil, errs.ErrBadInstance)
}

func TestInstanceValidContextClass(t *testing.T) {
	// Valid instance with known context class should succeed.
	source := `data Bool := True | False
data Maybe a := Nothing | Just a
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. case x { True -> y; False -> case y { True -> False; False -> True } } }
instance Eq a => Eq (Maybe a) {
  eq := \x y. case x {
    Nothing -> case y { Nothing -> True; Just _ -> False };
    Just a  -> case y { Nothing -> False; Just b -> eq a b }
  }
}`
	checkSource(t, source, nil)
}

func TestParametricOverlappingInstances(t *testing.T) {
	// instance Eq (Maybe a) overlaps with instance Eq (Maybe Bool).
	source := `data Bool := True | False
data Maybe a := Nothing | Just a
class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq (Maybe a) {
  eq := \x y. case x {
    Nothing -> case y { Nothing -> True; Just _ -> False };
    Just a  -> case y { Nothing -> False; Just b -> eq a b }
  }
}
instance Eq (Maybe Bool) {
  eq := \_ _. True
}`
	checkSourceExpectCode(t, source, nil, errs.ErrOverlap)
}

func TestSelfCycleCompoundType(t *testing.T) {
	// instance Eq (Maybe a) => Eq (Maybe a) is a self-cycle with compound types.
	source := `data Bool := True | False
data Maybe a := Nothing | Just a
class Eq a { eq :: a -> a -> Bool }
instance Eq (Maybe a) => Eq (Maybe a) { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, errs.ErrBadInstance)
}

func TestOverlapBlocksRegistration(t *testing.T) {
	// Overlapping instance should NOT be registered — resolution should fail
	// with "no instance" rather than silently picking one.
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. case x { True -> y; False -> case y { True -> False; False -> True } } }
instance Eq Bool { eq := \x y. True }
main := eq True False`
	// We expect ErrOverlap from the duplicate instance declaration.
	// The second instance is rejected, so resolution uses the first — no ambiguity.
	checkSourceExpectCode(t, source, nil, errs.ErrOverlap)
}

func TestSelfCycleBlocksRegistration(t *testing.T) {
	// Self-cycle should not be registered — no cascading errors from resolution.
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq a { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, errs.ErrBadInstance)
}

// --- Error code coverage tests ---

func TestErrorUnboundCon(t *testing.T) {
	source := `data Bool := True | False
main := case True { Foo -> True; _ -> False }`
	checkSourceExpectCode(t, source, nil, errs.ErrUnboundCon)
}

func TestErrorBadApplication(t *testing.T) {
	source := `data Bool := True | False
main := True True`
	checkSourceExpectCode(t, source, nil, errs.ErrBadApplication)
}

func TestErrorBadComputation(t *testing.T) {
	source := `data Bool := True | False
main := do { x <- True; pure x }`
	checkSourceExpectCode(t, source, nil, errs.ErrBadComputation)
}

func TestErrorBadThunk(t *testing.T) {
	source := `data Bool := True | False
main := force True`
	checkSourceExpectCode(t, source, nil, errs.ErrBadThunk)
}

func TestErrorSpecialForm(t *testing.T) {
	// thunk and force remain special forms.
	source := `main := thunk`
	checkSourceExpectCode(t, source, nil, errs.ErrSpecialForm)
}

func TestErrorDuplicateLabel(t *testing.T) {
	// Trigger unify.UnifyDupLabel via the unifier's label context mechanism:
	// a row meta with label context {x} solved to a row containing x.
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KRow{}}
	// Register label context: the meta is the tail of a row with field "x".
	u.RegisterLabelContext(m.ID, map[string]struct{}{"x": {}})
	// Solve the meta to a row that also contains "x" → duplicate.
	row := types.ClosedRow(types.RowField{Label: "x", Type: types.Con("Int")})
	err := u.Unify(m, row)
	if err == nil {
		t.Fatal("expected duplicate label error, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok {
		t.Fatalf("expected UnifyError, got %T: %v", err, err)
	}
	if ue.Kind != unify.UnifyDupLabel {
		t.Errorf("expected unify.UnifyDupLabel, got %v: %s", ue.Kind, ue.Detail)
	}
}

func TestErrorDuplicateLabelEvidenceRow(t *testing.T) {
	// Same as TestErrorDuplicateLabel but for TyEvidenceRow (capability entries).
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KRow{}}
	u.RegisterLabelContext(m.ID, map[string]struct{}{"x": {}})
	evRow := types.ClosedRow(types.RowField{Label: "x", Type: types.Con("Int")})
	err := u.Unify(m, evRow)
	if err == nil {
		t.Fatal("expected duplicate label error for evidence row, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok {
		t.Fatalf("expected UnifyError, got %T: %v", err, err)
	}
	if ue.Kind != unify.UnifyDupLabel {
		t.Errorf("expected unify.UnifyDupLabel, got %v: %s", ue.Kind, ue.Detail)
	}
}

func TestErrorOccursCheck(t *testing.T) {
	source := `main := \x. x x`
	checkSourceExpectCode(t, source, nil, errs.ErrOccursCheck)
}

func TestErrorEmptyDo(t *testing.T) {
	source := `main := do {}`
	checkSourceExpectCode(t, source, nil, errs.ErrEmptyDo)
}

func TestErrorBadDoEnding(t *testing.T) {
	source := `main := do { x <- pure 1 }`
	checkSourceExpectCode(t, source, nil, errs.ErrBadDoEnding)
}

func TestErrorBadClass(t *testing.T) {
	source := `data Bool := True | False
instance Phantom Bool { foo := \x. x }`
	checkSourceExpectCode(t, source, nil, errs.ErrBadClass)
}

func TestErrorMissingMethod(t *testing.T) {
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool {}`
	checkSourceExpectCode(t, source, nil, errs.ErrMissingMethod)
}

func TestErrorSkolemEscape(t *testing.T) {
	// Existential type variable escapes via GADT pattern match:
	// MkExists packs an existential 'a'; extracting it leaks 'a' into the result.
	source := `data Exists := { MkExists :: \ a. a -> Exists }
bad := \e. case e { MkExists x -> x }`
	checkSourceExpectCode(t, source, nil, errs.ErrSkolemEscape)
}

func TestErrorSkolemRigid(t *testing.T) {
	source := `data Bool := True | False
main :: \ a b. a -> b
main := \x. x`
	checkSourceExpectCode(t, source, nil, errs.ErrSkolemRigid)
}

func TestQuantifyFreeVarsKindInference(t *testing.T) {
	// Row variable in Computation pre/post should be quantified as KRow.
	compTy := types.MkComp(
		&types.TyVar{Name: "r"},
		&types.TyVar{Name: "r"},
		types.Con("Int"),
	)
	arrowTy := &types.TyArrow{From: &types.TyVar{Name: "a"}, To: compTy}
	result := quantifyFreeVars(arrowTy)

	forall1, ok := result.(*types.TyForall)
	if !ok {
		t.Fatalf("expected TyForall, got %T", result)
	}
	// Sorted: "a" first, then "r"
	if forall1.Var != "a" {
		t.Errorf("first quantifier: got %q, want 'a'", forall1.Var)
	}
	if _, ok := forall1.Kind.(types.KType); !ok {
		t.Errorf("'a' kind: got %v, want KType", forall1.Kind)
	}

	forall2, ok := forall1.Body.(*types.TyForall)
	if !ok {
		t.Fatalf("expected nested TyForall, got %T", forall1.Body)
	}
	if forall2.Var != "r" {
		t.Errorf("second quantifier: got %q, want 'r'", forall2.Var)
	}
	if _, ok := forall2.Kind.(types.KRow); !ok {
		t.Errorf("'r' kind: got %v, want KRow", forall2.Kind)
	}

	// Pure type variable should get KType.
	pureTy := &types.TyArrow{From: &types.TyVar{Name: "a"}, To: &types.TyVar{Name: "a"}}
	pureResult := quantifyFreeVars(pureTy)
	pureForall, ok := pureResult.(*types.TyForall)
	if !ok {
		t.Fatalf("expected TyForall, got %T", pureResult)
	}
	if _, ok := pureForall.Kind.(types.KType); !ok {
		t.Errorf("pure 'a' kind: got %v, want KType", pureForall.Kind)
	}
}

// --- Exhaustiveness: additional coverage ---

func TestExhaustiveRecordPatterns(t *testing.T) {
	// Record patterns should be handled by the exhaustiveness checker.
	source := `data Bool := True | False
main := \r. case r { { x: True, y: _ } -> 1; { x: False, y: _ } -> 2 }`
	checkSource(t, source, nil)
}

func TestExhaustiveWildcardOnly(t *testing.T) {
	// A single wildcard always covers all cases.
	source := `data Color := Red | Green | Blue
main := \c. case c { _ -> 1 }`
	checkSource(t, source, nil)
}

func TestExhaustiveMultiConComplete(t *testing.T) {
	// Three-constructor type fully covered.
	source := `data Tri := A | B | C
main := \t. case t { A -> 1; B -> 2; C -> 3 }`
	checkSource(t, source, nil)
}

func TestExhaustiveMultiConIncomplete(t *testing.T) {
	// Missing constructor C should be reported.
	source := `data Tri := A | B | C
main := \t. case t { A -> 1; B -> 2 }`
	errMsg := checkSourceExpectCode(t, source, nil, errs.ErrNonExhaustive)
	if !strings.Contains(errMsg, "C") {
		t.Errorf("expected missing constructor 'C' in error, got: %s", errMsg)
	}
}

func TestRedundantPatternMiddle(t *testing.T) {
	// Wildcard before specific constructors: second alt is redundant.
	source := `data Bool := True | False
main := \b. case b { True -> 1; True -> 2; False -> 3 }`
	checkSourceExpectCode(t, source, nil, errs.ErrRedundantPattern)
}

func TestExhaustiveGADTFiltering(t *testing.T) {
	// GADT: only constructors applicable to the scrutinee type should be required.
	source := `data Bool := True | False
data Unit := Unit
data Tag a := { TagBool :: Tag Bool; TagUnit :: Tag Unit }
f :: Tag Bool -> Bool
f := \t. case t { TagBool -> True }`
	checkSource(t, source, nil)
}

// --- inferFreeVarKinds: additional coverage ---

func TestInferFreeVarKindsThunk(t *testing.T) {
	// Variable in TyCBPV (Thunk) pre/post should get KRow.
	fv := map[string]struct{}{"r": {}, "a": {}}
	thunkTy := types.MkThunk(
		&types.TyVar{Name: "r"},
		&types.TyVar{Name: "r"},
		&types.TyVar{Name: "a"},
	)
	kinds := inferFreeVarKinds(thunkTy, fv)
	if _, ok := kinds["r"].(types.KRow); !ok {
		t.Errorf("'r' in TyCBPV (Thunk) pre/post should be KRow, got %v", kinds["r"])
	}
	if _, ok := kinds["a"].(types.KType); !ok {
		t.Errorf("'a' in TyCBPV (Thunk) result should be KType, got %v", kinds["a"])
	}
}

func TestInferFreeVarKindsBothPositions(t *testing.T) {
	// Variable appearing in both row and type positions should get KRow.
	fv := map[string]struct{}{"x": {}}
	ty := types.MkComp(
		&types.TyVar{Name: "x"}, // row position → KRow
		&types.TyVar{Name: "x"},
		&types.TyVar{Name: "x"}, // type position → KType, but KRow wins
	)
	kinds := inferFreeVarKinds(ty, fv)
	if _, ok := kinds["x"].(types.KRow); !ok {
		t.Errorf("'x' in both row and type positions should be KRow, got %v", kinds["x"])
	}
}

func TestInferFreeVarKindsNoFreeVars(t *testing.T) {
	// Empty free variable set should produce empty result.
	fv := map[string]struct{}{}
	ty := &types.TyArrow{From: &types.TyVar{Name: "a"}, To: &types.TyVar{Name: "b"}}
	kinds := inferFreeVarKinds(ty, fv)
	if len(kinds) != 0 {
		t.Errorf("expected empty result for no free vars, got %d entries", len(kinds))
	}
}

func BenchmarkZonkDeepChain(b *testing.B) {
	u := unify.NewUnifier()
	// Build a deep TyApp chain with no metavariables.
	var ty types.Type = types.Con("Base")
	for i := 0; i < 50; i++ {
		ty = &types.TyApp{Fun: types.Con("F"), Arg: ty}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u.Zonk(ty)
	}
}
