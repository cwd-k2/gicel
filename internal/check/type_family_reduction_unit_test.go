// Type family reduction unit tests — matchTyPattern, collectPatternVars, mangling internals.
// Does NOT cover: integration-level reduction (type_family_reduction_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// ==========================================
// Unit-level mutation tests for type system extensions.
// These test internal functions directly rather than
// through full compilation, catching finer mutations.
// ==========================================

// -----------------------------------------------
// matchTyPattern unit tests
// -----------------------------------------------

func TestMatchTyPatternUnit_TyVarBinds(t *testing.T) {
	ch := newTestChecker()
	pat := &types.TyVar{Name: "a"}
	arg := &types.TyCon{Name: "Int"}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchSuccess {
		t.Fatalf("expected matchSuccess, got %d", result)
	}
	if subst["a"] == nil {
		t.Fatal("expected 'a' bound in subst")
	}
	if con, ok := subst["a"].(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected a = Int, got %v", subst["a"])
	}
}

func TestMatchTyPatternUnit_WildcardDoesNotBind(t *testing.T) {
	ch := newTestChecker()
	pat := &types.TyVar{Name: "_"}
	arg := &types.TyCon{Name: "Int"}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchSuccess {
		t.Fatalf("expected matchSuccess, got %d", result)
	}
	if _, ok := subst["_"]; ok {
		t.Error("wildcard '_' should not be bound in subst")
	}
}

func TestMatchTyPatternUnit_TyConExactMatch(t *testing.T) {
	ch := newTestChecker()
	pat := &types.TyCon{Name: "Int"}
	arg := &types.TyCon{Name: "Int"}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchSuccess {
		t.Fatalf("expected matchSuccess, got %d", result)
	}
}

func TestMatchTyPatternUnit_TyConMismatch(t *testing.T) {
	ch := newTestChecker()
	pat := &types.TyCon{Name: "Int"}
	arg := &types.TyCon{Name: "Bool"}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchFail {
		t.Fatalf("expected matchFail, got %d", result)
	}
}

func TestMatchTyPatternUnit_TyConVsMeta(t *testing.T) {
	ch := newTestChecker()
	pat := &types.TyCon{Name: "Int"}
	arg := &types.TyMeta{ID: 1, Kind: types.KType{}}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchIndeterminate {
		t.Fatalf("expected matchIndeterminate, got %d", result)
	}
}

func TestMatchTyPatternUnit_TyAppVsMeta(t *testing.T) {
	ch := newTestChecker()
	pat := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: &types.TyVar{Name: "a"}}
	arg := &types.TyMeta{ID: 1, Kind: types.KType{}}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchIndeterminate {
		t.Fatalf("expected matchIndeterminate, got %d", result)
	}
}

func TestMatchTyPatternUnit_TyAppDecompose(t *testing.T) {
	ch := newTestChecker()
	pat := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: &types.TyVar{Name: "a"}}
	arg := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: &types.TyCon{Name: "Int"}}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchSuccess {
		t.Fatalf("expected matchSuccess, got %d", result)
	}
	if con, ok := subst["a"].(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected a = Int, got %v", subst["a"])
	}
}

func TestMatchTyPatternUnit_TyAppHeadMismatch(t *testing.T) {
	ch := newTestChecker()
	pat := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: &types.TyVar{Name: "a"}}
	arg := &types.TyApp{Fun: &types.TyCon{Name: "Maybe"}, Arg: &types.TyCon{Name: "Int"}}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchFail {
		t.Fatalf("expected matchFail, got %d", result)
	}
}

func TestMatchTyPatternUnit_ConsistencyCheckRejects(t *testing.T) {
	ch := newTestChecker()
	// Pattern: (a, a) where a is bound twice.
	pat := &types.TyVar{Name: "a"}
	subst := map[string]types.Type{
		"a": &types.TyCon{Name: "Int"},
	}
	arg := &types.TyCon{Name: "Bool"} // inconsistent with existing binding
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchFail {
		t.Fatalf("expected matchFail for inconsistent binding, got %d", result)
	}
}

