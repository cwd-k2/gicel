package engine

import (
	"bytes"
	"crypto/sha256"
	"reflect"
	"strconv"

	"github.com/cwd-k2/gicel/internal/infra/cache"
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
		sourceHash:         sha256.Sum256([]byte(source)),
		runtimeFingerprint: pc.runtimeFp,
	}
}

func (pc *pipelineCtx) computeModuleCacheKey(source string) moduleCacheKey {
	return moduleCacheKey{
		sourceHash:     sha256.Sum256([]byte(source)),
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
