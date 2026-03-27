package optimize

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// coreEq compares two Core trees structurally (ignoring spans).
func coreEq(a, b ir.Core) bool {
	switch x := a.(type) {
	case *ir.Var:
		y, ok := b.(*ir.Var)
		return ok && x.Name == y.Name
	case *ir.Lit:
		y, ok := b.(*ir.Lit)
		return ok && x.Value == y.Value
	case *ir.Lam:
		y, ok := b.(*ir.Lam)
		return ok && x.Param == y.Param && coreEq(x.Body, y.Body)
	case *ir.App:
		y, ok := b.(*ir.App)
		return ok && coreEq(x.Fun, y.Fun) && coreEq(x.Arg, y.Arg)
	case *ir.Con:
		y, ok := b.(*ir.Con)
		if !ok || x.Name != y.Name || len(x.Args) != len(y.Args) {
			return false
		}
		for i := range x.Args {
			if !coreEq(x.Args[i], y.Args[i]) {
				return false
			}
		}
		return true
	case *ir.Case:
		y, ok := b.(*ir.Case)
		if !ok || len(x.Alts) != len(y.Alts) || !coreEq(x.Scrutinee, y.Scrutinee) {
			return false
		}
		for i := range x.Alts {
			if !coreEq(x.Alts[i].Body, y.Alts[i].Body) {
				return false
			}
		}
		return true
	case *ir.Pure:
		y, ok := b.(*ir.Pure)
		return ok && coreEq(x.Expr, y.Expr)
	case *ir.Bind:
		y, ok := b.(*ir.Bind)
		return ok && x.Var == y.Var && coreEq(x.Comp, y.Comp) && coreEq(x.Body, y.Body)
	case *ir.Thunk:
		y, ok := b.(*ir.Thunk)
		return ok && coreEq(x.Comp, y.Comp)
	case *ir.Force:
		y, ok := b.(*ir.Force)
		return ok && coreEq(x.Expr, y.Expr)
	case *ir.PrimOp:
		y, ok := b.(*ir.PrimOp)
		if !ok || x.Name != y.Name || len(x.Args) != len(y.Args) {
			return false
		}
		for i := range x.Args {
			if !coreEq(x.Args[i], y.Args[i]) {
				return false
			}
		}
		return true
	case *ir.RecordLit:
		y, ok := b.(*ir.RecordLit)
		if !ok || len(x.Fields) != len(y.Fields) {
			return false
		}
		for i := range x.Fields {
			if x.Fields[i].Label != y.Fields[i].Label || !coreEq(x.Fields[i].Value, y.Fields[i].Value) {
				return false
			}
		}
		return true
	case *ir.RecordProj:
		y, ok := b.(*ir.RecordProj)
		return ok && x.Label == y.Label && coreEq(x.Record, y.Record)
	case *ir.RecordUpdate:
		y, ok := b.(*ir.RecordUpdate)
		if !ok || len(x.Updates) != len(y.Updates) || !coreEq(x.Record, y.Record) {
			return false
		}
		for i := range x.Updates {
			if x.Updates[i].Label != y.Updates[i].Label || !coreEq(x.Updates[i].Value, y.Updates[i].Value) {
				return false
			}
		}
		return true
	case *ir.TyApp:
		y, ok := b.(*ir.TyApp)
		return ok && coreEq(x.Expr, y.Expr)
	case *ir.TyLam:
		y, ok := b.(*ir.TyLam)
		return ok && coreEq(x.Body, y.Body)
	}
	return false
}

func v(name string) ir.Core                    { return &ir.Var{Name: name} }
func lit(val any) ir.Core                      { return &ir.Lit{Value: val} }
func app(f, x ir.Core) ir.Core                 { return &ir.App{Fun: f, Arg: x} }
func lam(p string, b ir.Core) ir.Core          { return &ir.Lam{Param: p, Body: b} }
func con(name string, args ...ir.Core) ir.Core { return &ir.Con{Name: name, Args: args} }
func pcon(name string, args ...ir.Pattern) ir.Pattern {
	return &ir.PCon{Con: name, Args: args}
}
func pvar(name string) ir.Pattern             { return &ir.PVar{Name: name} }
func alt(pat ir.Pattern, body ir.Core) ir.Alt { return ir.Alt{Pattern: pat, Body: body} }
func cas(scrut ir.Core, alts ...ir.Alt) ir.Core {
	return &ir.Case{Scrutinee: scrut, Alts: alts}
}
func primop(name string, arity int, args ...ir.Core) ir.Core {
	return &ir.PrimOp{Name: name, Arity: arity, Args: args}
}

