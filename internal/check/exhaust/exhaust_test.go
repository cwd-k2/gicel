package exhaust

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/types"
)

// --- formatWitness ---

func TestFormatWitnessNullary(t *testing.T) {
	w := formatWitness(pCon{con: "Nothing", arity: 0, args: nil})
	if w != "Nothing" {
		t.Errorf("expected 'Nothing', got %q", w)
	}
}

func TestFormatWitnessWithArgs(t *testing.T) {
	w := formatWitness(pCon{con: "Just", arity: 1, args: []pat{pWild{}}})
	if w != "Just _" {
		t.Errorf("expected 'Just _', got %q", w)
	}
}

func TestFormatWitnessNested(t *testing.T) {
	inner := pCon{con: "Just", arity: 1, args: []pat{pWild{}}}
	w := formatWitness(pCon{con: "Pair", arity: 2, args: []pat{inner, pWild{}}})
	if w != "Pair (Just _) _" {
		t.Errorf("expected 'Pair (Just _) _', got %q", w)
	}
}

func TestFormatWitnessWild(t *testing.T) {
	if formatWitness(pWild{}) != "_" {
		t.Error("expected '_' for pWild")
	}
}

func TestFormatWitnessRecord(t *testing.T) {
	r := pRecord{fields: map[string]pat{"x": pWild{}, "y": pWild{}}}
	w := formatWitness(r)
	if w != "{ x: _, y: _ }" {
		t.Errorf("expected '{ x: _, y: _ }', got %q", w)
	}
}

func TestFormatWitnessEmptyRecord(t *testing.T) {
	r := pRecord{fields: map[string]pat{}}
	if formatWitness(r) != "{}" {
		t.Error("expected '{}' for empty record")
	}
}

// --- specialize / defaultMatrix unit tests ---

func TestSpecializeConMatch(t *testing.T) {
	mx := patMatrix{
		{pCon{con: "True", arity: 0}, pWild{}},
		{pCon{con: "False", arity: 0}, pWild{}},
		{pWild{}, pWild{}},
	}
	result := specialize(mx, "True", 0)
	// Should have 2 rows: the True row + the wildcard row
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
}

func TestDefaultMatrixFilters(t *testing.T) {
	mx := patMatrix{
		{pCon{con: "True", arity: 0}, pWild{}},
		{pWild{}, pCon{con: "A", arity: 0}},
		{pCon{con: "False", arity: 0}, pWild{}},
	}
	result := defaultMatrix(mx)
	// Only the wildcard row (second row) survives
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
}

func TestColumnHeadCons(t *testing.T) {
	mx := patMatrix{
		{pCon{con: "A", arity: 0}},
		{pCon{con: "B", arity: 1, args: []pat{pWild{}}}},
		{pWild{}},
		{pCon{con: "A", arity: 0}},
	}
	result := columnHeadCons(mx)
	if len(result) != 2 {
		t.Fatalf("expected 2 unique constructors, got %d", len(result))
	}
	if _, ok := result["A"]; !ok {
		t.Error("expected 'A' in result")
	}
	if _, ok := result["B"]; !ok {
		t.Error("expected 'B' in result")
	}
}

// --- exhaust.go coverage: internal helpers ---

func TestAllRecordLabelsEmpty(t *testing.T) {
	// Empty matrix should return no labels.
	labels := allRecordLabels(patMatrix{})
	if len(labels) != 0 {
		t.Errorf("expected no labels from empty matrix, got %v", labels)
	}
}

func TestAllRecordLabelsWildcardOnly(t *testing.T) {
	// Matrix with only wildcards should return no labels.
	mx := patMatrix{
		{pWild{}},
		{pWild{}},
	}
	labels := allRecordLabels(mx)
	if len(labels) != 0 {
		t.Errorf("expected no labels from wildcard-only matrix, got %v", labels)
	}
}

func TestAllRecordLabelsCollectsAndSorts(t *testing.T) {
	// Multiple rows with different record patterns should collect unique, sorted labels.
	mx := patMatrix{
		{pRecord{fields: map[string]pat{"y": pWild{}, "x": pWild{}}}},
		{pRecord{fields: map[string]pat{"x": pWild{}, "z": pWild{}}}},
	}
	labels := allRecordLabels(mx)
	if len(labels) != 3 {
		t.Fatalf("expected 3 labels, got %d: %v", len(labels), labels)
	}
	if labels[0] != "x" || labels[1] != "y" || labels[2] != "z" {
		t.Errorf("expected [x y z], got %v", labels)
	}
}

func TestSpecializeRecordExpandsLabels(t *testing.T) {
	labels := []string{"a", "b"}
	mx := patMatrix{
		{pRecord{fields: map[string]pat{"a": pCon{con: "True", arity: 0}}}},
		{pWild{}},
	}
	result := specializeRecord(mx, labels)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
	// First row: a=True, b=_
	if cp, ok := result[0][0].(pCon); !ok || cp.con != "True" {
		t.Errorf("row[0][0] should be True constructor, got %v", result[0][0])
	}
	if _, ok := result[0][1].(pWild); !ok {
		t.Errorf("row[0][1] should be wildcard for missing label b, got %v", result[0][1])
	}
	// Second row: all wildcards
	for i := range 2 {
		if _, ok := result[1][i].(pWild); !ok {
			t.Errorf("row[1][%d] should be wildcard, got %v", i, result[1][i])
		}
	}
}

