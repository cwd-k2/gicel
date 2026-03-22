// Constraint solver infrastructure tests — Ct, Worklist, InertSet.
// Does NOT cover: solver.go, deferred.go.
package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Worklist tests ---

func TestWorklistPushPop(t *testing.T) {
	var w Worklist
	if !w.Empty() {
		t.Fatal("new worklist should be empty")
	}
	c1 := &CtClass{Placeholder: "a", ClassName: "Eq"}
	c2 := &CtClass{Placeholder: "b", ClassName: "Num"}
	w.Push(c1)
	w.Push(c2)
	if w.Len() != 2 {
		t.Fatalf("expected len 2, got %d", w.Len())
	}
	got, ok := w.Pop()
	if !ok || got != c1 {
		t.Fatal("expected c1 from Pop")
	}
	got, ok = w.Pop()
	if !ok || got != c2 {
		t.Fatal("expected c2 from Pop")
	}
	_, ok = w.Pop()
	if ok {
		t.Fatal("expected Pop to fail on empty worklist")
	}
}

func TestWorklistPushFront(t *testing.T) {
	var w Worklist
	c1 := &CtClass{Placeholder: "a", ClassName: "Eq"}
	c2 := &CtClass{Placeholder: "b", ClassName: "Num"}
	c3 := &CtClass{Placeholder: "c", ClassName: "Show"}
	w.Push(c1)
	w.PushFront(c2, c3)
	// Order should be: c2, c3, c1
	got, _ := w.Pop()
	if got != c2 {
		t.Fatal("expected c2 first after PushFront")
	}
	got, _ = w.Pop()
	if got != c3 {
		t.Fatal("expected c3 second")
	}
	got, _ = w.Pop()
	if got != c1 {
		t.Fatal("expected c1 third")
	}
}

func TestWorklistReset(t *testing.T) {
	var w Worklist
	w.Push(&CtClass{Placeholder: "a"})
	w.Push(&CtClass{Placeholder: "b"})
	w.Reset()
	if !w.Empty() {
		t.Fatal("worklist should be empty after Reset")
	}
}

func TestWorklistPushFrontEmpty(t *testing.T) {
	var w Worklist
	w.Push(&CtClass{Placeholder: "a"})
	w.PushFront() // no-op
	if w.Len() != 1 {
		t.Fatal("PushFront with no args should be no-op")
	}
}

// --- InertSet tests ---

func TestInertSetInsertLookupClass(t *testing.T) {
	var is InertSet
	ct1 := &CtClass{Placeholder: "d1", ClassName: "Eq", Args: []types.Type{&types.TyCon{Name: "Int"}}}
	ct2 := &CtClass{Placeholder: "d2", ClassName: "Eq", Args: []types.Type{&types.TyCon{Name: "Bool"}}}
	ct3 := &CtClass{Placeholder: "d3", ClassName: "Num", Args: []types.Type{&types.TyCon{Name: "Int"}}}
	is.InsertClass(ct1, "")
	is.InsertClass(ct2, "")
	is.InsertClass(ct3, "")
	eqs := is.LookupClass("Eq")
	if len(eqs) != 2 {
		t.Fatalf("expected 2 Eq constraints, got %d", len(eqs))
	}
	nums := is.LookupClass("Num")
	if len(nums) != 1 {
		t.Fatalf("expected 1 Num constraint, got %d", len(nums))
	}
	if len(is.LookupClass("Show")) != 0 {
		t.Fatal("expected no Show constraints")
	}
}

func TestInertSetInsertLookupFunEq(t *testing.T) {
	var is InertSet
	ct := &CtFunEq{
		FamilyName: "Add",
		Args:       []types.Type{&types.TyCon{Name: "Int"}, &types.TyCon{Name: "Int"}},
		ResultMeta: &types.TyMeta{ID: 1},
		BlockingOn: []int{},
	}
	is.InsertFunEq(ct)
	found := is.LookupFunEq("Add")
	if len(found) != 1 || found[0] != ct {
		t.Fatal("expected to find the Add equation")
	}
	if len(is.LookupFunEq("Mul")) != 0 {
		t.Fatal("expected no Mul equations")
	}
}

func TestInertSetKickOut(t *testing.T) {
	var is InertSet
	meta := &types.TyMeta{ID: 42}
	ct1 := &CtClass{
		Placeholder: "d1", ClassName: "Eq",
		Args: []types.Type{meta},
	}
	ct2 := &CtClass{
		Placeholder: "d2", ClassName: "Num",
		Args: []types.Type{&types.TyCon{Name: "Int"}},
	}
	ct3 := &CtFunEq{
		FamilyName: "F", ResultMeta: &types.TyMeta{ID: 99},
		Args: []types.Type{meta}, BlockingOn: []int{42},
	}
	is.InsertClass(ct1, "")
	is.InsertClass(ct2, "")
	is.InsertFunEq(ct3)

	kicked := is.KickOut(42)
	if len(kicked) != 2 {
		t.Fatalf("expected 2 kicked constraints, got %d", len(kicked))
	}
	// ct1 should be removed from classMap
	if len(is.LookupClass("Eq")) != 0 {
		t.Fatal("Eq should be empty after kickout")
	}
	// ct2 should remain
	if len(is.LookupClass("Num")) != 1 {
		t.Fatal("Num should remain")
	}
	// ct3 should be removed from funEqs
	if len(is.LookupFunEq("F")) != 0 {
		t.Fatal("F should be empty after kickout")
	}
}

