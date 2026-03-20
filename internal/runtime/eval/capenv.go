package eval

import "sort"

// CapEnv is a capability environment: label -> capability state.
// Copy-on-write semantics.
type CapEnv struct {
	caps map[string]any
	cow  bool
}

// EmptyCapEnv creates an empty capability environment.
func EmptyCapEnv() CapEnv {
	return CapEnv{caps: make(map[string]any), cow: false}
}

// NewCapEnv creates a CapEnv from a map. The map is shared with the caller
// under copy-on-write: the first Set or Delete will copy before mutating,
// leaving the caller's original map untouched.
func NewCapEnv(m map[string]any) CapEnv {
	if m == nil {
		m = make(map[string]any)
	}
	return CapEnv{caps: m, cow: true}
}

// Get retrieves a capability by label.
func (c CapEnv) Get(label string) (any, bool) {
	v, ok := c.caps[label]
	return v, ok
}

// Set returns a new CapEnv with the label set to the given value.
func (c CapEnv) Set(label string, val any) CapEnv {
	if c.cow {
		newCaps := make(map[string]any, len(c.caps)+1)
		for k, v := range c.caps {
			newCaps[k] = v
		}
		newCaps[label] = val
		return CapEnv{caps: newCaps, cow: false}
	}
	c.caps[label] = val
	return c
}

// Delete returns a new CapEnv with the label removed.
func (c CapEnv) Delete(label string) CapEnv {
	if c.cow {
		newCaps := make(map[string]any, len(c.caps))
		for k, v := range c.caps {
			if k != label {
				newCaps[k] = v
			}
		}
		return CapEnv{caps: newCaps, cow: false}
	}
	delete(c.caps, label)
	return c
}

// Labels returns all capability labels, sorted.
func (c CapEnv) Labels() []string {
	labels := make([]string, 0, len(c.caps))
	for k := range c.caps {
		labels = append(labels, k)
	}
	sort.Strings(labels)
	return labels
}

// MarkShared marks this CapEnv as shared, ensuring future Set/Delete copy.
func (c CapEnv) MarkShared() CapEnv {
	c.cow = true
	return c
}
