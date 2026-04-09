package engine

import (
	"bytes"
	"crypto/sha256"
	"reflect"
	"strconv"
	"sync"
)

// CacheStore is a thread-safe cache for compiled modules and assembled
// Runtimes. Both caches are keyed by SHA-256 fingerprints and use a
// capacity limit with random eviction to prevent unbounded growth in
// long-running processes (LSP, embedded servers).
//
// The default process-global CacheStore is used when Engine.cacheStore
// is nil. Use SetCacheStore on Engine for testing or multi-tenant
// isolation.
type CacheStore struct {
	runtime struct {
		mu    sync.RWMutex
		items map[runtimeCacheKey]*Runtime
	}
	module struct {
		mu    sync.RWMutex
		items map[moduleCacheKey]*compiledModule
	}
	cap int // max entries per sub-cache; 0 = unlimited
}

// NewCacheStore creates a CacheStore with the given per-sub-cache
// capacity. Pass 0 for unlimited (suitable for CLI one-shot processes).
func NewCacheStore(cap int) *CacheStore {
	cs := &CacheStore{cap: cap}
	cs.runtime.items = make(map[runtimeCacheKey]*Runtime)
	cs.module.items = make(map[moduleCacheKey]*compiledModule)
	return cs
}

// defaultCacheStore is the process-global cache. Engines use this when
// no explicit CacheStore is set.
var defaultCacheStore = NewCacheStore(0)

// --- Runtime cache ---

type runtimeCacheKey struct {
	sourceHash         [32]byte
	runtimeFingerprint [32]byte
}

func (cs *CacheStore) GetRuntime(key runtimeCacheKey) (*Runtime, bool) {
	cs.runtime.mu.RLock()
	defer cs.runtime.mu.RUnlock()
	rt, ok := cs.runtime.items[key]
	return rt, ok
}

func (cs *CacheStore) PutRuntime(key runtimeCacheKey, rt *Runtime) {
	cs.runtime.mu.Lock()
	defer cs.runtime.mu.Unlock()
	if cs.cap > 0 && len(cs.runtime.items) >= cs.cap {
		evictOne(cs.runtime.items)
	}
	cs.runtime.items[key] = rt
}

func (cs *CacheStore) ResetRuntime() {
	cs.runtime.mu.Lock()
	defer cs.runtime.mu.Unlock()
	cs.runtime.items = make(map[runtimeCacheKey]*Runtime)
}

func (cs *CacheStore) RuntimeLen() int {
	cs.runtime.mu.RLock()
	defer cs.runtime.mu.RUnlock()
	return len(cs.runtime.items)
}

// --- Module cache ---

type moduleCacheKey struct {
	sourceHash     [32]byte
	envFingerprint [32]byte
}

func (cs *CacheStore) GetModule(key moduleCacheKey) (*compiledModule, bool) {
	cs.module.mu.RLock()
	defer cs.module.mu.RUnlock()
	mod, ok := cs.module.items[key]
	return mod, ok
}

func (cs *CacheStore) PutModule(key moduleCacheKey, mod *compiledModule) {
	cs.module.mu.Lock()
	defer cs.module.mu.Unlock()
	if cs.cap > 0 && len(cs.module.items) >= cs.cap {
		evictOne(cs.module.items)
	}
	cs.module.items[key] = mod
}

func (cs *CacheStore) ResetModule() {
	cs.module.mu.Lock()
	defer cs.module.mu.Unlock()
	cs.module.items = make(map[moduleCacheKey]*compiledModule)
}

func (cs *CacheStore) ModuleLen() int {
	cs.module.mu.RLock()
	defer cs.module.mu.RUnlock()
	return len(cs.module.items)
}

// evictOne removes one arbitrary entry from the map. Go's map iteration
// order is non-deterministic, which provides a reasonable approximation
// of random eviction without tracking insertion order or access times.
func evictOne[K comparable, V any](m map[K]V) {
	for k := range m {
		delete(m, k)
		return
	}
}

// --- Backward-compatible public functions (delegate to defaultCacheStore) ---

// ResetRuntimeCache clears the global runtime cache. Intended for testing.
func ResetRuntimeCache() { defaultCacheStore.ResetRuntime() }

// RuntimeCacheLen returns the number of entries in the global runtime cache.
func RuntimeCacheLen() int { return defaultCacheStore.RuntimeLen() }

// ResetModuleCache clears the global module cache. Intended for testing.
func ResetModuleCache() { defaultCacheStore.ResetModule() }

