// Family reduce tests — pattern matching and type family reduction.
// Does NOT cover: row_family_test.go (builtin row families), verify_test.go (injectivity).
package family

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- test helpers (shared across test files in this package) ---

// testEnvConfig holds optional overrides for testEnv.
type testEnvConfig struct {
	families map[string]*env.TypeFamilyInfo
	tfLimit  int
	stuckFn  func(string, []types.Type, types.Type, span.Span) *types.TyMeta
}

type testHarness struct {
	env    *ReduceEnv
	u      *unify.Unifier
	budget *budget.CheckBudget
	errors []string
	nextID int
}

func newTestHarness(cfg *testEnvConfig) *testHarness {
	if cfg == nil {
		cfg = &testEnvConfig{}
	}
	u := unify.NewUnifier()
	b := budget.NewCheckBudget(context.Background())
	limit := MaxReductionWork
	if cfg.tfLimit > 0 {
		limit = cfg.tfLimit
	}
	b.SetTFStepLimit(limit)

	h := &testHarness{u: u, budget: b}

	h.env = &ReduceEnv{
		LookupFamily: func(name string) (*env.TypeFamilyInfo, bool) {
			if cfg.families != nil {
				f, ok := cfg.families[name]
				return f, ok
			}
			return nil, false
		},
		Budget:  b,
		Unifier: u,
		FreshMeta: func(k types.Type) *types.TyMeta {
			h.nextID++
			return &types.TyMeta{ID: h.nextID + 1000, Kind: k}
		},
		AddError: func(_ diagnostic.Code, _ span.Span, msg string) {
			h.errors = append(h.errors, msg)
		},
		TryUnify: func(a, b types.Type) bool {
			snap := u.Snapshot()
			err := u.Unify(a, b)
			u.Restore(snap)
			return err == nil
		},
		RegisterStuckFn: cfg.stuckFn,
	}
	return h
}

// Type constructors for concise test setup.

func con(name string) *types.TyCon     { return types.Con(name) }
func tyvar(name string) *types.TyVar   { return &types.TyVar{Name: name} }
func app(f, a types.Type) *types.TyApp { return &types.TyApp{Fun: f, Arg: a} }

func meta(id int) *types.TyMeta {
	return &types.TyMeta{ID: id, Kind: types.TypeOfTypes}
}

func familyApp(name string, args ...types.Type) *types.TyFamilyApp {
	return &types.TyFamilyApp{Name: name, Args: args, Kind: types.TypeOfTypes}
}

// --- MatchTyPattern tests ---

func TestMatchTyPattern_VarBinds(t *testing.T) {
	h := newTestHarness(nil)
	subst := make(map[string]types.Type)

	r := h.env.MatchTyPattern(tyvar("a"), con("Int"), subst)
	if r != env.MatchSuccess {
		t.Fatalf("expected MatchSuccess, got %v", r)
	}
	if got := subst["a"]; !types.Equal(got, con("Int")) {
		t.Fatalf("expected Int, got %v", got)
	}
}

func TestMatchTyPattern_VarReuseSame(t *testing.T) {
	h := newTestHarness(nil)
	subst := map[string]types.Type{"a": con("Int")}

	r := h.env.MatchTyPattern(tyvar("a"), con("Int"), subst)
	if r != env.MatchSuccess {
		t.Fatalf("expected MatchSuccess for same binding, got %v", r)
	}
}

func TestMatchTyPattern_VarReuseDifferent(t *testing.T) {
	h := newTestHarness(nil)
	subst := map[string]types.Type{"a": con("Int")}

	r := h.env.MatchTyPattern(tyvar("a"), con("Bool"), subst)
	if r != env.MatchFail {
		t.Fatalf("expected MatchFail for conflicting binding, got %v", r)
	}
}

func TestMatchTyPattern_Wildcard(t *testing.T) {
	h := newTestHarness(nil)
	subst := make(map[string]types.Type)

	r := h.env.MatchTyPattern(tyvar("_"), con("Int"), subst)
	if r != env.MatchSuccess {
		t.Fatalf("expected MatchSuccess for wildcard, got %v", r)
	}
	if _, ok := subst["_"]; ok {
		t.Fatal("wildcard should not be stored in subst")
	}
}

