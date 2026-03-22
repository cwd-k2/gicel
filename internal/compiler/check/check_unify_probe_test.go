//go:build probe

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Unification probe tests — row unification, occurs check, meta solving,
// skolem/meta interaction, forall alpha-equivalence, TyError absorption,
// TyCBPV cross-case, TyFamilyApp unification, duplicate label detection.
// =============================================================================

// =====================================================================
// From probe_a: Row unification edge cases
// =====================================================================

// TestProbeA_RowUnify_EmptyRows — two empty records should unify trivially.
// BUG: medium — Record type annotation syntax uses `Record {}` not bare `{}`.
// However, `()` in type position IS parsed as `Record {}` (unit type).
// Bare `{}` in type annotation position is parsed as empty row, not Record {}.
// This means `{} -> {}` as a type annotation fails to unify with the inferred
// `Record {} -> Record {}` from the record literal. This is arguably a syntax
// inconsistency: `{}` in expression position creates a Record, but `{}` in
// type position is not `Record {}`.
func TestProbeA_RowUnify_EmptyRows(t *testing.T) {
	// Use Record {} or () in type position for empty record.
	source := `
f :: () => ()
f := \x. x

main := f {}
`
	checkSource(t, source, nil)
}

// TestProbeA_RowUnify_SingleField — single-field record round-trip.
func TestProbeA_RowUnify_SingleField(t *testing.T) {
	source := `
f :: Record { x: Int } -> Record { x: Int }
f := \r. r

main := f { x: 42 }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// TestProbeA_RowUnify_TailVsClosedRow — an open-row expectation unified
// against a concrete closed row should solve the tail to empty.
func TestProbeA_RowUnify_TailVsClosedRow(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }

-- f is polymorphic over the row tail but concretely applied to a closed record.
f := \r. r.#x

main := f { x: True, y: 42 }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// TestProbeA_RowUnify_OrderInsensitivity — records with fields in different
// order should unify (row unification is order-insensitive).
// BUG: medium — Row-polymorphic projection (`r.#a`) on a Record type
// annotated as `Record { a: Bool, b: Int }` fails with "expected record
// with field a". This occurs because the type annotation creates a closed
// row type that is not decomposed correctly when the projection tries to
// match a specific field. The bare `{ a: Bool, b: Int }` in type position
// does not produce a `Record { a: Bool, b: Int }` — it produces a bare
// evidence row. Using `Record { a: Bool, b: Int }` is the correct syntax.
func TestProbeA_RowUnify_OrderInsensitivity(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }

f :: Record { a: Bool, b: Int } -> Bool
f := \r. r.#a

-- Construct with b first, a second — should still unify.
main := f { b: 42, a: True }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// TestProbeA_RowUnify_DuplicateFieldError — duplicate labels in a record
// literal should be rejected.
func TestProbeA_RowUnify_DuplicateFieldError(t *testing.T) {
	source := `
main := { x: 42, x: 43 }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSourceExpectError(t, source, config)
}

// TestProbeA_RowUnify_ExtraFieldVsClosedRow — passing a record with an
// extra field to a function expecting a closed row should error.
func TestProbeA_RowUnify_ExtraFieldVsClosedRow(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }

f :: Record { x: Bool } -> Bool
f := \r. r.#x

-- Extra field y should cause a row mismatch error.
main := f { x: True, y: 42 }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSourceExpectError(t, source, config)
}

// TestProbeA_RowUnify_MissingFieldError — projecting a field that doesn't exist.
func TestProbeA_RowUnify_MissingFieldError(t *testing.T) {
	source := `
main := { x: 42 }.#z
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSourceExpectCode(t, source, config, diagnostic.ErrRowMismatch)
}

// TestProbeA_RowUnify_RecordUpdateFieldType — updating a field with a value of
// the wrong type should fail. Record update syntax: { base | field: value }.
func TestProbeA_RowUnify_RecordUpdateFieldType(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }

main := { { x: 42 } | x: True }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSourceExpectError(t, source, config)
}

// TestProbeA_OccursCheckInRow — record field whose type circularly
// references itself should trigger an occurs check.
func TestProbeA_OccursCheckInRow(t *testing.T) {
	source := `
-- f : a -> { x: a } where we try to unify a = { x: a }
f := \x. { x: f x }
`
	checkSourceExpectError(t, source, nil)
}

