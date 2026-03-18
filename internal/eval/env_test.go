package eval

import "testing"

func TestEnvExtendLookup(t *testing.T) {
	env := EmptyEnv()
	env = env.Extend("x", &HostVal{Inner: 42})
	v, ok := env.Lookup("x")
	if !ok {
		t.Fatal("expected x to be found")
	}
	if hv, ok := v.(*HostVal); !ok || hv.Inner != 42 {
		t.Errorf("expected 42, got %v", v)
	}
}

func TestEnvShadowing(t *testing.T) {
	env := EmptyEnv()
	env = env.Extend("x", &HostVal{Inner: 1})
	env = env.Extend("x", &HostVal{Inner: 2})
	v, _ := env.Lookup("x")
	if hv := v.(*HostVal); hv.Inner != 2 {
		t.Errorf("expected shadowed value 2, got %v", hv.Inner)
	}
}

func TestEnvNotFound(t *testing.T) {
	env := EmptyEnv()
	_, ok := env.Lookup("missing")
	if ok {
		t.Error("expected not found in empty env")
	}
}

func TestEnvExtendMany(t *testing.T) {
	env := EmptyEnv().Extend("a", &HostVal{Inner: 1})
	env = env.ExtendMany(map[string]Value{
		"b": &HostVal{Inner: 2},
		"c": &HostVal{Inner: 3},
	})
	for _, name := range []string{"a", "b", "c"} {
		if _, ok := env.Lookup(name); !ok {
			t.Errorf("expected %s to be found", name)
		}
	}
}

func TestEnvFlatten(t *testing.T) {
	env := EmptyEnv()
	for i := range 50 { // exceed flatThreshold
		env = env.Extend("v"+string(rune('a'+i%26))+string(rune('0'+i/26)), &HostVal{Inner: i})
	}
	env.Flatten()
	// Should still find values after explicit flatten.
	v, ok := env.Lookup("va0")
	if !ok {
		t.Fatal("expected va0 after flatten")
	}
	if hv := v.(*HostVal); hv.Inner != 0 {
		t.Errorf("expected 0, got %v", hv.Inner)
	}
}

func TestEnvTrimTo(t *testing.T) {
	env := EmptyEnv()
	env = env.Extend("a", &HostVal{Inner: 1})
	env = env.Extend("b", &HostVal{Inner: 2})
	env = env.Extend("c", &HostVal{Inner: 3})
	trimmed := env.TrimTo([]string{"a", "c"})
	if _, ok := trimmed.Lookup("a"); !ok {
		t.Error("expected a in trimmed")
	}
	if _, ok := trimmed.Lookup("b"); ok {
		t.Error("b should not be in trimmed")
	}
	if _, ok := trimmed.Lookup("c"); !ok {
		t.Error("expected c in trimmed")
	}
	if trimmed.Len() != 2 {
		t.Errorf("expected len 2, got %d", trimmed.Len())
	}
}

func TestEnvLen(t *testing.T) {
	env := EmptyEnv()
	if env.Len() != 0 {
		t.Error("empty env should have len 0")
	}
	env = env.Extend("x", &HostVal{Inner: 1})
	if env.Len() != 1 {
		t.Errorf("expected len 1, got %d", env.Len())
	}
}
