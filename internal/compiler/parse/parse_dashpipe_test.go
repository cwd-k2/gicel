// DashPipe (-|) parse tests — right-associative type application operator.
// Does NOT cover: lexer edge cases (lexer_test.go).

package parse

import (
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive
)

func TestParseDashPipeSimple(t *testing.T) {
	// F -| A  =  F A  =  TyExprApp(F, A)
	prog := parseMustSucceed(t, `f :: F -| A`)
	ann := prog.Decls[0].(*DeclTypeAnn)
	app, ok := ann.Type.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp, got %T", ann.Type)
	}
	fun, ok := app.Fun.(*TyExprCon)
	if !ok || fun.Name != "F" {
		t.Errorf("expected fun F, got %T %v", app.Fun, app.Fun)
	}
	arg, ok := app.Arg.(*TyExprCon)
	if !ok || arg.Name != "A" {
		t.Errorf("expected arg A, got %T %v", app.Arg, app.Arg)
	}
}

func TestParseDashPipeRightAssociative(t *testing.T) {
	// F -| A -| B  =  F (A B)  =  TyExprApp(F, TyExprApp(A, B))
	prog := parseMustSucceed(t, `f :: F -| A -| B`)
	ann := prog.Decls[0].(*DeclTypeAnn)
	outer, ok := ann.Type.(*TyExprApp)
	if !ok {
		t.Fatalf("expected outer TyExprApp, got %T", ann.Type)
	}
	fun, ok := outer.Fun.(*TyExprCon)
	if !ok || fun.Name != "F" {
		t.Errorf("expected outer fun F, got %T", outer.Fun)
	}
	inner, ok := outer.Arg.(*TyExprApp)
	if !ok {
		t.Fatalf("expected inner TyExprApp, got %T", outer.Arg)
	}
	if c, ok := inner.Fun.(*TyExprCon); !ok || c.Name != "A" {
		t.Errorf("expected inner fun A, got %T", inner.Fun)
	}
	if c, ok := inner.Arg.(*TyExprCon); !ok || c.Name != "B" {
		t.Errorf("expected inner arg B, got %T", inner.Arg)
	}
}

func TestParseDashPipeTriple(t *testing.T) {
	// Map String -| List -| Maybe -| Int
	// = Map String (List (Maybe Int))
	// = TyExprApp(TyExprApp(Map, String), TyExprApp(List, TyExprApp(Maybe, Int)))
	prog := parseMustSucceed(t, `f :: Map String -| List -| Maybe -| Int`)
	ann := prog.Decls[0].(*DeclTypeAnn)

	outer, ok := ann.Type.(*TyExprApp)
	if !ok {
		t.Fatalf("expected outer TyExprApp, got %T", ann.Type)
	}
	// outer.Fun = Map String (juxtaposition)
	mapApp, ok := outer.Fun.(*TyExprApp)
	if !ok {
		t.Fatalf("expected Map String app, got %T", outer.Fun)
	}
	if c, ok := mapApp.Fun.(*TyExprCon); !ok || c.Name != "Map" {
		t.Errorf("expected Map, got %T", mapApp.Fun)
	}
	// outer.Arg = List (Maybe Int)
	listApp, ok := outer.Arg.(*TyExprApp)
	if !ok {
		t.Fatalf("expected List(...) app, got %T", outer.Arg)
	}
	if c, ok := listApp.Fun.(*TyExprCon); !ok || c.Name != "List" {
		t.Errorf("expected List, got %T", listApp.Fun)
	}
	// listApp.Arg = Maybe Int
	maybeApp, ok := listApp.Arg.(*TyExprApp)
	if !ok {
		t.Fatalf("expected Maybe Int app, got %T", listApp.Arg)
	}
	if c, ok := maybeApp.Fun.(*TyExprCon); !ok || c.Name != "Maybe" {
		t.Errorf("expected Maybe, got %T", maybeApp.Fun)
	}
	if c, ok := maybeApp.Arg.(*TyExprCon); !ok || c.Name != "Int" {
		t.Errorf("expected Int, got %T", maybeApp.Arg)
	}
}

func TestParseDashPipePrecedenceVsArrow(t *testing.T) {
	// F -| A -> B  =  (F A) -> B
	prog := parseMustSucceed(t, `f :: F -| A -> B`)
	ann := prog.Decls[0].(*DeclTypeAnn)
	arrow, ok := ann.Type.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow, got %T", ann.Type)
	}
	_, ok = arrow.From.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp as arrow from, got %T", arrow.From)
	}
}

func TestParseDashPipePrecedenceVsArrowRight(t *testing.T) {
	// A -> F -| B  =  A -> (F B)
	prog := parseMustSucceed(t, `f :: A -> F -| B`)
	ann := prog.Decls[0].(*DeclTypeAnn)
	arrow, ok := ann.Type.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow, got %T", ann.Type)
	}
	if c, ok := arrow.From.(*TyExprCon); !ok || c.Name != "A" {
		t.Errorf("expected A as arrow from, got %T", arrow.From)
	}
	app, ok := arrow.To.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp as arrow to, got %T", arrow.To)
	}
	if c, ok := app.Fun.(*TyExprCon); !ok || c.Name != "F" {
		t.Errorf("expected F, got %T", app.Fun)
	}
}

func TestParseDashPipePrecedenceVsTilde(t *testing.T) {
	// F -| A ~ B  =  (F A) ~ B
	prog := parseMustSucceed(t, `f :: F -| A ~ B`)
	ann := prog.Decls[0].(*DeclTypeAnn)
	eq, ok := ann.Type.(*TyExprEq)
	if !ok {
		t.Fatalf("expected TyExprEq, got %T", ann.Type)
	}
	_, ok = eq.Lhs.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp as eq lhs, got %T", eq.Lhs)
	}
}

func TestParseDashPipePrecedenceVsFatArrow(t *testing.T) {
	// C a => F -| A  =  C a => (F A)
	prog := parseMustSucceed(t, `f :: C a => F -| A`)
	ann := prog.Decls[0].(*DeclTypeAnn)
	qual, ok := ann.Type.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual, got %T", ann.Type)
	}
	app, ok := qual.Body.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp as qual body, got %T", qual.Body)
	}
	if c, ok := app.Fun.(*TyExprCon); !ok || c.Name != "F" {
		t.Errorf("expected F, got %T", app.Fun)
	}
}

func TestParseDashPipeJuxtapositionTighter(t *testing.T) {
	// F A -| G B  =  (F A) (G B)  — juxtaposition binds tighter than -|
	prog := parseMustSucceed(t, `f :: F A -| G B`)
	ann := prog.Decls[0].(*DeclTypeAnn)
	app, ok := ann.Type.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp, got %T", ann.Type)
	}
	// Fun should be F A (juxtaposition)
	fa, ok := app.Fun.(*TyExprApp)
	if !ok {
		t.Fatalf("expected Fun to be TyExprApp(F, A), got %T", app.Fun)
	}
	if c, ok := fa.Fun.(*TyExprCon); !ok || c.Name != "F" {
		t.Errorf("expected F, got %T", fa.Fun)
	}
	// Arg should be G B (juxtaposition)
	gb, ok := app.Arg.(*TyExprApp)
	if !ok {
		t.Fatalf("expected Arg to be TyExprApp(G, B), got %T", app.Arg)
	}
	if c, ok := gb.Fun.(*TyExprCon); !ok || c.Name != "G" {
		t.Errorf("expected G, got %T", gb.Fun)
	}
}
