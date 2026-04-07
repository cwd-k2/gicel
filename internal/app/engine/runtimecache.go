package engine

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// runtimeCache is the process-level cache for fully assembled Runtimes.
//
// A *Runtime is the output of NewRuntime — the post-precompileVM bundle of
// Protos, global slot map, builtin closures, prim registry clone, and the
// per-Engine limits. The Runtime is documented as immutable and goroutine-
// safe (see runtime.go), so a single cached pointer can be returned to many
// callers without copying.
//
// The cache lives at the same scope as moduleCache (process lifetime).
// While moduleCache short-circuits parse + check + optimize for individual
// modules, runtimeCache short-circuits the full NewRuntime pipeline,
// including precompileVM — which dominates per-call alloc on warm paths
// (~91% of NewRuntime allocs measured on BenchmarkEngineMultiCompileSameSource).
//
// See tmp/perf/c2-design.md for the design discussion and the limitations
// of fingerprint-based identity (in particular, the closure-capture L1
// limitation on prim impls).
var runtimeCache = struct {
	mu    sync.RWMutex
	items map[runtimeCacheKey]*Runtime
}{
	items: make(map[runtimeCacheKey]*Runtime),
}

type runtimeCacheKey struct {
	sourceHash         [32]byte // SHA-256 of main source text
	runtimeFingerprint [32]byte // SHA-256 of full runtime environment
}

func runtimeCacheGet(key runtimeCacheKey) (*Runtime, bool) {
	runtimeCache.mu.RLock()
	defer runtimeCache.mu.RUnlock()
	rt, ok := runtimeCache.items[key]
	return rt, ok
}

func runtimeCachePut(key runtimeCacheKey, rt *Runtime) {
	runtimeCache.mu.Lock()
	defer runtimeCache.mu.Unlock()
	runtimeCache.items[key] = rt
}

// ResetRuntimeCache clears the global runtime cache. Intended for testing
// and for cold-path benchmarks that need to defeat the cache.
func ResetRuntimeCache() {
	runtimeCache.mu.Lock()
	defer runtimeCache.mu.Unlock()
	runtimeCache.items = make(map[runtimeCacheKey]*Runtime)
}

// RuntimeCacheLen returns the number of entries in the global runtime cache.
func RuntimeCacheLen() int {
	runtimeCache.mu.RLock()
	defer runtimeCache.mu.RUnlock()
	return len(runtimeCache.items)
}

// computeRuntimeCacheKey builds a deterministic cache key from the main
// source and the full runtime environment. The fingerprint is a strict
// superset of moduleCacheKey.envFingerprint — anything that influences
// the *Runtime value (post-precompileVM Protos, global slot layout, prim
// resolution, runtime limits, pipeline flags) must contribute to it.
//
// Inputs (in canonical order, separated by NUL):
//
//  1. Existing env: registeredTys, assumptions, host bindings, gatedBuiltins
//  2. Existing env: registered modules with source hashes
//  3. Existing env: compiler limits (nestingLimit, maxTFSteps, maxSolverSteps, maxResolveDepth)
//  4. NEW: runtime limits (stepLimit, depthLimit, allocLimit)
//  5. NEW: store.recursion flag (gates fix/rec at NewRuntime time)
//  6. NEW: pipeline flags (entryPoint, denyAssumptions, noInline, verifyIR)
//  7. NEW: prim impl identities — sorted name + reflect.Pointer() of each impl
//
// See tmp/perf/c2-design.md L1 for the closure-capture limitation.
func (pc *pipelineCtx) computeRuntimeCacheKey(source string) runtimeCacheKey {
	sourceHash := sha256.Sum256([]byte(source))

	var b strings.Builder
	b.Grow(1024)

	// Sections 1-3: identical to moduleCache envFingerprint.
	writeTypesMap(&b, pc.host.registeredTys)
	b.WriteByte(0)
	writeTypesMap(&b, pc.host.assumptions)
	b.WriteByte(0)
	writeTypesMap(&b, pc.host.bindings)
	b.WriteByte(0)
	writeBoolMap(&b, pc.host.gatedBuiltins)
	b.WriteByte(0)

	// Registered modules: name + source hash, sorted.
	modNames := pc.store.sortedModuleNames()
	for _, n := range modNames {
		b.WriteString(n)
		b.WriteByte('=')
		mod := pc.store.modules[n]
		if mod.source != nil {
			h := sha256.Sum256([]byte(mod.source.Text))
			b.Write(h[:])
		}
		b.WriteByte(0)
	}
	b.WriteByte(0)

	// Compiler limits.
	fmt.Fprintf(&b, "cl:%d/%d/%d/%d",
		pc.limits.nestingLimit,
		pc.limits.maxTFSteps,
		pc.limits.maxSolverSteps,
		pc.limits.maxResolveDepth,
	)
	b.WriteByte(0)

	// Section 4: runtime limits.
	fmt.Fprintf(&b, "rl:%d/%d/%d",
		pc.limits.stepLimit,
		pc.limits.depthLimit,
		pc.limits.allocLimit,
	)
	b.WriteByte(0)

	// Section 5: store.recursion flag.
	if pc.store.recursion {
		b.WriteString("rec:1")
	} else {
		b.WriteString("rec:0")
	}
	b.WriteByte(0)

	// Section 6: pipeline flags.
	fmt.Fprintf(&b, "pf:%s/%t/%t/%t",
		pc.entryPoint,
		pc.denyAssumptions,
		pc.noInline,
		pc.verifyIR,
	)
	b.WriteByte(0)

	// Section 7: prim impl identities. Function pointer identifies the
	// code body (not closure captures) — see L1 in c2-design.md.
	for _, name := range pc.host.prims.SortedNames() {
		impl, _ := pc.host.prims.Lookup(name)
		ptr := reflect.ValueOf(impl).Pointer()
		fmt.Fprintf(&b, "p:%s=%x\n", name, ptr)
	}
	b.WriteByte(0)

	fingerprint := sha256.Sum256([]byte(b.String()))
	return runtimeCacheKey{sourceHash: sourceHash, runtimeFingerprint: fingerprint}
}
