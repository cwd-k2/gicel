// Root API smoke tests — verify public re-exports resolve to non-nil values
// and that thin wrapper helpers round-trip correctly.
// Does NOT cover: engine/runtime behavior (internal/app/engine/*_test.go).

package gicel_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// TestPublicConstructorsNonNil guards against accidental nil assignment of
// the top-level function re-exports in gicel.go.
func TestPublicConstructorsNonNil(t *testing.T) {
	checks := []struct {
		name string
		fn   any
	}{
		{"NewEngine", gicel.NewEngine},
		{"NewCacheStore", gicel.NewCacheStore},
		{"DefaultCacheStore", gicel.DefaultCacheStore},
		{"RunSandbox", gicel.RunSandbox},
		{"ToValue", gicel.ToValue},
		{"FromBool", gicel.FromBool},
		{"FromHost", gicel.FromHost},
		{"FromCon", gicel.FromCon},
		{"ToList", gicel.ToList},
		{"FromList", gicel.FromList},
		{"FromRecord", gicel.FromRecord},
		{"NewRecord", gicel.NewRecord},
		{"NewRecordFromMap", gicel.NewRecordFromMap},
		{"NewRow", gicel.NewRow},
		{"KindType", gicel.KindType},
		{"KindRow", gicel.KindRow},
		{"EmptyRowType", gicel.EmptyRowType},
		{"ClosedRowType", gicel.ClosedRowType},
		{"ResetBudgetCounters", gicel.ResetBudgetCounters},
	}
	for _, c := range checks {
		if c.fn == nil {
			t.Errorf("%s is nil", c.name)
		}
	}

	// TypeOps methods (replaced standalone type constructors).
	ops := &gicel.TypeOps{}
	if ops.Con("Int") == nil {
		t.Error("TypeOps.Con returned nil")
	}
	if ops.Arrow(ops.Con("Int"), ops.Con("Int")) == nil {
		t.Error("TypeOps.Arrow returned nil")
	}
	if ops.Comp(gicel.EmptyRowType(), gicel.EmptyRowType(), ops.Con("Bool"), nil) == nil {
		t.Error("TypeOps.Comp returned nil")
	}
	if ops.Var("a") == nil {
		t.Error("TypeOps.Var returned nil")
	}
	if ops.App(ops.Con("List"), ops.Con("Int")) == nil {
		t.Error("TypeOps.App returned nil")
	}
	if ops.Pretty(ops.Con("Int")) == "" {
		t.Error("TypeOps.Pretty returned empty string")
	}
}

// TestPacksNonNil guards against accidental nil Pack assignment.
func TestPacksNonNil(t *testing.T) {
	packs := map[string]gicel.Pack{
		"Prelude":       gicel.Prelude,
		"EffectFail":    gicel.EffectFail,
		"EffectState":   gicel.EffectState,
		"EffectIO":      gicel.EffectIO,
		"EffectArray":   gicel.EffectArray,
		"EffectMap":     gicel.EffectMap,
		"EffectSet":     gicel.EffectSet,
		"EffectRef":     gicel.EffectRef,
		"EffectSession": gicel.EffectSession,
		"DataStream":    gicel.DataStream,
		"DataSlice":     gicel.DataSlice,
		"DataMap":       gicel.DataMap,
		"DataSet":       gicel.DataSet,
		"DataJSON":      gicel.DataJSON,
		"DataMath":      gicel.DataMath,
		"DataSequence":  gicel.DataSequence,
	}
	for name, p := range packs {
		if p == nil {
			t.Errorf("%s pack is nil", name)
		}
	}
}

// TestRunSandbox_Basic exercises the headline single-call API.
// Engine internals are tested elsewhere; this guards the public wrapper.
func TestRunSandbox_Basic(t *testing.T) {
	r, err := gicel.RunSandbox("import Prelude\nmain := 1 + 2", nil)
	if err != nil {
		t.Fatalf("RunSandbox: %v", err)
	}
	if r == nil {
		t.Fatal("RunSandbox: nil result")
	}
}

