package eval

import "testing"

func TestLocalPushLookup(t *testing.T) {
	var locals []Value
	locals = Push(locals, &HostVal{Inner: 10})
	locals = Push(locals, &HostVal{Inner: 20})
	locals = Push(locals, &HostVal{Inner: 30})

	// Index 0 = innermost (last pushed)
	v := LookupLocal(locals, 0)
	if hv := v.(*HostVal); hv.Inner != 30 {
		t.Errorf("index 0: expected 30, got %v", hv.Inner)
	}
	v = LookupLocal(locals, 1)
	if hv := v.(*HostVal); hv.Inner != 20 {
		t.Errorf("index 1: expected 20, got %v", hv.Inner)
	}
	v = LookupLocal(locals, 2)
	if hv := v.(*HostVal); hv.Inner != 10 {
		t.Errorf("index 2: expected 10, got %v", hv.Inner)
	}
}

func TestCapture(t *testing.T) {
	var locals []Value
	locals = Push(locals, &HostVal{Inner: 10}) // index 2
	locals = Push(locals, &HostVal{Inner: 20}) // index 1
	locals = Push(locals, &HostVal{Inner: 30}) // index 0

	// Capture indices 0 and 2 (innermost and outermost).
	captured := Capture(locals, []int{0, 2}, 0)
	// Captured: [30, 10]
	// Index 0 = 10 (last in captured), Index 1 = 30 (first in captured)
	v := LookupLocal(captured, 0)
	if hv := v.(*HostVal); hv.Inner != 10 {
		t.Errorf("captured index 0: expected 10, got %v", hv.Inner)
	}
	v = LookupLocal(captured, 1)
	if hv := v.(*HostVal); hv.Inner != 30 {
		t.Errorf("captured index 1: expected 30, got %v", hv.Inner)
	}
}

func TestPushMany(t *testing.T) {
	var locals []Value
	locals = Push(locals, &HostVal{Inner: 1})
	locals = PushMany(locals, []Value{&HostVal{Inner: 2}, &HostVal{Inner: 3}})
	// Layout: [1, 2, 3]
	// Index 0 = 3, index 1 = 2, index 2 = 1
	v := LookupLocal(locals, 0)
	if hv := v.(*HostVal); hv.Inner != 3 {
		t.Errorf("index 0: expected 3, got %v", hv.Inner)
	}
	v = LookupLocal(locals, 2)
	if hv := v.(*HostVal); hv.Inner != 1 {
		t.Errorf("index 2: expected 1, got %v", hv.Inner)
	}
}

func TestCaptureAll(t *testing.T) {
	var locals []Value
	locals = Push(locals, &HostVal{Inner: 1})
	locals = Push(locals, &HostVal{Inner: 2})

	all := CaptureAll(locals, 0)
	v := LookupLocal(all, 0)
	if hv := v.(*HostVal); hv.Inner != 2 {
		t.Errorf("expected 2, got %v", hv.Inner)
	}

	// Original locals should be unaffected by modifications to captured.
	all = Push(all, &HostVal{Inner: 99})
	if len(locals) != 2 {
		t.Errorf("original locals should still have 2 entries, got %d", len(locals))
	}
}

func TestPushNil(t *testing.T) {
	// Push on nil locals should work correctly.
	locals := Push(nil, &HostVal{Inner: 42})
	if len(locals) != 1 {
		t.Fatalf("expected 1, got %d", len(locals))
	}
	v := LookupLocal(locals, 0)
	if hv := v.(*HostVal); hv.Inner != 42 {
		t.Errorf("expected 42, got %v", hv.Inner)
	}
}

func TestCaptureEmpty(t *testing.T) {
	// Capture with no indices produces nil.
	locals := Push(nil, &HostVal{Inner: 1})
	captured := Capture(locals, []int{}, 0)
	if captured != nil {
		t.Errorf("Capture([]) should produce nil, got %v", captured)
	}
}
