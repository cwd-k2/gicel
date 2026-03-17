package core

import (
	"strings"
	"testing"
)

func TestWalkCountNodes(t *testing.T) {
	// App(Lam("x", Var("x")), Con("Unit", []))
	term := &App{
		Fun: &Lam{Param: "x", Body: &Var{Name: "x"}},
		Arg: &Con{Name: "Unit"},
	}
	count := 0
	Walk(term, func(c Core) bool {
		count++
		return true
	})
	if count != 4 { // App, Lam, Var, Con
		t.Errorf("expected 4 nodes, got %d", count)
	}
}

func TestWalkCollectVars(t *testing.T) {
	term := &App{
		Fun: &Var{Name: "f"},
		Arg: &Var{Name: "x"},
	}
	var names []string
	Walk(term, func(c Core) bool {
		if v, ok := c.(*Var); ok {
			names = append(names, v.Name)
		}
		return true
	})
	if len(names) != 2 || names[0] != "f" || names[1] != "x" {
		t.Errorf("expected [f, x], got %v", names)
	}
}

func TestTransformIdentity(t *testing.T) {
	term := &Pure{Expr: &Con{Name: "Unit"}}
	result := Transform(term, func(c Core) Core { return c })
	if Pretty(result) != Pretty(term) {
		t.Errorf("identity transform changed output: %q vs %q", Pretty(result), Pretty(term))
	}
}

func TestFreeVars(t *testing.T) {
	// \x. App(x, y) — y is free
	term := &Lam{
		Param: "x",
		Body:  &App{Fun: &Var{Name: "x"}, Arg: &Var{Name: "y"}},
	}
	fv := FreeVars(term)
	if _, ok := fv["x"]; ok {
		t.Error("'x' should be bound")
	}
	if _, ok := fv["y"]; !ok {
		t.Error("'y' should be free")
	}
}

func TestFreeVarsBind(t *testing.T) {
	// Bind(Var("c"), "x", Var("x")) — c is free, x is bound in body
	term := &Bind{Comp: &Var{Name: "c"}, Var: "x", Body: &Var{Name: "x"}}
	fv := FreeVars(term)
	if _, ok := fv["c"]; !ok {
		t.Error("'c' should be free")
	}
	if _, ok := fv["x"]; ok {
		t.Error("'x' should be bound in bind body")
	}
}

func TestFreeVarsLamSibling(t *testing.T) {
	// App(Lam("x", Var("x")), Var("x")) — outer x is free.
	// Catches: unbind counter inversion, missing unbind after Lam body.
	term := &App{
		Fun: &Lam{Param: "x", Body: &Var{Name: "x"}},
		Arg: &Var{Name: "x"},
	}
	fv := FreeVars(term)
	if _, ok := fv["x"]; !ok {
		t.Error("'x' should be free in Arg position (sibling of Lam)")
	}
}

func TestFreeVarsCaseAltBody(t *testing.T) {
	// case Unit of { _ -> y } — y is free.
	// Catches: skipping Case alt body recursion in freeVarsRec.
	term := &Case{
		Scrutinee: &Con{Name: "Unit"},
		Alts: []Alt{{
			Pattern: &PWild{},
			Body:    &Var{Name: "y"},
		}},
	}
	fv := FreeVars(term)
	if _, ok := fv["y"]; !ok {
		t.Error("'y' should be free in case alt body")
	}
}

func TestFreeVarsNestedLamShadow(t *testing.T) {
	// Lam("x", App(Lam("x", Var("x")), Var("y"))) — only y is free.
	// Double bind of same name must unbind correctly.
	term := &Lam{
		Param: "x",
		Body: &App{
			Fun: &Lam{Param: "x", Body: &Var{Name: "x"}},
			Arg: &Var{Name: "y"},
		},
	}
	fv := FreeVars(term)
	if _, ok := fv["x"]; ok {
		t.Error("'x' should not be free (doubly bound)")
	}
	if _, ok := fv["y"]; !ok {
		t.Error("'y' should be free")
	}
}

func TestPatternBindings(t *testing.T) {
	p := &PCon{
		Con: "Pair",
		Args: []Pattern{
			&PVar{Name: "a"},
			&PVar{Name: "b"},
		},
	}
	bs := p.Bindings()
	if len(bs) != 2 || bs[0] != "a" || bs[1] != "b" {
		t.Errorf("expected [a, b], got %v", bs)
	}
}

func TestPrettySimple(t *testing.T) {
	term := &Bind{
		Comp: &PrimOp{Name: "dbOpen"},
		Var:  "_",
		Body: &Pure{Expr: &Con{Name: "Unit"}},
	}
	got := Pretty(term)
	if !strings.Contains(got, "bind") || !strings.Contains(got, "dbOpen") {
		t.Errorf("unexpected pretty output: %s", got)
	}
}
