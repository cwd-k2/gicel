// Eval nesting tests — PrettyValue and Match depth guards.
// Does NOT cover: evalStep nesting (eval_nesting_test.go).
package eval

import (
	"strings"
	"testing"
)

// deepConVal builds ConVal("C", [ConVal("C", [... HostVal(1) ...])]) at the given depth.
func deepConVal(depth int) Value {
	var v Value = &HostVal{Inner: int64(1)}
	for range depth {
		v = &ConVal{Con: "C", Args: []Value{v}}
	}
	return v
}

func TestPrettyValue_DepthLimit(t *testing.T) {
	// Deep value should not crash — should truncate with a marker.
	v := deepConVal(10000)
	s := PrettyValue(v)
	if s == "" {
		t.Fatal("PrettyValue returned empty string for deep value")
	}
	// Should contain truncation marker somewhere in the output.
	if !strings.Contains(s, "...") {
		t.Error("expected truncation marker '...' in deeply nested PrettyValue output")
	}
}

func TestPrettyValue_ShallowOK(t *testing.T) {
	// Shallow value should render normally without truncation.
	v := deepConVal(5)
	s := PrettyValue(v)
	if strings.Contains(s, "...") {
		t.Errorf("unexpected truncation in shallow value: %s", s)
	}
	if !strings.Contains(s, "C") {
		t.Errorf("expected 'C' in output, got: %s", s)
	}
}

func TestPrettyValue_CyclicIndirectVal(t *testing.T) {
	// Cyclic IndirectVal should not stack overflow.
	var v Value
	ind := &IndirectVal{Ref: &v}
	v = ind // cycle: ind.Ref → v → ind
	s := PrettyValue(ind)
	if s == "" {
		t.Fatal("PrettyValue returned empty string for cyclic IndirectVal")
	}
}