// TestRunSandbox_SourceTooLarge covers the explicit size guard in RunSandbox.
func TestRunSandbox_SourceTooLarge(t *testing.T) {
	src := "main := 1 " + strings.Repeat("+ 1 ", 1000)
	_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{MaxSourceSize: 32})
	if err == nil {
		t.Fatal("expected size-limit error")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("expected size message, got %v", err)
	}
}

// TestRunSandbox_CancelledContext verifies external cancellation propagates.
func TestRunSandbox_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := gicel.RunSandbox(
		"import Prelude\nmain := 1 + 2",
		&gicel.SandboxConfig{Context: ctx},
	)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

// TestTryHost_TypeMismatch ensures TryHost returns (zero, false) rather than
// panicking when the wrapped Go type does not match.
func TestTryHost_TypeMismatch(t *testing.T) {
	hv := &gicel.HostVal{Inner: int64(42)}
	if _, ok := gicel.TryHost[string](hv); ok {
		t.Fatal("expected TryHost[string] to fail on int64 inner")
	}
	if v, ok := gicel.TryHost[int64](hv); !ok || v != 42 {
		t.Fatalf("TryHost[int64] = (%v, %v), want (42, true)", v, ok)
	}
}

// TestTryHost_NonHostValue covers the non-HostVal branch.
func TestTryHost_NonHostValue(t *testing.T) {
	rv := gicel.NewRecord(nil)
	if _, ok := gicel.TryHost[int64](rv); ok {
		t.Fatal("expected TryHost to fail on *RecordVal")
	}
}

// TestMustHost_PanicOnMismatch ensures MustHost panics with a non-HostVal.
func TestMustHost_PanicOnMismatch(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = gicel.MustHost[int64](gicel.NewRecord(nil))
}

// TestPrettyValue covers the PrettyValue wrapper. Exact format is owned
// by eval.PrettyValue — here we only guard that the wrapper forwards and
// produces a non-empty string for a non-trivial value.
func TestPrettyValue(t *testing.T) {
	r, err := gicel.RunSandbox("import Prelude\nmain := [1, 2, 3]", nil)
	if err != nil {
		t.Fatalf("RunSandbox: %v", err)
	}
	s := gicel.PrettyValue(r.Value)
	if s == "" {
		t.Fatal("PrettyValue returned empty string")
	}
}

// TestTupleLabel covers the thin re-export for tuple field labels.
func TestTupleLabel(t *testing.T) {
	cases := []struct {
		pos  int
		want string
	}{{1, "_1"}, {2, "_2"}, {10, "_10"}}
	for _, c := range cases {
		if got := gicel.TupleLabel(c.pos); got != c.want {
			t.Errorf("TupleLabel(%d) = %q, want %q", c.pos, got, c.want)
		}
	}
}

// TestValidateModuleName covers the name validator re-export.
func TestValidateModuleName(t *testing.T) {
	if err := gicel.ValidateModuleName("Foo"); err != nil {
		t.Errorf("valid name rejected: %v", err)
	}
	if err := gicel.ValidateModuleName(""); err == nil {
		t.Error("empty name accepted")
	}
	if err := gicel.ValidateModuleName("lower"); err == nil {
		t.Error("lowercase name accepted")
	}
}

// TestDefaultEntryPoint guards the constant export.
func TestDefaultEntryPoint(t *testing.T) {
	if gicel.DefaultEntryPoint == "" {
		t.Fatal("DefaultEntryPoint is empty")
	}
}

// TestRunSandbox_CompileError exposes a CompileError via errors.As so that
// callers can inspect Diagnostics.
func TestRunSandbox_CompileError(t *testing.T) {
	_, err := gicel.RunSandbox("main := +", nil)
	if err == nil {
		t.Fatal("expected compile error")
	}
	var ce *gicel.CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CompileError, got %T: %v", err, err)
	}
	if len(ce.Diagnostics()) == 0 {
		t.Fatal("CompileError carried no diagnostics")
	}
}