// ===== Phase 1: Algebraic simplifications =====

// R1: Case-of-known-constructor
func TestR1_CaseOfKnownCtor(t *testing.T) {
	// case (Just x) of { Just y -> y; Nothing -> z }  →  x
	input := cas(
		con("Just", v("x")),
		alt(pcon("Just", pvar("y")), v("y")),
		alt(pcon("Nothing"), v("z")),
	)
	result := optimize(input, nil)
	if !coreEq(result, v("x")) {
		t.Fatalf("R1 failed: got %v", result)
	}
}

func TestR1_CaseOfKnownCtorMultiArg(t *testing.T) {
	// case (Cons a b) of { Cons x y -> x; Nil -> z }  →  a
	input := cas(
		con("Cons", v("a"), v("b")),
		alt(pcon("Cons", pvar("x"), pvar("y")), v("x")),
		alt(pcon("Nil"), v("z")),
	)
	result := optimize(input, nil)
	if !coreEq(result, v("a")) {
		t.Fatalf("R1 multi-arg failed: got %v", result)
	}
}

// R2: Beta reduction
func TestR2_BetaReduction(t *testing.T) {
	// (\x. x) y  →  y
	input := app(lam("x", v("x")), v("y"))
	result := optimize(input, nil)
	if !coreEq(result, v("y")) {
		t.Fatalf("R2 failed: got %v", result)
	}
}

func TestR2_BetaReductionNested(t *testing.T) {
	// (\f. \x. f x) g  →  \x. g x
	input := app(lam("f", lam("x", app(v("f"), v("x")))), v("g"))
	expected := lam("x", app(v("g"), v("x")))
	result := optimize(input, nil)
	if !coreEq(result, expected) {
		t.Fatalf("R2 nested failed: got %v", result)
	}
}

// R3: Bind-Pure elimination
func TestR3_BindPure(t *testing.T) {
	// bind (pure e) x body  →  body[x := e]
	input := &ir.Bind{Comp: &ir.Pure{Expr: v("e")}, Var: "x", Body: v("x")}
	result := optimize(input, nil)
	if !coreEq(result, v("e")) {
		t.Fatalf("R3 failed: got %v", result)
	}
}

// R4: Force-Thunk elimination
func TestR4_ForceThunk(t *testing.T) {
	// force (thunk comp)  →  comp
	input := &ir.Force{Expr: &ir.Thunk{Comp: v("comp")}}
	result := optimize(input, nil)
	if !coreEq(result, v("comp")) {
		t.Fatalf("R4 failed: got %v", result)
	}
}

// R5: RecordProj of known literal
func TestR5_RecordProjKnown(t *testing.T) {
	// { x: 1, y: 2 }.#x  →  1
	input := &ir.RecordProj{
		Record: &ir.RecordLit{Fields: []ir.RecordField{
			{Label: "x", Value: lit(int64(1))},
			{Label: "y", Value: lit(int64(2))},
		}},
		Label: "x",
	}
	result := optimize(input, nil)
	if !coreEq(result, lit(int64(1))) {
		t.Fatalf("R5 failed: got %v", result)
	}
}

// R6: RecordUpdate chain collapse
func TestR6_RecordUpdateChain(t *testing.T) {
	// { { r | x: 1 } | y: 2 }  →  { r | x: 1, y: 2 }
	input := &ir.RecordUpdate{
		Record: &ir.RecordUpdate{
			Record:  v("r"),
			Updates: []ir.RecordField{{Label: "x", Value: lit(int64(1))}},
		},
		Updates: []ir.RecordField{{Label: "y", Value: lit(int64(2))}},
	}
	result := optimize(input, nil)
	upd, ok := result.(*ir.RecordUpdate)
	if !ok {
		t.Fatalf("R6: expected RecordUpdate, got %T", result)
	}
	if !coreEq(upd.Record, v("r")) {
		t.Fatalf("R6: base should be r, got %v", upd.Record)
	}
	if len(upd.Updates) != 2 {
		t.Fatalf("R6: expected 2 updates, got %d", len(upd.Updates))
	}
}