func TestSpecializeRecordEmptyLabels(t *testing.T) {
	// Empty labels list should produce rows with rest columns only.
	mx := patMatrix{
		{pRecord{fields: map[string]pat{}}, pWild{}},
	}
	result := specializeRecord(mx, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	// Row should have only the rest column (pWild).
	if len(result[0]) != 1 {
		t.Errorf("expected 1 column, got %d", len(result[0]))
	}
}

func TestNilTypesWithTail(t *testing.T) {
	tail := []types.Type{&types.TyCon{Name: "Int"}, &types.TyCon{Name: "Bool"}}
	result := nilTypesWithTail(3, tail)
	if len(result) != 5 {
		t.Fatalf("expected 5 elements, got %d", len(result))
	}
	for i := range 3 {
		if result[i] != nil {
			t.Errorf("result[%d] should be nil, got %v", i, result[i])
		}
	}
	if result[3].(*types.TyCon).Name != "Int" {
		t.Errorf("result[3] should be Int, got %v", result[3])
	}
	if result[4].(*types.TyCon).Name != "Bool" {
		t.Errorf("result[4] should be Bool, got %v", result[4])
	}
}

func TestNilTypesWithTailZero(t *testing.T) {
	tail := []types.Type{&types.TyCon{Name: "X"}}
	result := nilTypesWithTail(0, tail)
	if len(result) != 1 {
		t.Fatalf("expected 1 element, got %d", len(result))
	}
	if result[0].(*types.TyCon).Name != "X" {
		t.Errorf("expected X, got %v", result[0])
	}
}

func TestNilTypesWithTailEmptyTail(t *testing.T) {
	result := nilTypesWithTail(2, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result))
	}
	for i := range 2 {
		if result[i] != nil {
			t.Errorf("result[%d] should be nil", i)
		}
	}
}

func TestMakeWildcardVec(t *testing.T) {
	tail := patVec{pCon{con: "X", arity: 0}}
	result := makeWildcardVec(3, tail)
	if len(result) != 4 {
		t.Fatalf("expected 4 patterns, got %d", len(result))
	}
	for i := range 3 {
		if _, ok := result[i].(pWild); !ok {
			t.Errorf("result[%d] should be wildcard, got %v", i, result[i])
		}
	}
	if cp, ok := result[3].(pCon); !ok || cp.con != "X" {
		t.Errorf("result[3] should be X constructor, got %v", result[3])
	}
}

func TestMakeWildcardVecZero(t *testing.T) {
	tail := patVec{pWild{}}
	result := makeWildcardVec(0, tail)
	if len(result) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(result))
	}
}

func TestHasRecordPats(t *testing.T) {
	// No record patterns.
	mx := patMatrix{
		{pWild{}},
		{pCon{con: "A", arity: 0}},
	}
	if hasRecordPats(mx) {
		t.Error("expected false for matrix without record patterns")
	}

	// With record pattern.
	mx2 := patMatrix{
		{pWild{}},
		{pRecord{fields: map[string]pat{"x": pWild{}}}},
	}
	if !hasRecordPats(mx2) {
		t.Error("expected true for matrix with record pattern")
	}

	// Empty matrix.
	if hasRecordPats(patMatrix{}) {
		t.Error("expected false for empty matrix")
	}

	// Row with empty vec.
	mx3 := patMatrix{{}}
	if hasRecordPats(mx3) {
		t.Error("expected false for matrix with empty row")
	}
}

func TestCoreToPatRecord(t *testing.T) {
	// PRecord pattern should convert to pRecord.
	p := &core.PRecord{Fields: []core.PRecordField{
		{Label: "x", Pattern: &core.PVar{Name: "a"}},
		{Label: "y", Pattern: &core.PWild{}},
	}}
	result := coreToPat(p)
	rp, ok := result.(pRecord)
	if !ok {
		t.Fatalf("expected pRecord, got %T", result)
	}
	if len(rp.fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rp.fields))
	}
	if _, ok := rp.fields["x"].(pWild); !ok {
		t.Error("PVar should convert to pWild")
	}
	if _, ok := rp.fields["y"].(pWild); !ok {
		t.Error("PWild should convert to pWild")
	}
}

func TestCoreToPatNilDefault(t *testing.T) {
	// Unknown pattern type should default to pWild.
	// We can test this with nil interface (which is the default case).
	var p core.Pattern // nil
	result := coreToPat(p)
	if _, ok := result.(pWild); !ok {
		t.Fatalf("expected pWild for nil pattern, got %T", result)
	}
}

