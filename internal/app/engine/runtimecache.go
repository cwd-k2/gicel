package engine

import (
	"bytes"
	"crypto/sha256"
	"reflect"
	"strconv"
	"sync"
	"unsafe"
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
// source and the full runtime environment. The runtime fingerprint is a
// strict superset of moduleCacheKey.envFingerprint — anything that
// influences the *Runtime value (post-precompileVM Protos, global slot
// layout, prim resolution, runtime limits, pipeline flags) must
// contribute to it. See tmp/perf/c2-design.md L1 for the closure-capture
// limitation.
//
// The fingerprint itself is cached on the Engine and invalidated on
// mutation; see Engine.runtimeFingerprint.
func (pc *pipelineCtx) computeRuntimeCacheKey(source string) runtimeCacheKey {
	return runtimeCacheKey{
		sourceHash:         sha256SumString(source),
		runtimeFingerprint: pc.engine.runtimeFingerprint(),
	}
}

// sha256SumString hashes a string without the per-call []byte(source)
// allocation. sha256.Sum256 reads its input only and never retains a
// reference, so an unsafe view into the string's read-only bytes is
// sound — the alternative is the standard `[]byte(source)` cast which
// copies the entire source on every call (the dominant remaining
// allocation on the cached NewRuntime path).
func sha256SumString(s string) [32]byte {
	if len(s) == 0 {
		return sha256.Sum256(nil)
	}
	return sha256.Sum256(unsafe.Slice(unsafe.StringData(s), len(s)))
}

// invalidateFingerprints marks both the module-env and runtime
// fingerprints as stale. Called by mutators that change any part of
// Engine state that affects compiled module output.
//
// Invalidation is idempotent — re-invalidating an already-stale cache
// is a no-op. The single-goroutine invariant on Engine means no sync.
func (e *Engine) invalidateFingerprints() {
	e.moduleEnvFpValid = false
	e.runtimeFpValid = false
}

// invalidateRuntimeFingerprint marks only the runtime fingerprint as
// stale. Called by mutators that change runtime-only state (prim impls,
// runtime limits, pipeline flags) without affecting compiled module
// output.
func (e *Engine) invalidateRuntimeFingerprint() {
	e.runtimeFpValid = false
}

// moduleEnvFingerprint returns the cached SHA-256 digest of the module
// compilation environment: registered types, assumptions, host bindings,
// gated builtins, registered modules (name + source hash), and compiler
// limits. This is exactly the content that moduleCacheKey.envFingerprint
// previously computed — the digest is unchanged.
//
// Recomputed lazily on first call after invalidation; stable otherwise.
// Uses Engine.fpScratch as the work buffer so the underlying byte slice
// is reused across recomputations after invalidation.
func (e *Engine) moduleEnvFingerprint() [32]byte {
	if e.moduleEnvFpValid {
		return e.moduleEnvFp
	}

	e.fpScratch.Reset()
	e.writeModuleEnvSection(&e.fpScratch)
	e.moduleEnvFp = sha256.Sum256(e.fpScratch.Bytes())
	e.moduleEnvFpValid = true
	return e.moduleEnvFp
}

// runtimeFingerprint returns the cached SHA-256 digest of the full
// runtime environment. The digest is computed by hashing the cached
// moduleEnvFingerprint as a prefix, followed by the runtime-specific
// sections: runtime limits, store.recursion, pipeline flags, and prim
// impl identities.
//
// Recomputed lazily on first call after invalidation; stable otherwise.
// Uses Engine.fpScratch as the work buffer.
func (e *Engine) runtimeFingerprint() [32]byte {
	if e.runtimeFpValid {
		return e.runtimeFp
	}

	// moduleEnvFingerprint has its own cache; recomputed only if the
	// module env was invalidated. Calling it here either returns the
	// cached digest or populates it (using fpScratch). Either way the
	// scratch is in a known state when we return.
	moduleFp := e.moduleEnvFingerprint()

	// Reset the scratch buffer for the runtime-specific section.
	// bytes.Buffer.Reset() preserves the underlying byte slice so the
	// previous Grow does not need to be repeated.
	e.fpScratch.Reset()

	// Prefix with the module env digest. This makes runtime fingerprint
	// a strict function of (moduleEnvFp, runtime-specific state) and
	// preserves the C2 invariant: anything in moduleCacheKey flows into
	// runtimeCacheKey as well.
	e.fpScratch.Write(moduleFp[:])
	e.fpScratch.WriteByte(0)

	e.writeRuntimeSpecificSection(&e.fpScratch)

	e.runtimeFp = sha256.Sum256(e.fpScratch.Bytes())
	e.runtimeFpValid = true
	return e.runtimeFp
}

// writeModuleEnvSection writes the module-env section (registered
// types, assumptions, bindings, gated builtins, modules, compiler
// limits) into b in canonical order. Used by moduleEnvFingerprint and
// kept as a separate function so the section is documented in one place.
//
// All integer formatting goes through strconv.AppendInt rather than
// fmt.Fprintf to avoid the per-call interface boxing fmt does on every
// argument — for the cold compile path this is one of the dominant
// allocation sources.
func (e *Engine) writeModuleEnvSection(b *bytes.Buffer) {
	var numBuf [20]byte

	// 1. Registered types (sorted for determinism).
	writeTypesMap(b, e.host.registeredTys)
	b.WriteByte(0)

	// 2. Assumptions.
	writeTypesMap(b, e.host.assumptions)
	b.WriteByte(0)

	// 3. Bindings.
	writeTypesMap(b, e.host.bindings)
	b.WriteByte(0)

	// 4. Gated builtins.
	writeBoolMap(b, e.host.gatedBuiltins)
	b.WriteByte(0)

	// 5. Registered modules: name + source hash (sorted).
	modNames := e.store.sortedModuleNames()
	for _, n := range modNames {
		b.WriteString(n)
		b.WriteByte('=')
		mod := e.store.modules[n]
		if mod.source != nil {
			h := sha256SumString(mod.source.Text)
			b.Write(h[:])
		}
		b.WriteByte(0)
	}
	b.WriteByte(0)

	// 6. Compiler limits (unchanged format — matches legacy modcache).
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.nestingLimit), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.maxTFSteps), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.maxSolverSteps), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.maxResolveDepth), 10))
}

