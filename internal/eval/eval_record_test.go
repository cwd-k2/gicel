package eval

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/core"
)

func TestEvalRecordLitEmpty(t *testing.T) {
	ev := newTestEval()
	term := &core.RecordLit{}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
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
	term := &core.RecordLit{
		Fields: []core.RecordField{
			{Label: "x", Value: &core.Lit{Value: int64(42)}},
			{Label: "y", Value: &core.Lit{Value: int64(7)}},
		},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
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
	term := &core.RecordProj{
		Record: &core.RecordLit{
			Fields: []core.RecordField{
				{Label: "x", Value: &core.Lit{Value: int64(42)}},
				{Label: "y", Value: &core.Lit{Value: int64(7)}},
			},
		},
		Label: "y",
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
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
	term := &core.RecordProj{
		Record: &core.RecordLit{
			Fields: []core.RecordField{
				{Label: "x", Value: &core.Lit{Value: int64(42)}},
			},
		},
		Label: "z",
	}
	_, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
	if err == nil {
		t.Fatal("expected error for missing field")
	}
}

func TestEvalRecordUpdate(t *testing.T) {
	ev := newTestEval()
	term := &core.RecordUpdate{
		Record: &core.RecordLit{
			Fields: []core.RecordField{
				{Label: "x", Value: &core.Lit{Value: int64(42)}},
				{Label: "y", Value: &core.Lit{Value: int64(7)}},
			},
		},
		Updates: []core.RecordField{
			{Label: "x", Value: &core.Lit{Value: int64(100)}},
		},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
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
	term := &core.Case{
		Scrutinee: &core.RecordLit{
			Fields: []core.RecordField{
				{Label: "x", Value: &core.Lit{Value: int64(42)}},
				{Label: "y", Value: &core.Lit{Value: int64(7)}},
			},
		},
		Alts: []core.Alt{
			{
				Pattern: &core.PRecord{
					Fields: []core.PRecordField{
						{Label: "x", Pattern: &core.PVar{Name: "a"}},
					},
				},
				Body: &core.Var{Name: "a"},
			},
		},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
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
	term := &core.Case{
		Scrutinee: &core.RecordLit{
			Fields: []core.RecordField{
				{Label: "x", Value: &core.Lit{Value: int64(42)}},
			},
		},
		Alts: []core.Alt{
			{
				Pattern: &core.PRecord{
					Fields: []core.PRecordField{
						{Label: "x", Pattern: &core.PWild{}},
					},
				},
				Body: &core.Lit{Value: int64(0)},
			},
		},
	}
	r, err := ev.Eval(EmptyEnv(), EmptyCapEnv(), term)
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
