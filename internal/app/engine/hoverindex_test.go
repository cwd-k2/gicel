package engine

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func tycon(name string, s span.Span) *types.TyCon {
	return &types.TyCon{Name: name, S: s}
}

func sp(start, end int) span.Span {
	return span.Span{Start: span.Pos(start), End: span.Pos(end)}
}

func TestHoverIndex_RecordAndLen(t *testing.T) {
	idx := NewHoverIndex()
	idx.Record(sp(0, 5), tycon("Int", sp(0, 0)))
	idx.Record(sp(6, 10), tycon("String", sp(0, 0)))
	if idx.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", idx.Len())
	}
}

func TestHoverIndex_FilterZeroSpan(t *testing.T) {
	idx := NewHoverIndex()
	idx.Record(span.Span{}, tycon("Int", sp(0, 0)))
	if idx.Len() != 0 {
		t.Fatalf("zero-value span should be filtered, got %d entries", idx.Len())
	}
}

func TestHoverIndex_FilterZeroWidthSpan(t *testing.T) {
	idx := NewHoverIndex()
	idx.Record(sp(5, 5), tycon("Int", sp(0, 0)))
	if idx.Len() != 0 {
		t.Fatalf("zero-width span should be filtered, got %d entries", idx.Len())
	}
}

func TestHoverIndex_FilterTyError(t *testing.T) {
	idx := NewHoverIndex()
	idx.Record(sp(0, 5), &types.TyError{S: sp(0, 5)})
	if idx.Len() != 0 {
		t.Fatalf("TyError should be filtered, got %d entries", idx.Len())
	}
}

func TestHoverIndex_TypeAtBasic(t *testing.T) {
	idx := NewHoverIndex()
	idx.Record(sp(0, 3), tycon("Int", sp(0, 0)))
	idx.Record(sp(5, 11), tycon("String", sp(0, 0)))
	idx.Finalize()

	// Inside first span.
	ty := idx.TypeAt(span.Pos(1))
	if ty == nil {
		t.Fatal("expected type at pos 1")
	}
	if ty.(*types.TyCon).Name != "Int" {
		t.Fatalf("expected Int, got %s", ty.(*types.TyCon).Name)
	}

	// Inside second span.
	ty = idx.TypeAt(span.Pos(7))
	if ty == nil {
		t.Fatal("expected type at pos 7")
	}
	if ty.(*types.TyCon).Name != "String" {
		t.Fatalf("expected String, got %s", ty.(*types.TyCon).Name)
	}

	// Outside any span.
	ty = idx.TypeAt(span.Pos(4))
	if ty != nil {
		t.Fatalf("expected nil at pos 4, got %v", ty)
	}
}

func TestHoverIndex_InnermostSpan(t *testing.T) {
	idx := NewHoverIndex()
	// Outer: [0, 20), inner: [5, 10).
	idx.Record(sp(0, 20), tycon("Outer", sp(0, 0)))
	idx.Record(sp(5, 10), tycon("Inner", sp(0, 0)))
	idx.Finalize()

	// At pos 7: inner span should win.
	ty := idx.TypeAt(span.Pos(7))
	if ty == nil {
		t.Fatal("expected type at pos 7")
	}
	if ty.(*types.TyCon).Name != "Inner" {
		t.Fatalf("expected Inner, got %s", ty.(*types.TyCon).Name)
	}

	// At pos 2: only outer matches.
	ty = idx.TypeAt(span.Pos(2))
	if ty == nil {
		t.Fatal("expected type at pos 2")
	}
	if ty.(*types.TyCon).Name != "Outer" {
		t.Fatalf("expected Outer, got %s", ty.(*types.TyCon).Name)
	}
}

func TestHoverIndex_EmptyIndex(t *testing.T) {
	idx := NewHoverIndex()
	idx.Finalize()
	if ty := idx.TypeAt(span.Pos(0)); ty != nil {
		t.Fatalf("expected nil from empty index, got %v", ty)
	}
}