func TestMatchTyPattern_ConMatch(t *testing.T) {
	h := newTestHarness(nil)
	subst := make(map[string]types.Type)

	r := h.env.MatchTyPattern(con("Int"), con("Int"), subst)
	if r != env.MatchSuccess {
		t.Fatalf("expected MatchSuccess, got %v", r)
	}
}

func TestMatchTyPattern_ConMismatch(t *testing.T) {
	h := newTestHarness(nil)
	subst := make(map[string]types.Type)

	r := h.env.MatchTyPattern(con("Int"), con("Bool"), subst)
	if r != env.MatchFail {
		t.Fatalf("expected MatchFail, got %v", r)
	}
}

func TestMatchTyPattern_ConVsMeta(t *testing.T) {
	h := newTestHarness(nil)
	subst := make(map[string]types.Type)

	r := h.env.MatchTyPattern(con("Int"), meta(1), subst)
	if r != env.MatchIndeterminate {
		t.Fatalf("expected MatchIndeterminate, got %v", r)
	}
}

func TestMatchTyPattern_AppMatch(t *testing.T) {
	h := newTestHarness(nil)
	subst := make(map[string]types.Type)

	pat := app(con("List"), tyvar("a"))
	arg := app(con("List"), con("Int"))

	r := h.env.MatchTyPattern(pat, arg, subst)
	if r != env.MatchSuccess {
		t.Fatalf("expected MatchSuccess, got %v", r)
	}
	if !types.Equal(subst["a"], con("Int")) {
		t.Fatalf("expected a=Int, got %v", subst["a"])
	}
}

func TestMatchTyPattern_AppMismatch(t *testing.T) {
	h := newTestHarness(nil)
	subst := make(map[string]types.Type)

	pat := app(con("List"), tyvar("a"))
	arg := app(con("Maybe"), con("Int"))

	r := h.env.MatchTyPattern(pat, arg, subst)
	if r != env.MatchFail {
		t.Fatalf("expected MatchFail, got %v", r)
	}
}

func TestMatchTyPattern_AppVsMeta(t *testing.T) {
	h := newTestHarness(nil)
	subst := make(map[string]types.Type)

	pat := app(con("List"), tyvar("a"))
	r := h.env.MatchTyPattern(pat, meta(1), subst)
	if r != env.MatchIndeterminate {
		t.Fatalf("expected MatchIndeterminate, got %v", r)
	}
}

func TestMatchTyPattern_AppVsCon(t *testing.T) {
	h := newTestHarness(nil)
	subst := make(map[string]types.Type)

	pat := app(con("List"), tyvar("a"))
	r := h.env.MatchTyPattern(pat, con("Int"), subst)
	if r != env.MatchFail {
		t.Fatalf("expected MatchFail, got %v", r)
	}
}

func TestMatchTyPattern_ConVsArrow(t *testing.T) {
	// TyCon pattern against a non-TyCon/non-TyMeta type → MatchFail.
	h := newTestHarness(nil)
	subst := make(map[string]types.Type)

	arrow := &types.TyArrow{From: con("Int"), To: con("Bool")}
	r := h.env.MatchTyPattern(con("Int"), arrow, subst)
	if r != env.MatchFail {
		t.Fatalf("expected MatchFail for TyCon vs TyArrow, got %v", r)
	}
}

func TestMatchTyPattern_ArrowPattern(t *testing.T) {
	// Non-standard pattern type (TyArrow) → default branch → MatchFail.
	h := newTestHarness(nil)
	subst := make(map[string]types.Type)

	arrow := &types.TyArrow{From: con("Int"), To: con("Bool")}
	r := h.env.MatchTyPattern(arrow, con("Int"), subst)
	if r != env.MatchFail {
		t.Fatalf("expected MatchFail for unsupported pattern type, got %v", r)
	}
}

func TestMatchTyPattern_ZonksMeta(t *testing.T) {
	h := newTestHarness(nil)
	// Solve meta 1 = Int, then match against it.
	m := meta(1)
	h.u.SolveFreshMeta(m, con("Int"))

	subst := make(map[string]types.Type)
	r := h.env.MatchTyPattern(con("Int"), m, subst)
	if r != env.MatchSuccess {
		t.Fatalf("expected MatchSuccess after zonking, got %v", r)
	}
}

// --- MatchTyPatterns tests ---

