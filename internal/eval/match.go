package eval

import "github.com/cwd-k2/gicel/internal/core"

// Match attempts to match a value against a pattern.
// Returns the bindings on success, or nil on failure.
func Match(val Value, pat core.Pattern) map[string]Value {
	switch p := pat.(type) {
	case *core.PVar:
		return map[string]Value{p.Name: val}
	case *core.PWild:
		return map[string]Value{}
	case *core.PCon:
		cv, ok := val.(*ConVal)
		if !ok || cv.Con != p.Con || len(cv.Args) != len(p.Args) {
			return nil
		}
		bindings := map[string]Value{}
		for i, arg := range p.Args {
			sub := Match(cv.Args[i], arg)
			if sub == nil {
				return nil
			}
			for k, v := range sub {
				bindings[k] = v
			}
		}
		return bindings
	case *core.PRecord:
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
			sub := Match(fv, f.Pattern)
			if sub == nil {
				return nil
			}
			for k, v := range sub {
				bindings[k] = v
			}
		}
		return bindings
	case *core.PLit:
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
