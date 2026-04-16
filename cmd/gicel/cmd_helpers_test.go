// CLI helper tests — flag normalization, catalogue formatting, source
// budget accounting, engine setup pre-validation, and JSON value shaping.
// Does NOT cover: end-to-end CLI behavior (scripts/smoke-test.sh).

package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// --- normalizeFlagError --------------------------------------------------

func TestNormalizeFlagError(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{
			"flag provided but not defined: -packs",
			"unknown flag: --packs",
		},
		{
			"flag provided but not defined: -e",
			"unknown flag: -e",
		},
		{
			// Defensive: triple-dash is user malformed input.
			"flag provided but not defined: --packs",
			"unknown flag: --packs",
		},
		{
			"invalid value \"bogus\" for flag -max-steps: parse error",
			"invalid value \"bogus\" for flag --max-steps: parse error",
		},
		{
			// Unrelated messages pass through untouched.
			"some other error",
			"some other error",
		},
	}
	for _, tc := range tests {
		if got := normalizeFlagError(tc.in); got != tc.want {
			t.Errorf("normalizeFlagError(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- isHelpFlag ----------------------------------------------------------

func TestIsHelpFlag(t *testing.T) {
	pos := []string{"--help", "-h", "-help", "help"}
	neg := []string{"", "--h", "-?", "helpme", "HELP"}
	for _, s := range pos {
		if !isHelpFlag(s) {
			t.Errorf("isHelpFlag(%q) = false, want true", s)
		}
	}
	for _, s := range neg {
		if isHelpFlag(s) {
			t.Errorf("isHelpFlag(%q) = true, want false", s)
		}
	}
}

// --- isUnitValue ---------------------------------------------------------

func TestIsUnitValue(t *testing.T) {
	// Unit: empty RecordVal.
	if !isUnitValue(gicel.NewRecord(nil)) {
		t.Error("empty RecordVal should be unit")
	}
	// Non-unit: RecordVal with a field.
	rv := gicel.NewRecordFromMap(map[string]gicel.Value{
		"x": &gicel.HostVal{Inner: int64(1)},
	})
	if isUnitValue(rv) {
		t.Error("non-empty RecordVal should not be unit")
	}
	// Non-record value.
	if isUnitValue(&gicel.HostVal{Inner: int64(0)}) {
		t.Error("HostVal should not be unit")
	}
}

// --- exprFlag ------------------------------------------------------------

func TestExprFlag(t *testing.T) {
	var e exprFlag
	if e.String() != "" {
		t.Errorf("initial String = %q, want \"\"", e.String())
	}
	if err := e.Set("main := 1"); err != nil {
		t.Fatal(err)
	}
	if e.value != "main := 1" || e.count != 1 {
		t.Errorf("after first Set: value=%q count=%d", e.value, e.count)
	}
	// Duplicate Set must bump count so callers can detect repeats.
	if err := e.Set("main := 2"); err != nil {
		t.Fatal(err)
	}
	if e.count != 2 {
		t.Errorf("after second Set: count=%d, want 2", e.count)
	}
}

// --- moduleFlags ---------------------------------------------------------

func TestModuleFlags(t *testing.T) {
	var m moduleFlags
	if err := m.Set("Foo=a.gicel"); err != nil {
		t.Fatal(err)
	}
	if err := m.Set("Bar=b.gicel"); err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(m))
	}
	if got := m.String(); got != "Foo=a.gicel, Bar=b.gicel" {
		t.Errorf("String = %q, want %q", got, "Foo=a.gicel, Bar=b.gicel")
	}
	// Missing '=' is rejected.
	var bad moduleFlags
	if err := bad.Set("Foo"); err == nil {
		t.Error("expected error for missing '='")
	}
}

// --- printCatalog --------------------------------------------------------

// TestPrintCatalog_GroupingAndOrder captures stdout and verifies that
// entries are grouped by dot-prefix, groups appear in first-seen order,
// and descriptions are aligned only when present.
func TestPrintCatalog_GroupingAndOrder(t *testing.T) {
	out := captureStdout(t, func() {
		printCatalog([]catalogEntry{
			{name: "basics.hello", desc: "Hello world"},
			{name: "basics.echo"}, // no desc — exercises the else branch
			{name: "types.gadts", desc: "GADT examples"},
			{name: "topLevel", desc: "No category"},
		}, "Uncategorized")
	})
	// Group labels.
	for _, want := range []string{"basics:", "types:", "Uncategorized:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected group label %q in output", want)
		}
	}
	// Entry with desc shows padded name + desc.
	if !strings.Contains(out, "Hello world") {
		t.Errorf("expected description in output")
	}
	// First-seen group order.
	iBasics := strings.Index(out, "basics:")
	iTypes := strings.Index(out, "types:")
	iUncat := strings.Index(out, "Uncategorized:")
	if !(iBasics < iTypes && iTypes < iUncat) {
		t.Errorf("group order wrong: basics=%d types=%d uncat=%d", iBasics, iTypes, iUncat)
	}
}