// ModuleCacheLen returns the number of entries in the global module cache.
func ModuleCacheLen() int { return defaultCacheStore.ModuleLen() }

// --- Cache key computation ---

func (pc *pipelineCtx) computeRuntimeCacheKey(source string) runtimeCacheKey {
	return runtimeCacheKey{
		sourceHash:         sha256.Sum256([]byte(source)),
		runtimeFingerprint: pc.engine.runtimeFingerprint(),
	}
}

func (pc *pipelineCtx) computeModuleCacheKey(source string) moduleCacheKey {
	return moduleCacheKey{
		sourceHash:     sha256.Sum256([]byte(source)),
		envFingerprint: pc.engine.moduleEnvFingerprint(),
	}
}

// --- Fingerprint computation (stays on Engine — reads Engine state) ---

// invalidateFingerprints marks both the module-env and runtime
// fingerprints as stale. Called by mutators that change any part of
// Engine state that affects compiled module output.
func (e *Engine) invalidateFingerprints() {
	e.moduleEnvFpValid = false
	e.runtimeFpValid = false
}

// invalidateRuntimeFingerprint marks only the runtime fingerprint as
// stale. Called by mutators that change runtime-only state.
func (e *Engine) invalidateRuntimeFingerprint() {
	e.runtimeFpValid = false
}

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

func (e *Engine) runtimeFingerprint() [32]byte {
	if e.runtimeFpValid {
		return e.runtimeFp
	}
	moduleFp := e.moduleEnvFingerprint()
	e.fpScratch.Reset()
	e.fpScratch.Write(moduleFp[:])
	e.fpScratch.WriteByte(0)
	e.writeRuntimeSpecificSection(&e.fpScratch)
	e.runtimeFp = sha256.Sum256(e.fpScratch.Bytes())
	e.runtimeFpValid = true
	return e.runtimeFp
}

func (e *Engine) writeModuleEnvSection(b *bytes.Buffer) {
	var numBuf [20]byte
	writeTypesMap(b, e.host.registeredTys)
	b.WriteByte(0)
	writeTypesMap(b, e.host.assumptions)
	b.WriteByte(0)
	writeTypesMap(b, e.host.bindings)
	b.WriteByte(0)
	writeBoolMap(b, e.host.gatedBuiltins)
	b.WriteByte(0)
	modNames := e.store.sortedModuleNames()
	for _, n := range modNames {
		b.WriteString(n)
		b.WriteByte('=')
		mod := e.store.modules[n]
		if mod.source != nil {
			h := sha256.Sum256([]byte(mod.source.Text))
			b.Write(h[:])
		}
		b.WriteByte(0)
	}
	b.WriteByte(0)
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.nestingLimit), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.maxTFSteps), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.maxSolverSteps), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.maxResolveDepth), 10))
}

// writeRuntimeSpecificSection writes runtime-only state into b.
// Prim identity uses a monotonic registration counter (primSeq) instead
// of function pointers, eliminating the closure-capture blindness (L1)
// and removing the reflect/unsafe dependencies.
func (e *Engine) writeRuntimeSpecificSection(b *bytes.Buffer) {
	var numBuf [20]byte

	b.WriteString("rl:")
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.stepLimit), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(e.limits.depthLimit), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], e.limits.allocLimit, 10))
	b.WriteByte(0)

	if e.store.recursion {
		b.WriteString("rec:1")
	} else {
		b.WriteString("rec:0")
	}
	b.WriteByte(0)

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

	// Prim identity: function pointer per-prim (distinguishes different
	// code bodies across Engines) plus a monotonic registration counter
	// (distinguishes re-registration of same-code closures within one
	// Engine). The function pointer cannot distinguish closures with
	// identical code but different captured state (L1 limitation).
	for _, name := range e.host.prims.SortedNames() {
		impl, _ := e.host.prims.Lookup(name)
		ptr := reflect.ValueOf(impl).Pointer()
		b.WriteString("p:")
		b.WriteString(name)
		b.WriteByte('=')
		b.Write(strconv.AppendUint(numBuf[:0], uint64(ptr), 16))
		b.WriteByte('\n')
	}
	b.WriteString("ps:")
	b.Write(strconv.AppendUint(numBuf[:0], e.primSeq, 10))
	b.WriteByte(0)
}

func writeBool(b *bytes.Buffer, v bool) {
	if v {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
}