func TestMatchTyPatterns_LengthMismatch(t *testing.T) {
	h := newTestHarness(nil)
	_, r := h.env.MatchTyPatterns(
		[]types.Type{tyvar("a"), tyvar("b")},
		[]types.Type{con("Int")},
	)
	if r != env.MatchFail {
		t.Fatalf("expected MatchFail for length mismatch, got %v", r)
	}
}

func TestMatchTyPatterns_AllMatch(t *testing.T) {
	h := newTestHarness(nil)
	subst, r := h.env.MatchTyPatterns(
		[]types.Type{tyvar("a"), con("Bool")},
		[]types.Type{con("Int"), con("Bool")},
	)
	if r != env.MatchSuccess {
		t.Fatalf("expected MatchSuccess, got %v", r)
	}
	if !types.Equal(subst["a"], con("Int")) {
		t.Fatalf("expected a=Int, got %v", subst["a"])
	}
}

func TestMatchTyPatterns_EarlyIndeterminate(t *testing.T) {
	h := newTestHarness(nil)
	_, r := h.env.MatchTyPatterns(
		[]types.Type{con("Int"), tyvar("b")},
		[]types.Type{meta(1), con("Bool")},
	)
	if r != env.MatchIndeterminate {
		t.Fatalf("expected MatchIndeterminate, got %v", r)
	}
}

// --- ReduceTyFamily tests ---

func TestReduceTyFamily_SimpleMatch(t *testing.T) {
	// type family F a where F Int = Bool
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
				},
			},
		},
	})

	result, ok := h.env.ReduceTyFamily("F", []types.Type{con("Int")}, span.Span{})
	if !ok {
		t.Fatal("expected reduction to succeed")
	}
	if !types.Equal(result, con("Bool")) {
		t.Fatalf("expected Bool, got %v", result)
	}
}

func TestReduceTyFamily_WithSubstitution(t *testing.T) {
	// type family Id a where Id a = a
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"Id": {
				Name:       "Id",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{tyvar("a")}, RHS: tyvar("a")},
				},
			},
		},
	})

	result, ok := h.env.ReduceTyFamily("Id", []types.Type{con("Int")}, span.Span{})
	if !ok {
		t.Fatal("expected reduction to succeed")
	}
	if !types.Equal(result, con("Int")) {
		t.Fatalf("expected Int, got %v", result)
	}
}

func TestReduceTyFamily_NoMatch(t *testing.T) {
	// type family F a where F Int = Bool   (no equation for Bool)
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
				},
			},
		},
	})

	_, ok := h.env.ReduceTyFamily("F", []types.Type{con("Bool")}, span.Span{})
	if ok {
		t.Fatal("expected stuck (no matching equation)")
	}
}

func TestReduceTyFamily_UnknownFamily(t *testing.T) {
	h := newTestHarness(nil)
	_, ok := h.env.ReduceTyFamily("Unknown", []types.Type{con("Int")}, span.Span{})
	if ok {
		t.Fatal("expected stuck for unknown family")
	}
}

func TestReduceTyFamily_MetaStuck(t *testing.T) {
	// type family F a where F Int = Bool
	// F ?m → indeterminate → stuck
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
				},
			},
		},
	})

	_, ok := h.env.ReduceTyFamily("F", []types.Type{meta(1)}, span.Span{})
	if ok {
		t.Fatal("expected stuck when arg is unsolved meta")
	}
}

func TestReduceTyFamily_FallThrough(t *testing.T) {
	// type family F a where
	//   F Int  = Bool
	//   F Bool = Int
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
					{Patterns: []types.Type{con("Bool")}, RHS: con("Int")},
				},
			},
		},
	})

	result, ok := h.env.ReduceTyFamily("F", []types.Type{con("Bool")}, span.Span{})
	if !ok {
		t.Fatal("expected second equation to match")
	}
	if !types.Equal(result, con("Int")) {
		t.Fatalf("expected Int, got %v", result)
	}
}