func TestR6_RecordUpdateOverwrite(t *testing.T) {
	// { { r | x: 1 } | x: 2 }  →  { r | x: 2 }
	input := &ir.RecordUpdate{
		Record: &ir.RecordUpdate{
			Record:  v("r"),
			Updates: []ir.RecordField{{Label: "x", Value: lit(int64(1))}},
		},
		Updates: []ir.RecordField{{Label: "x", Value: lit(int64(2))}},
	}
	result := optimize(input, nil)
	upd, ok := result.(*ir.RecordUpdate)
	if !ok {
		t.Fatalf("R6 overwrite: expected RecordUpdate, got %T", result)
	}
	if len(upd.Updates) != 1 || upd.Updates[0].Label != "x" {
		t.Fatalf("R6 overwrite: expected single x update, got %v", upd.Updates)
	}
	if !coreEq(upd.Updates[0].Value, lit(int64(2))) {
		t.Fatalf("R6 overwrite: expected value 2")
	}
}

// ===== Phase 4: Ad-hoc fusion (rules passed as parameters) =====

// testRoundtrip builds a roundtrip elimination rule for testing.
func testRoundtrip(outer, inner string) func(ir.Core) ir.Core {
	return func(c ir.Core) ir.Core {
		po, ok := c.(*ir.PrimOp)
		if !ok || po.Name != outer || len(po.Args) != 1 {
			return c
		}
		inn, ok := po.Args[0].(*ir.PrimOp)
		if !ok || inn.Name != inner || len(inn.Args) != 1 {
			return c
		}
		return inn.Args[0]
	}
}

// testMapMapFusion is a test-local fusion rule for _sliceMap∘_sliceMap.
func testMapMapFusion(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != "_sliceMap" || len(po.Args) != 2 {
		return c
	}
	inner, ok := po.Args[1].(*ir.PrimOp)
	if !ok || inner.Name != "_sliceMap" || len(inner.Args) != 2 {
		return c
	}
	f, g, xs := po.Args[0], inner.Args[0], inner.Args[1]
	x := "$opt_x"
	composed := &ir.Lam{Param: x, Body: &ir.App{
		Fun: f, Arg: &ir.App{Fun: g, Arg: &ir.Var{Name: x}},
	}}
	return &ir.PrimOp{Name: "_sliceMap", Arity: 2, Args: []ir.Core{composed, xs}, S: po.S}
}

// R10: Slice map/map fusion
func TestR10_SliceMapMap(t *testing.T) {
	input := primop("_sliceMap", 2, v("f"), primop("_sliceMap", 2, v("g"), v("xs")))
	result := optimize(input, []func(ir.Core) ir.Core{testMapMapFusion})
	po, ok := result.(*ir.PrimOp)
	if !ok || po.Name != "_sliceMap" {
		t.Fatalf("R10: expected _sliceMap, got %T", result)
	}
	if len(po.Args) != 2 {
		t.Fatalf("R10: expected 2 args, got %d", len(po.Args))
	}
	comp, ok := po.Args[0].(*ir.Lam)
	if !ok {
		t.Fatalf("R10: expected composed lambda, got %T", po.Args[0])
	}
	innerApp, ok := comp.Body.(*ir.App)
	if !ok || !coreEq(innerApp.Fun, v("f")) {
		t.Fatalf("R10: expected f applied to (g $x)")
	}
	if !coreEq(po.Args[1], v("xs")) {
		t.Fatalf("R10: expected xs as second arg")
	}
}

// R12: Slice packed roundtrip
func TestR12_SlicePackedRoundtrip(t *testing.T) {
	rules := []func(ir.Core) ir.Core{testRoundtrip("_sliceToList", "_sliceFromList")}
	input := primop("_sliceToList", 1, primop("_sliceFromList", 1, v("xs")))
	result := optimize(input, rules)
	if !coreEq(result, v("xs")) {
		t.Fatalf("R12 failed: got %v", result)
	}
}

// R13: String packed roundtrip
func TestR13_StringPackedRoundtrip(t *testing.T) {
	rules := []func(ir.Core) ir.Core{testRoundtrip("_fromRunes", "_toRunes")}
	input := primop("_fromRunes", 1, primop("_toRunes", 1, v("x")))
	result := optimize(input, rules)
	if !coreEq(result, v("x")) {
		t.Fatalf("R13 failed: got %v", result)
	}
}

