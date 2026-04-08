package engine

import (
	"sort"
	"sync"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// moduleCache is the process-level cache for compiled modules.
// compiledModule values are immutable after postCheck, so concurrent
// read access from multiple Engine instances is safe without copying.
var moduleCache = struct {
	mu    sync.RWMutex
	items map[moduleCacheKey]*compiledModule
}{
	items: make(map[moduleCacheKey]*compiledModule),
}

type moduleCacheKey struct {
	sourceHash     [32]byte // SHA-256 of module source text
	envFingerprint [32]byte // SHA-256 of compile environment
}

func moduleCacheGet(key moduleCacheKey) (*compiledModule, bool) {
	moduleCache.mu.RLock()
	defer moduleCache.mu.RUnlock()
	mod, ok := moduleCache.items[key]
	return mod, ok
}

func moduleCachePut(key moduleCacheKey, mod *compiledModule) {
	moduleCache.mu.Lock()
	defer moduleCache.mu.Unlock()
	moduleCache.items[key] = mod
}

// ResetModuleCache clears the global module cache. Intended for testing.
func ResetModuleCache() {
	moduleCache.mu.Lock()
	defer moduleCache.mu.Unlock()
	moduleCache.items = make(map[moduleCacheKey]*compiledModule)
}

// ModuleCacheLen returns the number of entries in the global module cache.
func ModuleCacheLen() int {
	moduleCache.mu.RLock()
	defer moduleCache.mu.RUnlock()
	return len(moduleCache.items)
}

// computeModuleCacheKey builds a deterministic cache key from the
// source text and the full compile environment. The environment
// fingerprint content is defined by Engine.moduleEnvFingerprint and
// cached there — this function just combines it with the main source
// hash. Hashes the source via sha256SumString to skip the []byte
// conversion alloc.
func (pc *pipelineCtx) computeModuleCacheKey(source string) moduleCacheKey {
	return moduleCacheKey{
		sourceHash:     sha256SumString(source),
		envFingerprint: pc.engine.moduleEnvFingerprint(),
	}
}

// writeTypesMap writes a sorted map[string]types.Type as a deterministic
// byte sequence using WriteTypeKey (injective). Accepts any
// types.KeyWriter so the same writer can buffer to a strings.Builder
// (one-shot) or a bytes.Buffer (reusable across fingerprint computations).
func writeTypesMap(b types.KeyWriter, m map[string]types.Type) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		types.WriteTypeKey(b, m[k])
		b.WriteByte(0)
	}
}

// writeBoolMap writes a sorted map[string]bool as a deterministic byte sequence.
func writeBoolMap(b types.KeyWriter, m map[string]bool) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(k)
		if m[k] {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
		b.WriteByte(0)
	}
}
