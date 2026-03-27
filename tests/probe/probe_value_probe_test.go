//go:build probe

// Value conversion edge-case tests.
package probe_test

import (
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe D: Value conversion edge cases
// ===================================================================

func TestProbeD_Convert_NilToValue(t *testing.T) {
	v := gicel.ToValue(nil)
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal (unit), got %T", v)
	}
	if rec.Len() != 0 {
		t.Fatalf("expected empty record (unit), got %d fields", rec.Len())
	}
}

func TestProbeD_Convert_BoolTrue(t *testing.T) {
	v := gicel.ToValue(true)
	pdAssertCon(t, v, "True")
}

func TestProbeD_Convert_BoolFalse(t *testing.T) {
	v := gicel.ToValue(false)
	pdAssertCon(t, v, "False")
}

func TestProbeD_Convert_FromBoolNonCon(t *testing.T) {
	_, ok := gicel.FromBool(&gicel.HostVal{Inner: 42})
	if ok {
		t.Fatal("expected ok=false for non-ConVal")
	}
}

func TestProbeD_Convert_FromConVal(t *testing.T) {
	nested := &gicel.ConVal{Con: "Just", Args: []gicel.Value{
		&gicel.ConVal{Con: "Just", Args: []gicel.Value{
			&gicel.HostVal{Inner: int64(99)},
		}},
	}}
	name, args, ok := gicel.FromCon(nested)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if name != "Just" {
		t.Fatalf("expected Just, got %s", name)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	innerName, innerArgs, ok := gicel.FromCon(args[0])
	if !ok || innerName != "Just" || len(innerArgs) != 1 {
		t.Fatalf("unexpected inner structure: %v", args[0])
	}
}

func TestProbeD_Convert_ToListEmpty(t *testing.T) {
	v := gicel.ToList(nil)
	items, ok := gicel.FromList(v)
	if !ok {
		t.Fatal("expected valid list from nil")
	}
	if len(items) != 0 {
		t.Fatalf("expected empty list, got %d items", len(items))
	}
}

func TestProbeD_Convert_ToListRoundtrip(t *testing.T) {
	original := []any{int64(1), int64(2), int64(3)}
	v := gicel.ToList(original)
	items, ok := gicel.FromList(v)
	if !ok {
		t.Fatal("expected valid list")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestProbeD_Convert_FromRecordNonRecord(t *testing.T) {
	_, ok := gicel.FromRecord(&gicel.HostVal{Inner: 42})
	if ok {
		t.Fatal("expected ok=false for non-RecordVal")
	}
}

func TestProbeD_Convert_MustHostPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic from MustHost on wrong type")
		}
	}()
	_ = gicel.MustHost[int64](&gicel.ConVal{Con: "True"})
}