// ===== Multi-pass =====

func TestMultiPass_BetaThenCaseOfKnown(t *testing.T) {
	// (\d. case d of { Just y -> y }) (Just x)
	// Pass 1: beta → case (Just x) of { Just y -> y }
	// Pass 2: case-of-known → x
	input := app(
		lam("d", cas(v("d"), alt(pcon("Just", pvar("y")), v("y")))),
		con("Just", v("x")),
	)
	result := optimize(input, nil)
	if !coreEq(result, v("x")) {
		t.Fatalf("multi-pass beta+case failed: got %v", result)
	}
}

// ===== No-op cases (should not transform) =====

func TestNoOp_CaseNotKnown(t *testing.T) {
	// case x of { Just y -> y }  →  unchanged
	input := cas(v("x"), alt(pcon("Just", pvar("y")), v("y")))
	result := optimize(input, nil)
	if !coreEq(result, input) {
		t.Fatalf("should not transform non-known scrutinee")
	}
}

func TestNoOp_ForceNonThunk(t *testing.T) {
	// force x  →  unchanged
	input := &ir.Force{Expr: v("x")}
	result := optimize(input, nil)
	if !coreEq(result, input) {
		t.Fatalf("should not transform force of non-thunk")
	}
}

// ===== Substitution shadowing (M39-M42) =====

func TestSubst_LamShadowing(t *testing.T) {
	// (\x. \x. x) y  →  \x. x  (inner x shadows, must NOT become y)
	input := app(lam("x", lam("x", v("x"))), v("y"))
	result := optimize(input, nil)
	expected := lam("x", v("x"))
	if !coreEq(result, expected) {
		t.Fatalf("Lam shadowing: expected \\x. x, got %v", result)
	}
}

func TestSubst_FixShadowing(t *testing.T) {
	// (\x. fix x in \y. x) y  →  fix x in \y. x
	inner := &ir.Fix{Name: "x", Body: lam("y", v("x"))}
	input := app(lam("x", inner), v("y"))
	result := optimize(input, nil)
	// The fix shadows x, so the body must remain unchanged.
	fx, ok := result.(*ir.Fix)
	if !ok {
		t.Fatalf("Fix shadowing: expected Fix, got %T", result)
	}
	lm, ok := fx.Body.(*ir.Lam)
	if !ok {
		t.Fatalf("Fix shadowing: expected Lam body, got %T", fx.Body)
	}
	if !coreEq(lm.Body, v("x")) {
		t.Fatalf("Fix shadowing: inner body should be x, got %v", lm.Body)
	}
}

func TestSubst_BindShadowing(t *testing.T) {
	// (\x. bind comp x (x)) y  →  bind comp[x:=y] x (x)
	// The bind variable x shadows, so the body must remain v("x").
	inner := &ir.Bind{Comp: v("x"), Var: "x", Body: v("x")}
	input := app(lam("x", inner), v("y"))
	result := optimize(input, nil)
	b, ok := result.(*ir.Bind)
	if !ok {
		t.Fatalf("Bind shadowing: expected Bind, got %T", result)
	}
	// Comp should be substituted: x -> y
	if !coreEq(b.Comp, v("y")) {
		t.Fatalf("Bind shadowing: comp should be y, got %v", b.Comp)
	}
	// Body should NOT be substituted (x is shadowed by bind var)
	if !coreEq(b.Body, v("x")) {
		t.Fatalf("Bind shadowing: body should be x (shadowed), got %v", b.Body)
	}
}

func TestSubst_CasePatternShadowing(t *testing.T) {
	// (\x. case z of { Just x -> x; Nothing -> x }) y
	// In the Just branch, x is bound by the pattern — must NOT become y.
	// In the Nothing branch, x is free — must become y.
	inner := cas(
		v("z"),
		alt(pcon("Just", pvar("x")), v("x")),
		alt(pcon("Nothing"), v("x")),
	)
	input := app(lam("x", inner), v("y"))
	result := optimize(input, nil)
	cs, ok := result.(*ir.Case)
	if !ok {
		t.Fatalf("Case shadowing: expected Case, got %T", result)
	}
	// Just branch: x is pattern-bound, body stays v("x")
	if !coreEq(cs.Alts[0].Body, v("x")) {
		t.Fatalf("Case shadowing: Just branch body should be x, got %v", cs.Alts[0].Body)
	}
	// Nothing branch: x is free, body becomes v("y")
	if !coreEq(cs.Alts[1].Body, v("y")) {
		t.Fatalf("Case shadowing: Nothing branch body should be y, got %v", cs.Alts[1].Body)
	}
}