func TestMatchTyPatternUnit_ConsistencyCheckAccepts(t *testing.T) {
	ch := newTestChecker()
	pat := &types.TyVar{Name: "a"}
	subst := map[string]types.Type{
		"a": &types.TyCon{Name: "Int"},
	}
	arg := &types.TyCon{Name: "Int"} // consistent with existing binding
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchSuccess {
		t.Fatalf("expected matchSuccess for consistent binding, got %d", result)
	}
}

// TyApp pattern vs non-TyApp, non-Meta arg should fail.
func TestMatchTyPatternUnit_TyAppVsTyCon(t *testing.T) {
	ch := newTestChecker()
	pat := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: &types.TyVar{Name: "a"}}
	arg := &types.TyCon{Name: "Int"}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchFail {
		t.Fatalf("expected matchFail, got %d", result)
	}
}

// Unsupported pattern form (e.g., TyArrow) should fail.
func TestMatchTyPatternUnit_UnsupportedPatternForm(t *testing.T) {
	ch := newTestChecker()
	pat := &types.TyArrow{From: &types.TyCon{Name: "Int"}, To: &types.TyCon{Name: "Bool"}}
	arg := &types.TyCon{Name: "Int"}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, arg, subst)
	if result != matchFail {
		t.Fatalf("expected matchFail for unsupported pattern, got %d", result)
	}
}

// -----------------------------------------------
// matchTyPatterns unit tests
// -----------------------------------------------

func TestMatchTyPatternsUnit_AllMatch(t *testing.T) {
	ch := newTestChecker()
	patterns := []types.Type{
		&types.TyCon{Name: "Bool"},
		&types.TyVar{Name: "a"},
	}
	args := []types.Type{
		&types.TyCon{Name: "Bool"},
		&types.TyCon{Name: "Int"},
	}
	subst, result := ch.matchTyPatterns(patterns, args)
	if result != matchSuccess {
		t.Fatalf("expected matchSuccess, got %d", result)
	}
	if subst["a"] == nil {
		t.Fatal("expected 'a' bound")
	}
}

func TestMatchTyPatternsUnit_FirstFails(t *testing.T) {
	ch := newTestChecker()
	patterns := []types.Type{
		&types.TyCon{Name: "Bool"},
		&types.TyVar{Name: "a"},
	}
	args := []types.Type{
		&types.TyCon{Name: "Int"}, // mismatch
		&types.TyCon{Name: "Int"},
	}
	_, result := ch.matchTyPatterns(patterns, args)
	if result != matchFail {
		t.Fatalf("expected matchFail, got %d", result)
	}
}

func TestMatchTyPatternsUnit_SecondFails(t *testing.T) {
	ch := newTestChecker()
	patterns := []types.Type{
		&types.TyCon{Name: "Bool"},
		&types.TyCon{Name: "Int"},
	}
	args := []types.Type{
		&types.TyCon{Name: "Bool"},
		&types.TyCon{Name: "Bool"}, // mismatch in second position
	}
	_, result := ch.matchTyPatterns(patterns, args)
	if result != matchFail {
		t.Fatalf("expected matchFail, got %d", result)
	}
}

func TestMatchTyPatternsUnit_IndeterminateStopsEarly(t *testing.T) {
	ch := newTestChecker()
	patterns := []types.Type{
		&types.TyCon{Name: "Bool"}, // concrete
		&types.TyVar{Name: "a"},    // variable (would match anything)
	}
	args := []types.Type{
		&types.TyMeta{ID: 1, Kind: types.KType{}}, // unsolved meta
		&types.TyCon{Name: "Int"},
	}
	_, result := ch.matchTyPatterns(patterns, args)
	if result != matchIndeterminate {
		t.Fatalf("expected matchIndeterminate, got %d", result)
	}
}

// -----------------------------------------------
// reduceTyFamily unit tests
// -----------------------------------------------

func TestReduceTyFamilyUnit_UnknownFamily(t *testing.T) {
	ch := newTestChecker()
	result, ok := ch.reduceTyFamily("NonExistent", []types.Type{&types.TyCon{Name: "Int"}}, span.Span{})
	if ok {
		t.Fatal("expected false for unknown family")
	}
	if result != nil {
		t.Error("expected nil result for unknown family")
	}
}