func TestInertSetKickOutNoMatch(t *testing.T) {
	var is InertSet
	ct := &CtClass{Placeholder: "d1", ClassName: "Eq", Args: []types.Type{&types.TyCon{Name: "Int"}}}
	is.InsertClass(ct, "")
	kicked := is.KickOut(999) // meta 999 not in any constraint
	if len(kicked) != 0 {
		t.Fatal("expected no kicked constraints for unknown meta")
	}
	if len(is.LookupClass("Eq")) != 1 {
		t.Fatal("Eq should remain after no-op kickout")
	}
}

func TestInertSetCollectClassResiduals(t *testing.T) {
	var is InertSet
	ct1 := &CtClass{Placeholder: "d1", ClassName: "Eq", Args: []types.Type{&types.TyCon{Name: "Int"}}}
	ct2 := &CtClass{Placeholder: "d2", ClassName: "Num", Args: []types.Type{&types.TyCon{Name: "Int"}}}
	is.InsertClass(ct1, "")
	is.InsertClass(ct2, "")
	residuals := is.CollectClassResiduals()
	if len(residuals) != 2 {
		t.Fatalf("expected 2 residuals, got %d", len(residuals))
	}
}

func TestInertSetReset(t *testing.T) {
	var is InertSet
	is.InsertClass(&CtClass{Placeholder: "d1", ClassName: "Eq", Args: []types.Type{&types.TyCon{Name: "Int"}}}, "")
	is.InsertFunEq(&CtFunEq{FamilyName: "F", BlockingOn: []int{1}})
	is.Reset()
	if len(is.CollectClassResiduals()) != 0 {
		t.Fatal("expected no residuals after Reset")
	}
	if len(is.LookupFunEq("F")) != 0 {
		t.Fatal("expected no funEqs after Reset")
	}
}

// --- Ct interface tests ---

func TestCtInterfaceCompliance(t *testing.T) {
	s := span.Span{Start: 0, End: 10}
	var ct Ct

	cc := &CtClass{Placeholder: "p1", ClassName: "Eq", S: s}
	ct = cc
	if cc.ctPlaceholder() != "p1" {
		t.Fatal("CtClass placeholder mismatch")
	}
	if ct.ctSpan() != s {
		t.Fatal("CtClass span mismatch")
	}

	ct = &CtFunEq{FamilyName: "F", S: s}
	if ct.ctSpan() != s {
		t.Fatal("CtFunEq span mismatch")
	}

	ct = &CtImplication{S: s}
	if ct.ctSpan() != s {
		t.Fatal("CtImplication span mismatch")
	}
}

// --- collectMetaIDs tests ---

func TestCollectMetaIDs(t *testing.T) {
	m1 := &types.TyMeta{ID: 1}
	m2 := &types.TyMeta{ID: 2}
	ids := collectMetaIDs([]types.Type{
		&types.TyArrow{From: m1, To: &types.TyCon{Name: "Int"}},
		m2,
		m1, // duplicate
	})
	if len(ids) != 2 {
		t.Fatalf("expected 2 unique meta IDs, got %d", len(ids))
	}
	idSet := make(map[int]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	if !idSet[1] || !idSet[2] {
		t.Fatalf("expected IDs {1, 2}, got %v", ids)
	}
}

func TestCollectMetaIDsNoMetas(t *testing.T) {
	ids := collectMetaIDs([]types.Type{&types.TyCon{Name: "Int"}, &types.TyCon{Name: "Bool"}})
	if len(ids) != 0 {
		t.Fatalf("expected 0 meta IDs, got %d", len(ids))
	}
}

// --- KickOut with nested meta ---

func TestInertSetKickOutNestedMeta(t *testing.T) {
	var is InertSet
	meta := &types.TyMeta{ID: 7}
	// Meta nested inside TyApp
	ct := &CtClass{
		Placeholder: "d1", ClassName: "Show",
		Args: []types.Type{&types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: meta}},
	}
	is.InsertClass(ct, "")
	kicked := is.KickOut(7)
	if len(kicked) != 1 {
		t.Fatalf("expected 1 kicked constraint for nested meta, got %d", len(kicked))
	}
	if len(is.LookupClass("Show")) != 0 {
		t.Fatal("Show should be removed after kickout")
	}
}
