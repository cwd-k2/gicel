// Cache store public API tests — SetCacheStore validation, default access.
// Does NOT cover: cache hit/miss behavior (runtimecache_test.go).

package engine

import (
	"testing"
)

// TestSetCacheStoreNilPanics verifies that passing nil to SetCacheStore
// panics with a clear message. cacheStore is dereferenced unconditionally
// in NewRuntime / RegisterModule, so nil has no meaningful behavior — the
// failure is forced at the boundary instead of leaking into an internal
// nil dereference at compile time.
func TestSetCacheStoreNilPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on SetCacheStore(nil)")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic value, got %T: %v", r, r)
		}
		if msg == "" {
			t.Fatalf("expected non-empty panic message")
		}
	}()
	NewEngine().SetCacheStore(nil)
}

// TestDefaultCacheStoreRoundTrip verifies that DefaultCacheStore() can be
// passed back into SetCacheStore to restore the default after a temporary
// override. This is the supported way to "undo" a SetCacheStore call.
func TestDefaultCacheStoreRoundTrip(t *testing.T) {
	eng := NewEngine()
	custom := NewCacheStore(0)
	eng.SetCacheStore(custom)
	if eng.cacheStore != custom {
		t.Fatalf("expected custom store to be installed")
	}
	eng.SetCacheStore(DefaultCacheStore())
	if eng.cacheStore != defaultCacheStore {
		t.Fatalf("expected default store after restore")
	}
}