func TestReduceTyFamily_TypeSizeGuard(t *testing.T) {
	// RHS that expands beyond maxReductionTypeSize should be rejected.
	// Build a deeply nested type: Pair(Pair(Pair(...))) exceeding the limit.
	var huge types.Type = con("X")
	for range maxReductionTypeSize + 1 {
		huge = app(con("P"), huge)
	}
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"Big": {
				Name:       "Big",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{tyvar("a")}, RHS: huge},
				},
			},
		},
	})

	_, ok := h.env.ReduceTyFamily("Big", []types.Type{con("Int")}, span.Span{})
	if ok {
		t.Fatal("expected rejection for oversized result type")
	}
	if len(h.errors) == 0 {
		t.Fatal("expected a type-too-large error")
	}
}

func TestReduceTyFamily_BuiltinRowDispatch(t *testing.T) {
	// Merge should be dispatched as a builtin row family via ReduceTyFamily.
	h := newTestHarness(nil)
	lhs := capRow("Fail", con("Unit"))
	rhs := capRow("State", con("Int"))

	result, ok := h.env.ReduceTyFamily("Merge", []types.Type{lhs, rhs}, span.Span{})
	if !ok {
		t.Fatal("expected Merge to be dispatched as builtin row family")
	}
	row := result.(*types.TyEvidenceRow)
	if len(row.CapFields()) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(row.CapFields()))
	}
}

func TestReduceTyFamily_BudgetExceeded(t *testing.T) {
	h := newTestHarness(&testEnvConfig{
		tfLimit: 1, // exhausted after 1 step
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{tyvar("a")}, RHS: tyvar("a")},
				},
			},
		},
	})

	// First call consumes the budget.
	h.env.ReduceTyFamily("F", []types.Type{con("Int")}, span.Span{})
	// Second call exceeds the limit.
	_, ok := h.env.ReduceTyFamily("F", []types.Type{con("Int")}, span.Span{})
	if ok {
		t.Fatal("expected failure after budget exceeded")
	}
	if len(h.errors) == 0 {
		t.Fatal("expected an error about reduction limit")
	}
}

// --- reduceFamilyApps tests ---

func TestReduceFamilyApps_TyFamilyAppNode(t *testing.T) {
	// F Int reduces to Bool via explicit TyFamilyApp.
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
				},
			},
		},
	})

	input := familyApp("F", con("Int"))
	result := h.env.ReduceAll(input)
	if !types.Equal(result, con("Bool")) {
		t.Fatalf("expected Bool, got %v", result)
	}
}

func TestReduceFamilyApps_NestedReduction(t *testing.T) {
	// type family F a where F Int = Bool; F Bool = String
	// F (F Int) → F Bool → String
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
					{Patterns: []types.Type{con("Bool")}, RHS: con("String")},
				},
			},
		},
	})

	inner := familyApp("F", con("Int"))
	outer := familyApp("F", inner)
	result := h.env.ReduceAll(outer)
	if !types.Equal(result, con("String")) {
		t.Fatalf("expected String, got %v", result)
	}
}

func TestReduceFamilyApps_TyAppChainFamily(t *testing.T) {
	// TyApp chain: (TyCon "F") applied to (TyCon "Int")
	// should be detected as a saturated type family application.
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
				},
			},
		},
	})

	input := app(con("F"), con("Int"))
	result := h.env.ReduceAll(input)
	if !types.Equal(result, con("Bool")) {
		t.Fatalf("expected Bool via TyApp chain, got %v", result)
	}
}

func TestReduceFamilyApps_NonFamilyPassThrough(t *testing.T) {
	h := newTestHarness(nil)
	input := app(con("List"), con("Int"))
	result := h.env.ReduceAll(input)
	if !types.Equal(result, input) {
		t.Fatalf("non-family type should pass through unchanged")
	}
}

func TestReduceFamilyApps_StuckRegistration(t *testing.T) {
	var registered bool
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
				},
			},
		},
		stuckFn: func(name string, _ []types.Type, _ types.Type, _ span.Span) *types.TyMeta {
			registered = true
			return meta(99)
		},
	})

	// F ?m is stuck → stuckFn should be called.
	input := familyApp("F", meta(1))
	result := h.env.ReduceAll(input)
	if !registered {
		t.Fatal("expected stuckFn to be called for stuck application")
	}
	if m, ok := result.(*types.TyMeta); !ok || m.ID != 99 {
		t.Fatalf("expected placeholder meta 99, got %v", result)
	}
}