func TestReduceTyFamilyUnit_EmptyEquations(t *testing.T) {
	ch := newTestChecker()
	ch.reg.families["F"] = &TypeFamilyInfo{
		Name:       "F",
		Params:     []TFParam{{Name: "a", Kind: types.KType{}}},
		ResultKind: types.KType{},
		Equations:  nil,
	}
	result, ok := ch.reduceTyFamily("F", []types.Type{&types.TyCon{Name: "Int"}}, span.Span{})
	if ok {
		t.Fatal("expected false for empty equations")
	}
	if result != nil {
		t.Error("expected nil result for empty equations")
	}
}

func TestReduceTyFamilyUnit_MatchFirstEquation(t *testing.T) {
	ch := newTestChecker()
	ch.reg.families["F"] = &TypeFamilyInfo{
		Name:       "F",
		Params:     []TFParam{{Name: "a", Kind: types.KType{}}},
		ResultKind: types.KType{},
		Equations: []tfEquation{
			{Patterns: []types.Type{&types.TyCon{Name: "Int"}}, RHS: &types.TyCon{Name: "Bool"}},
			{Patterns: []types.Type{&types.TyVar{Name: "a"}}, RHS: &types.TyCon{Name: "Unit"}},
		},
	}
	result, ok := ch.reduceTyFamily("F", []types.Type{&types.TyCon{Name: "Int"}}, span.Span{})
	if !ok {
		t.Fatal("expected successful reduction")
	}
	con, ok := result.(*types.TyCon)
	if !ok || con.Name != "Bool" {
		t.Errorf("expected Bool, got %v", result)
	}
}

func TestReduceTyFamilyUnit_FallsThrough(t *testing.T) {
	ch := newTestChecker()
	ch.reg.families["F"] = &TypeFamilyInfo{
		Name:       "F",
		Params:     []TFParam{{Name: "a", Kind: types.KType{}}},
		ResultKind: types.KType{},
		Equations: []tfEquation{
			{Patterns: []types.Type{&types.TyCon{Name: "Int"}}, RHS: &types.TyCon{Name: "Bool"}},
			{Patterns: []types.Type{&types.TyVar{Name: "a"}}, RHS: &types.TyCon{Name: "Unit"}},
		},
	}
	result, ok := ch.reduceTyFamily("F", []types.Type{&types.TyCon{Name: "String"}}, span.Span{})
	if !ok {
		t.Fatal("expected successful reduction via fallthrough")
	}
	con, ok := result.(*types.TyCon)
	if !ok || con.Name != "Unit" {
		t.Errorf("expected Unit, got %v", result)
	}
}

func TestReduceTyFamilyUnit_DepthLimit(t *testing.T) {
	ch := newTestChecker()
	ch.reg.families["Loop"] = &TypeFamilyInfo{
		Name:       "Loop",
		Params:     []TFParam{{Name: "a", Kind: types.KType{}}},
		ResultKind: types.KType{},
		Equations: []tfEquation{
			{Patterns: []types.Type{&types.TyVar{Name: "a"}}, RHS: &types.TyFamilyApp{Name: "Loop", Args: []types.Type{&types.TyVar{Name: "a"}}}},
		},
	}
	// Manually loop to hit the depth limit.
	ch.reductionDepth = maxReductionDepth
	_, ok := ch.reduceTyFamily("Loop", []types.Type{&types.TyCon{Name: "Unit"}}, span.Span{})
	if ok {
		t.Fatal("expected false when depth limit exceeded")
	}
	// Check error was added.
	if !ch.errors.HasErrors() {
		t.Fatal("expected error for depth limit")
	}
}

func TestReduceTyFamilyUnit_SubstApplied(t *testing.T) {
	ch := newTestChecker()
	ch.reg.families["Id"] = &TypeFamilyInfo{
		Name:       "Id",
		Params:     []TFParam{{Name: "a", Kind: types.KType{}}},
		ResultKind: types.KType{},
		Equations: []tfEquation{
			{Patterns: []types.Type{&types.TyVar{Name: "a"}}, RHS: &types.TyVar{Name: "a"}},
		},
	}
	result, ok := ch.reduceTyFamily("Id", []types.Type{&types.TyCon{Name: "Int"}}, span.Span{})
	if !ok {
		t.Fatal("expected successful reduction")
	}
	con, ok := result.(*types.TyCon)
	if !ok || con.Name != "Int" {
		t.Errorf("expected Int (substitution applied), got %v", result)
	}
}