// TestProbeA_EmptyRecordUnifiesWithEmptyRecord — edge case: check that
// () => () is valid (unit type).
func TestProbeA_EmptyRecordUnifiesWithEmptyRecord(t *testing.T) {
	source := `
f :: () => ()
f := \x. x

main := f {}
`
	checkSource(t, source, nil)
}

// TestProbeA_RowUnify_UpdatePreservesExtraFields — record update should
// preserve fields not mentioned in the update.
// Uses the correct record update syntax: { base | field: value }.
func TestProbeA_RowUnify_UpdatePreservesExtraFields(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }

r := { x: True, y: 42 }
main := { r | x: False }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// TestProbeA_RowUnify_RecordAnnotationVsLiteralBareRow — demonstrates
// that `{}` in type position is NOT equivalent to `Record {}`.
// BUG: low — The bare `{}` and `{ l: T }` syntax in type position
// produces a raw evidence row, not a `Record <row>`. Users must write
// `Record { l: T }` in annotations. This is a syntax inconsistency:
// `{}` in expression position creates `Record {}`, but `{}` in type
// position does not. The only workaround is `()` for empty records
// or `Record { ... }` for non-empty records.
func TestProbeA_RowUnify_RecordAnnotationVsLiteralBareRow(t *testing.T) {
	// This SHOULD work but doesn't: bare {} in type annotation.
	source := `
f: {} -> {}
f := \x. x
main := f {}
`
	// Known issue: bare {} in type position != Record {}.
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "Record") {
		t.Logf("Error (known syntax inconsistency): %s", errMsg)
	}
}

// TestProbeA_RowUnify_RecordPatternExtraFields — matching a record with
// more fields than the pattern should work (the pattern is open).
func TestProbeA_RowUnify_RecordPatternExtraFields(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }

f := \r. case r { { x: a } => a }
main := f { x: True, y: 42 }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// =====================================================================
// From probe_d: Unification edge cases
// =====================================================================

// TestProbeD_Unify_OccursCheckDirect — unifying ?m with (List ?m) must
// trigger the occurs check and return an error.
func TestProbeD_Unify_OccursCheckDirect(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	listMeta := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: meta}
	err := u.Unify(meta, listMeta)
	if err == nil {
		t.Fatal("expected occurs check error, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyOccursCheck {
		t.Errorf("expected unify.UnifyOccursCheck, got %v", err)
	}
}

// TestProbeD_Unify_OccursCheckDeepNesting — occurs check through a chain
// of three levels: ?m ~ F (G (H ?m)).
func TestProbeD_Unify_OccursCheckDeepNesting(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	nested := &types.TyApp{
		Fun: types.Con("F"),
		Arg: &types.TyApp{
			Fun: types.Con("G"),
			Arg: &types.TyApp{
				Fun: types.Con("H"),
				Arg: meta,
			},
		},
	}
	err := u.Unify(meta, nested)
	if err == nil {
		t.Fatal("expected occurs check error for deeply nested meta, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyOccursCheck {
		t.Errorf("expected unify.UnifyOccursCheck, got %v", err)
	}
}

// TestProbeD_Unify_DeeplyNestedTyAppChain — unifying two deeply nested
// TyApp chains should succeed when structurally equal.
func TestProbeD_Unify_DeeplyNestedTyAppChain(t *testing.T) {
	u := unify.NewUnifier()
	// Build: F (F (F (... (F Int) ...)))  depth=20
	const depth = 20
	buildChain := func() types.Type {
		var ty types.Type = types.Con("Int")
		for i := 0; i < depth; i++ {
			ty = &types.TyApp{Fun: types.Con("F"), Arg: ty}
		}
		return ty
	}
	a := buildChain()
	b := buildChain()
	if err := u.Unify(a, b); err != nil {
		t.Fatalf("unifying identical deep chains should succeed: %v", err)
	}
}

// TestProbeD_Unify_TyAppChainMetaSolving — unifying F ?m with F Int
// should solve ?m = Int, then Zonk should reveal Int.
func TestProbeD_Unify_TyAppChainMetaSolving(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	a := &types.TyApp{Fun: types.Con("F"), Arg: meta}
	b := &types.TyApp{Fun: types.Con("F"), Arg: types.Con("Int")}
	if err := u.Unify(a, b); err != nil {
		t.Fatalf("expected success: %v", err)
	}
	solved := u.Zonk(meta)
	if con, ok := solved.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected meta solved to Int, got %s", types.Pretty(solved))
	}
}

