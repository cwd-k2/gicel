// Package cache provides a generic bounded concurrent map for
// compilation caches. Eviction uses random deletion (Go map iteration
// order) to avoid LRU tracking overhead.
package cache

import "sync"

// Map is a thread-safe bounded map. Cap=0 means unlimited.
type Map[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]V
	cap   int
}

// NewMap creates a Map with the given capacity. Pass 0 for unlimited.
func NewMap[K comparable, V any](cap int) *Map[K, V] {
	return &Map[K, V]{
		items: make(map[K]V),
		cap:   cap,
	}
}

// Get returns the value for key and whether it was found.
func (m *Map[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.items[key]
	return v, ok
}

// Put stores a value, evicting one random entry if at capacity.
func (m *Map[K, V]) Put(key K, val V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cap > 0 && len(m.items) >= m.cap {
		for k := range m.items {
			delete(m.items, k)
			break
		}
	}
	m.items[key] = val
}

// Len returns the number of entries.
func (m *Map[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

// Reset clears all entries.
func (m *Map[K, V]) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	clear(m.items)
}