func TestReduceTyFamilyUnit_IndeterminateStopsBeforeLater(t *testing.T) {
	ch := newTestChecker()
	// Equation 1: F Bool = Int (concrete pattern)
	// Equation 2: F a = Bool (variable pattern, catches all)
	// Argument: unsolved meta -> equation 1 is indeterminate, should NOT try equation 2.
	ch.reg.families["F"] = &TypeFamilyInfo{
		Name:       "F",
		Params:     []TFParam{{Name: "a", Kind: types.KType{}}},
		ResultKind: types.KType{},
		Equations: []tfEquation{
			{Patterns: []types.Type{&types.TyCon{Name: "Bool"}}, RHS: &types.TyCon{Name: "Int"}},
			{Patterns: []types.Type{&types.TyVar{Name: "a"}}, RHS: &types.TyCon{Name: "Bool"}},
		},
	}
	meta := &types.TyMeta{ID: 99, Kind: types.KType{}}
	result, ok := ch.reduceTyFamily("F", []types.Type{meta}, span.Span{})
	if ok {
		t.Fatalf("expected stuck (not reduced), but got result: %v", result)
	}
}

// -----------------------------------------------
// collectPatternVars unit tests
// -----------------------------------------------

func TestCollectPatternVarsUnit_Empty(t *testing.T) {
	vars := collectPatternVars(nil)
	if len(vars) != 0 {
		t.Errorf("expected 0 vars, got %d", len(vars))
	}
}

func TestCollectPatternVarsUnit_OnlyConcrete(t *testing.T) {
	patterns := []types.Type{&types.TyCon{Name: "Int"}, &types.TyCon{Name: "Bool"}}
	vars := collectPatternVars(patterns)
	if len(vars) != 0 {
		t.Errorf("expected 0 vars from concrete patterns, got %d", len(vars))
	}
}

func TestCollectPatternVarsUnit_SimpleVars(t *testing.T) {
	patterns := []types.Type{&types.TyVar{Name: "a"}, &types.TyVar{Name: "b"}}
	vars := collectPatternVars(patterns)
	if len(vars) != 2 {
		t.Fatalf("expected 2 vars, got %d", len(vars))
	}
}

func TestCollectPatternVarsUnit_DedupsSameVar(t *testing.T) {
	patterns := []types.Type{&types.TyVar{Name: "a"}, &types.TyVar{Name: "a"}}
	vars := collectPatternVars(patterns)
	if len(vars) != 1 {
		t.Fatalf("expected 1 var (deduped), got %d", len(vars))
	}
}

func TestCollectPatternVarsUnit_SkipsWildcard(t *testing.T) {
	patterns := []types.Type{&types.TyVar{Name: "_"}, &types.TyVar{Name: "a"}}
	vars := collectPatternVars(patterns)
	if len(vars) != 1 {
		t.Fatalf("expected 1 var (wildcard skipped), got %d", len(vars))
	}
	if vars[0] != "a" {
		t.Errorf("expected 'a', got %q", vars[0])
	}
}

func TestCollectPatternVarsUnit_NestedInTyApp(t *testing.T) {
	patterns := []types.Type{
		&types.TyApp{
			Fun: &types.TyCon{Name: "List"},
			Arg: &types.TyVar{Name: "a"},
		},
	}
	vars := collectPatternVars(patterns)
	if len(vars) != 1 || vars[0] != "a" {
		t.Errorf("expected ['a'], got %v", vars)
	}
}

// -----------------------------------------------
// typeNameForMangling unit tests
// -----------------------------------------------

func TestTypeNameForMangling_TyCon(t *testing.T) {
	name := typeNameForMangling(&types.TyCon{Name: "List"})
	if name != "List" {
		t.Errorf("expected 'List', got %q", name)
	}
}

func TestTypeNameForMangling_TyApp(t *testing.T) {
	ty := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: &types.TyCon{Name: "Int"}}
	name := typeNameForMangling(ty)
	if name != "List" {
		t.Errorf("expected 'List' (head of TyApp), got %q", name)
	}
}

func TestTypeNameForMangling_TyVar(t *testing.T) {
	name := typeNameForMangling(&types.TyVar{Name: "a"})
	if name != "a" {
		t.Errorf("expected 'a', got %q", name)
	}
}

