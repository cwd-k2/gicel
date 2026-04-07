package engine

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
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

// computeModuleCacheKey builds a deterministic cache key from the source
// text and the full compile environment. The environment fingerprint
// includes everything that can affect the compilation result.
func (pc *pipelineCtx) computeModuleCacheKey(source string) moduleCacheKey {
	sourceHash := sha256.Sum256([]byte(source))

	var b strings.Builder
	b.Grow(512)

	// 1. Registered types (sorted for determinism).
	writeTypesMap(&b, pc.host.registeredTys)
	b.WriteByte(0)

	// 2. Assumptions.
	writeTypesMap(&b, pc.host.assumptions)
	b.WriteByte(0)

	// 3. Bindings.
	writeTypesMap(&b, pc.host.bindings)
	b.WriteByte(0)

	// 4. Gated builtins.
	writeBoolMap(&b, pc.host.gatedBuiltins)
	b.WriteByte(0)

	// 5. Already-compiled module names and source hashes (sorted).
	// Import resolution depends on which modules are available AND their
	// contents. Two Engine instances may register different implementations
	// of the same module name; including source hashes prevents incorrect
	// cache sharing across incompatible upstream definitions.
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

	// 6. Compiler limits that affect type checking output.
	fmt.Fprintf(&b, "%d/%d/%d/%d",
		pc.limits.nestingLimit,
		pc.limits.maxTFSteps,
		pc.limits.maxSolverSteps,
		pc.limits.maxResolveDepth,
	)

	envFingerprint := sha256.Sum256([]byte(b.String()))
	return moduleCacheKey{sourceHash: sourceHash, envFingerprint: envFingerprint}
}

// writeTypesMap writes a sorted map[string]types.Type as a deterministic
// byte sequence using WriteTypeKey (injective).
func writeTypesMap(b *strings.Builder, m map[string]types.Type) {
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
func writeBoolMap(b *strings.Builder, m map[string]bool) {
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