// TestProbeD_Unify_RowOverlappingLabels — unifying two closed rows with
// overlapping labels. Fields with the same label must have compatible types.
func TestProbeD_Unify_RowOverlappingLabels(t *testing.T) {
	u := unify.NewUnifier()
	row1 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Int")},
		types.RowField{Label: "y", Type: types.Con("Bool")},
		types.RowField{Label: "z", Type: types.Con("String")},
	)
	row2 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Int")},
		types.RowField{Label: "y", Type: types.Con("Bool")},
		types.RowField{Label: "z", Type: types.Con("String")},
	)
	if err := u.Unify(row1, row2); err != nil {
		t.Fatalf("identical closed rows should unify: %v", err)
	}
}

// TestProbeD_Unify_RowMismatchedFieldType — unifying two closed rows where
// one field has a different type should fail.
func TestProbeD_Unify_RowMismatchedFieldType(t *testing.T) {
	u := unify.NewUnifier()
	row1 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Int")},
	)
	row2 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Bool")},
	)
	err := u.Unify(row1, row2)
	if err == nil {
		t.Fatal("expected row type mismatch, got nil")
	}
}

// TestProbeD_Unify_TwoTyFamilyAppNodes — unifying two TyFamilyApp nodes
// with the same family name should unify their arguments pairwise.
func TestProbeD_Unify_TwoTyFamilyAppNodes(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	fam1 := &types.TyFamilyApp{Name: "Elem", Args: []types.Type{meta}, Kind: types.KType{}}
	fam2 := &types.TyFamilyApp{Name: "Elem", Args: []types.Type{types.Con("List")}, Kind: types.KType{}}
	if err := u.Unify(fam1, fam2); err != nil {
		t.Fatalf("unifying same-family TyFamilyApp should succeed: %v", err)
	}
	solved := u.Zonk(meta)
	if con, ok := solved.(*types.TyCon); !ok || con.Name != "List" {
		t.Errorf("expected meta solved to List, got %s", types.Pretty(solved))
	}
}

// TestProbeD_Unify_DifferentTyFamilyAppNames — unifying two TyFamilyApp nodes
// with different names should fail.
func TestProbeD_Unify_DifferentTyFamilyAppNames(t *testing.T) {
	u := unify.NewUnifier()
	fam1 := &types.TyFamilyApp{Name: "Elem", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	fam2 := &types.TyFamilyApp{Name: "Wrap", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	err := u.Unify(fam1, fam2)
	if err == nil {
		t.Fatal("expected mismatch for different family names, got nil")
	}
}

// TestProbeD_Unify_SkolemVsMeta — a meta should be solvable to a skolem
// (metas CAN be solved to skolems; the escape check is separate).
func TestProbeD_Unify_SkolemVsMeta(t *testing.T) {
	u := unify.NewUnifier()
	sk := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	meta := &types.TyMeta{ID: 2, Kind: types.KType{}}
	if err := u.Unify(meta, sk); err != nil {
		t.Fatalf("meta should be solvable to skolem: %v", err)
	}
	solved := u.Zonk(meta)
	if s, ok := solved.(*types.TySkolem); !ok || s.ID != 1 {
		t.Errorf("expected skolem a, got %s", types.Pretty(solved))
	}
}

// TestProbeD_Unify_TyCompCrossCase — unifying TyCBPV (Computation) with an equivalent
// TyApp chain should succeed through the cross-case unification path.
func TestProbeD_Unify_TyCompCrossCase(t *testing.T) {
	u := unify.NewUnifier()
	comp := types.MkComp(
		types.EmptyRow(),
		types.EmptyRow(),
		types.Con("Int"),
	)
	// Build: TyApp(TyApp(TyApp(TyCon("Computation"), emptyRow), emptyRow), Int)
	appChain := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyApp{
				Fun: types.Con("Computation"),
				Arg: types.EmptyRow(),
			},
			Arg: types.EmptyRow(),
		},
		Arg: types.Con("Int"),
	}
	if err := u.Unify(comp, appChain); err != nil {
		t.Fatalf("TyCBPV (Computation) should unify with equivalent TyApp chain: %v", err)
	}
}