// writeRuntimeSpecificSection writes the runtime-only section (runtime
// limits, store.recursion, pipeline flags, prim impl identities) into b.
// Used by runtimeFingerprint after the module-env digest prefix.
//
// The per-prim loop and integer formatting both avoid fmt.Fprintf:
// profiling showed `fmt.Fprintf("p:%s=%x\n")` accounting for ~63% of
// cold-path NewRuntime allocations on small workloads, because every
// argument is boxed through an interface{} and the format string is
// re-parsed per call. Manual writes via strconv.Append* eliminate the
// per-prim allocations entirely.
func (e *Engine) writeRuntimeSpecificSection(b *bytes.Buffer) {
	var numBuf [20]byte

	// Runtime limits.
	b.WriteString("rl:")
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.stepLimit), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.depthLimit), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], e.limits.allocLimit, 10))
	b.WriteByte(0)

	// Store.recursion flag (gates fix/rec at NewRuntime time).
	if e.store.recursion {
		b.WriteString("rec:1")
	} else {
		b.WriteString("rec:0")
	}
	b.WriteByte(0)

	// Pipeline flags. entryPoint defaults to DefaultEntryPoint so the
	// cached digest matches what pipeline() would produce.
	ep := e.entryPoint
	if ep == "" {
		ep = DefaultEntryPoint
	}
	b.WriteString("pf:")
	b.WriteString(ep)
	b.WriteByte('/')
	writeBool(b, e.denyAssumptions)
	b.WriteByte('/')
	writeBool(b, e.noInline)
	b.WriteByte('/')
	writeBool(b, e.verifyIR)
	b.WriteByte(0)

	// Prim impl identities. Function pointer identifies the code body
	// (not closure captures) — see L1 in c2-design.md.
	for _, name := range e.host.prims.SortedNames() {
		impl, _ := e.host.prims.Lookup(name)
		ptr := reflect.ValueOf(impl).Pointer()
		b.WriteString("p:")
		b.WriteString(name)
		b.WriteByte('=')
		b.Write(strconv.AppendUint(numBuf[:0], uint64(ptr), 16))
		b.WriteByte('\n')
	}
	b.WriteByte(0)
}

// writeBool appends "true" or "false" to b without going through
// fmt.Fprintf (which boxes through interface{}).
func writeBool(b *bytes.Buffer, v bool) {
	if v {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
}
