package opt

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/core"
)

// coreEq compares two Core trees structurally (ignoring spans).
func coreEq(a, b core.Core) bool {
	switch x := a.(type) {
	case *core.Var:
		y, ok := b.(*core.Var)
		return ok && x.Name == y.Name
	case *core.Lit:
		y, ok := b.(*core.Lit)
		return ok && x.Value == y.Value
	case *core.Lam:
		y, ok := b.(*core.Lam)
		return ok && x.Param == y.Param && coreEq(x.Body, y.Body)
	case *core.App:
		y, ok := b.(*core.App)
		return ok && coreEq(x.Fun, y.Fun) && coreEq(x.Arg, y.Arg)
	case *core.Con:
		y, ok := b.(*core.Con)
		if !ok || x.Name != y.Name || len(x.Args) != len(y.Args) {
			return false
		}
		for i := range x.Args {
			if !coreEq(x.Args[i], y.Args[i]) {
				return false
			}
		}
		return true
	case *core.Case:
		y, ok := b.(*core.Case)
		if !ok || len(x.Alts) != len(y.Alts) || !coreEq(x.Scrutinee, y.Scrutinee) {
			return false
		}
		for i := range x.Alts {
			if !coreEq(x.Alts[i].Body, y.Alts[i].Body) {
				return false
			}
		}
		return true
	case *core.Pure:
		y, ok := b.(*core.Pure)
		return ok && coreEq(x.Expr, y.Expr)
	case *core.Bind:
		y, ok := b.(*core.Bind)
		return ok && x.Var == y.Var && coreEq(x.Comp, y.Comp) && coreEq(x.Body, y.Body)
	case *core.Thunk:
		y, ok := b.(*core.Thunk)
		return ok && coreEq(x.Comp, y.Comp)
	case *core.Force:
		y, ok := b.(*core.Force)
		return ok && coreEq(x.Expr, y.Expr)
	case *core.PrimOp:
		y, ok := b.(*core.PrimOp)
		if !ok || x.Name != y.Name || len(x.Args) != len(y.Args) {
			return false
		}
		for i := range x.Args {
			if !coreEq(x.Args[i], y.Args[i]) {
				return false
			}
		}
		return true
	case *core.RecordLit:
		y, ok := b.(*core.RecordLit)
		if !ok || len(x.Fields) != len(y.Fields) {
			return false
		}
		for i := range x.Fields {
			if x.Fields[i].Label != y.Fields[i].Label || !coreEq(x.Fields[i].Value, y.Fields[i].Value) {
				return false
			}
		}
		return true
	case *core.RecordProj:
		y, ok := b.(*core.RecordProj)
		return ok && x.Label == y.Label && coreEq(x.Record, y.Record)
	case *core.RecordUpdate:
		y, ok := b.(*core.RecordUpdate)
		if !ok || len(x.Updates) != len(y.Updates) || !coreEq(x.Record, y.Record) {
			return false
		}
		for i := range x.Updates {
			if x.Updates[i].Label != y.Updates[i].Label || !coreEq(x.Updates[i].Value, y.Updates[i].Value) {
				return false
			}
		}
		return true
	case *core.TyApp:
		y, ok := b.(*core.TyApp)
		return ok && coreEq(x.Expr, y.Expr)
	case *core.TyLam:
		y, ok := b.(*core.TyLam)
		return ok && coreEq(x.Body, y.Body)
	}
	return false
}

func v(name string) core.Core          { return &core.Var{Name: name} }
func lit(val any) core.Core            { return &core.Lit{Value: val} }
func app(f, x core.Core) core.Core     { return &core.App{Fun: f, Arg: x} }
func lam(p string, b core.Core) core.Core { return &core.Lam{Param: p, Body: b} }
func con(name string, args ...core.Core) core.Core { return &core.Con{Name: name, Args: args} }
func pcon(name string, args ...core.Pattern) core.Pattern {
	return &core.PCon{Con: name, Args: args}
}
func pvar(name string) core.Pattern { return &core.PVar{Name: name} }
func alt(pat core.Pattern, body core.Core) core.Alt { return core.Alt{Pattern: pat, Body: body} }
func cas(scrut core.Core, alts ...core.Alt) core.Core {
	return &core.Case{Scrutinee: scrut, Alts: alts}
}
func primop(name string, arity int, args ...core.Core) core.Core {
	return &core.PrimOp{Name: name, Arity: arity, Args: args}
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
	result := Optimize(input)
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
	result := Optimize(input)
	if !coreEq(result, v("a")) {
		t.Fatalf("R1 multi-arg failed: got %v", result)
	}
}

// R2: Beta reduction
func TestR2_BetaReduction(t *testing.T) {
	// (\x -> x) y  →  y
	input := app(lam("x", v("x")), v("y"))
	result := Optimize(input)
	if !coreEq(result, v("y")) {
		t.Fatalf("R2 failed: got %v", result)
	}
}

func TestR2_BetaReductionNested(t *testing.T) {
	// (\f -> \x -> f x) g  →  \x -> g x
	input := app(lam("f", lam("x", app(v("f"), v("x")))), v("g"))
	expected := lam("x", app(v("g"), v("x")))
	result := Optimize(input)
	if !coreEq(result, expected) {
		t.Fatalf("R2 nested failed: got %v", result)
	}
}