// ===== Capture-avoiding substitution =====

func TestSubst_LamCaptureAvoidance(t *testing.T) {
	// subst (Lam "y" (App (Var "x") (Var "y"))) "x" (Var "y")
	// Without capture guard, this would produce: Lam "y" (App (Var "y") (Var "y"))
	// which captures the free "y" in the replacement. With the guard, we bail out.
	expr := lam("y", app(v("x"), v("y")))
	replacement := v("y")
	result := substFV(expr, "x", replacement, ir.FreeVars(replacement))
	// The guard should detect that "y" is free in replacement and Lam binds "y",
	// so subst bails out, returning the original expression unchanged.
	if !coreEq(result, expr) {
		t.Fatalf("Lam capture: expected unchanged expr, got %v", result)
	}
}

func TestSubst_FixCaptureAvoidance(t *testing.T) {
	// fix f in (App (Var "f") (Var "x"))
	// subst "x" (Var "f") — "f" is free in replacement and bound by fix
	expr := &ir.Fix{Name: "f", Body: lam("z", app(v("f"), v("x")))}
	replacement := v("f")
	// Verify free var detection.
	if _, free := ir.FreeVars(replacement)["f"]; !free {
		t.Fatalf("FreeVars should detect 'f' free in Var{f}")
	}
	result := substFV(expr, "x", replacement, ir.FreeVars(replacement))
	// Guard should bail out — result should be the exact same pointer.
	if result != expr {
		t.Fatalf("Fix capture: expected same pointer (bail out), got different object")
	}
}

func TestSubst_BindCaptureAvoidance(t *testing.T) {
	// Bind (Pure (Var "a")) "y" (App (Var "x") (Var "y"))
	// subst "x" (Var "y") — "y" in replacement would be captured by bind var
	expr := &ir.Bind{
		Comp: &ir.Pure{Expr: v("a")},
		Var:  "y",
		Body: app(v("x"), v("y")),
	}
	replacement := v("y")
	result := substFV(expr, "x", replacement, ir.FreeVars(replacement))
	// Comp should still be substituted (it's not under the binder),
	// but body should be left unchanged due to capture risk.
	bind, ok := result.(*ir.Bind)
	if !ok {
		t.Fatalf("expected Bind, got %T", result)
	}
	// Comp is substituted: Pure (Var "a") → Pure (Var "a") (no "x" in comp, so unchanged)
	if !coreEq(bind.Comp, &ir.Pure{Expr: v("a")}) {
		t.Fatalf("Bind capture: comp changed unexpectedly")
	}
	// Body should be unchanged due to capture guard
	if !coreEq(bind.Body, app(v("x"), v("y"))) {
		t.Fatalf("Bind capture: body should be unchanged, got %v", bind.Body)
	}
}

// ===== Subst through various Core nodes =====

func TestSubst_ThroughTyLam(t *testing.T) {
	// subst (TyLam "a" (Var "x")) "x" (Var "y") → TyLam "a" (Var "y")
	expr := &ir.TyLam{TyParam: "a", Body: v("x")}
	result := substFV(expr, "x", v("y"), ir.FreeVars(v("y")))
	tl, ok := result.(*ir.TyLam)
	if !ok {
		t.Fatalf("expected TyLam, got %T", result)
	}
	if !coreEq(tl.Body, v("y")) {
		t.Fatalf("TyLam subst: body should be y, got %v", tl.Body)
	}
}

func TestSubst_ThroughPrimOp(t *testing.T) {
	// subst (PrimOp "_f" [Var "x", Var "z"]) "x" (Var "y")
	//   → PrimOp "_f" [Var "y", Var "z"]
	expr := primop("_f", 2, v("x"), v("z"))
	result := substFV(expr, "x", v("y"), ir.FreeVars(v("y")))
	po, ok := result.(*ir.PrimOp)
	if !ok {
		t.Fatalf("expected PrimOp, got %T", result)
	}
	if !coreEq(po.Args[0], v("y")) {
		t.Fatalf("PrimOp subst: arg 0 should be y, got %v", po.Args[0])
	}
	if !coreEq(po.Args[1], v("z")) {
		t.Fatalf("PrimOp subst: arg 1 should be z, got %v", po.Args[1])
	}
}

