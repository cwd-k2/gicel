package eval

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/ir"
)

func TestEvalRecordLitEmpty(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordLit{}
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := r.Value.(*RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T", r.Value)
	}
	if len(rv.Fields) != 0 {
		t.Errorf("expected empty record, got %d fields", len(rv.Fields))
	}
}

func TestEvalRecordLit(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordLit{
		Fields: []ir.RecordField{
			{Label: "x", Value: &ir.Lit{Value: int64(42)}},
			{Label: "y", Value: &ir.Lit{Value: int64(7)}},
		},
	}
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := r.Value.(*RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T", r.Value)
	}
	xv, ok := rv.Fields["x"]
	if !ok {
		t.Fatal("missing field x")
	}
	if hv, ok := xv.(*HostVal); !ok || hv.Inner != int64(42) {
		t.Errorf("expected x = 42, got %v", xv)
	}
}

func TestEvalRecordProj(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordProj{
		Record: &ir.RecordLit{
			Fields: []ir.RecordField{
				{Label: "x", Value: &ir.Lit{Value: int64(42)}},
				{Label: "y", Value: &ir.Lit{Value: int64(7)}},
			},
		},
		Label: "y",
	}
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(7) {
		t.Errorf("expected HostVal(7), got %v", r.Value)
	}
}

func TestEvalRecordProjMissing(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordProj{
		Record: &ir.RecordLit{
			Fields: []ir.RecordField{
				{Label: "x", Value: &ir.Lit{Value: int64(42)}},
			},
		},
		Label: "z",
	}
	_, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for missing field")
	}
}

func TestEvalRecordUpdate(t *testing.T) {
	ev := newTestEval()
	term := &ir.RecordUpdate{
		Record: &ir.RecordLit{
			Fields: []ir.RecordField{
				{Label: "x", Value: &ir.Lit{Value: int64(42)}},
				{Label: "y", Value: &ir.Lit{Value: int64(7)}},
			},
		},
		Updates: []ir.RecordField{
			{Label: "x", Value: &ir.Lit{Value: int64(100)}},
		},
	}
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := r.Value.(*RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T", r.Value)
	}
	xv := rv.Fields["x"].(*HostVal)
	if xv.Inner != int64(100) {
		t.Errorf("expected x = 100, got %v", xv.Inner)
	}
	yv := rv.Fields["y"].(*HostVal)
	if yv.Inner != int64(7) {
		t.Errorf("expected y = 7 (unchanged), got %v", yv.Inner)
	}
}

func TestEvalRecordPattern(t *testing.T) {
	ev := newTestEval()
	// case { x: 42, y: 7 } of { x: a } -> a
	term := &ir.Case{
		Scrutinee: &ir.RecordLit{
			Fields: []ir.RecordField{
				{Label: "x", Value: &ir.Lit{Value: int64(42)}},
				{Label: "y", Value: &ir.Lit{Value: int64(7)}},
			},
		},
		Alts: []ir.Alt{
			{
				Pattern: &ir.PRecord{
					Fields: []ir.PRecordField{
						{Label: "x", Pattern: &ir.PVar{Name: "a"}},
					},
				},
				Body: &ir.Var{Name: "a"},
			},
		},
	}
	ir.AnnotateFreeVars(term)
	ir.AssignIndices(term)
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected HostVal(42), got %v", r.Value)
	}
}

func TestEvalRecordPatternWild(t *testing.T) {
	ev := newTestEval()
	// case { x: 42 } of { x: _ } -> 0
	term := &ir.Case{
		Scrutinee: &ir.RecordLit{
			Fields: []ir.RecordField{
				{Label: "x", Value: &ir.Lit{Value: int64(42)}},
			},
		},
		Alts: []ir.Alt{
			{
				Pattern: &ir.PRecord{
					Fields: []ir.PRecordField{
						{Label: "x", Pattern: &ir.PWild{}},
					},
				},
				Body: &ir.Lit{Value: int64(0)},
			},
		},
	}
	r, err := ev.Eval(nil, EmptyCapEnv(), term)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := r.Value.(*HostVal)
	if !ok || hv.Inner != int64(0) {
		t.Errorf("expected HostVal(0), got %v", r.Value)
	}
}

// Ensure unused import is used
var _ = context.Background
