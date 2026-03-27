// Constraint solver infrastructure tests — Ct, Worklist, InertSet.
// Does NOT cover: solver.go, deferred.go.
package solve

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

// --- Scope-aware Reset tests ---

func TestInertSetScopeAwareReset(t *testing.T) {
	// Constraints inserted at outer scope survive Reset at inner scope.
	var is InertSet
	outerCt := &CtClass{Placeholder: "d1", ClassName: "Eq", Args: []types.Type{&types.TyCon{Name: "Int"}}}
	is.InsertClass(outerCt, "")

	is.EnterScope()
	innerCt := &CtClass{Placeholder: "d2", ClassName: "Num", Args: []types.Type{&types.TyCon{Name: "Int"}}}
	is.InsertClass(innerCt, "")

	// Reset at inner scope: only innerCt should be cleared.
	is.Reset()

	if len(is.LookupClass("Eq")) != 1 {
		t.Fatal("outer Eq should survive inner Reset")
	}
	if len(is.LookupClass("Num")) != 0 {
		t.Fatal("inner Num should be cleared by Reset")
	}
	is.LeaveScope()
}

func TestInertSetLeaveScope(t *testing.T) {
	// LeaveScope clears inner constraints and decrements depth.
	var is InertSet
	outerCt := &CtFunEq{FamilyName: "F", BlockingOn: []int{1}}
	is.InsertFunEq(outerCt)

	is.EnterScope()
	innerCt := &CtFunEq{FamilyName: "G", BlockingOn: []int{2}}
	is.InsertFunEq(innerCt)

	is.LeaveScope()

	if len(is.LookupFunEq("F")) != 1 {
		t.Fatal("outer F should survive LeaveScope")
	}
	if len(is.LookupFunEq("G")) != 0 {
		t.Fatal("inner G should be cleared by LeaveScope")
	}
	if is.ScopeDepth() != 0 {
		t.Fatalf("expected depth 0 after LeaveScope, got %d", is.ScopeDepth())
	}
}

func TestInertSetResetAtDepthZero(t *testing.T) {
	// Reset at depth 0 clears all constraints (backward compatible).
	var is InertSet
	is.InsertClass(&CtClass{Placeholder: "d1", ClassName: "Eq", Args: []types.Type{&types.TyCon{Name: "Int"}}}, "")
	is.InsertFunEq(&CtFunEq{FamilyName: "F", BlockingOn: []int{1}})
	is.Reset()
	if len(is.CollectClassResiduals()) != 0 {
		t.Fatal("expected no residuals after Reset at depth 0")
	}
	if len(is.LookupFunEq("F")) != 0 {
		t.Fatal("expected no funEqs after Reset at depth 0")
	}
}

func TestInertSetNestedScopes(t *testing.T) {
	// Three levels: constraints at each level survive resets at deeper levels.
	var is InertSet
	ct0 := &CtClass{Placeholder: "d0", ClassName: "A", Args: []types.Type{&types.TyCon{Name: "Int"}}}
	is.InsertClass(ct0, "")

	is.EnterScope() // depth 1
	ct1 := &CtClass{Placeholder: "d1", ClassName: "B", Args: []types.Type{&types.TyCon{Name: "Int"}}}
	is.InsertClass(ct1, "")

	is.EnterScope() // depth 2
	ct2 := &CtClass{Placeholder: "d2", ClassName: "C", Args: []types.Type{&types.TyCon{Name: "Int"}}}
	is.InsertClass(ct2, "")

	// Reset at depth 2: only ct2 cleared.
	is.Reset()
	if len(is.LookupClass("A")) != 1 {
		t.Fatal("A should survive")
	}
	if len(is.LookupClass("B")) != 1 {
		t.Fatal("B should survive")
	}
	if len(is.LookupClass("C")) != 0 {
		t.Fatal("C should be cleared")
	}

	is.LeaveScope() // back to depth 1
	is.LeaveScope() // back to depth 0

	// Only ct0 remains.
	if len(is.LookupClass("A")) != 1 {
		t.Fatal("A should survive all scope exits")
	}
	if len(is.LookupClass("B")) != 0 {
		t.Fatal("B should be cleared by LeaveScope")
	}
}

// --- Ct interface tests ---