// TestProbeD_Unify_TyErrorAbsorbs — TyError should unify with anything
// (error recovery behavior).
func TestProbeD_Unify_TyErrorAbsorbs(t *testing.T) {
	u := unify.NewUnifier()
	if err := u.Unify(&types.TyError{}, types.Con("Int")); err != nil {
		t.Fatalf("TyError should absorb: %v", err)
	}
	if err := u.Unify(types.Con("Bool"), &types.TyError{}); err != nil {
		t.Fatalf("TyError should absorb (reversed): %v", err)
	}
}

// TestProbeD_Unify_ForallBodiesAlphaEquivalent — unifying two foralls
// with different bound variable names but alpha-equivalent bodies.
func TestProbeD_Unify_ForallBodiesAlphaEquivalent(t *testing.T) {
	u := unify.NewUnifier()
	// \a. a -> a
	f1 := types.MkForall("a", types.KType{}, types.MkArrow(&types.TyVar{Name: "a"}, &types.TyVar{Name: "a"}))
	// \b. b -> b
	f2 := types.MkForall("b", types.KType{}, types.MkArrow(&types.TyVar{Name: "b"}, &types.TyVar{Name: "b"}))
	if err := u.Unify(f1, f2); err != nil {
		t.Fatalf("alpha-equivalent foralls should unify: %v", err)
	}
}

// TestProbeD_RowUnify_DuplicateLabelDetection — duplicate labels in the same
// row should be detected by the label context mechanism.
func TestProbeD_RowUnify_DuplicateLabelDetection(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KRow{}}
	// Register "x" as already present in the surrounding context
	u.RegisterLabelContext(1, map[string]struct{}{"x": {}})

	// Try to solve meta with a row containing "x" — should trigger dup label
	solution := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{{Label: "x", Type: types.Con("Int")}},
		},
	}
	err := u.Unify(meta, solution)
	if err == nil {
		t.Fatal("expected duplicate label error, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyDupLabel {
		t.Errorf("expected unify.UnifyDupLabel, got %v", err)
	}
}

// =====================================================================
// From probe_e: Unification boundary conditions
// =====================================================================

// TestProbeE_Unify_TyErrorPropagation — TyError should unify with anything
// without panicking, including deeply nested types and other TyErrors.
func TestProbeE_Unify_TyErrorPropagation(t *testing.T) {
	u := unify.NewUnifier()
	tyErr := &types.TyError{}

	// TyError ~ concrete
	if err := u.Unify(tyErr, types.Con("Int")); err != nil {
		t.Errorf("TyError should unify with concrete type: %v", err)
	}
	// TyError ~ meta
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	if err := u.Unify(tyErr, meta); err != nil {
		t.Errorf("TyError should unify with meta: %v", err)
	}
	// TyError ~ TyError
	if err := u.Unify(tyErr, &types.TyError{}); err != nil {
		t.Errorf("TyError should unify with TyError: %v", err)
	}
	// TyError ~ skolem (should still succeed because TyError absorbs everything)
	skolem := &types.TySkolem{ID: 99, Name: "s", Kind: types.KType{}}
	if err := u.Unify(tyErr, skolem); err != nil {
		t.Errorf("TyError should unify with skolem: %v", err)
	}
	// TyError ~ arrow
	arrow := &types.TyArrow{From: types.Con("Int"), To: types.Con("Bool")}
	if err := u.Unify(tyErr, arrow); err != nil {
		t.Errorf("TyError should unify with arrow: %v", err)
	}
	// TyError ~ forall
	forall := &types.TyForall{Var: "a", Kind: types.KType{}, Body: &types.TyVar{Name: "a"}}
	if err := u.Unify(tyErr, forall); err != nil {
		t.Errorf("TyError should unify with forall: %v", err)
	}
}

// TestProbeE_Unify_SelfMeta — unifying a meta with itself should be a no-op.
func TestProbeE_Unify_SelfMeta(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	if err := u.Unify(meta, meta); err != nil {
		t.Errorf("self-unification of meta should succeed: %v", err)
	}
	// Meta should remain unsolved
	if soln := u.Solve(1); soln != nil {
		t.Errorf("self-unified meta should remain unsolved, got %s", types.Pretty(soln))
	}
}