func TestTypeNameForMangling_Unknown(t *testing.T) {
	name := typeNameForMangling(&types.TyArrow{From: &types.TyCon{Name: "Int"}, To: &types.TyCon{Name: "Bool"}})
	if name != "X" {
		t.Errorf("expected 'X' for unknown type form, got %q", name)
	}
}

// -----------------------------------------------
// mangledDataFamilyName unit tests
// -----------------------------------------------

func TestMangledDataFamilyNameUnit_Simple(t *testing.T) {
	ch := newTestChecker()
	name := ch.mangledDataFamilyName("Elem", []types.Type{
		&types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: &types.TyVar{Name: "a"}},
	})
	// mangledDataFamilyName: "Elem" + "$$1" + "$" + typeNameForMangling(TyApp{List, a}) = "Elem$$1$List"
	if name != "Elem$$1$List" {
		t.Errorf("expected 'Elem$$1$List', got %q", name)
	}
}

func TestMangledDataFamilyNameUnit_Multiple(t *testing.T) {
	ch := newTestChecker()
	name := ch.mangledDataFamilyName("F", []types.Type{
		&types.TyCon{Name: "Int"},
		&types.TyCon{Name: "Bool"},
	})
	// "F" + "$$2" + "$Int" + "$Bool" = "F$$2$Int$Bool"
	if name != "F$$2$Int$Bool" {
		t.Errorf("expected 'F$$2$Int$Bool', got %q", name)
	}
}

func TestMangledDataFamilyNameUnit_DifferentPatternsDistinct(t *testing.T) {
	ch := newTestChecker()
	name1 := ch.mangledDataFamilyName("Elem", []types.Type{
		&types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: &types.TyVar{Name: "a"}},
	})
	name2 := ch.mangledDataFamilyName("Elem", []types.Type{
		&types.TyApp{Fun: &types.TyCon{Name: "Maybe"}, Arg: &types.TyVar{Name: "a"}},
	})
	if name1 == name2 {
		t.Errorf("different patterns should produce different names: %q == %q", name1, name2)
	}
}

// -----------------------------------------------
// lubPostStates unit tests (directly)
// -----------------------------------------------

func TestLubPostStatesUnit_ZeroPosts(t *testing.T) {
	ch := newTestChecker()
	result := ch.lubPostStates(nil, span.Span{})
	// Should return a fresh meta (not nil).
	if result == nil {
		t.Fatal("expected non-nil result for zero posts")
	}
	if _, ok := result.(*types.TyMeta); !ok {
		t.Errorf("expected TyMeta for zero posts, got %T", result)
	}
}

func TestLubPostStatesUnit_SinglePost(t *testing.T) {
	ch := newTestChecker()
	var post types.Type = types.ClosedRow(types.RowField{Label: "x", Type: &types.TyCon{Name: "Int"}})
	result := ch.lubPostStates([]types.Type{post}, span.Span{})
	// Should return the single post directly (pointer equality).
	if result != post {
		t.Errorf("expected same post returned for single branch, got %T", result)
	}
}