func TestReduceFamilyApps_TyAppChainStuckNoStuckFn(t *testing.T) {
	// TyApp chain with meta arg, stuckFn=nil → stuck TyFamilyApp preserved.
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
				},
			},
		},
		// stuckFn intentionally nil
	})

	input := app(con("F"), meta(1))
	result := h.env.ReduceAll(input)
	if tf, ok := result.(*types.TyFamilyApp); !ok {
		t.Fatalf("expected stuck TyFamilyApp, got %T", result)
	} else if tf.Name != "F" {
		t.Fatalf("expected family name F, got %s", tf.Name)
	}
}

func TestReduceFamilyApps_TyAppChainStuckWithStuckFn(t *testing.T) {
	// TyApp chain with meta arg, stuckFn set → placeholder returned.
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
				},
			},
		},
		stuckFn: func(_ string, _ []types.Type, _ types.Type, _ span.Span) *types.TyMeta {
			return meta(42)
		},
	})

	input := app(con("F"), meta(1))
	result := h.env.ReduceAll(input)
	if m, ok := result.(*types.TyMeta); !ok || m.ID != 42 {
		t.Fatalf("expected placeholder meta 42, got %v", result)
	}
}

func TestReduceFamilyApps_TyFamilyAppStuckNoStuckFn(t *testing.T) {
	// Explicit TyFamilyApp with meta arg, stuckFn=nil → preserved as TyFamilyApp.
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
				},
			},
		},
	})

	input := familyApp("F", meta(1))
	result := h.env.ReduceAll(input)
	if tf, ok := result.(*types.TyFamilyApp); !ok {
		t.Fatalf("expected TyFamilyApp, got %T", result)
	} else if tf.Name != "F" {
		t.Fatalf("expected family name F, got %s", tf.Name)
	}
}

func TestReduceFamilyApps_BudgetExhaustion(t *testing.T) {
	// When TF step budget is exhausted during reduceFamilyAppsN,
	// the type is returned as-is (early return).
	h := newTestHarness(&testEnvConfig{
		tfLimit: 1,
	})
	// Exhaust the budget.
	h.budget.ResetTFSteps()
	h.budget.TFStep()

	input := familyApp("F", con("Int"))
	result := h.env.ReduceAll(input)
	// Budget reset happens in ReduceAll, but with limit=1, the first
	// TFStep inside reduceFamilyAppsN succeeds, then the inner call
	// to ReduceTyFamily will exceed. So we just check it terminates.
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestReduceFamilyApps_StructuralRecursion(t *testing.T) {
	// Case 3: non-TyFamilyApp, non-TyApp types should be walked structurally.
	// TyArrow(F Int, F Bool) → TyArrow(Bool, String)
	h := newTestHarness(&testEnvConfig{
		families: map[string]*env.TypeFamilyInfo{
			"F": {
				Name:       "F",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
					{Patterns: []types.Type{con("Bool")}, RHS: con("String")},
				},
			},
		},
	})

	input := &types.TyArrow{From: familyApp("F", con("Int")), To: familyApp("F", con("Bool"))}
	result := h.env.ReduceAll(input)
	arrow, ok := result.(*types.TyArrow)
	if !ok {
		t.Fatalf("expected TyArrow, got %T", result)
	}
	if !types.Equal(arrow.From, con("Bool")) || !types.Equal(arrow.To, con("String")) {
		t.Fatalf("expected Bool -> String, got %v -> %v", arrow.From, arrow.To)
	}
}

func TestReduceFamilyApps_CycleCache(t *testing.T) {
	// type family Grow a where Grow a = Pair (Grow a) (Grow a)
	// Without the cache sentinel, this would be exponential.
	// The cache returns the stuck sentinel on re-entry.
	h := newTestHarness(&testEnvConfig{
		tfLimit: 200,
		families: map[string]*env.TypeFamilyInfo{
			"Grow": {
				Name:       "Grow",
				Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
				ResultKind: types.TypeOfTypes,
				Equations: []env.TFEquation{
					{
						Patterns: []types.Type{tyvar("a")},
						RHS:      app(app(con("Pair"), familyApp("Grow", tyvar("a"))), familyApp("Grow", tyvar("a"))),
					},
				},
			},
		},
	})

	input := familyApp("Grow", con("Int"))
	// Should terminate (not hang or exhaust budget catastrophically).
	result := h.env.ReduceAll(input)
	if result == nil {
		t.Fatal("expected a result (possibly stuck), got nil")
	}
}
