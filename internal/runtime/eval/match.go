package eval

import "github.com/cwd-k2/gicel/internal/lang/ir"

// maxMatchDepth is the maximum recursion depth for pattern matching.
const maxMatchDepth = 256

// binding is a name-value pair collected during pattern matching.
type binding struct {
	name string
	val  Value
}

// Match attempts to match a value against a pattern.
// Returns a slice of values in pattern-binding order on success, or nil on failure.
func Match(val Value, pat ir.Pattern) []Value {
	bindings := collectBindings(val, pat, 0, []binding{})
	if bindings == nil {
		return nil
	}
	if len(bindings) == 0 {
		return []Value{} // success with no bindings (wildcard/literal)
	}
	vals := make([]Value, len(bindings))
	for i, b := range bindings {
		vals[i] = b.val
	}
	return vals
}

// MatchNamed attempts to match and returns a name→value map (for explain/diagnostics).
func MatchNamed(val Value, pat ir.Pattern) map[string]Value {
	bindings := collectBindings(val, pat, 0, []binding{})
	if bindings == nil {
		return nil
	}
	m := make(map[string]Value, len(bindings))
	for _, b := range bindings {
		m[b.name] = b.val
	}
	return m
}

// collectBindings collects pattern bindings into a slice, avoiding
// intermediate map allocations. Returns nil on match failure.
func collectBindings(val Value, pat ir.Pattern, depth int, acc []binding) []binding {
	if depth > maxMatchDepth {
		return nil
	}
	switch p := pat.(type) {
	case *ir.PVar:
		return append(acc, binding{p.Name, val})
	case *ir.PWild:
		return acc
	case *ir.PCon:
		cv, ok := val.(*ConVal)
		if !ok || cv.Con != p.Con || len(cv.Args) != len(p.Args) {
			return nil
		}
		for i, arg := range p.Args {
			acc = collectBindings(cv.Args[i], arg, depth+1, acc)
			if acc == nil {
				return nil
			}
		}
		return acc
	case *ir.PRecord:
		rv, ok := val.(*RecordVal)
		if !ok {
			return nil
		}
		for _, f := range p.Fields {
			fv, ok := rv.Get(f.Label)
			if !ok {
				return nil
			}
			acc = collectBindings(fv, f.Pattern, depth+1, acc)
			if acc == nil {
				return nil
			}
		}
		return acc
	case *ir.PLit:
		hv, ok := val.(*HostVal)
		if !ok {
			return nil
		}
		if hv.Inner == p.Value {
			return acc
		}
		return nil
	}
	return nil
}