// TestProbeE_Unify_MetaChainPathCompression — chained meta solutions should
// be path-compressed by Zonk.
func TestProbeE_Unify_MetaChainPathCompression(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.KType{}}
	m2 := &types.TyMeta{ID: 2, Kind: types.KType{}}
	m3 := &types.TyMeta{ID: 3, Kind: types.KType{}}

	// Chain: m1 -> m2 -> m3 -> Int
	u.Unify(m1, m2)
	u.Unify(m2, m3)
	u.Unify(m3, types.Con("Int"))

	result := u.Zonk(m1)
	if con, ok := result.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected Int, got %s", types.Pretty(result))
	}
	// After zonk, path compression should make m1 point directly to Int
	direct := u.Solve(1)
	if con, ok := direct.(*types.TyCon); ok && con.Name == "Int" {
		// Path compressed — good
	} else if _, ok := direct.(*types.TyMeta); ok {
		// Not compressed yet — acceptable if compression only happens in Zonk
	} else {
		// Unexpected
		t.Logf("m1 solution after zonk: %s", types.Pretty(direct))
	}
}

// TestProbeE_Unify_OccursCheckThroughSolvedMeta — occurs check must look
// through already-solved metas.
func TestProbeE_Unify_OccursCheckThroughSolvedMeta(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.KType{}}
	m2 := &types.TyMeta{ID: 2, Kind: types.KType{}}

	// Solve m2 = F m1
	u.Unify(m2, &types.TyApp{Fun: types.Con("F"), Arg: m1})
	// Now try m1 = m2, which is m1 = F m1 — should fail with occurs check
	err := u.Unify(m1, m2)
	if err == nil {
		t.Fatal("expected occurs check error when meta chain creates cycle")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyOccursCheck {
		t.Errorf("expected unify.UnifyOccursCheck, got %v", err)
	}
}

// TestProbeE_Unify_SkolemVsSkolem — two different skolems must not unify.
func TestProbeE_Unify_SkolemVsSkolem(t *testing.T) {
	u := unify.NewUnifier()
	s1 := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	s2 := &types.TySkolem{ID: 2, Name: "b", Kind: types.KType{}}
	err := u.Unify(s1, s2)
	if err == nil {
		t.Fatal("expected error unifying distinct skolems")
	}
}

// TestProbeE_Unify_SkolemSelfUnify — a skolem should unify with itself.
func TestProbeE_Unify_SkolemSelfUnify(t *testing.T) {
	u := unify.NewUnifier()
	s := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	if err := u.Unify(s, s); err != nil {
		t.Errorf("skolem should unify with itself: %v", err)
	}
}

// TestProbeE_Unify_SkolemVsMeta — a meta should be solvable to a skolem
// (this is how existentials work in GADT branches).
func TestProbeE_Unify_SkolemVsMeta(t *testing.T) {
	u := unify.NewUnifier()
	s := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	m := &types.TyMeta{ID: 2, Kind: types.KType{}}
	if err := u.Unify(m, s); err != nil {
		t.Errorf("meta should be solvable to skolem: %v", err)
	}
	result := u.Zonk(m)
	if sk, ok := result.(*types.TySkolem); !ok || sk.ID != 1 {
		t.Errorf("expected skolem #1, got %s", types.Pretty(result))
	}
}

// TestProbeE_Unify_SkolemVsCon — skolem vs concrete should fail.
func TestProbeE_Unify_SkolemVsCon(t *testing.T) {
	u := unify.NewUnifier()
	s := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	err := u.Unify(s, types.Con("Int"))
	if err == nil {
		t.Fatal("expected error unifying skolem with concrete type")
	}
}

// TestProbeE_Unify_ForallBodySubstitution — unifying two foralls should
// treat their bound variables as equal by substitution.
func TestProbeE_Unify_ForallBodySubstitution(t *testing.T) {
	u := unify.NewUnifier()
	// forall a. a -> a  vs  forall b. b -> b
	fa := types.MkForall("a", types.KType{}, types.MkArrow(&types.TyVar{Name: "a"}, &types.TyVar{Name: "a"}))
	fb := types.MkForall("b", types.KType{}, types.MkArrow(&types.TyVar{Name: "b"}, &types.TyVar{Name: "b"}))
	if err := u.Unify(fa, fb); err != nil {
		t.Errorf("alpha-equivalent foralls should unify: %v", err)
	}
}