// --- sourceBudget --------------------------------------------------------

func TestSourceBudget_Read(t *testing.T) {
	b := &sourceBudget{}
	data, err := b.read(strings.NewReader("main := 1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "main := 1" {
		t.Errorf("read returned %q, want %q", data, "main := 1")
	}
	if b.used != 9 {
		t.Errorf("used = %d, want 9", b.used)
	}
}

// TestSourceBudget_Exceeds covers the 10 MiB aggregate limit.
// A reader that would exceed the budget must fail even on read, and the
// error message must name the limit so CLI users can diagnose it.
func TestSourceBudget_Exceeds(t *testing.T) {
	b := &sourceBudget{used: maxTotalSourceSize - 10}
	big := strings.NewReader(strings.Repeat("x", 100))
	_, err := b.read(big)
	if err == nil {
		t.Fatal("expected budget error")
	}
	if !strings.Contains(err.Error(), "exceeds limit") {
		t.Errorf("expected 'exceeds limit' in error, got %v", err)
	}
}

// --- setupEngine ---------------------------------------------------------

func TestSetupEngine_EmptyRejected(t *testing.T) {
	_, err := setupEngine("")
	if err == nil || !strings.Contains(err.Error(), "no packs") {
		t.Fatalf("expected 'no packs' error, got %v", err)
	}
}

func TestSetupEngine_UnknownPack(t *testing.T) {
	_, err := setupEngine("prelude,bogus")
	if err == nil || !strings.Contains(err.Error(), "unknown pack") {
		t.Fatalf("expected 'unknown pack' error, got %v", err)
	}
}

// TestSetupEngine_MissingPrelude covers the dependency pre-check:
// non-prelude packs require prelude, because they all build on its types.
func TestSetupEngine_MissingPrelude(t *testing.T) {
	_, err := setupEngine("state")
	if err == nil || !strings.Contains(err.Error(), "requires prelude") {
		t.Fatalf("expected 'requires prelude' error, got %v", err)
	}
}

func TestSetupEngine_AllAlias(t *testing.T) {
	eng, err := setupEngine("all")
	if err != nil || eng == nil {
		t.Fatalf("setupEngine(all): %v", err)
	}
}

func TestSetupEngine_Deduplicates(t *testing.T) {
	// "prelude,prelude" should not return an error — the second occurrence
	// must be silently skipped, not re-applied (would duplicate bindings).
	if _, err := setupEngine("prelude,prelude"); err != nil {
		t.Fatalf("duplicate pack rejected: %v", err)
	}
}

// --- readSource ----------------------------------------------------------

func TestReadSource_Expr(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	_ = fs.Parse(nil)
	budget := &sourceBudget{}
	data, err := readSource(fs, "main := 1", true, budget)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "main := 1" {
		t.Errorf("got %q", data)
	}
}

func TestReadSource_NoSource(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	_ = fs.Parse(nil)
	_, err := readSource(fs, "", false, &sourceBudget{})
	if err == nil || !strings.Contains(err.Error(), "no source file") {
		t.Fatalf("expected 'no source file' error, got %v", err)
	}
}

// --- JSON shaping --------------------------------------------------------

func TestSummarizeSteps_SectionsAndOps(t *testing.T) {
	steps := []gicel.ExplainStep{
		{Kind: gicel.ExplainLabel, Detail: gicel.ExplainDetail{LabelKind: "section", Name: "main"}},
		{Kind: gicel.ExplainBind},
		{Kind: gicel.ExplainEffect, Detail: gicel.ExplainDetail{Op: "put"}},
		{Kind: gicel.ExplainEffect, Detail: gicel.ExplainDetail{Op: "get"}},
		{Kind: gicel.ExplainEffect, Detail: gicel.ExplainDetail{Op: "put"}}, // duplicate op
		{Kind: gicel.ExplainMatch},
		{Kind: gicel.ExplainResult, Detail: gicel.ExplainDetail{Value: "42"}},
	}
	raw := summarizeSteps(steps)
	secs, ok := raw.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map, got %T", raw)
	}
	if len(secs) != 1 {
		t.Fatalf("expected 1 section, got %d", len(secs))
	}
	s := secs[0]
	if s["section"] != "main" || s["binds"] != 1 || s["effects"] != 3 || s["matches"] != 1 || s["result"] != "42" {
		t.Errorf("section mismatch: %+v", s)
	}
	ops, _ := s["ops"].([]string)
	if len(ops) != 2 || ops[0] != "put" || ops[1] != "get" {
		t.Errorf("expected unique ops [put get], got %v", ops)
	}
}