func TestCtInterfaceCompliance(t *testing.T) {
	s := span.Span{Start: 0, End: 10}
	var ct Ct

	cc := &CtClass{Placeholder: "p1", ClassName: "Eq", S: s}
	ct = cc
	if cc.Placeholder != "p1" {
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

// --- CtFlavor tests ---

func TestCtFlavorDefaults(t *testing.T) {
	// Zero value of CtEq.Flavor is CtWanted (backward compatible).
	ct := &CtEq{Lhs: &types.TyCon{Name: "Int"}, Rhs: &types.TyCon{Name: "Int"}}
	if ct.Flavor != CtWanted {
		t.Fatalf("expected default Flavor to be CtWanted (0), got %d", ct.Flavor)
	}
}

func TestCtFlavorGiven(t *testing.T) {
	ct := &CtEq{
		Lhs: &types.TySkolem{ID: 1}, Rhs: &types.TyCon{Name: "Int"},
		Flavor: CtGiven,
	}
	if ct.Flavor != CtGiven {
		t.Fatal("expected CtGiven flavor")
	}
}

// --- typeMentionsSkolem / typesMentionSkolem tests ---

func TestTypeMentionsSkolem(t *testing.T) {
	sk := &types.TySkolem{ID: 5}
	if !typeMentionsSkolem(sk, 5) {
		t.Fatal("direct skolem should be found")
	}
	if typeMentionsSkolem(sk, 6) {
		t.Fatal("wrong skolem ID should not match")
	}
	if typeMentionsSkolem(&types.TyCon{Name: "Int"}, 5) {
		t.Fatal("TyCon should not mention any skolem")
	}
}

func TestTypeMentionsSkolemNested(t *testing.T) {
	sk := &types.TySkolem{ID: 3}
	nested := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: sk}
	if !typeMentionsSkolem(nested, 3) {
		t.Fatal("nested skolem should be found")
	}
	if typeMentionsSkolem(nested, 99) {
		t.Fatal("wrong ID should not match in nested type")
	}
}

func TestTypesMentionSkolemSlice(t *testing.T) {
	sk := &types.TySkolem{ID: 10}
	tys := []types.Type{&types.TyCon{Name: "Int"}, sk, &types.TyCon{Name: "Bool"}}
	if !typesMentionSkolem(tys, 10) {
		t.Fatal("slice containing skolem should report mention")
	}
	if typesMentionSkolem(tys, 11) {
		t.Fatal("slice should not report mention for wrong ID")
	}
}

func TestTypesMentionSkolemEmpty(t *testing.T) {
	if typesMentionSkolem(nil, 1) {
		t.Fatal("nil slice should not mention any skolem")
	}
	if typesMentionSkolem([]types.Type{}, 1) {
		t.Fatal("empty slice should not mention any skolem")
	}
}

// --- InertSet given equality tests ---

func TestInertSetInsertGiven(t *testing.T) {
	var is InertSet
	given := &CtEq{
		Lhs: &types.TySkolem{ID: 1}, Rhs: &types.TyCon{Name: "Int"},
		Flavor: CtGiven, S: span.Span{Start: 0, End: 5},
	}
	is.InsertGiven(given)
	if len(is.givenEqs) != 1 {
		t.Fatalf("expected 1 given eq, got %d", len(is.givenEqs))
	}
	if is.givenEqs[0] != given {
		t.Fatal("given eq mismatch")
	}
}

func TestInertSetGivenClearedOnLeaveScope(t *testing.T) {
	var is InertSet
	is.EnterScope()
	given := &CtEq{
		Lhs: &types.TySkolem{ID: 1}, Rhs: &types.TyCon{Name: "Int"},
		Flavor: CtGiven,
	}
	is.InsertGiven(given)
	if len(is.givenEqs) != 1 {
		t.Fatal("given should be present before LeaveScope")
	}
	is.LeaveScope()
	if len(is.givenEqs) != 0 {
		t.Fatalf("expected 0 given eqs after LeaveScope, got %d", len(is.givenEqs))
	}
}

func TestInertSetGivenSurvivesOuterScope(t *testing.T) {
	var is InertSet
	outerGiven := &CtEq{
		Lhs: &types.TySkolem{ID: 1}, Rhs: &types.TyCon{Name: "Int"},
		Flavor: CtGiven,
	}
	is.InsertGiven(outerGiven)
	is.EnterScope()
	innerGiven := &CtEq{
		Lhs: &types.TySkolem{ID: 2}, Rhs: &types.TyCon{Name: "Bool"},
		Flavor: CtGiven,
	}
	is.InsertGiven(innerGiven)
	is.LeaveScope()
	if len(is.givenEqs) != 1 {
		t.Fatalf("expected 1 given eq (outer), got %d", len(is.givenEqs))
	}
	if is.givenEqs[0] != outerGiven {
		t.Fatal("outer given should survive inner LeaveScope")
	}
}

// --- KickOutMentioningSkolem tests ---

func TestKickOutMentioningSkolemFunEq(t *testing.T) {
	var is InertSet
	sk := &types.TySkolem{ID: 5}
	meta := &types.TyMeta{ID: 99}
	ct := &CtFunEq{
		FamilyName: "F",
		Args:       []types.Type{sk, &types.TyCon{Name: "Int"}},
		ResultMeta: meta,
		BlockingOn: []int{99},
	}
	is.InsertFunEq(ct)
	kicked := is.KickOutMentioningSkolem(5)
	if len(kicked) != 1 {
		t.Fatalf("expected 1 kicked, got %d", len(kicked))
	}
	if kicked[0] != ct {
		t.Fatal("kicked constraint mismatch")
	}
	if len(is.LookupFunEq("F")) != 0 {
		t.Fatal("F should be empty after kick-out")
	}
}

func TestKickOutMentioningSkolemNoMatch(t *testing.T) {
	var is InertSet
	ct := &CtFunEq{
		FamilyName: "F",
		Args:       []types.Type{&types.TyCon{Name: "Int"}},
		ResultMeta: &types.TyMeta{ID: 1},
		BlockingOn: []int{1},
	}
	is.InsertFunEq(ct)
	kicked := is.KickOutMentioningSkolem(5)
	if len(kicked) != 0 {
		t.Fatalf("expected 0 kicked for unrelated skolem, got %d", len(kicked))
	}
	if len(is.LookupFunEq("F")) != 1 {
		t.Fatal("F should remain")
	}
}

func TestKickOutMentioningSkolemNestedInApp(t *testing.T) {
	var is InertSet
	sk := &types.TySkolem{ID: 3}
	nested := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: sk}
	ct := &CtFunEq{
		FamilyName: "G",
		Args:       []types.Type{nested},
		ResultMeta: &types.TyMeta{ID: 50},
		BlockingOn: []int{50},
	}
	is.InsertFunEq(ct)
	kicked := is.KickOutMentioningSkolem(3)
	if len(kicked) != 1 {
		t.Fatalf("expected 1 kicked for nested skolem, got %d", len(kicked))
	}
}

