package engine

import (
	"reflect"
	"strconv"

	"github.com/cwd-k2/gicel/internal/infra/cache"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// CacheStore holds the compilation caches for modules and runtimes.
// Built on generic cache.Map — the domain-specific key types are the
// only engine knowledge here.
type CacheStore struct {
	runtimes *cache.Map[runtimeCacheKey, *Runtime]
	modules  *cache.Map[moduleCacheKey, *compiledModule]
}

// NewCacheStore creates a CacheStore with per-sub-cache capacity.
// Pass 0 for unlimited (suitable for CLI one-shot processes).
func NewCacheStore(cap int) *CacheStore {
	return &CacheStore{
		runtimes: cache.NewMap[runtimeCacheKey, *Runtime](cap),
		modules:  cache.NewMap[moduleCacheKey, *compiledModule](cap),
	}
}

var defaultCacheStore = NewCacheStore(0)

type runtimeCacheKey struct {
	sourceHash         [32]byte
	runtimeFingerprint [32]byte
}

type moduleCacheKey struct {
	sourceHash     [32]byte
	envFingerprint [32]byte
}

func (cs *CacheStore) GetRuntime(key runtimeCacheKey) (*Runtime, bool) { return cs.runtimes.Get(key) }
func (cs *CacheStore) PutRuntime(key runtimeCacheKey, rt *Runtime)     { cs.runtimes.Put(key, rt) }
func (cs *CacheStore) ResetRuntime()                                   { cs.runtimes.Reset() }
func (cs *CacheStore) RuntimeLen() int                                 { return cs.runtimes.Len() }

func (cs *CacheStore) GetModule(key moduleCacheKey) (*compiledModule, bool) {
	return cs.modules.Get(key)
}
func (cs *CacheStore) PutModule(key moduleCacheKey, mod *compiledModule) { cs.modules.Put(key, mod) }
func (cs *CacheStore) ResetModule()                                      { cs.modules.Reset() }
func (cs *CacheStore) ModuleLen() int                                    { return cs.modules.Len() }

// --- Backward-compatible public functions ---

func ResetRuntimeCache()   { defaultCacheStore.ResetRuntime() }
func RuntimeCacheLen() int { return defaultCacheStore.RuntimeLen() }
func ResetModuleCache()    { defaultCacheStore.ResetModule() }
func ModuleCacheLen() int  { return defaultCacheStore.ModuleLen() }

// --- Cache key computation ---

func (pc *pipelineCtx) computeRuntimeCacheKey(source string) runtimeCacheKey {
	return runtimeCacheKey{
		sourceHash:         hashString(source),
		runtimeFingerprint: pc.runtimeFp,
	}
}

func (pc *pipelineCtx) computeModuleCacheKey(source string) moduleCacheKey {
	return moduleCacheKey{
		sourceHash:     hashString(source),
		envFingerprint: pc.modEnvFp,
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
	e.fpHasher.h.Reset()
	e.writeModuleEnvSection(&e.fpHasher)
	e.moduleEnvFp = e.fpHasher.Sum()
	e.moduleEnvFpValid = true
	return e.moduleEnvFp
}

func (e *Engine) runtimeFingerprint() [32]byte {
	if e.runtimeFpValid {
		return e.runtimeFp
	}
	// moduleEnvFingerprint may have used fpHasher transiently; by the
	// time it returns, the digest is captured in moduleEnvFp and the
	// hasher state is no longer needed. Reset before our own use.
	moduleFp := e.moduleEnvFingerprint()
	e.fpHasher.h.Reset()
	_, _ = e.fpHasher.Write(moduleFp[:])
	_ = e.fpHasher.WriteByte(0)
	e.writeRuntimeSpecificSection(&e.fpHasher)
	e.runtimeFp = e.fpHasher.Sum()
	e.runtimeFpValid = true
	return e.runtimeFp
}

func (e *Engine) writeModuleEnvSection(b types.KeyWriter) {
	writeTypesMap(b, e.host.registeredTys, e.typeOps)
	_ = b.WriteByte(0)
	writeTypesMap(b, e.host.assumptions, e.typeOps)
	_ = b.WriteByte(0)
	writeTypesMap(b, e.host.bindings, e.typeOps)
	_ = b.WriteByte(0)
	writeBoolMap(b, e.host.gatedBuiltins)
	_ = b.WriteByte(0)
	modNames := e.store.sortedModuleNames()
	for _, n := range modNames {
		_, _ = b.WriteString(n)
		_ = b.WriteByte('=')
		mod := e.store.modules[n]
		if mod.source != nil {
			// Per-module source is hashed to a fixed 32 bytes before
			// being written into the outer stream. The double hash
			// preserves injectivity against arbitrary source content
			// (delimiter-free), and bounds per-module contribution to
			// the outer hash regardless of source size.
			//
			// hashString (vs sha256.Sum256([]byte(s))) elides the
			// string→[]byte copy — see hashkey.go.
			h := hashString(mod.source.Text)
			_, _ = b.Write(h[:])
		}
		_ = b.WriteByte(0)
	}
	_ = b.WriteByte(0)
	e.compilerLimits.writeKey(b)
}

// writeRuntimeSpecificSection writes runtime-only state into b.
// Prim identity uses a monotonic registration counter (primSeq) instead
// of function pointers, eliminating the closure-capture blindness (L1)
// and removing the reflect/unsafe dependencies.
func (e *Engine) writeRuntimeSpecificSection(b types.KeyWriter) {
	var numBuf [20]byte

	e.runtimeLimits.writeKey(b)

	if e.store.recursion {
		_, _ = b.WriteString("rec:1")
	} else {
		_, _ = b.WriteString("rec:0")
	}
	_ = b.WriteByte(0)

	e.pipelineFlags.writeKey(b)

	// Prim identity: per-prim function pointer or explicit key.
	// When RegisterPrimWithKey was used, the key replaces the function
	// pointer in the fingerprint, resolving the L1 closure-capture
	// blindness limitation. For prims without an explicit key, the
	// function pointer is used (which cannot distinguish closures with
	// identical code but different captured state).
	for _, name := range e.host.prims.SortedNames() {
		_, _ = b.WriteString("p:")
		_, _ = b.WriteString(name)
		_ = b.WriteByte('=')
		if key, ok := e.primKeys[name]; ok {
			_, _ = b.WriteString("k:")
			_, _ = b.WriteString(key)
		} else {
			impl, _ := e.host.prims.Lookup(name)
			ptr := reflect.ValueOf(impl).Pointer()
			_, _ = b.Write(strconv.AppendUint(numBuf[:0], uint64(ptr), 16))
		}
		_ = b.WriteByte('\n')
	}
	_, _ = b.WriteString("ps:")
	_, _ = b.Write(strconv.AppendUint(numBuf[:0], e.primSeq, 10))
	_ = b.WriteByte(0)
}

func writeBool(b types.KeyWriter, v bool) {
	if v {
		_, _ = b.WriteString("true")
	} else {
		_, _ = b.WriteString("false")
	}
}