// R3: Bind-Pure elimination
func TestR3_BindPure(t *testing.T) {
	// bind (pure e) x body  →  body[x := e]
	input := &core.Bind{Comp: &core.Pure{Expr: v("e")}, Var: "x", Body: v("x")}
	result := Optimize(input)
	if !coreEq(result, v("e")) {
		t.Fatalf("R3 failed: got %v", result)
	}
}

// R4: Force-Thunk elimination
func TestR4_ForceThunk(t *testing.T) {
	// force (thunk comp)  →  comp
	input := &core.Force{Expr: &core.Thunk{Comp: v("comp")}}
	result := Optimize(input)
	if !coreEq(result, v("comp")) {
		t.Fatalf("R4 failed: got %v", result)
	}
}

// R5: RecordProj of known literal
func TestR5_RecordProjKnown(t *testing.T) {
	// { x = 1, y = 2 }!#x  →  1
	input := &core.RecordProj{
		Record: &core.RecordLit{Fields: []core.RecordField{
			{Label: "x", Value: lit(int64(1))},
			{Label: "y", Value: lit(int64(2))},
		}},
		Label: "x",
	}
	result := Optimize(input)
	if !coreEq(result, lit(int64(1))) {
		t.Fatalf("R5 failed: got %v", result)
	}
}

// R6: RecordUpdate chain collapse
func TestR6_RecordUpdateChain(t *testing.T) {
	// { { r | x = 1 } | y = 2 }  →  { r | x = 1, y = 2 }
	input := &core.RecordUpdate{
		Record: &core.RecordUpdate{
			Record:  v("r"),
			Updates: []core.RecordField{{Label: "x", Value: lit(int64(1))}},
		},
		Updates: []core.RecordField{{Label: "y", Value: lit(int64(2))}},
	}
	result := Optimize(input)
	upd, ok := result.(*core.RecordUpdate)
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
	// { { r | x = 1 } | x = 2 }  →  { r | x = 2 }
	input := &core.RecordUpdate{
		Record: &core.RecordUpdate{
			Record:  v("r"),
			Updates: []core.RecordField{{Label: "x", Value: lit(int64(1))}},
		},
		Updates: []core.RecordField{{Label: "x", Value: lit(int64(2))}},
	}
	result := Optimize(input)
	upd, ok := result.(*core.RecordUpdate)
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

// ===== Phase 4: Ad-hoc fusion =====

// R10: Slice map/map fusion
func TestR10_SliceMapMap(t *testing.T) {
	// _sliceMap f (_sliceMap g xs)  →  _sliceMap (compose f g) xs
	input := primop("_sliceMap", 2, v("f"), primop("_sliceMap", 2, v("g"), v("xs")))
	result := Optimize(input)
	po, ok := result.(*core.PrimOp)
	if !ok || po.Name != "_sliceMap" {
		t.Fatalf("R10: expected _sliceMap, got %T", result)
	}
	if len(po.Args) != 2 {
		t.Fatalf("R10: expected 2 args, got %d", len(po.Args))
	}
	// First arg should be a composition lambda: \$x -> f (g $x)
	comp, ok := po.Args[0].(*core.Lam)
	if !ok {
		t.Fatalf("R10: expected composed lambda, got %T", po.Args[0])
	}
	innerApp, ok := comp.Body.(*core.App)
	if !ok || !coreEq(innerApp.Fun, v("f")) {
		t.Fatalf("R10: expected f applied to (g $x)")
	}
	if !coreEq(po.Args[1], v("xs")) {
		t.Fatalf("R10: expected xs as second arg")
	}
}

// R12: Slice packed roundtrip
func TestR12_SlicePackedRoundtrip(t *testing.T) {
	// _sliceToList (_sliceFromList xs)  →  xs
	input := primop("_sliceToList", 1, primop("_sliceFromList", 1, v("xs")))
	result := Optimize(input)
	if !coreEq(result, v("xs")) {
		t.Fatalf("R12 failed: got %v", result)
	}
}

// R13: String packed roundtrip
func TestR13_StringPackedRoundtrip(t *testing.T) {
	// _fromRunes (_toRunes x)  →  x
	input := primop("_fromRunes", 1, primop("_toRunes", 1, v("x")))
	result := Optimize(input)
	if !coreEq(result, v("x")) {
		t.Fatalf("R13 failed: got %v", result)
	}
}

// ===== Multi-pass =====

func TestMultiPass_BetaThenCaseOfKnown(t *testing.T) {
	// (\d -> case d of { Just y -> y }) (Just x)
	// Pass 1: beta → case (Just x) of { Just y -> y }
	// Pass 2: case-of-known → x
	input := app(
		lam("d", cas(v("d"), alt(pcon("Just", pvar("y")), v("y")))),
		con("Just", v("x")),
	)
	result := Optimize(input)
	if !coreEq(result, v("x")) {
		t.Fatalf("multi-pass beta+case failed: got %v", result)
	}
}

// ===== No-op cases (should not transform) =====

func TestNoOp_CaseNotKnown(t *testing.T) {
	// case x of { Just y -> y }  →  unchanged
	input := cas(v("x"), alt(pcon("Just", pvar("y")), v("y")))
	result := Optimize(input)
	if !coreEq(result, input) {
		t.Fatalf("should not transform non-known scrutinee")
	}
}

func TestNoOp_ForceNonThunk(t *testing.T) {
	// force x  →  unchanged
	input := &core.Force{Expr: v("x")}
	result := Optimize(input)
	if !coreEq(result, input) {
		t.Fatalf("should not transform force of non-thunk")
	}
}