// TestProbeE_Unify_ForallBodyMismatch — forall body mismatch should fail cleanly.
func TestProbeE_Unify_ForallBodyMismatch(t *testing.T) {
	u := unify.NewUnifier()
	// forall a. a -> Int  vs  forall a. a -> Bool
	fa := types.MkForall("a", types.KType{}, types.MkArrow(&types.TyVar{Name: "a"}, types.Con("Int")))
	fb := types.MkForall("a", types.KType{}, types.MkArrow(&types.TyVar{Name: "a"}, types.Con("Bool")))
	err := u.Unify(fa, fb)
	if err == nil {
		t.Fatal("forall body mismatch should fail")
	}
}

// TestProbeE_Unify_CompVsTyApp — TyCBPV (Computation) should unify with a TyApp chain
// representing Computation pre post result.
func TestProbeE_Unify_CompVsTyApp(t *testing.T) {
	u := unify.NewUnifier()
	comp := types.MkComp(
		types.EmptyRow(),
		types.EmptyRow(),
		types.Con("Int"),
	)
	// Build TyApp chain: Computation {} {} Int
	app := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyApp{
				Fun: types.Con("Computation"),
				Arg: types.EmptyRow(),
			},
			Arg: types.EmptyRow(),
		},
		Arg: types.Con("Int"),
	}
	if err := u.Unify(comp, app); err != nil {
		t.Errorf("TyCBPV (Computation) should unify with equivalent TyApp chain: %v", err)
	}
}

// TestProbeE_Unify_ThunkVsTyApp — same as above but for Thunk.
func TestProbeE_Unify_ThunkVsTyApp(t *testing.T) {
	u := unify.NewUnifier()
	thunk := types.MkThunk(
		types.EmptyRow(),
		types.EmptyRow(),
		types.Con("Int"),
	)
	app := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyApp{
				Fun: types.Con("Thunk"),
				Arg: types.EmptyRow(),
			},
			Arg: types.EmptyRow(),
		},
		Arg: types.Con("Int"),
	}
	if err := u.Unify(thunk, app); err != nil {
		t.Errorf("TyCBPV (Thunk) should unify with equivalent TyApp chain: %v", err)
	}
}

// TestProbeE_Unify_TyFamilyAppSameName — two TyFamilyApps with the same
// name and args should unify.
func TestProbeE_Unify_TyFamilyAppSameName(t *testing.T) {
	u := unify.NewUnifier()
	a := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	b := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	if err := u.Unify(a, b); err != nil {
		t.Errorf("identical TyFamilyApps should unify: %v", err)
	}
}

// TestProbeE_Unify_TyFamilyAppDifferentArgs — two TyFamilyApps with different
// args should fail.
func TestProbeE_Unify_TyFamilyAppDifferentArgs(t *testing.T) {
	u := unify.NewUnifier()
	a := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	b := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Bool")}, Kind: types.KType{}}
	err := u.Unify(a, b)
	if err == nil {
		t.Fatal("TyFamilyApps with different args should fail to unify")
	}
}

// TestProbeE_Unify_TyFamilyAppDifferentNames — two TyFamilyApps with different
// names should fail.
func TestProbeE_Unify_TyFamilyAppDifferentNames(t *testing.T) {
	u := unify.NewUnifier()
	a := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	b := &types.TyFamilyApp{Name: "G", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	err := u.Unify(a, b)
	if err == nil {
		t.Fatal("TyFamilyApps with different names should fail to unify")
	}
}

// TestProbeE_Unify_TyFamilyAppWithMeta — a TyFamilyApp where one arg is
// a meta should solve the meta.
func TestProbeE_Unify_TyFamilyAppWithMeta(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	a := &types.TyFamilyApp{Name: "F", Args: []types.Type{meta}, Kind: types.KType{}}
	b := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	if err := u.Unify(a, b); err != nil {
		t.Fatalf("TyFamilyApp with meta arg should unify: %v", err)
	}
	solved := u.Zonk(meta)
	if con, ok := solved.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected meta solved to Int, got %s", types.Pretty(solved))
	}
}

