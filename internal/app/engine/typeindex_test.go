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

func TestTypeIndex_RecordAndLen(t *testing.T) {
	idx := NewTypeIndex()
	idx.Record(sp(0, 5), tycon("Int", sp(0, 0)))
	idx.Record(sp(6, 10), tycon("String", sp(0, 0)))
	if idx.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", idx.Len())
	}
}

func TestTypeIndex_FilterZeroSpan(t *testing.T) {
	idx := NewTypeIndex()
	idx.Record(span.Span{}, tycon("Int", sp(0, 0)))
	if idx.Len() != 0 {
		t.Fatalf("zero-value span should be filtered, got %d entries", idx.Len())
	}
}

func TestTypeIndex_FilterZeroWidthSpan(t *testing.T) {
	idx := NewTypeIndex()
	idx.Record(sp(5, 5), tycon("Int", sp(0, 0)))
	if idx.Len() != 0 {
		t.Fatalf("zero-width span should be filtered, got %d entries", idx.Len())
	}
}

func TestTypeIndex_FilterTyError(t *testing.T) {
	idx := NewTypeIndex()
	idx.Record(sp(0, 5), &types.TyError{S: sp(0, 5)})
	if idx.Len() != 0 {
		t.Fatalf("TyError should be filtered, got %d entries", idx.Len())
	}
}

func TestTypeIndex_TypeAtBasic(t *testing.T) {
	idx := NewTypeIndex()
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

func TestTypeIndex_InnermostSpan(t *testing.T) {
	idx := NewTypeIndex()
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

func TestTypeIndex_EmptyIndex(t *testing.T) {
	idx := NewTypeIndex()
	idx.Finalize()
	if ty := idx.TypeAt(span.Pos(0)); ty != nil {
		t.Fatalf("expected nil from empty index, got %v", ty)
	}
}

func TestTypeIndex_BoundaryPositions(t *testing.T) {
	idx := NewTypeIndex()
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