func TestHoverIndex_BoundaryPositions(t *testing.T) {
	idx := NewHoverIndex()
	idx.Record(sp(5, 10), tycon("A", sp(0, 0)))
	idx.Finalize()

	// Start position (inclusive).
	if ty := idx.TypeAt(span.Pos(5)); ty == nil {
		t.Fatal("start position should match")
	}
	// End position (exclusive, should NOT match).
	if ty := idx.TypeAt(span.Pos(10)); ty != nil {
		t.Fatalf("end position should not match, got %v", ty)
	}
	// One before end.
	if ty := idx.TypeAt(span.Pos(9)); ty == nil {
		t.Fatal("pos 9 should match")
	}
}

func TestHoverIndex_RecordDecl(t *testing.T) {
	idx := NewHoverIndex()
	intTy := tycon("Int", sp(0, 0))
	idx.RecordDecl(sp(0, 12), HoverBinding, "main", intTy, "")
	idx.Finalize()

	hover := idx.HoverAt(span.Pos(0))
	if hover == "" {
		t.Fatal("expected hover at binding position")
	}
	if hover != "main :: Int" {
		t.Fatalf("expected 'main :: Int', got %q", hover)
	}
}

func TestHoverIndex_ExprWinsOverDecl(t *testing.T) {
	idx := NewHoverIndex()
	intTy := tycon("Int", sp(0, 0))
	// Binding declaration covers [0, 15).
	idx.RecordDecl(sp(0, 15), HoverBinding, "main", intTy, "")
	// Expression literal covers [8, 10) — inside the binding.
	idx.Record(sp(8, 10), intTy)
	idx.Finalize()

	// At pos 9 (on the literal): expression should win.
	hover := idx.HoverAt(span.Pos(9))
	if hover != "Int" {
		t.Fatalf("expected expression hover 'Int', got %q", hover)
	}

	// At pos 2 (on the binding name): declaration should show.
	hover = idx.HoverAt(span.Pos(2))
	if hover != "main :: Int" {
		t.Fatalf("expected binding hover 'main :: Int', got %q", hover)
	}
}

func TestHoverIndex_FormatHover(t *testing.T) {
	tests := []struct {
		kind  HoverKind
		label string
		ty    types.Type
		want  string
	}{
		{HoverExpr, "", tycon("Int", sp(0, 0)), "Int"},
		{HoverBinding, "main", tycon("Int", sp(0, 0)), "main :: Int"},
		{HoverForm, "Maybe", types.MkArrow(types.TypeOfTypes, types.TypeOfTypes), "form Maybe :: Type -> Type"},
		{HoverConstructor, "Just", types.MkArrow(&types.TyVar{Name: "a"}, tycon("Maybe", sp(0, 0))), "Just :: a -> Maybe"},
		{HoverImport, "Prelude", nil, "import Prelude"},
		{HoverTypeAnn, "foo", types.MkArrow(tycon("Int", sp(0, 0)), tycon("Int", sp(0, 0))), "foo :: Int -> Int"},
		{HoverTypeAlias, "MyType", types.TypeOfTypes, "type MyType :: Type"},
		{HoverImpl, "", tycon("Eq Int", sp(0, 0)), "impl Eq Int"},
	}
	// HoverOperator tested separately (requires fixity field).
	opEntry := &hoverEntry{
		kind: HoverOperator, label: "+", module: "Prelude",
		ty:     types.MkArrow(tycon("Int", sp(0, 0)), types.MkArrow(tycon("Int", sp(0, 0)), tycon("Int", sp(0, 0)))),
		fixity: &OperatorFixity{Assoc: "infixl", Prec: 6},
	}
	opGot := formatHover(opEntry)
	opWant := "(Prelude.+) :: Int -> Int -> Int\ninfixl 6"
	if opGot != opWant {
		t.Errorf("formatHover(HoverOperator) = %q, want %q", opGot, opWant)
	}
	for _, tt := range tests {
		e := &hoverEntry{kind: tt.kind, label: tt.label, ty: tt.ty}
		got := formatHover(e)
		if got != tt.want {
			t.Errorf("formatHover(%v) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestHoverIndex_RezonkAll(t *testing.T) {
	idx := NewHoverIndex()
	meta := &types.TyMeta{ID: 42}
	idx.Record(sp(0, 5), meta)
	idx.Finalize()

	// Before re-zonk: should see TyMeta.
	ty := idx.TypeAt(span.Pos(1))
	if _, ok := ty.(*types.TyMeta); !ok {
		t.Fatalf("expected TyMeta before re-zonk, got %T", ty)
	}
}