func TestSubst_ThroughRecordLit(t *testing.T) {
	// subst (RecordLit {a: Var "x"}) "x" (Var "y") → RecordLit {a: Var "y"}
	expr := &ir.RecordLit{Fields: []ir.RecordField{{Label: "a", Value: v("x")}}}
	result := substFV(expr, "x", v("y"), ir.FreeVars(v("y")))
	rec, ok := result.(*ir.RecordLit)
	if !ok {
		t.Fatalf("expected RecordLit, got %T", result)
	}
	if !coreEq(rec.Fields[0].Value, v("y")) {
		t.Fatalf("RecordLit subst: field should be y, got %v", rec.Fields[0].Value)
	}
}

// ===== R11: foldr/map fusion =====

func TestR11_SliceFoldrMapFusion(t *testing.T) {
	// _sliceFoldr k z (_sliceMap f xs) → _sliceFoldr (\$x $acc -> k (f $x) $acc) z xs
	fusionRule := func(c ir.Core) ir.Core {
		po, ok := c.(*ir.PrimOp)
		if !ok || po.Name != "_sliceFoldr" || len(po.Args) != 3 {
			return c
		}
		inner, ok := po.Args[2].(*ir.PrimOp)
		if !ok || inner.Name != "_sliceMap" || len(inner.Args) != 2 {
			return c
		}
		k, z, f, xs := po.Args[0], po.Args[1], inner.Args[0], inner.Args[1]
		x, acc := "$opt_x", "$opt_acc"
		fused := &ir.Lam{Param: x, Body: &ir.Lam{Param: acc, Body: &ir.App{
			Fun: &ir.App{Fun: k, Arg: &ir.App{Fun: f, Arg: &ir.Var{Name: x}}},
			Arg: &ir.Var{Name: acc},
		}}}
		return &ir.PrimOp{Name: "_sliceFoldr", Arity: 3, Args: []ir.Core{fused, z, xs}, S: po.S}
	}
	input := primop("_sliceFoldr", 3, v("k"), v("z"),
		primop("_sliceMap", 2, v("f"), v("xs")))
	result := optimize(input, []func(ir.Core) ir.Core{fusionRule})

	po, ok := result.(*ir.PrimOp)
	if !ok || po.Name != "_sliceFoldr" {
		t.Fatalf("R11: expected _sliceFoldr, got %T", result)
	}
	if len(po.Args) != 3 {
		t.Fatalf("R11: expected 3 args, got %d", len(po.Args))
	}
	// Fused function: \$opt_x -> \$opt_acc -> k (f $opt_x) $opt_acc
	outerLam, ok := po.Args[0].(*ir.Lam)
	if !ok || outerLam.Param != "$opt_x" {
		t.Fatalf("R11: expected outer lambda with param $opt_x")
	}
	innerLam, ok := outerLam.Body.(*ir.Lam)
	if !ok || innerLam.Param != "$opt_acc" {
		t.Fatalf("R11: expected inner lambda with param $opt_acc")
	}
	// z and xs should be passed through
	if !coreEq(po.Args[1], v("z")) {
		t.Fatalf("R11: second arg should be z")
	}
	if !coreEq(po.Args[2], v("xs")) {
		t.Fatalf("R11: third arg should be xs")
	}
}

// ===== Multi-pass convergence (M12) =====

func TestMultiPass_NestedBetaRequiresIteration(t *testing.T) {
	// (\a. \b. case a of { Just x -> x }) (Just ((\c. c) z))
	// Pass 1 (bottom-up): inner beta (\c. c) z → z, outer beta fires → \b. case (Just z) of ...
	// Pass 2: case-of-known-constructor fires → \b. z
	input := app(
		lam("a", lam("b", cas(v("a"), alt(pcon("Just", pvar("x")), v("x"))))),
		con("Just", app(lam("c", v("c")), v("z"))),
	)
	result := optimize(input, nil)
	expected := lam("b", v("z"))
	if !coreEq(result, expected) {
		t.Fatalf("multi-pass nested beta: expected \\b. z, got %v", result)
	}
}
