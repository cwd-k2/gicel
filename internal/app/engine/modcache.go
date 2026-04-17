package engine

import (
	"sort"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// writeTypesMap writes a sorted map[string]types.Type as a deterministic
// byte sequence using WriteTypeKey (injective).
func writeTypesMap(b types.KeyWriter, m map[string]types.Type, ops *types.TypeOps) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		ops.WriteTypeKey(b, m[k])
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