// TestSummarizeSteps_NoSection covers the "pre-section events are discarded"
// path — no current section buffer means no entry is flushed.
func TestSummarizeSteps_NoSection(t *testing.T) {
	steps := []gicel.ExplainStep{
		{Kind: gicel.ExplainBind},
		{Kind: gicel.ExplainEffect},
	}
	raw := summarizeSteps(steps)
	if raw == nil {
		return // zero sections is fine
	}
	secs, _ := raw.([]map[string]any)
	if len(secs) != 0 {
		t.Errorf("expected no sections, got %+v", secs)
	}
}

func TestFormatValue_BoolShortcut(t *testing.T) {
	// RunSandbox gives us real Bool ConVals built from Prelude.
	r, err := gicel.RunSandbox("import Prelude\nmain := True", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := formatValue(r.Value)
	if b, ok := got.(bool); !ok || !b {
		t.Fatalf("formatValue(True) = %#v, want bool true", got)
	}
}

func TestFormatValue_ListToArray(t *testing.T) {
	r, err := gicel.RunSandbox("import Prelude\nmain := [1, 2, 3]", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := formatValue(r.Value)
	arr, ok := got.([]any)
	if !ok || len(arr) != 3 {
		t.Fatalf("formatValue([1,2,3]) = %#v", got)
	}
}

func TestFormatValue_NaNAndInfSanitized(t *testing.T) {
	// NaN must become nil so json.Encoder does not reject it.
	hv := &gicel.HostVal{Inner: nanFloat()}
	if formatValue(hv) != nil {
		t.Errorf("NaN not sanitized to nil")
	}
	if formatValue(&gicel.HostVal{Inner: posInfFloat()}) != nil {
		t.Errorf("+Inf not sanitized to nil")
	}
}

func TestFormatCapEntry(t *testing.T) {
	cases := []struct {
		in   any
		want any
	}{
		{"hello", "hello"},
		{int64(42), int64(42)},
		{float64(1.5), float64(1.5)},
		{true, true},
		{[]string{"a", "b"}, []string{"a", "b"}},
		{nil, nil},
	}
	for _, tc := range cases {
		got := formatCapEntry(tc.in)
		if !equalAny(got, tc.want) {
			t.Errorf("formatCapEntry(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
	// Unknown types fall back to %v.
	type custom struct{ N int }
	got := formatCapEntry(custom{N: 7})
	if s, _ := got.(string); !strings.Contains(s, "7") {
		t.Errorf("custom type did not stringify: %v", got)
	}
}

// --- test utilities ------------------------------------------------------

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		io.Copy(&buf, r)
		close(done)
	}()

	fn()
	w.Close()
	<-done
	return buf.String()
}

func nanFloat() float64 { return zero() / zero() }
func posInfFloat() float64 {
	return 1.0 / zero()
}
func zero() float64 { return 0 }

func equalAny(a, b any) bool {
	switch ax := a.(type) {
	case []string:
		bx, ok := b.([]string)
		if !ok || len(ax) != len(bx) {
			return false
		}
		for i := range ax {
			if ax[i] != bx[i] {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
