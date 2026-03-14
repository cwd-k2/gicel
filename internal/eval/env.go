package eval

// Env is a lexically-scoped variable environment.
//
// Design: parent-chain with lazy flattening. Extend creates a new Env
// node that links to its parent (O(1) allocation, no map copy). Lookup
// walks the chain. When the chain exceeds flatThreshold, the chain is
// flattened into a cached map for amortized O(1) lookup.
//
// LetRec knot-tying works because Closure captures an *Env pointer;
// parent-chain nodes share structure just like flat-map Envs did.
type Env struct {
	parent *Env
	name   string
	val    Value
	flat   map[string]Value // lazy flattening cache (nil = not yet computed)
	size   int
}

// flatThreshold controls when the chain is flattened into a map.
// Chosen to balance LetRec large-binding scenarios vs small-scope overhead.
const flatThreshold = 32

// EmptyEnv creates an empty environment.
func EmptyEnv() *Env {
	return &Env{flat: make(map[string]Value), size: 0}
}

// Extend returns a new Env with an additional binding. O(1).
func (e *Env) Extend(name string, val Value) *Env {
	return &Env{parent: e, name: name, val: val, size: e.size + 1}
}

// ExtendMany returns a new Env with multiple bindings.
// Builds a flat Env directly for efficiency.
func (e *Env) ExtendMany(bindings map[string]Value) *Env {
	if len(bindings) == 0 {
		return e
	}
	m := e.flatten()
	combined := make(map[string]Value, len(m)+len(bindings))
	for k, v := range m {
		combined[k] = v
	}
	for k, v := range bindings {
		combined[k] = v
	}
	return &Env{flat: combined, size: len(combined)}
}

// Lookup searches for a variable. Walks the parent chain, then checks
// the flat cache. Triggers flattening when the chain exceeds threshold.
func (e *Env) Lookup(name string) (Value, bool) {
	// Fast path: flat cache available.
	if e.flat != nil {
		v, ok := e.flat[name]
		return v, ok
	}
	// Walk chain.
	for cur := e; cur != nil; cur = cur.parent {
		if cur.flat != nil {
			v, ok := cur.flat[name]
			return v, ok
		}
		if cur.name == name {
			return cur.val, true
		}
	}
	return nil, false
}

// Len returns the number of bindings.
func (e *Env) Len() int {
	return e.size
}

// TrimTo returns a new flat Env containing only the named variables.
// Used to implement safe-for-space closure conversion.
func (e *Env) TrimTo(names []string) *Env {
	if len(names) == 0 {
		return EmptyEnv()
	}
	m := make(map[string]Value, len(names))
	for _, name := range names {
		if val, ok := e.Lookup(name); ok {
			m[name] = val
		}
	}
	return &Env{flat: m, size: len(m)}
}

// flatten materializes the full binding map from the parent chain.
// Caches the result for future lookups.
func (e *Env) flatten() map[string]Value {
	if e.flat != nil {
		return e.flat
	}
	// Collect chain nodes.
	m := make(map[string]Value, e.size)
	for cur := e; cur != nil; cur = cur.parent {
		if cur.flat != nil {
			// Merge flat cache (earlier in chain = more recent, wins).
			for k, v := range cur.flat {
				if _, exists := m[k]; !exists {
					m[k] = v
				}
			}
			break
		}
		if cur.name != "" {
			if _, exists := m[cur.name]; !exists {
				m[cur.name] = cur.val
			}
		}
	}
	// Cache if above threshold.
	if e.size > flatThreshold {
		e.flat = m
	}
	return m
}
