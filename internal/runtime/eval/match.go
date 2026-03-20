package eval

import "github.com/cwd-k2/gicel/internal/lang/ir"

// maxMatchDepth is the maximum recursion depth for pattern matching.
const maxMatchDepth = 256

// Match attempts to match a value against a pattern.
// Returns the bindings on success, or nil on failure.
func Match(val Value, pat ir.Pattern) map[string]Value {
	return matchDepth(val, pat, 0)
}

func matchDepth(val Value, pat ir.Pattern, depth int) map[string]Value {
	if depth > maxMatchDepth {
		return nil
	}
	switch p := pat.(type) {
	case *ir.PVar:
		return map[string]Value{p.Name: val}
	case *ir.PWild:
		return map[string]Value{}
	case *ir.PCon:
		cv, ok := val.(*ConVal)
		if !ok || cv.Con != p.Con || len(cv.Args) != len(p.Args) {
			return nil
		}
		bindings := map[string]Value{}
		for i, arg := range p.Args {
			sub := matchDepth(cv.Args[i], arg, depth+1)
			if sub == nil {
				return nil
			}
			for k, v := range sub {
				bindings[k] = v
			}
		}
		return bindings
	case *ir.PRecord:
		rv, ok := val.(*RecordVal)
		if !ok {
			return nil
		}
		bindings := map[string]Value{}
		for _, f := range p.Fields {
			fv, ok := rv.Fields[f.Label]
			if !ok {
				return nil
			}
			sub := matchDepth(fv, f.Pattern, depth+1)
			if sub == nil {
				return nil
			}
			for k, v := range sub {
				bindings[k] = v
			}
		}
		return bindings
	case *ir.PLit:
		hv, ok := val.(*HostVal)
		if !ok {
			return nil
		}
		if hv.Inner == p.Value {
			return map[string]Value{}
		}
		return nil
	}
	return nil
}
