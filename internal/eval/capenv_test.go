package eval

import "testing"

func TestCapEnvGetSet(t *testing.T) {
	ce := EmptyCapEnv()
	_, ok := ce.Get("x")
	if ok {
		t.Error("expected not found in empty capenv")
	}
	ce2 := ce.Set("x", 42)
	v, ok := ce2.Get("x")
	if !ok || v != 42 {
		t.Errorf("expected x=42, got %v ok=%v", v, ok)
	}
	// EmptyCapEnv has cow=false, so Set mutates in place.
	// This is expected behavior — COW only activates for NewCapEnv.
}

func TestCapEnvCOW(t *testing.T) {
	orig := map[string]any{"a": 1, "b": 2}
	ce := NewCapEnv(orig)
	// First Set triggers copy.
	ce2 := ce.Set("c", 3)
	if _, ok := ce2.Get("c"); !ok {
		t.Error("expected c in ce2")
	}
	// Original map untouched.
	if _, ok := orig["c"]; ok {
		t.Error("COW violated: original map was modified")
	}
}

func TestCapEnvDelete(t *testing.T) {
	ce := EmptyCapEnv().Set("x", 1).Set("y", 2)
	ce2 := ce.Delete("x")
	if _, ok := ce2.Get("x"); ok {
		t.Error("x should be deleted")
	}
	if _, ok := ce2.Get("y"); !ok {
		t.Error("y should remain")
	}
}

func TestCapEnvDeleteCOW(t *testing.T) {
	orig := map[string]any{"a": 1, "b": 2}
	ce := NewCapEnv(orig)
	ce2 := ce.Delete("a")
	if _, ok := ce2.Get("a"); ok {
		t.Error("a should be deleted in ce2")
	}
	// Original map untouched.
	if _, ok := orig["a"]; !ok {
		t.Error("COW violated on delete: original map was modified")
	}
}

func TestCapEnvLabels(t *testing.T) {
	ce := EmptyCapEnv().Set("z", 1).Set("a", 2).Set("m", 3)
	labels := ce.Labels()
	if len(labels) != 3 || labels[0] != "a" || labels[1] != "m" || labels[2] != "z" {
		t.Errorf("expected sorted [a m z], got %v", labels)
	}
}

func TestCapEnvMarkShared(t *testing.T) {
	ce := EmptyCapEnv().Set("x", 1)
	shared := ce.MarkShared()
	// After MarkShared, next Set should copy.
	ce2 := shared.Set("y", 2)
	if _, ok := ce2.Get("y"); !ok {
		t.Error("y should exist in ce2")
	}
}