func TestHeadTyCon(t *testing.T) {
	// Simple TyCon.
	tc := &types.TyCon{Name: "Bool"}
	if name := headTyCon(tc); name != "Bool" {
		t.Errorf("expected Bool, got %s", name)
	}

	// TyApp with TyCon head.
	ta := &types.TyApp{Fun: &types.TyCon{Name: "Maybe"}, Arg: &types.TyCon{Name: "Int"}}
	if name := headTyCon(ta); name != "Maybe" {
		t.Errorf("expected Maybe, got %s", name)
	}

	// Nested TyApp.
	nested := &types.TyApp{
		Fun: &types.TyApp{Fun: &types.TyCon{Name: "Either"}, Arg: &types.TyCon{Name: "Int"}},
		Arg: &types.TyCon{Name: "String"},
	}
	if name := headTyCon(nested); name != "Either" {
		t.Errorf("expected Either, got %s", name)
	}

	// TyVar (no constructor).
	tv := &types.TyVar{Name: "a"}
	if name := headTyCon(tv); name != "" {
		t.Errorf("expected empty string for TyVar, got %s", name)
	}

	// TyArrow (no constructor).
	arr := &types.TyArrow{From: &types.TyCon{Name: "Int"}, To: &types.TyCon{Name: "Bool"}}
	if name := headTyCon(arr); name != "" {
		t.Errorf("expected empty string for TyArrow, got %s", name)
	}
}

func TestReconstructConInnerMatch(t *testing.T) {
	// When inner is a pCon matching the constructor name, args should propagate.
	inner := pCon{con: "Just", arity: 1, args: []pat{pCon{con: "True", arity: 0}}}
	result := reconstructCon("Just", 1, inner, nil)
	rc, ok := result.(pCon)
	if !ok {
		t.Fatalf("expected pCon, got %T", result)
	}
	if rc.con != "Just" {
		t.Errorf("expected Just, got %s", rc.con)
	}
	if len(rc.args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(rc.args))
	}
	if inner, ok := rc.args[0].(pCon); !ok || inner.con != "True" {
		t.Errorf("expected True sub-pattern, got %v", rc.args[0])
	}
}

func TestReconstructConInnerMismatch(t *testing.T) {
	// When inner doesn't match (different constructor name), args should be wildcards.
	inner := pCon{con: "Nothing", arity: 0}
	result := reconstructCon("Just", 1, inner, nil)
	rc := result.(pCon)
	if _, ok := rc.args[0].(pWild); !ok {
		t.Errorf("expected wildcard for mismatched inner, got %v", rc.args[0])
	}
}

func TestReconstructConZeroArity(t *testing.T) {
	result := reconstructCon("True", 0, pWild{}, nil)
	rc := result.(pCon)
	if rc.con != "True" || rc.arity != 0 || len(rc.args) != 0 {
		t.Errorf("expected True/0, got %s/%d", rc.con, rc.arity)
	}
}

func TestFormatWitnessDefault(t *testing.T) {
	// nil pat should return "_".
	result := formatWitness(nil)
	if result != "_" {
		t.Errorf("expected '_' for nil witness, got %q", result)
	}
}

func TestColumnHeadConsEmpty(t *testing.T) {
	result := columnHeadCons(patMatrix{})
	if len(result) != 0 {
		t.Errorf("expected empty map for empty matrix, got %v", result)
	}
}

func TestColumnHeadConsEmptyRows(t *testing.T) {
	// Rows with empty vecs should be skipped.
	mx := patMatrix{{}, {pCon{con: "A", arity: 0}}}
	result := columnHeadCons(mx)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if _, ok := result["A"]; !ok {
		t.Error("expected entry for A")
	}
}

func TestDefaultMatrixEmpty(t *testing.T) {
	result := defaultMatrix(patMatrix{})
	if len(result) != 0 {
		t.Errorf("expected empty default matrix, got %d rows", len(result))
	}
}

func TestDefaultMatrixEmptyRows(t *testing.T) {
	mx := patMatrix{{}, {pWild{}, pCon{con: "X", arity: 0}}}
	result := defaultMatrix(mx)
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	// The remaining column should be X.
	if cp, ok := result[0][0].(pCon); !ok || cp.con != "X" {
		t.Errorf("expected X, got %v", result[0][0])
	}
}

func TestSpecializeEmpty(t *testing.T) {
	result := specialize(patMatrix{}, "A", 0)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d rows", len(result))
	}
}

func TestSpecializeEmptyRows(t *testing.T) {
	mx := patMatrix{{}}
	result := specialize(mx, "A", 0)
	if len(result) != 0 {
		t.Errorf("expected empty result (empty rows skipped), got %d", len(result))
	}
}

func TestSpecializeRecordSkipsConstructor(t *testing.T) {
	// specializeRecord should skip pCon patterns (not pRecord or pWild).
	labels := []string{"x"}
	mx := patMatrix{
		{pCon{con: "A", arity: 0}},
	}
	result := specializeRecord(mx, labels)
	if len(result) != 0 {
		t.Errorf("expected empty result (pCon skipped), got %d rows", len(result))
	}
}