// --- extractSkolemGiven tests ---

func TestExtractSkolemGivenLhs(t *testing.T) {
	sk := &types.TySkolem{ID: 1}
	ty := &types.TyCon{Name: "Int"}
	gotSk, gotConcrete := extractSkolemGiven(sk, ty)
	if gotSk != sk {
		t.Fatal("expected skolem from LHS")
	}
	if gotConcrete != ty {
		t.Fatal("expected concrete from RHS")
	}
}

func TestExtractSkolemGivenRhs(t *testing.T) {
	sk := &types.TySkolem{ID: 2}
	ty := &types.TyCon{Name: "Bool"}
	gotSk, gotConcrete := extractSkolemGiven(ty, sk)
	if gotSk != sk {
		t.Fatal("expected skolem from RHS")
	}
	if gotConcrete != ty {
		t.Fatal("expected concrete from LHS")
	}
}

func TestExtractSkolemGivenBothConcrete(t *testing.T) {
	gotSk, _ := extractSkolemGiven(&types.TyCon{Name: "Int"}, &types.TyCon{Name: "Bool"})
	if gotSk != nil {
		t.Fatal("expected nil skolem when both sides are concrete")
	}
}

// --- GenerationScope tests ---

func TestGenerationScopeLifecycle(t *testing.T) {
	s := &Solver{inertSet: NewInertSet()}

	// Initially no scope is active.
	if s.GenerationScopeActive() {
		t.Fatal("expected no generation scope initially")
	}

	// ExitGenerationScope without Enter returns nil.
	cts := s.ExitGenerationScope()
	if cts != nil {
		t.Fatal("expected nil from ExitGenerationScope without Enter")
	}

	// Enter scope.
	s.EnterGenerationScope()
	if !s.GenerationScopeActive() {
		t.Fatal("expected generation scope to be active after Enter")
	}

	// Manually collect constraints (simulating future Emit diversion).
	s.genScope.collected = append(s.genScope.collected,
		&CtEq{Lhs: &types.TyCon{Name: "Int"}, Rhs: &types.TyCon{Name: "Int"}, S: span.Span{}},
		&CtClass{Placeholder: "p1", ClassName: "Eq", S: span.Span{}},
	)

	// Exit scope: collected constraints returned.
	collected := s.ExitGenerationScope()
	if len(collected) != 2 {
		t.Fatalf("expected 2 collected constraints, got %d", len(collected))
	}

	// After exit, scope is inactive.
	if s.GenerationScopeActive() {
		t.Fatal("expected generation scope inactive after Exit")
	}
}

func TestGenerationScopeEmptyCollection(t *testing.T) {
	s := &Solver{inertSet: NewInertSet()}

	s.EnterGenerationScope()
	// Exit immediately without collecting anything.
	collected := s.ExitGenerationScope()
	if len(collected) != 0 {
		t.Fatalf("expected 0 collected constraints, got %d", len(collected))
	}
}

func TestGenerationScopeDoesNotAffectWorklist(t *testing.T) {
	// Verify that the generation scope infrastructure does not
	// interfere with normal worklist operations (Emit is not diverted).
	s := &Solver{inertSet: NewInertSet()}

	s.EnterGenerationScope()

	// Normal Emit still goes to the worklist (not diverted yet).
	ct := &CtClass{Placeholder: "p1", ClassName: "Eq", S: span.Span{}}
	s.Emit(ct)

	if s.worklist.Len() != 1 {
		t.Fatalf("expected worklist len 1, got %d", s.worklist.Len())
	}

	// genScope should be empty since Emit is not yet modified.
	collected := s.ExitGenerationScope()
	if len(collected) != 0 {
		t.Fatalf("expected 0 collected (Emit not diverted), got %d", len(collected))
	}
}
