package eval

import "testing"

func TestEnvGlobalExtendLookup(t *testing.T) {
	env := EmptyEnv()
	env.Extend("x", &HostVal{Inner: 42})
	v, ok := env.LookupGlobal("x")
	if !ok {
		t.Fatal("expected x to be found")
	}
	if hv, ok := v.(*HostVal); !ok || hv.Inner != 42 {
		t.Errorf("expected 42, got %v", v)
	}
}

func TestEnvGlobalShadowing(t *testing.T) {
	env := EmptyEnv()
	env.Extend("x", &HostVal{Inner: 1})
	env.Extend("x", &HostVal{Inner: 2})
	v, _ := env.LookupGlobal("x")
	if hv := v.(*HostVal); hv.Inner != 2 {
		t.Errorf("expected shadowed value 2, got %v", hv.Inner)
	}
}

func TestEnvGlobalNotFound(t *testing.T) {
	env := EmptyEnv()
	_, ok := env.LookupGlobal("missing")
	if ok {
		t.Error("expected not found in empty env")
	}
}

func TestEnvLocalPushLookup(t *testing.T) {
	env := EmptyEnv()
	env = env.Push(&HostVal{Inner: 10})
	env = env.Push(&HostVal{Inner: 20})
	env = env.Push(&HostVal{Inner: 30})

	// Index 0 = innermost (last pushed)
	v := env.LookupLocal(0)
	if hv := v.(*HostVal); hv.Inner != 30 {
		t.Errorf("index 0: expected 30, got %v", hv.Inner)
	}
	v = env.LookupLocal(1)
	if hv := v.(*HostVal); hv.Inner != 20 {
		t.Errorf("index 1: expected 20, got %v", hv.Inner)
	}
	v = env.LookupLocal(2)
	if hv := v.(*HostVal); hv.Inner != 10 {
		t.Errorf("index 2: expected 10, got %v", hv.Inner)
	}
}

func TestEnvCapture(t *testing.T) {
	env := EmptyEnv()
	env = env.Push(&HostVal{Inner: 10}) // index 2
	env = env.Push(&HostVal{Inner: 20}) // index 1
	env = env.Push(&HostVal{Inner: 30}) // index 0

	// Capture indices 0 and 2 (innermost and outermost).
	captured := env.Capture([]int{0, 2})
	// Captured env: [30, 10]
	// Index 0 = 10 (last in captured), Index 1 = 30 (first in captured)
	v := captured.LookupLocal(0)
	if hv := v.(*HostVal); hv.Inner != 10 {
		t.Errorf("captured index 0: expected 10, got %v", hv.Inner)
	}
	v = captured.LookupLocal(1)
	if hv := v.(*HostVal); hv.Inner != 30 {
		t.Errorf("captured index 1: expected 30, got %v", hv.Inner)
	}
}

func TestEnvPushMany(t *testing.T) {
	env := EmptyEnv()
	env = env.Push(&HostVal{Inner: 1})
	env = env.PushMany([]Value{&HostVal{Inner: 2}, &HostVal{Inner: 3}})
	// Layout: [1, 2, 3]
	// Index 0 = 3, index 1 = 2, index 2 = 1
	v := env.LookupLocal(0)
	if hv := v.(*HostVal); hv.Inner != 3 {
		t.Errorf("index 0: expected 3, got %v", hv.Inner)
	}
	v = env.LookupLocal(2)
	if hv := v.(*HostVal); hv.Inner != 1 {
		t.Errorf("index 2: expected 1, got %v", hv.Inner)
	}
}

func TestEnvCaptureAll(t *testing.T) {
	env := EmptyEnv()
	env = env.Push(&HostVal{Inner: 1})
	env = env.Push(&HostVal{Inner: 2})

	all := env.CaptureAll()
	v := all.LookupLocal(0)
	if hv := v.(*HostVal); hv.Inner != 2 {
		t.Errorf("expected 2, got %v", hv.Inner)
	}

	// Original env should be unaffected by modifications to captured.
	all = all.Push(&HostVal{Inner: 99})
	if len(env.locals) != 2 {
		t.Errorf("original env should still have 2 locals, got %d", len(env.locals))
	}
}