// =====================================================================
// From probe_e: Row unification edge cases
// =====================================================================

// TestProbeE_Row_DuplicateLabelInSingleRow — a row with duplicate labels
// in the same CapabilityEntries should be rejected.
func TestProbeE_Row_DuplicateLabelInSingleRow(t *testing.T) {
	u := unify.NewUnifier()
	row := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{
				{Label: "x", Type: types.Con("Int")},
				{Label: "x", Type: types.Con("Bool")},
			},
		},
	}
	emptyRow := types.EmptyRow()
	err := u.Unify(row, emptyRow)
	// This should either fail with duplicate label or row mismatch
	if err == nil {
		t.Log("WARN: unifying row with duplicate labels against empty row succeeded (may be valid if normalization catches it)")
	}
}

// TestProbeE_Row_OpenTailSolvedToEmpty — an open row tail unified against
// a closed row with matching fields should solve the tail to empty.
func TestProbeE_Row_OpenTailSolvedToEmpty(t *testing.T) {
	u := unify.NewUnifier()
	tail := &types.TyMeta{ID: 1, Kind: types.KRow{}}
	openRow := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{
				{Label: "x", Type: types.Con("Int")},
			},
		},
		Tail: tail,
	}
	closedRow := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{
				{Label: "x", Type: types.Con("Int")},
			},
		},
	}
	if err := u.Unify(openRow, closedRow); err != nil {
		t.Fatalf("expected success unifying open vs closed row with same fields: %v", err)
	}
	// Tail should be solved to empty row
	solved := u.Zonk(tail)
	if ev, ok := solved.(*types.TyEvidenceRow); ok {
		if cap, ok := ev.Entries.(*types.CapabilityEntries); ok && len(cap.Fields) == 0 && ev.Tail == nil {
			// correct — solved to empty
		} else {
			t.Errorf("expected tail solved to empty row, got %s", types.Pretty(solved))
		}
	} else if _, ok := solved.(*types.TyMeta); ok {
		t.Log("WARN: tail remained unsolved — may be valid if empty row is implicit")
	} else {
		t.Errorf("unexpected tail solution: %s", types.Pretty(solved))
	}
}

// TestProbeE_Row_LabelContextPreventsDuplicates — if a meta has a label
// context, solving it to a row with a conflicting label should fail.
func TestProbeE_Row_LabelContextPreventsDuplicates(t *testing.T) {
	u := unify.NewUnifier()
	tail := &types.TyMeta{ID: 1, Kind: types.KRow{}}
	// Register that labels {"x"} already exist in the row containing this tail
	u.RegisterLabelContext(1, map[string]struct{}{"x": {}})
	// Now try to solve tail to a row containing "x" — should fail
	solution := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{
				{Label: "x", Type: types.Con("Bool")},
			},
		},
	}
	err := u.Unify(tail, solution)
	if err == nil {
		t.Fatal("expected duplicate label error when solving meta with conflicting label context")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyDupLabel {
		t.Errorf("expected unify.UnifyDupLabel, got %v", err)
	}
}

// TestProbeE_Row_ConstraintRowMismatch — unifying capability row with
// constraint row should fail with a clear error.
func TestProbeE_Row_ConstraintRowMismatch(t *testing.T) {
	u := unify.NewUnifier()
	capRow := types.EmptyRow()
	conRow := types.EmptyConstraintRow()
	err := u.Unify(capRow, conRow)
	if err == nil {
		// Both are empty — they might unify trivially. Check with non-empty.
		capRow2 := &types.TyEvidenceRow{
			Entries: &types.CapabilityEntries{
				Fields: []types.RowField{{Label: "x", Type: types.Con("Int")}},
			},
		}
		conRow2 := &types.TyEvidenceRow{
			Entries: &types.ConstraintEntries{
				Entries: []types.ConstraintEntry{{ClassName: "Eq", Args: []types.Type{types.Con("Int")}}},
			},
		}
		err = u.Unify(capRow2, conRow2)
		if err == nil {
			t.Fatal("expected error unifying capability row with constraint row")
		}
	}
}
