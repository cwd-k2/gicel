package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/check/unify"
	"github.com/cwd-k2/gicel/internal/types"
)

// =============================================================================
// CtxEvidence
// =============================================================================

func TestCtxEvidenceIsCtxEntry(t *testing.T) {
	e := &CtxEvidence{
		ClassName: "Eq",
		Args:      []types.Type{types.Con("Int")},
		DictName:  "$d_Eq_1",
		DictType:  types.Con("Eq$Dict"),
	}
	// Must satisfy CtxEntry interface.
	var _ CtxEntry = e
}

func TestContextLookupEvidence(t *testing.T) {
	ctx := NewContext()
	ctx.Push(&CtxEvidence{
		ClassName: "Eq",
		Args:      []types.Type{types.Con("Int")},
		DictName:  "$d_Eq_1",
		DictType:  &types.TyApp{Fun: types.Con("Eq$Dict"), Arg: types.Con("Int")},
	})
	ctx.Push(&CtxEvidence{
		ClassName: "Ord",
		Args:      []types.Type{types.Con("Int")},
		DictName:  "$d_Ord_2",
		DictType:  &types.TyApp{Fun: types.Con("Ord$Dict"), Arg: types.Con("Int")},
	})

	// LookupEvidence should find Eq.
	evs := ctx.LookupEvidence("Eq")
	if len(evs) != 1 {
		t.Fatalf("expected 1 Eq evidence, got %d", len(evs))
	}
	if evs[0].DictName != "$d_Eq_1" {
		t.Errorf("expected dict name $d_Eq_1, got %s", evs[0].DictName)
	}

	// LookupEvidence should find Ord.
	evs = ctx.LookupEvidence("Ord")
	if len(evs) != 1 {
		t.Fatalf("expected 1 Ord evidence, got %d", len(evs))
	}

	// Non-existent class.
	evs = ctx.LookupEvidence("Show")
	if len(evs) != 0 {
		t.Errorf("expected 0 Show evidence, got %d", len(evs))
	}
}

func TestContextEvidencePopRestoresScope(t *testing.T) {
	ctx := NewContext()
	ctx.Push(&CtxEvidence{
		ClassName: "Eq",
		Args:      []types.Type{types.Con("Int")},
		DictName:  "$d_Eq_1",
		DictType:  types.Con("Eq$Dict"),
	})
	ctx.Pop()
	evs := ctx.LookupEvidence("Eq")
	if len(evs) != 0 {
		t.Errorf("after Pop, should have 0 evidence entries, got %d", len(evs))
	}
}

// =============================================================================
// collectContextEvidence / classifyEvidence
// =============================================================================

func TestCollectContextEvidence(t *testing.T) {
	ch := &Checker{
		ctx: NewContext(),
	}
	ch.unifier = unify.NewUnifierShared(&ch.freshID)

	ch.ctx.Push(&CtxEvidence{
		ClassName: "Eq",
		Args:      []types.Type{types.Con("Int")},
		DictName:  "$d_Eq_1",
		DictType:  types.Con("Eq$Dict"),
	})
	ch.ctx.Push(&CtxVar{Name: "x", Type: types.Con("Int")})
	ch.ctx.Push(&CtxEvidence{
		ClassName: "Ord",
		Args:      []types.Type{types.Con("Int")},
		DictName:  "$d_Ord_2",
		DictType:  types.Con("Ord$Dict"),
	})

	avail := ch.collectContextEvidence()
	if len(avail) != 2 {
		t.Fatalf("expected 2 available evidence, got %d", len(avail))
	}
	// Should be in reverse order (most recent first).
	if avail[0].className != "Ord" {
		t.Errorf("expected Ord first, got %s", avail[0].className)
	}
	if avail[1].className != "Eq" {
		t.Errorf("expected Eq second, got %s", avail[1].className)
	}
}

func TestClassifyEvidenceAllMatched(t *testing.T) {
	ch := &Checker{
		ctx: NewContext(),
	}
	ch.unifier = unify.NewUnifierShared(&ch.freshID)

	available := []availableEvidence{
		{className: "Eq", args: []types.Type{types.Con("Int")}},
	}
	wanted := []deferredConstraint{
		{placeholder: "$dict_1", className: "Eq", args: []types.Type{types.Con("Int")}},
	}
	matched, unmatched := ch.classifyEvidence(wanted, available)
	if len(matched) != 1 {
		t.Fatalf("expected 1 matched, got %d", len(matched))
	}
	if len(unmatched) != 0 {
		t.Fatalf("expected 0 unmatched, got %d", len(unmatched))
	}
}

func TestClassifyEvidencePartial(t *testing.T) {
	ch := &Checker{
		ctx: NewContext(),
	}
	ch.unifier = unify.NewUnifierShared(&ch.freshID)

	available := []availableEvidence{
		{className: "Eq", args: []types.Type{types.Con("Int")}},
	}
	wanted := []deferredConstraint{
		{placeholder: "$dict_1", className: "Eq", args: []types.Type{types.Con("Int")}},
		{placeholder: "$dict_2", className: "Ord", args: []types.Type{types.Con("Int")}},
	}
	matched, unmatched := ch.classifyEvidence(wanted, available)
	if len(matched) != 1 {
		t.Fatalf("expected 1 matched, got %d", len(matched))
	}
	if len(unmatched) != 1 {
		t.Fatalf("expected 1 unmatched, got %d", len(unmatched))
	}
	if unmatched[0].className != "Ord" {
		t.Errorf("expected unmatched Ord, got %s", unmatched[0].className)
	}
}

// =============================================================================
// Integration: TyEvidence check mode works with CtxEvidence
// =============================================================================

func TestCheckTyEvidenceWithEvidence(t *testing.T) {
	// Regression test: TyEvidence check mode works with CtxEvidence.
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y
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

func TestCheckMultiConstraintResolution(t *testing.T) {
	// Test that multiple constraints ((Eq a, Ord a) => ...) resolve correctly.
	source := `data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }
f :: \ a. (Eq a, Ord a) => a -> Bool
f := \x. eq x x
main := f True`
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
