// Module cache tests — cache hit/miss, key sensitivity, concurrent safety.
// Does NOT cover: pipeline_test.go (full pipeline).
package engine

import (
	"context"
	"sync"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

func TestModuleCache_HitOnSameSource(t *testing.T) {
	ResetModuleCache()
	defer ResetModuleCache()

	source := `
form Bool := { True: Bool; False: Bool; }
not :: Bool -> Bool
not := \b. case b { True => False; False => True }
`
	eng1 := NewEngine()
	if err := eng1.RegisterModule("Lib", source); err != nil {
		t.Fatal(err)
	}
	cacheAfterFirst := ModuleCacheLen()

	eng2 := NewEngine()
	if err := eng2.RegisterModule("Lib", source); err != nil {
		t.Fatal(err)
	}
	cacheAfterSecond := ModuleCacheLen()

	// Cache should not grow — second compilation is a hit.
	// cacheAfterFirst includes Core (from NewEngine) + Lib.
	// cacheAfterSecond should stay the same.
	if cacheAfterSecond != cacheAfterFirst {
		t.Errorf("expected cache size %d (hit), got %d", cacheAfterFirst, cacheAfterSecond)
	}
}

func TestModuleCache_MissOnDifferentSource(t *testing.T) {
	ResetModuleCache()
	defer ResetModuleCache()

	eng1 := NewEngine()
	eng1.RegisterModule("A", `form X := MkX`)

	sizeAfterA := ModuleCacheLen()

	eng2 := NewEngine()
	eng2.RegisterModule("B", `form Y := MkY`)

	sizeAfterB := ModuleCacheLen()

	if sizeAfterB <= sizeAfterA {
		t.Errorf("expected cache to grow for different source, got %d → %d", sizeAfterA, sizeAfterB)
	}
}

func TestModuleCache_MissOnDifferentEnv(t *testing.T) {
	ResetModuleCache()
	defer ResetModuleCache()

	source := `form X := MkX`

	// Engine 1: default env.
	eng1 := NewEngine()
	eng1.RegisterModule("Lib", source)
	sizeAfterFirst := ModuleCacheLen()

	// Engine 2: different env (extra registered type).
	eng2 := NewEngine()
	eng2.RegisterType("Extra", nil)
	eng2.RegisterModule("Lib", source)
	sizeAfterSecond := ModuleCacheLen()

	if sizeAfterSecond <= sizeAfterFirst {
		t.Errorf("expected cache miss for different env, got %d → %d", sizeAfterFirst, sizeAfterSecond)
	}
}

func TestModuleCache_CorrectResult(t *testing.T) {
	ResetModuleCache()
	defer ResetModuleCache()

	source := `
form Bool := { True: Bool; False: Bool; }
not :: Bool -> Bool
not := \b. case b { True => False; False => True }
`
	// First engine: compile and run.
	eng1 := NewEngine()
	eng1.RegisterModule("Lib", source)
	rt1, err := eng1.NewRuntime(context.Background(), `
import Lib
main := not True
`)
	if err != nil {
		t.Fatal(err)
	}
	res1, err := rt1.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Second engine: should use cached module and produce same result.
	eng2 := NewEngine()
	eng2.RegisterModule("Lib", source)
	rt2, err := eng2.NewRuntime(context.Background(), `
import Lib
main := not True
`)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	con1, _ := res1.Value.(*eval.ConVal)
	con2, _ := res2.Value.(*eval.ConVal)
	if con1.Con != "False" || con2.Con != "False" {
		t.Errorf("expected False from both engines, got %v and %v", con1, con2)
	}
}

func TestModuleCache_PreludeShared(t *testing.T) {
	ResetModuleCache()
	defer ResetModuleCache()

	// Two engines with Prelude should share the cached Prelude.
	eng1 := NewEngine()
	eng1.Use(stdlib.Prelude)
	sizeAfterFirst := ModuleCacheLen()

	eng2 := NewEngine()
	eng2.Use(stdlib.Prelude)
	sizeAfterSecond := ModuleCacheLen()

	if sizeAfterSecond != sizeAfterFirst {
		t.Errorf("expected Prelude cache hit, got %d → %d", sizeAfterFirst, sizeAfterSecond)
	}

	// Verify it still works.
	rt, err := eng2.NewRuntime(context.Background(), `
import Prelude
main := 1 + 2
`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if hv := MustHost[int64](res.Value); hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}
}

func TestModuleCache_Concurrent(t *testing.T) {
	ResetModuleCache()
	defer ResetModuleCache()

	source := `form X := { MkX: X; }`
	const N = 8

	var wg sync.WaitGroup
	errs := make(chan error, N)

	for range N {
		wg.Add(1)
		go func() {
			defer wg.Done()
			eng := NewEngine()
			if err := eng.RegisterModule("Lib", source); err != nil {
				errs <- err
				return
			}
			rt, err := eng.NewRuntime(context.Background(), `
import Lib
main := MkX
`)
			if err != nil {
				errs <- err
				return
			}
			_, err = rt.RunWith(context.Background(), nil)
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}
}