func TestLubPostStatesUnit_TwoIdenticalPosts(t *testing.T) {
	ch := newTestChecker()
	var post1 types.Type = types.ClosedRow(types.RowField{Label: "x", Type: &types.TyCon{Name: "Int"}})
	var post2 types.Type = types.ClosedRow(types.RowField{Label: "x", Type: &types.TyCon{Name: "Int"}})
	result := ch.lubPostStates([]types.Type{post1, post2}, span.Span{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// -----------------------------------------------
// intersectCapRows unit tests (directly)
// -----------------------------------------------

func TestIntersectCapRowsUnit_EmptyInput(t *testing.T) {
	ch := newTestChecker()
	result := ch.intersectCapRows(nil, span.Span{})
	row, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	cap, ok := row.Entries.(*types.CapabilityEntries)
	if !ok {
		t.Fatalf("expected CapabilityEntries, got %T", row.Entries)
	}
	if len(cap.Fields) != 0 {
		t.Errorf("expected 0 fields for empty input, got %d", len(cap.Fields))
	}
}

func TestIntersectCapRowsUnit_SingleRow(t *testing.T) {
	ch := newTestChecker()
	evRow := types.ClosedRow(
		types.RowField{Label: "a", Type: &types.TyCon{Name: "Int"}},
		types.RowField{Label: "b", Type: &types.TyCon{Name: "Bool"}},
	)
	result := ch.intersectCapRows([]*types.TyEvidenceRow{evRow}, span.Span{})
	resRow, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	fields := resRow.CapFields()
	if len(fields) != 2 {
		t.Errorf("expected 2 fields for single row, got %d", len(fields))
	}
}

func TestIntersectCapRowsUnit_Intersection(t *testing.T) {
	ch := newTestChecker()
	row1 := types.ClosedRow(
		types.RowField{Label: "a", Type: &types.TyCon{Name: "Int"}},
		types.RowField{Label: "b", Type: &types.TyCon{Name: "Bool"}},
	)
	row2 := types.ClosedRow(
		types.RowField{Label: "b", Type: &types.TyCon{Name: "Bool"}},
		types.RowField{Label: "c", Type: &types.TyCon{Name: "String"}},
	)

	result := ch.intersectCapRows([]*types.TyEvidenceRow{row1, row2}, span.Span{})
	resRow, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	fields := resRow.CapFields()
	if len(fields) != 1 {
		t.Fatalf("expected 1 shared field, got %d", len(fields))
	}
	if fields[0].Label != "b" {
		t.Errorf("expected shared label 'b', got %q", fields[0].Label)
	}
}

func TestIntersectCapRowsUnit_NoSharedLabels(t *testing.T) {
	ch := newTestChecker()
	row1 := types.ClosedRow(
		types.RowField{Label: "a", Type: &types.TyCon{Name: "Int"}},
	)
	row2 := types.ClosedRow(
		types.RowField{Label: "b", Type: &types.TyCon{Name: "Bool"}},
	)

	result := ch.intersectCapRows([]*types.TyEvidenceRow{row1, row2}, span.Span{})
	resRow, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	fields := resRow.CapFields()
	if len(fields) != 0 {
		t.Errorf("expected 0 shared fields, got %d", len(fields))
	}
}

func TestIntersectCapRowsUnit_ThreeWay(t *testing.T) {
	ch := newTestChecker()
	row1 := types.ClosedRow(
		types.RowField{Label: "a", Type: &types.TyCon{Name: "Int"}},
		types.RowField{Label: "b", Type: &types.TyCon{Name: "Int"}},
	)
	row2 := types.ClosedRow(
		types.RowField{Label: "b", Type: &types.TyCon{Name: "Int"}},
		types.RowField{Label: "c", Type: &types.TyCon{Name: "Int"}},
	)
	row3 := types.ClosedRow(
		types.RowField{Label: "a", Type: &types.TyCon{Name: "Int"}},
		types.RowField{Label: "c", Type: &types.TyCon{Name: "Int"}},
	)

	result := ch.intersectCapRows([]*types.TyEvidenceRow{row1, row2, row3}, span.Span{})
	resRow, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	fields := resRow.CapFields()
	// 'a' in row1, row3 (not row2) -> not shared by all
	// 'b' in row1, row2 (not row3) -> not shared by all
	// 'c' in row2, row3 (not row1) -> not shared by all
	if len(fields) != 0 {
		t.Errorf("expected 0 shared fields in three-way, got %d: %v", len(fields), fields)
	}
}

// -----------------------------------------------
// verifyInjectivity unit tests (directly)
// -----------------------------------------------

func TestVerifyInjectivityUnit_NoEquations(t *testing.T) {
	ch := newTestChecker()
	info := &TypeFamilyInfo{
		Name:       "F",
		ResultName: "r",
		Equations:  nil,
	}
	ch.verifyInjectivity(info)
	if ch.errors.HasErrors() {
		t.Error("expected no errors for empty equations")
	}
}

func TestVerifyInjectivityUnit_SingleEquation(t *testing.T) {
	ch := newTestChecker()
	info := &TypeFamilyInfo{
		Name:       "Id",
		ResultName: "r",
		Equations: []tfEquation{
			{
				Patterns: []types.Type{&types.TyVar{Name: "a"}},
				RHS:      &types.TyVar{Name: "a"},
			},
		},
	}
	ch.verifyInjectivity(info)
	if ch.errors.HasErrors() {
		t.Error("expected no errors for single equation")
	}
}

func TestVerifyInjectivityUnit_Violation(t *testing.T) {
	ch := newTestChecker()
	info := &TypeFamilyInfo{
		Name:       "F",
		ResultName: "r",
		Equations: []tfEquation{
			{
				Patterns: []types.Type{
					&types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: &types.TyVar{Name: "a"}},
				},
				RHS: &types.TyVar{Name: "a"},
			},
			{
				Patterns: []types.Type{&types.TyCon{Name: "Unit"}},
				RHS:      &types.TyCon{Name: "Unit"},
			},
		},
	}
	ch.verifyInjectivity(info)
	if !ch.errors.HasErrors() {
		t.Error("expected injectivity violation error")
	}
	found := false
	for _, e := range ch.errors.Errs {
		if e.Code == errs.ErrInjectivity {
			found = true
		}
	}
	if !found {
		t.Error("expected ErrInjectivity error code")
	}
}

func TestVerifyInjectivityUnit_DistinctRHSesNoViolation(t *testing.T) {
	ch := newTestChecker()
	info := &TypeFamilyInfo{
		Name:       "F",
		ResultName: "r",
		Equations: []tfEquation{
			{
				Patterns: []types.Type{&types.TyCon{Name: "Int"}},
				RHS:      &types.TyCon{Name: "Bool"},
			},
			{
				Patterns: []types.Type{&types.TyCon{Name: "Bool"}},
				RHS:      &types.TyCon{Name: "Int"},
			},
		},
	}
	ch.verifyInjectivity(info)
	if ch.errors.HasErrors() {
		t.Errorf("expected no errors for distinct RHSes, got: %v", ch.errors.Format())
	}
}

// -----------------------------------------------
// matchResult constants: verify enum values
// -----------------------------------------------

func TestMatchResultConstants(t *testing.T) {
	// Ensure matchSuccess, matchFail, matchIndeterminate are distinct.
	if matchSuccess == matchFail {
		t.Error("matchSuccess and matchFail should be distinct")
	}
	if matchSuccess == matchIndeterminate {
		t.Error("matchSuccess and matchIndeterminate should be distinct")
	}
	if matchFail == matchIndeterminate {
		t.Error("matchFail and matchIndeterminate should be distinct")
	}
	// Verify specific values (iota ordering).
	if matchSuccess != 0 {
		t.Errorf("expected matchSuccess = 0, got %d", matchSuccess)
	}
	if matchFail != 1 {
		t.Errorf("expected matchFail = 1, got %d", matchFail)
	}
	if matchIndeterminate != 2 {
		t.Errorf("expected matchIndeterminate = 2, got %d", matchIndeterminate)
	}
}

// -----------------------------------------------
// maxReductionDepth constant
// -----------------------------------------------

func TestMaxReductionDepthValue(t *testing.T) {
	if maxReductionDepth != 100 {
		t.Errorf("expected maxReductionDepth = 100, got %d", maxReductionDepth)
	}
}

// -----------------------------------------------
// matchTyPattern: zonking solves meta before matching
// -----------------------------------------------

func TestMatchTyPatternUnit_ZonksMeta(t *testing.T) {
	ch := newTestChecker()
	// Create a meta that has been solved.
	meta := &types.TyMeta{ID: 42, Kind: types.KType{}}
	ch.unifier.InstallTempSolution(42, &types.TyCon{Name: "Bool"})

	pat := &types.TyCon{Name: "Bool"}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, meta, subst)
	// After zonking, meta resolves to Bool, which matches the pattern.
	if result != matchSuccess {
		t.Fatalf("expected matchSuccess after zonking, got %d", result)
	}
}

func TestMatchTyPatternUnit_ZonksMetaToMismatch(t *testing.T) {
	ch := newTestChecker()
	meta := &types.TyMeta{ID: 42, Kind: types.KType{}}
	ch.unifier.InstallTempSolution(42, &types.TyCon{Name: "Int"})

	pat := &types.TyCon{Name: "Bool"}
	subst := make(map[string]types.Type)
	result := ch.matchTyPattern(pat, meta, subst)
	// After zonking, meta resolves to Int, which doesn't match Bool.
	if result != matchFail {
		t.Fatalf("expected matchFail after zonking to wrong type, got %d", result)
	}
}
